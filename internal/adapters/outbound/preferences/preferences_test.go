package preferences_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/preferences"
	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestFileAdapter_GetPreferences_Default(t *testing.T) {
	tempDir := t.TempDir()
	adapter := preferences.NewFileAdapterWithBase(tempDir)
	user := &domain.User{
		Username:      "alice",
		HomeDirectory: filepath.Join(tempDir, "alice"),
	}

	prefs, err := adapter.GetPreferences(context.Background(), user)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if prefs.Dashboard.ColumnCount != 3 {
		t.Errorf("expected default column count 3, got %d", prefs.Dashboard.ColumnCount)
	}
	if len(prefs.Dashboard.Layout) != 4 {
		t.Errorf("expected 4 default widgets, got %d", len(prefs.Dashboard.Layout))
	}
}

func TestFileAdapter_UpdateAndGetPreferences(t *testing.T) {
	tempDir := t.TempDir()
	adapter := preferences.NewFileAdapterWithBase(tempDir)
	user := &domain.User{
		Username:      "bob",
		HomeDirectory: filepath.Join(tempDir, "bob"),
	}

	customPrefs := &domain.UserPreferences{
		Dashboard: domain.DashboardPreferences{
			ColumnCount: 2,
			Layout: []domain.DashboardWidgetPreference{
				{ID: "widget-kpis", Type: "kpis", Lane: -1},
				{ID: "widget-pools", Type: "pools", Lane: 0},
				{ID: "widget-apps", Type: "apps", Lane: 1},
			},
		},
		Custom: map[string]any{
			"theme": "dark",
		},
	}

	updated, err := adapter.UpdatePreferences(context.Background(), user, customPrefs)
	if err != nil {
		t.Fatalf("failed to update preferences: %v", err)
	}
	if updated.Dashboard.ColumnCount != 2 {
		t.Errorf("expected column count 2, got %d", updated.Dashboard.ColumnCount)
	}

	// Verify file was written on disk at ~/.config/nas_preferences.yaml
	expectedPath := filepath.Join(tempDir, "bob", ".config", "nas_preferences.yaml")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected preferences file to exist at %s: %v", expectedPath, err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to parse written YAML: %v", err)
	}
	if raw["theme"] != "dark" {
		t.Errorf("expected theme 'dark' preserved in YAML, got %v", raw["theme"])
	}

	// Read back through adapter
	readBack, err := adapter.GetPreferences(context.Background(), user)
	if err != nil {
		t.Fatalf("failed to read back preferences: %v", err)
	}
	if readBack.Dashboard.ColumnCount != 2 {
		t.Errorf("expected column count 2, got %d", readBack.Dashboard.ColumnCount)
	}
	if len(readBack.Dashboard.Layout) != 3 {
		t.Fatalf("expected 3 widgets, got %d", len(readBack.Dashboard.Layout))
	}
	if readBack.Dashboard.Layout[2].Type != "apps" {
		t.Errorf("expected widget 2 type 'apps', got %s", readBack.Dashboard.Layout[2].Type)
	}
}

func TestFileAdapter_RejectsSymlinkedConfigAndPreferencesFile(t *testing.T) {
	tempDir := t.TempDir()
	outsideFile := filepath.Join(tempDir, "outside-secret.yaml")
	const originalSecret = "secret: do-not-overwrite\n"
	if err := os.WriteFile(outsideFile, []byte(originalSecret), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	adapter := preferences.NewFileAdapterWithBase(tempDir)

	// Case 1: .config is a symlink pointing outside the user's home directory
	u1Home := filepath.Join(tempDir, "mallory1")
	outsideDir := filepath.Join(tempDir, "outside-dir")
	_ = os.MkdirAll(u1Home, 0o755)
	_ = os.MkdirAll(outsideDir, 0o755)
	if err := os.Symlink(outsideDir, filepath.Join(u1Home, ".config")); err != nil {
		t.Fatalf("symlink .config: %v", err)
	}
	u1 := &domain.User{Username: "mallory1", HomeDirectory: u1Home}
	if _, err := adapter.UpdatePreferences(context.Background(), u1, preferences.DefaultUserPreferences()); err == nil {
		t.Fatalf("expected UpdatePreferences to reject symlinked .config directory")
	}
	if _, err := os.Stat(filepath.Join(outsideDir, preferences.PreferencesFileName)); err == nil {
		t.Fatalf("symlinked .config caused file creation outside home directory")
	}

	// Case 2: .config/nas_preferences.yaml is a symlink to an external file
	u2Home := filepath.Join(tempDir, "mallory2")
	_ = os.MkdirAll(filepath.Join(u2Home, ".config"), 0o755)
	if err := os.Symlink(outsideFile, filepath.Join(u2Home, ".config", preferences.PreferencesFileName)); err != nil {
		t.Fatalf("symlink preferences file: %v", err)
	}
	u2 := &domain.User{Username: "mallory2", HomeDirectory: u2Home}
	if _, err := adapter.GetPreferences(context.Background(), u2); err == nil {
		t.Fatalf("expected GetPreferences to reject symlinked preferences file")
	}
	if _, err := adapter.UpdatePreferences(context.Background(), u2, preferences.DefaultUserPreferences()); err == nil {
		t.Fatalf("expected UpdatePreferences to reject symlinked preferences file")
	}
	gotSecret, err := os.ReadFile(outsideFile)
	if err != nil || string(gotSecret) != originalSecret {
		t.Fatalf("outside target file was modified: %q (err=%v)", string(gotSecret), err)
	}
}
