package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
