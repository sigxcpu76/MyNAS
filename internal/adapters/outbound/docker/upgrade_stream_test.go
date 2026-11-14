package docker

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitImageForPull(t *testing.T) {
	tests := []struct {
		input     string
		wantImage string
		wantTag   string
	}{
		{"nginx", "nginx", "latest"},
		{"nginx:1.27", "nginx", "1.27"},
		{"ghcr.io/linuxserver/nginx:latest", "ghcr.io/linuxserver/nginx", "latest"},
		{"localhost:5000/myorg/app", "localhost:5000/myorg/app", "latest"},
		{"localhost:5000/myorg/app:v2.1.0", "localhost:5000/myorg/app", "v2.1.0"},
		{"nginx@sha256:abcdef123456", "nginx@sha256:abcdef123456", ""},
	}

	for _, tc := range tests {
		gotImg, gotTag := splitImageForPull(tc.input)
		if gotImg != tc.wantImage || gotTag != tc.wantTag {
			t.Errorf("splitImageForPull(%q) = (%q, %q), want (%q, %q)", tc.input, gotImg, gotTag, tc.wantImage, tc.wantTag)
		}
	}
}

func TestExtractComposeImagesFromYAML(t *testing.T) {
	yamlContent := `
services:
  web:
    image: jc21/nginx-proxy-manager:latest
  db:
    image: mariadb:10.11
  sidecar:
    image: jc21/nginx-proxy-manager:latest
`
	images := extractComposeImagesFromYAML(yamlContent)
	if len(images) != 2 {
		t.Fatalf("expected 2 unique images, got %d: %v", len(images), images)
	}
}

func TestParseDockerPullStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"status":"Pulling from library/nginx","id":"latest"}`,
		`{"status":"Pulling fs layer","progressDetail":{},"id":"layer1"}`,
		`{"status":"Pulling fs layer","progressDetail":{},"id":"layer2"}`,
		`{"status":"Downloading","progressDetail":{"current":500,"total":1000},"id":"layer1"}`,
		`{"status":"Downloading","progressDetail":{"current":1000,"total":2000},"id":"layer2"}`,
		`{"status":"Pull complete","progressDetail":{},"id":"layer1"}`,
		`{"status":"Pull complete","progressDetail":{},"id":"layer2"}`,
		`{"status":"Status: Downloaded newer image for nginx:latest"}`,
	}, "\n")

	var lastCur, lastTot int64
	err := parseDockerPullStream(strings.NewReader(stream), func(cur, tot int64) {
		lastCur = cur
		lastTot = tot
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastCur != 3000 || lastTot != 3000 {
		t.Fatalf("expected final bytes 3000/3000, got %d/%d", lastCur, lastTot)
	}
}

func TestParseDockerPullStreamError(t *testing.T) {
	stream := `{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`
	err := parseDockerPullStream(strings.NewReader(stream), nil)
	if err == nil || !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("expected manifest unknown error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestPullImageWithProgressSendsRegistryAuth(t *testing.T) {
	unitsDir := t.TempDir()
	a := NewAdapterFull("/nonexistent/docker.sock", unitsDir)

	cfgDir := a.registryConfigDir()
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir registry config: %v", err)
	}
	authB64 := base64.StdEncoding.EncodeToString([]byte("alice:s3cr3t"))
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"auths":{"ghcr.io":{"auth":"`+authB64+`"}}}`), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	var gotAuth string
	var gotFromImage, gotTag string
	a.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotAuth = r.Header.Get("X-Registry-Auth")
			gotFromImage = r.URL.Query().Get("fromImage")
			gotTag = r.URL.Query().Get("tag")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"Pull complete","id":"l1"}` + "\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := a.pullImageWithProgress(context.Background(), "ghcr.io/myorg/app:v2.0.0", nil); err != nil {
		t.Fatalf("pullImageWithProgress failed: %v", err)
	}
	if gotFromImage != "ghcr.io/myorg/app" || gotTag != "v2.0.0" {
		t.Fatalf("unexpected query params: fromImage=%q tag=%q", gotFromImage, gotTag)
	}
	if gotAuth == "" {
		t.Fatal("expected X-Registry-Auth header to be set")
	}
	decoded, err := base64.URLEncoding.DecodeString(gotAuth)
	if err != nil {
		t.Fatalf("decode X-Registry-Auth: %v", err)
	}
	if !strings.Contains(string(decoded), `"username":"alice"`) || !strings.Contains(string(decoded), `"password":"s3cr3t"`) {
		t.Fatalf("unexpected X-Registry-Auth payload: %s", string(decoded))
	}
}

func TestUpgradeAppStreamPullsBeforeMutatingComposeYAML(t *testing.T) {
	unitsDir := t.TempDir()
	appDir := filepath.Join(unitsDir, "myapp")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir appDir: %v", err)
	}
	origYAML := "services:\n  app:\n    image: ghcr.io/myorg/app:v1.0.0\n"
	composePath := filepath.Join(appDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte(origYAML), 0o644); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	a := NewAdapterFull("/nonexistent/docker.sock", unitsDir)
	a.hasSocket = true
	a.hasCompose = true
	a.composeCmd = []string{"false"} // ensures fallback `docker compose pull` fails immediately

	var pulledImage string
	var composeContentDuringPull string
	a.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			pulledImage = r.URL.Query().Get("fromImage") + ":" + r.URL.Query().Get("tag")
			raw, _ := os.ReadFile(composePath)
			composeContentDuringPull = string(raw)
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	transform := func(s string) string {
		return strings.ReplaceAll(s, "ghcr.io/myorg/app:v1.0.0", "ghcr.io/myorg/app:v2.0.0")
	}

	_, err := a.UpgradeAppStream(context.Background(), "myapp", false, transform, nil)
	if err == nil {
		t.Fatal("expected UpgradeAppStream to fail when pull fails")
	}
	if pulledImage != "ghcr.io/myorg/app:v2.0.0" {
		t.Fatalf("expected target image ghcr.io/myorg/app:v2.0.0 to be pulled first, got %q", pulledImage)
	}
	if composeContentDuringPull != origYAML {
		t.Fatalf("compose.yaml was mutated before pull! got:\n%s", composeContentDuringPull)
	}
	afterRaw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose.yaml after failed upgrade: %v", err)
	}
	if string(afterRaw) != origYAML {
		t.Fatalf("compose.yaml should remain unchanged after failed pull, got:\n%s", string(afterRaw))
	}
}
