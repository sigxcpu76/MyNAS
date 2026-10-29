package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestDokployIndexParsing(t *testing.T) {
	mockJSON := `[
		{
			"id": "grafana",
			"name": "Grafana",
			"version": "11.0.0",
			"description": "Observability and data visualization.",
			"logo": "https://raw.githubusercontent.com/Dokploy/templates/main/blueprints/grafana/logo.png",
			"tags": ["Monitoring", "Analytics"],
			"links": {
				"website": "https://grafana.com"
			}
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockJSON))
	}))
	defer server.Close()

	entries, raw, err := fetchDokployIndex(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchDokployIndex failed: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected raw body")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "grafana" {
		t.Errorf("unexpected ID: %s", entries[0].ID)
	}
}

func TestNormalizeDokployBlueprint(t *testing.T) {
	composeYAML := `services:
  grafana:
    image: grafana/grafana:latest
    restart: always
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD}
      - GF_SERVER_ROOT_URL=http://${DOKPLOY_DOMAIN}
    volumes:
      - grafana-data:/var/lib/grafana
volumes:
  grafana-data:
`
	templateTOML := `
[variables]
GRAFANA_ADMIN_PASSWORD = "${password:16}"
DOKPLOY_DOMAIN = "${domain}"

[[config.domains]]
serviceName = "grafana"
port = 3000
`

	normalized, defaultPort, portals, questions, containerImg := normalizeDokployBlueprint(
		composeYAML, templateTOML, "grafana", "fast", "/mnt/fast/apps",
	)

	if defaultPort != 3000 {
		t.Errorf("expected defaultPort 3000, got %d", defaultPort)
	}

	if containerImg != "grafana/grafana:latest" {
		t.Errorf("expected containerImg 'grafana/grafana:latest', got %q", containerImg)
	}

	// Verify password variable substituted or defaulted
	if strings.Contains(normalized, "${GRAFANA_ADMIN_PASSWORD}") {
		t.Errorf("expected GRAFANA_ADMIN_PASSWORD to be resolved, got:\n%s", normalized)
	}

	// Verify published port injected since dokploy compose only specified service without ports
	if !strings.Contains(normalized, `"3000:3000"`) && !strings.Contains(normalized, "3000:3000") {
		t.Errorf("expected port mapping 3000:3000 injected, got:\n%s", normalized)
	}

	// Verify x-portals
	if !strings.Contains(normalized, "x-portals:") || !strings.Contains(normalized, "port: 3000") {
		t.Errorf("expected x-portals to be injected, got:\n%s", normalized)
	}

	if len(portals) == 0 {
		t.Errorf("expected portals to be extracted")
	}

	if len(questions) == 0 {
		t.Errorf("expected questions to be generated from variables")
	}
}

func TestEnrichFromDokploy(t *testing.T) {
	composeYAML := `services:
  app:
    image: test/app:latest
`
	templateTOML := `
[[config.domains]]
serviceName = "app"
port = 8080
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "docker-compose.yml") {
			_, _ = w.Write([]byte(composeYAML))
			return
		}
		if strings.HasSuffix(r.URL.Path, "template.toml") {
			_, _ = w.Write([]byte(templateTOML))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := defaultDokployBlueprintURL
	SetDokployBlueprintURLForTest(server.URL)
	defer SetDokployBlueprintURLForTest(origURL)

	app := domain.CatalogApp{
		AppID: "testapp",
		Train: "dokploy",
	}

	enrichFromDokploy(context.Background(), server.Client(), &app, t.TempDir(), "tank", "/mnt/tank/apps")

	if app.DefaultPort != 8080 {
		t.Errorf("expected defaultPort 8080, got %d", app.DefaultPort)
	}
	if app.ContainerImage != "test/app:latest" {
		t.Errorf("expected containerImage 'test/app:latest', got %q", app.ContainerImage)
	}
	if !strings.Contains(app.ComposeYAML, "8080:8080") {
		t.Errorf("expected injected port 8080:8080, got: %s", app.ComposeYAML)
	}
}
