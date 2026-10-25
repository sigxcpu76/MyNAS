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
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("expected initial HTTPPort to be 8080, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Server.GRPCPort != 9090 {
		t.Errorf("expected initial GRPCPort to be 9090, got %d", cfg.Server.GRPCPort)
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
	cfg.Server.HTTPPort = 8088
	cfg.Server.GRPCPort = 9099
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
	if reloaded.Server.HTTPPort != 8088 {
		t.Fatalf("expected reloaded HTTPPort to be 8088, got %d", reloaded.Server.HTTPPort)
	}
	if reloaded.Server.GRPCPort != 9099 {
		t.Fatalf("expected reloaded GRPCPort to be 9099, got %d", reloaded.Server.GRPCPort)
	}
}
