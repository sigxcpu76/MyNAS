package docker

import (
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
