package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestCasaOSIndexParsing(t *testing.T) {
	mockJSON := `{
		"version": 2,
		"updated_at": "2024-10-29T10:00:00Z",
		"app_count": 2,
		"base_url": "https://example.com/repo",
		"apps": [
			{
				"id": "org.icewhale.uptimekuma",
				"title": "Uptime Kuma",
				"tagline": "A self-hosted monitoring tool.",
				"category": "Utilities",
				"categories": ["Utilities", "Network"],
				"icon": "/Apps/UptimeKuma/icon.png",
				"compose_url": "/Apps/UptimeKuma/docker-compose.yml",
				"version": "1.23.16"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockJSON))
	}))
	defer server.Close()

	idx, raw, err := fetchCasaOSIndex(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchCasaOSIndex failed: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected raw body")
	}
	if len(idx.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(idx.Apps))
	}
	app := idx.Apps[0]
	if app.ID != "org.icewhale.uptimekuma" {
		t.Errorf("unexpected ID: %s", app.ID)
	}
	if app.Title != "Uptime Kuma" {
		t.Errorf("unexpected Title: %s", app.Title)
	}
}

func TestNormalizeCasaOSCompose(t *testing.T) {
	casaOSCompose := `version: "3"
services:
  uptime-kuma:
    image: louislam/uptime-kuma:1.23.16
    container_name: uptime-kuma
    restart: always
    ports:
      - 3001:3001
    volumes:
      - /DATA/AppData/$AppID/data:/app/data
    environment:
      - PUID=$PUID
      - PGID=$PGID
      - TZ=$TZ
x-casaos:
  architectures:
    - amd64
    - arm64
  main: uptime-kuma
  port_map: "3001"
  scheme: http
  index: /
`

	normalized, defaultPort, portals, containerImg := normalizeCasaOSCompose(
		casaOSCompose, "uptime-kuma", "fast", "/mnt/fast/apps",
	)

	if defaultPort != 3001 {
		t.Errorf("expected defaultPort 3001, got %d", defaultPort)
	}

	if containerImg != "louislam/uptime-kuma:1.23.16" {
		t.Errorf("expected containerImg 'louislam/uptime-kuma:1.23.16', got %q", containerImg)
	}

	// Verify volume replaced
	if !strings.Contains(normalized, "/mnt/fast/apps/uptime-kuma/data:/app/data") {
		t.Errorf("expected normalized volume path, got:\n%s", normalized)
	}

	// Verify environment defaults replaced
	if strings.Contains(normalized, "$PUID") || strings.Contains(normalized, "$PGID") || strings.Contains(normalized, "$TZ") {
		t.Errorf("expected variables to be resolved, got:\n%s", normalized)
	}

	// Verify x-portals injected
	if !strings.Contains(normalized, "x-portals:") || !strings.Contains(normalized, "port: 3001") {
		t.Errorf("expected x-portals to be injected, got:\n%s", normalized)
	}

	if len(portals) == 0 {
		t.Errorf("expected at least 1 portal")
	}

	groups, questions := extractCasaOSQuestions(
		casaOSCompose, domain.CatalogApp{AppID: "uptime-kuma"}, defaultPort, portals, "fast", "/mnt/fast/apps",
	)
	if len(groups) == 0 || len(questions) == 0 {
		t.Errorf("expected questions generated from compose")
	}
}

func TestEnrichFromCasaOS(t *testing.T) {
	composeContent := `services:
  app:
    image: test/app:latest
    ports:
      - 8080:80
    volumes:
      - /DATA/AppData/$AppID:/config
x-casaos:
  port_map: "8080"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(composeContent))
	}))
	defer server.Close()

	app := domain.CatalogApp{
		AppID:   "testapp",
		Train:   "casaos",
		Sources: []string{server.URL},
	}

	enrichFromCasaOS(context.Background(), server.Client(), &app, t.TempDir(), "tank", "/mnt/tank/apps")

	if app.DefaultPort != 8080 {
		t.Errorf("expected defaultPort 8080, got %d", app.DefaultPort)
	}
	if app.ContainerImage != "test/app:latest" {
		t.Errorf("expected containerImage 'test/app:latest', got %q", app.ContainerImage)
	}
	if !strings.Contains(app.ComposeYAML, "/mnt/tank/apps/testapp:/config") {
		t.Errorf("expected /mnt/tank/apps/testapp in compose, got: %s", app.ComposeYAML)
	}
}
