package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
)

func TestCatalogAdapter_SeededAppsAndFiltering(t *testing.T) {
	tempDir := t.TempDir()
	adapter := catalog.NewAdapter(tempDir)
	ctx := context.Background()

	// 1. List seeded apps
	apps, err := adapter.ListApps(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListApps failed: %v", err)
	}
	if len(apps) == 0 {
		t.Fatalf("expected seeded apps in catalog, got 0")
	}

	// 2. Query filter
	syncthingApps, err := adapter.ListApps(ctx, "", "", "syncthing")
	if err != nil {
		t.Fatalf("ListApps with query failed: %v", err)
	}
	if len(syncthingApps) == 0 {
		t.Fatalf("expected to find syncthing with query 'syncthing'")
	}
	if syncthingApps[0].AppID != "syncthing" {
		t.Errorf("expected syncthing, got %s", syncthingApps[0].AppID)
	}

	// 3. Category filter
	mediaApps, err := adapter.ListApps(ctx, "", "media", "")
	if err != nil {
		t.Fatalf("ListApps with category failed: %v", err)
	}
	if len(mediaApps) == 0 {
		t.Fatalf("expected media apps, got 0")
	}

	// 4. GetApp with resolved compose YAML
	app, err := adapter.GetApp(ctx, "stable", "syncthing")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app.ComposeYAML == "" {
		t.Errorf("expected non-empty resolved ComposeYAML for syncthing")
	}
	if app.DefaultPort != 8384 {
		t.Errorf("expected default port 8384, got %d", app.DefaultPort)
	}
}

func TestCatalogAdapter_LiveSyncMock(t *testing.T) {
	mockCatalog := map[string]map[string]any{
		"stable": {
			"custom-app": map[string]any{
				"name":        "custom-app",
				"title":       "Custom Cloud App",
				"description": "A test custom app from TrueNAS catalog",
				"categories":  []string{"cloud", "storage"},
				"icon_url":    "https://example.com/icon.png",
			},
		},
		"community": {
			"comm-app": map[string]any{
				"name":        "comm-app",
				"title":       "Community App",
				"description": "Another test app",
				"categories":  []string{"tools"},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockCatalog)
	}))
	defer ts.Close()

	tempDir := t.TempDir()
	adapter := catalog.NewAdapter(tempDir)
	adapter.SetCatalogURLForTest(ts.URL)

	ctx := context.Background()
	count, syncTime, err := adapter.SyncCatalog(ctx, true)
	if err != nil {
		t.Fatalf("SyncCatalog failed: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 synced apps, got %d", count)
	}
	if syncTime.IsZero() {
		t.Errorf("expected non-zero sync time")
	}

	// Verify custom-app is retrieved
	app, err := adapter.GetApp(ctx, "stable", "custom-app")
	if err != nil {
		t.Fatalf("GetApp custom-app failed: %v", err)
	}
	if app.Title != "Custom Cloud App" {
		t.Errorf("expected 'Custom Cloud App', got %q", app.Title)
	}
}

