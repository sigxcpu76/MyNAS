package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type Adapter struct {
	mu            sync.RWMutex
	zfsPort       ports.ZfsEnginePort
	prereqChecker ports.PrerequisiteCheckerPort
	simulated     bool
	simulatedPools map[string]domain.DiscoveredPool
}

func NewAdapter(zfsPort ports.ZfsEnginePort, prereqChecker ports.PrerequisiteCheckerPort, simulated bool) *Adapter {
	a := &Adapter{
		zfsPort:       zfsPort,
		prereqChecker: prereqChecker,
		simulated:     simulated,
		simulatedPools: make(map[string]domain.DiscoveredPool),
	}

	// Sync initial main pool from config
	cfg := config.GetGlobal()
	if cfg.System.MainPool != "" {
		prereqChecker.SetMainPool(cfg.System.MainPool)
	}

	return a
}

func (a *Adapter) GetConfig(ctx context.Context) (*domain.SystemConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cfg := config.GetGlobal()
	mainPool := cfg.System.MainPool

	systemDataset := ""
	systemMountpoint := ""
	isInit := false

	if mainPool != "" {
		systemDataset = fmt.Sprintf("%s/%s", mainPool, cfg.System.SystemDataset)
		systemMountpoint = fmt.Sprintf("/%s/%s", mainPool, cfg.System.SystemDataset)
		// Check if dataset exists
		if ds, err := a.zfsPort.GetDataset(ctx, mainPool, cfg.System.SystemDataset); err == nil && ds != nil {
			isInit = true
			if ds.Mountpoint != "" && ds.Mountpoint != "none" && ds.Mountpoint != "legacy" {
				systemMountpoint = ds.Mountpoint
			}
		} else if a.simulated {
			isInit = true
		}
	}

	enabledModules := []string{"zfs"}
	for _, mod := range []string{"kvm", "docker", "lxc", "cifs", "webdav"} {
		if ok, _ := a.prereqChecker.IsModuleEnabled(mod); ok {
			enabledModules = append(enabledModules, mod)
		}
	}

	discovered, _ := a.DiscoverPools(ctx)

	return &domain.SystemConfig{
		MainPool:         mainPool,
		SystemDataset:    systemDataset,
		SystemMountpoint: systemMountpoint,
		IsInitialized:    isInit,
		EnabledModules:   enabledModules,
		ConfigFilePath:   config.GetConfigPath(),
		AutoDiscover:     cfg.System.AutoDiscover,
		DiscoveredPools:  discovered,
	}, nil
}

