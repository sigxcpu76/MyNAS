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
	cachedConfig  *domain.SystemConfig
	cachedTime    time.Time
}

func NewAdapter(zfsPort ports.ZfsEnginePort, prereqChecker ports.PrerequisiteCheckerPort) *Adapter {
	a := &Adapter{
		zfsPort:       zfsPort,
		prereqChecker: prereqChecker,
	}

	// Sync initial main pool from config
	cfg := config.GetGlobal()
	if cfg.System.MainPool != "" {
		prereqChecker.SetMainPool(cfg.System.MainPool)
	}

	// Pre-populate initial cached config so first calls return immediately without disk scan latency
	enabledModules := []string{"zfs"}
	if cfg.System.MainPool != "" {
		enabledModules = append(enabledModules, "kvm", "docker", "lxc", "cifs", "webdav")
	}
	a.cachedConfig = &domain.SystemConfig{
		MainPool:       cfg.System.MainPool,
		ConfigFilePath: config.GetConfigPath(),
		AutoDiscover:   cfg.System.AutoDiscover,
		EnabledModules: enabledModules,
	}
	a.cachedTime = time.Now()

	return a
}

func (a *Adapter) GetConfig(ctx context.Context) (*domain.SystemConfig, error) {
	a.mu.RLock()
	if a.cachedConfig != nil && time.Since(a.cachedTime) < 10*time.Second {
		cfgCopy := *a.cachedConfig
		a.mu.RUnlock()
		return &cfgCopy, nil
	}
	a.mu.RUnlock()

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cachedConfig != nil && time.Since(a.cachedTime) < 10*time.Second {
		cfgCopy := *a.cachedConfig
		return &cfgCopy, nil
	}

	cfg := config.GetGlobal()
	mainPool := cfg.System.MainPool

	systemDataset := ""
	systemMountpoint := ""
	isInit := false

	if mainPool != "" {
		systemDataset = fmt.Sprintf("%s/%s", mainPool, cfg.System.SystemDataset)
		poolMount := fmt.Sprintf("/nas/%s", mainPool)
		if p, pErr := a.zfsPort.GetPool(ctx, mainPool); pErr == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
			poolMount = p.Mountpoint
		}
		systemMountpoint = fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), cfg.System.SystemDataset)
		// Check if dataset exists
		if ds, err := a.zfsPort.GetDataset(ctx, mainPool, cfg.System.SystemDataset); err == nil && ds != nil {
			isInit = true
			if ds.Mountpoint != "" && ds.Mountpoint != "none" && ds.Mountpoint != "legacy" {
				systemMountpoint = ds.Mountpoint
			}
		}
	}

	enabledModules := []string{"zfs"}
	for _, mod := range []string{"kvm", "docker", "lxc", "cifs", "webdav"} {
		if ok, _ := a.prereqChecker.IsModuleEnabled(mod); ok {
			enabledModules = append(enabledModules, mod)
		}
	}

	discovered, _ := a.DiscoverPools(ctx)

	result := &domain.SystemConfig{
		MainPool:         mainPool,
		SystemDataset:    systemDataset,
		SystemMountpoint: systemMountpoint,
		IsInitialized:    isInit,
		EnabledModules:   enabledModules,
		ConfigFilePath:   config.GetConfigPath(),
		AutoDiscover:     cfg.System.AutoDiscover,
		DiscoveredPools:  discovered,
	}

	a.cachedConfig = result
	a.cachedTime = time.Now()

	cfgCopy := *result
	return &cfgCopy, nil
}

func (a *Adapter) SelectMainPool(ctx context.Context, poolName string, autoImportExisting bool) (*domain.SystemConfig, error) {
	cleanPool := strings.TrimPrefix(strings.TrimSpace(poolName), "zfsPools/")
	if cleanPool == "" {
		return nil, fmt.Errorf("%w: pool name cannot be empty", domain.ErrInvalidArgument)
	}

	// Verify pool exists in ZFS
	p, err := a.zfsPort.GetPool(ctx, cleanPool)
	if err != nil {
		return nil, fmt.Errorf("%w: pool %q does not exist or is not imported", domain.ErrNotFound, cleanPool)
	}

	cfg := config.GetGlobal()
	dsName := cfg.System.SystemDataset
	if dsName == "" {
		dsName = ".nas-system"
	}

	datasetPath := fmt.Sprintf("%s/%s", cleanPool, dsName)
	poolMount := fmt.Sprintf("/nas/%s", cleanPool)
	if p != nil && p.Mountpoint != "" && p.Mountpoint != "none" && p.Mountpoint != "legacy" {
		poolMount = p.Mountpoint
	}
	mountpoint := fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), dsName)

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
		if createErr != nil {
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

	a.mu.Lock()
	a.cachedTime = time.Time{}
	a.mu.Unlock()

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
		poolMount := fmt.Sprintf("/nas/%s", cleanID)
		if p.Mountpoint != "" && p.Mountpoint != "none" && p.Mountpoint != "legacy" {
			poolMount = p.Mountpoint
		}
		mp := fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), dsName)
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
