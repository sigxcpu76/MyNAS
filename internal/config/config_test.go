package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigxcpu/nas/internal/config"
)

func TestConfigLoadAndSave(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")

	// 1. Initial Load when file does not exist -> creates default
	cfg, path, err := config.Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.System.MainPool != "" {
		t.Errorf("expected initial MainPool to be empty, got %q", cfg.System.MainPool)
	}
	if !cfg.Modules.ZFS {
		t.Errorf("expected ZFS to always be enabled")
	}
	if path != configFile {
		t.Errorf("expected config path %q, got %q", configFile, path)
	}

	// Verify file was written
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("expected config.yaml to be created at %s", configFile)
	}

	// 2. Modify and Save
	cfg.System.MainPool = "tank"
	if err := config.Save(cfg, configFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 3. Reload and verify persistence
	reloaded, _, err := config.Load(configFile)
	if err != nil {
		t.Fatalf("re-load failed: %v", err)
	}
	if reloaded.System.MainPool != "tank" {
		t.Fatalf("expected reloaded MainPool to be 'tank', got %q", reloaded.System.MainPool)
	}
}