func (a *Adapter) SelectMainPool(ctx context.Context, poolName string, autoImportExisting bool) (*domain.SystemConfig, error) {
	cleanPool := strings.TrimPrefix(strings.TrimSpace(poolName), "zfsPools/")
	if cleanPool == "" {
		return nil, fmt.Errorf("%w: pool name cannot be empty", domain.ErrInvalidArgument)
	}

	// Verify pool exists in ZFS
	_, err := a.zfsPort.GetPool(ctx, cleanPool)
	if err != nil {
		return nil, fmt.Errorf("%w: pool %q does not exist or is not imported", domain.ErrNotFound, cleanPool)
	}

	cfg := config.GetGlobal()
	dsName := cfg.System.SystemDataset
	if dsName == "" {
		dsName = ".nas-system"
	}

	datasetPath := fmt.Sprintf("%s/%s", cleanPool, dsName)
	mountpoint := fmt.Sprintf("/%s/%s", cleanPool, dsName)

	// Check if the system dataset already exists (re-installation / recovery case)
	existingDs, err := a.zfsPort.GetDataset(ctx, cleanPool, dsName)
	datasetExisted := (err == nil && existingDs != nil)

	if !datasetExisted {
		// Create system dataset
		_, createErr := a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
			PoolID:      cleanPool,
			ID:          dsName,
			DatasetPath: datasetPath,
			Mountpoint:  mountpoint,
		})
		if createErr != nil && !a.simulated {
			return nil, fmt.Errorf("failed to create system dataset %s: %w", datasetPath, createErr)
		}
	} else if existingDs != nil && existingDs.Mountpoint != "" && existingDs.Mountpoint != "none" {
		mountpoint = existingDs.Mountpoint
	}

	// Initialize filesystem directories inside system dataset
	subdirs := []string{"config", "vm", "docker", "lxc", "shares", "catalog", "identity"}
	for _, sub := range subdirs {
		dir := filepath.Join(mountpoint, sub)
		_ = os.MkdirAll(dir, 0755)
	}

	poolConfigFile := filepath.Join(mountpoint, "config", "config.yaml")

	// If dataset existed and config.yaml exists on the pool, discover/load its configuration!
	if datasetExisted && autoImportExisting {
		if _, statErr := os.Stat(poolConfigFile); statErr == nil {
			if poolCfg, _, loadErr := config.Load(poolConfigFile); loadErr == nil && poolCfg != nil {
				// Re-discovered config from imported pool!
				cfg.Modules = poolCfg.Modules
				if poolCfg.Server.JWTSecret != "" {
					cfg.Server.JWTSecret = poolCfg.Server.JWTSecret
				}
			}
		}
	}

	// Update main pool in config
	cfg.System.MainPool = cleanPool
	a.prereqChecker.SetMainPool(cleanPool)

	// Save config both to host config path and to pool dataset config
	localPath := config.GetConfigPath()
	if localPath == "" {
		localPath = "config.yaml"
	}
	_ = config.Save(cfg, localPath)
	_ = config.Save(cfg, poolConfigFile)

	return a.GetConfig(ctx)
}

func (a *Adapter) DiscoverPools(ctx context.Context) ([]domain.DiscoveredPool, error) {
	pools, err := a.zfsPort.ListPools(ctx)
	if err != nil {
		pools = nil
	}

	cfg := config.GetGlobal()
	dsName := cfg.System.SystemDataset
	if dsName == "" {
		dsName = ".nas-system"
	}

	var results []domain.DiscoveredPool

	for _, p := range pools {
		cleanID := strings.TrimPrefix(p.ID, "zfsPools/")
		hasSys := false
		mp := fmt.Sprintf("/%s/%s", cleanID, dsName)
		lastMod := time.Now().UTC().Format(time.RFC3339)

		// Check if dataset exists
		if ds, dsErr := a.zfsPort.GetDataset(ctx, cleanID, dsName); dsErr == nil && ds != nil {
			hasSys = true
			if ds.Mountpoint != "" && ds.Mountpoint != "none" {
				mp = ds.Mountpoint
			}
			poolCfgPath := filepath.Join(mp, "config", "config.yaml")
			if fi, fiErr := os.Stat(poolCfgPath); fiErr == nil {
				lastMod = fi.ModTime().UTC().Format(time.RFC3339)
			}
		}

		results = append(results, domain.DiscoveredPool{
			PoolName:      cleanID,
			HasNasSystem:  hasSys,
			ConfigVersion: "1.0",
			LastModified:  lastMod,
			IsImported:    true,
			DatasetPath:   fmt.Sprintf("%s/%s", cleanID, dsName),
			Mountpoint:    mp,
		})
	}

	// Also check unimported pools via `zpool import` if host zpool binary is available
	if unimported, err := listUnimportedHostPools(ctx); err == nil {
		for _, name := range unimported {
			results = append(results, domain.DiscoveredPool{
				PoolName:      name,
				HasNasSystem:  false, // cannot verify dataset until imported
				ConfigVersion: "1.0",
				LastModified:  time.Now().UTC().Format(time.RFC3339),
				IsImported:    false,
				DatasetPath:   fmt.Sprintf("%s/%s", name, dsName),
				Mountpoint:    "",
			})
		}
	}

	return results, nil
}

func listUnimportedHostPools(ctx context.Context) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, "zpool", "import")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var pools []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pool:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				pools = append(pools, parts[1])
			}
		}
	}
	return pools, nil
}