func TestCatalogAdapter_NginxProxyManagerAndImageResolution(t *testing.T) {
	tempDir := t.TempDir()
	adapter := catalog.NewAdapter(tempDir)
	ctx := context.Background()

	// 1. Verify nginx-proxy-manager has correct image, ports, and volumes
	npm, err := adapter.GetApp(ctx, "community", "nginx-proxy-manager")
	if err != nil {
		t.Fatalf("GetApp nginx-proxy-manager failed: %v", err)
	}
	if !strings.HasPrefix(npm.ContainerImage, "jc21/nginx-proxy-manager") {
		t.Errorf("expected container image starting with 'jc21/nginx-proxy-manager', got %q", npm.ContainerImage)
	}
	if npm.DefaultPort != 81 {
		t.Errorf("expected default port 81, got %d", npm.DefaultPort)
	}
	if !strings.Contains(npm.ComposeYAML, "jc21/nginx-proxy-manager") {
		t.Errorf("compose YAML does not contain correct image: %s", npm.ComposeYAML)
	}
	if !strings.Contains(npm.ComposeYAML, "81:81") || !strings.Contains(npm.ComposeYAML, "80:80") {
		t.Errorf("compose YAML does not contain web and proxy ports: %s", npm.ComposeYAML)
	}
	if !strings.Contains(npm.ComposeYAML, "x-portals:") {
		t.Errorf("expected x-portals in compose YAML: %s", npm.ComposeYAML)
	}

	// 2. Mock TrueNAS catalog with sources and verify image resolution
	mockCatalog := map[string]map[string]any{
		"community": {
			"source-app-1": map[string]any{
				"name":        "source-app-1",
				"title":       "Docker Hub Source App",
				"description": "App with docker hub source",
				"sources":     []string{"https://hub.docker.com/r/myorg/myapp"},
			},
			"source-app-2": map[string]any{
				"name":        "source-app-2",
				"title":       "GHCR Source App",
				"description": "App with GHCR source",
				"sources":     []string{"https://ghcr.io/orgname/myrepo"},
			},
			"source-app-3": map[string]any{
				"name":        "source-app-3",
				"title":       "LinuxServer Source App",
				"description": "App with Linuxserver github source",
				"sources":     []string{"https://github.com/linuxserver/docker-myapp"},
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockCatalog)
	}))
	defer ts.Close()

	adapter.SetCatalogURLForTest(ts.URL)
	_, _, err = adapter.SyncCatalog(ctx, true)
	if err != nil {
		t.Fatalf("SyncCatalog failed: %v", err)
	}

	app1, err := adapter.GetApp(ctx, "community", "source-app-1")
	if err != nil {
		t.Fatalf("GetApp source-app-1 failed: %v", err)
	}
	if app1.ContainerImage != "myorg/myapp:latest" {
		t.Errorf("expected 'myorg/myapp:latest', got %q", app1.ContainerImage)
	}

	app2, err := adapter.GetApp(ctx, "community", "source-app-2")
	if err != nil {
		t.Fatalf("GetApp source-app-2 failed: %v", err)
	}
	if app2.ContainerImage != "ghcr.io/orgname/myrepo:latest" {
		t.Errorf("expected 'ghcr.io/orgname/myrepo:latest', got %q", app2.ContainerImage)
	}

	app3, err := adapter.GetApp(ctx, "community", "source-app-3")
	if err != nil {
		t.Fatalf("GetApp source-app-3 failed: %v", err)
	}
	if app3.ContainerImage != "lscr.io/linuxserver/myapp:latest" {
		t.Errorf("expected 'lscr.io/linuxserver/myapp:latest', got %q", app3.ContainerImage)
	}
}

func TestExtractPortalFromTrueNASTemplate(t *testing.T) {
	tpl := `
  {% do c1.add_port(values.network.web_port, {"container_port": 81}) %}
  {% do tpl.portals.add(values.network.web_port) %}
`
	port, name, scheme, path := catalog.ExtractPortalFromTemplate(tpl, "")
	if port != 81 || name != "Web UI" || scheme != "http" || path != "/" {
		t.Fatalf("unexpected extracted portal: port=%d name=%s scheme=%s path=%s", port, name, scheme, path)
	}

	jellyfinTpl := `
  {% do c1.add_port(values.network.web_port, {"container_port": values.consts.internal_web_port}) %}
  {% do tpl.portals.add(values.network.web_port) %}
`
	ixValues := `
consts:
  internal_web_port: 8096
`
	port2, _, _, _ := catalog.ExtractPortalFromTemplate(jellyfinTpl, ixValues)
	if port2 != 8096 {
		t.Fatalf("expected port 8096, got %d", port2)
	}
}

func TestExtractPortalsFromTemplate_MultipleAndFromQuestions(t *testing.T) {
	tpl := `
  {% do c1.add_port(values.network.web_port) %}
  {% do c1.add_port(values.network.bt_port) %}
  {% do tpl.portals.add(values.network.web_port, {"scheme": "http"}) %}
  {% do tpl.portals.add(values.network.web_port, {"name": "Admin Portal", "path": "/admin", "scheme": "https"}) %}
`
	questions := []byte(`
questions:
  - variable: network
    schema:
      attrs:
        - variable: web_port
          schema:
            attrs:
              - variable: port_number
                default: 30024
`)

	portals := catalog.ExtractPortalsFromTemplate(tpl, "", questions)
	if len(portals) != 2 {
		t.Fatalf("expected 2 portals, got %d", len(portals))
	}

	p1 := portals[0]
	if p1.Name != "Web UI" || p1.Port != 30024 || p1.Scheme != "http" || p1.Path != "/" {
		t.Errorf("unexpected portal 1: %+v", p1)
	}

	p2 := portals[1]
	if p2.Name != "Admin Portal" || p2.Port != 30024 || p2.Scheme != "https" || p2.Path != "/admin" {
		t.Errorf("unexpected portal 2: %+v", p2)
	}
}
