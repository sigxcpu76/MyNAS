package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func newRegistryTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // no ~/.docker to import
	a := NewAdapterWithClient(nil, "http://x")
	a.unitsDir = t.TempDir()
	a.registryVerify = func(ctx context.Context, server, user, pass string) error {
		if pass == "bad" {
			return errors.New("unauthorized")
		}
		return nil
	}
	return a
}

func readAuths(t *testing.T, a *Adapter) map[string]dockerAuthEntry {
	t.Helper()
	_, auths, err := a.readDockerConfig()
	if err != nil {
		t.Fatal(err)
	}
	return auths
}

func TestRegistryCRUDAndDockerConfig(t *testing.T) {
	ctx := context.Background()
	a := newRegistryTestAdapter(t)

	hub, err := a.CreateRegistry(ctx, domain.DockerRegistry{Server: "https://index.docker.io/v1/", Username: "alice", Password: "tok1"})
	if err != nil {
		t.Fatal(err)
	}
	if hub.Server != "docker.io" || hub.ID != "docker-io" || !hub.HasPassword || hub.Password != "" {
		t.Fatalf("unexpected hub record: %+v", hub)
	}
	if _, err := a.CreateRegistry(ctx, domain.DockerRegistry{Server: "ghcr.io", Username: "bob", Password: "tok2"}); err != nil {
		t.Fatal(err)
	}

	auths := readAuths(t, a)
	if auths[domain.DockerHubAuthKey].Auth != base64.StdEncoding.EncodeToString([]byte("alice:tok1")) {
		t.Fatalf("docker hub auth not stored under %s: %+v", domain.DockerHubAuthKey, auths)
	}
	if _, ok := auths["ghcr.io"]; !ok {
		t.Fatalf("ghcr auth missing: %+v", auths)
	}
	if fi, _ := os.Stat(filepath.Join(a.registryConfigDir(), "config.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("config.json must be 0600, got %v", fi.Mode().Perm())
	}

	// one credential per server
	if _, err := a.CreateRegistry(ctx, domain.DockerRegistry{ID: "other", Server: "ghcr.io", Username: "x", Password: "y"}); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// failed verification must not touch stored creds
	if _, err := a.UpdateRegistry(ctx, domain.DockerRegistry{ID: "ghcr-io", Password: "bad"}, []string{"password"}); err == nil {
		t.Fatal("expected verification failure")
	}
	if u, p, _ := decodeBasicAuth(readAuths(t, a)["ghcr.io"].Auth); u != "bob" || p != "tok2" {
		t.Fatalf("creds changed after failed verify: %s:%s", u, p)
	}

	// description-only update keeps the password
	upd, err := a.UpdateRegistry(ctx, domain.DockerRegistry{ID: "ghcr-io", Description: "packages"}, []string{"description"})
	if err != nil || upd.Description != "packages" || !upd.HasPassword {
		t.Fatalf("update failed: %+v %v", upd, err)
	}

	// Engine API auth header for an image on that registry
	hdr := a.registryAuthHeader("ghcr.io/sigxcpu76/quasar:0.1.23")
	raw, _ := base64.URLEncoding.DecodeString(hdr)
	var payload map[string]string
	_ = json.Unmarshal(raw, &payload)
	if payload["username"] != "bob" || payload["password"] != "tok2" {
		t.Fatalf("bad X-Registry-Auth payload: %s", raw)
	}
	if a.registryAuthHeader("quay.io/foo/bar") != "" {
		t.Fatal("expected no auth for unknown registry")
	}

	// list never exposes passwords
	list, _ := a.ListRegistries(ctx)
	b, _ := json.Marshal(list)
	if strings.Contains(string(b), "tok1") || strings.Contains(string(b), "tok2") {
		t.Fatalf("password leaked in list: %s", b)
	}

	if err := a.DeleteRegistry(ctx, "ghcr-io"); err != nil {
		t.Fatal(err)
	}
	if _, ok := readAuths(t, a)["ghcr.io"]; ok {
		t.Fatal("ghcr auth not removed on delete")
	}
	if list, _ := a.ListRegistries(ctx); len(list) != 1 {
		t.Fatalf("expected 1 registry after delete, got %d", len(list))
	}
}

func TestDockerCLIEnvPointsAtRegistryConfig(t *testing.T) {
	a := newRegistryTestAdapter(t)
	want := "DOCKER_CONFIG=" + filepath.Join(a.unitsDir, registryAuthDirName)
	for _, e := range a.dockerCLIEnv() {
		if e == want {
			return
		}
	}
	t.Fatalf("DOCKER_CONFIG not set to %s", want)
}

func TestSeedRegistriesFromHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".docker"), 0o700)
	cfg := `{"auths":{"https://index.docker.io/v1/":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("carol:pw")) + `"},"helper.example":{}}}`
	_ = os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(cfg), 0o600)

	a := NewAdapterWithClient(nil, "http://x")
	a.unitsDir = t.TempDir()
	a.seedRegistriesFromHome()
	list, err := a.ListRegistries(context.Background())
	if err != nil || len(list) != 1 || list[0].Server != "docker.io" || list[0].Username != "carol" || !list[0].HasPassword {
		t.Fatalf("unexpected seed result: %+v %v", list, err)
	}
}
