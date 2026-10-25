package system_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestSystemAdapterPoolSelectionAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")

	// Set temporary config file
	cfg, _, err := config.Load(configFile)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	zfsAdapter := zfs.NewAdapter(true)
	prereqChecker := prereq.NewSystemChecker(true)
	adapter := system.NewAdapter(zfsAdapter, prereqChecker, true)

	ctx := context.Background()

	// 1. Initial State: No main pool -> Only ZFS enabled
	sysCfg, err := adapter.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if sysCfg.MainPool != "" {
		t.Errorf("expected empty MainPool, got %q", sysCfg.MainPool)
	}
	if sysCfg.IsInitialized {
		t.Errorf("expected IsInitialized=false")
	}

	zfsOk, _ := prereqChecker.IsModuleEnabled("zfs")
	if !zfsOk {
		t.Errorf("expected ZFS to be enabled")
	}
	dockerOk, dockerReason := prereqChecker.IsModuleEnabled("docker")
	if dockerOk {
		t.Errorf("expected Docker to be disabled when no main pool is selected")
	}
	t.Logf("Docker gating verified: %s", dockerReason)

	// 2. Create a test pool in ZFS
	_, err = zfsAdapter.CreatePool(ctx, domain.ZfsPool{
		ID:    "mypool",
		State: "ONLINE",
	})
	if err != nil {
		t.Fatalf("CreatePool failed: %v", err)
	}

	// 3. Select "mypool" as Main Pool
	updatedCfg, err := adapter.SelectMainPool(ctx, "mypool", true)
	if err != nil {
		t.Fatalf("SelectMainPool failed: %v", err)
	}
	if updatedCfg.MainPool != "mypool" {
		t.Fatalf("expected MainPool 'mypool', got %q", updatedCfg.MainPool)
	}
	if updatedCfg.SystemDataset != "mypool/.nas-system" {
		t.Fatalf("expected dataset 'mypool/.nas-system', got %q", updatedCfg.SystemDataset)
	}

	// 4. Verify Docker and KVM are now enabled
	dockerNowOk, _ := prereqChecker.IsModuleEnabled("docker")
	if !dockerNowOk {
		t.Fatalf("expected Docker to be enabled after selecting main pool")
	}

	// 5. Test Reinstall / Discovery scenario:
	// Reset local config MainPool to "" as if the OS was reinstalled
	cfg.System.MainPool = ""
	_ = config.Save(cfg, configFile)
	prereqChecker.SetMainPool("")

	// Run DiscoverPools:
	discovered, err := adapter.DiscoverPools(ctx)
	if err != nil {
		t.Fatalf("DiscoverPools failed: %v", err)
	}
	found := false
	for _, p := range discovered {
		if p.PoolName == "mypool" && p.HasNasSystem {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mypool to be discovered with HasNasSystem=true, got %+v", discovered)
	}

	// Re-import / select discovered pool to restore configuration
	restored, err := adapter.SelectMainPool(ctx, "mypool", true)
	if err != nil {
		t.Fatalf("failed to restore pool: %v", err)
	}
	if restored.MainPool != "mypool" {
		t.Fatalf("expected restored pool 'mypool', got %q", restored.MainPool)
	}
	t.Logf("Successfully verified reinstall and config recovery from pool: %+v", restored)

	_ = os.RemoveAll(tempDir)
}
