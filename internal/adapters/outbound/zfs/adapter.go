package zfs

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type Adapter struct {
	mu       sync.RWMutex
	hybrid   bool
	pools    map[string]domain.ZfsPool
	datasets map[string]map[string]domain.ZfsDataset
	volumes  map[string]map[string]domain.ZfsVolume
}

func NewAdapter(hybrid bool) *Adapter {
	a := &Adapter{
		hybrid:   hybrid,
		pools:    make(map[string]domain.ZfsPool),
		datasets: make(map[string]map[string]domain.ZfsDataset),
		volumes:  make(map[string]map[string]domain.ZfsVolume),
	}
	if hybrid {
		a.seedDefaults()
	}
	return a
}

func (a *Adapter) canUseHostZfs() bool {
	if a.hybrid {
		return false
	}
	if _, err := exec.LookPath("zpool"); err != nil {
		return false
	}
	if _, err := exec.LookPath("zfs"); err != nil {
		return false
	}
	return true
}

func (a *Adapter) seedDefaults() {
	a.pools["tank"] = domain.ZfsPool{
		ID:                   "tank",
		PoolName:             "tank",
		State:                "ONLINE",
		SizeBytes:            16000000000000, // 16 TB
		AllocatedBytes:       5400000000000,  // 5.4 TB
		FreeBytes:            10600000000000, // 10.6 TB
		FragmentationPercent: 6,
		CapacityPercent:      33,
		DedupRatio:           1.04,
		VdevLayout:           "raidz1",
		Devices:              []string{"/dev/sda", "/dev/sdb", "/dev/sdc", "/dev/sdd"},
		Mountpoint:           "/tank",
		Compression:          "lz4",
		LastScrubStatus:      "0 errors repaired on 2026-10-20",
		HealthDetails:        "All vdevs ONLINE; RAIDZ1 parity healthy",
	}
	a.pools["nvme-fast"] = domain.ZfsPool{
		ID:                   "nvme-fast",
		PoolName:             "nvme-fast",
		State:                "ONLINE",
		SizeBytes:            2000000000000, // 2 TB
		AllocatedBytes:       420000000000,  // 420 GB
		FreeBytes:            1580000000000, // 1.58 TB
		FragmentationPercent: 3,
		CapacityPercent:      21,
		DedupRatio:           1.00,
		VdevLayout:           "mirror",
		Devices:              []string{"/dev/nvme0n1", "/dev/nvme1n1"},
		Mountpoint:           "/nvme-fast",
		Compression:          "zstd",
		LastScrubStatus:      "0 errors repaired on 2026-10-22",
		HealthDetails:        "2-way NVMe mirror ONLINE",
	}

	a.datasets["tank"] = map[string]domain.ZfsDataset{
		"media": {
			PoolID:          "tank",
			ID:              "media",
			DatasetPath:     "tank/media",
			Mountpoint:      "/tank/media",
			UsedBytes:       3800000000000,
			AvailableBytes:  10600000000000,
			ReferencedBytes: 3800000000000,
			QuotaBytes:      8000000000000,
			Compression:     "lz4",
			Readonly:        false,
			Atime:           false,
			Recordsize:      "1M",
			SnapshotsCount:  4,
			LastSnapshot:    "tank/media@auto-2026-10-23",
		},
		"backups": {
			PoolID:          "tank",
			ID:              "backups",
			DatasetPath:     "tank/backups",
			Mountpoint:      "/tank/backups",
			UsedBytes:       1200000000000,
			AvailableBytes:  10600000000000,
			ReferencedBytes: 1200000000000,
			QuotaBytes:      4000000000000,
			Compression:     "zstd",
			Readonly:        false,
			Atime:           false,
			Recordsize:      "128K",
			SnapshotsCount:  12,
			LastSnapshot:    "tank/backups@daily-2026-10-24",
		},
	}
	a.datasets["nvme-fast"] = map[string]domain.ZfsDataset{
		"appdata": {
			PoolID:          "nvme-fast",
			ID:              "appdata",
			DatasetPath:     "nvme-fast/appdata",
			Mountpoint:      "/nvme-fast/appdata",
			UsedBytes:       120000000000,
			AvailableBytes:  1580000000000,
			ReferencedBytes: 120000000000,
			QuotaBytes:      500000000000,
			Compression:     "zstd",
			Readonly:        false,
			Atime:           false,
			Recordsize:      "16K",
			SnapshotsCount:  7,
			LastSnapshot:    "nvme-fast/appdata@pre-upgrade",
		},
	}

	a.volumes["nvme-fast"] = map[string]domain.ZfsVolume{
		"vm-ubuntu-root": {
			PoolID:          "nvme-fast",
			ID:              "vm-ubuntu-root",
			VolumePath:      "nvme-fast/vm-ubuntu-root",
			DevicePath:      "/dev/zvol/nvme-fast/vm-ubuntu-root",
			SizeBytes:       64 * 1024 * 1024 * 1024, // 64 GiB
			UsedBytes:       18 * 1024 * 1024 * 1024,
			Volblocksize:    "16K",
			Compression:     "lz4",
			ThinProvisioned: true,
		},
	}
	a.volumes["tank"] = map[string]domain.ZfsVolume{
		"kvm-storage-lun0": {
			PoolID:          "tank",
			ID:              "kvm-storage-lun0",
			VolumePath:      "tank/kvm-storage-lun0",
			DevicePath:      "/dev/zvol/tank/kvm-storage-lun0",
			SizeBytes:       250 * 1024 * 1024 * 1024, // 250 GiB
			UsedBytes:       90 * 1024 * 1024 * 1024,
			Volblocksize:    "64K",
			Compression:     "lz4",
			ThinProvisioned: false,
		},
	}
}

// =============================================================================
// ZFS Pools
// =============================================================================

type zpoolPropertyJSON struct {
	Value string `json:"value"`
}

type zpoolPoolJSON struct {
	Name       string                       `json:"name"`
	Type       string                       `json:"type"`
	State      string                       `json:"state"`
	Properties map[string]zpoolPropertyJSON `json:"properties"`
}

type zpoolListJSON struct {
	Pools map[string]zpoolPoolJSON `json:"pools"`
}

type zpoolVdevJSON struct {
	Name           string                   `json:"name"`
	VdevType       string                   `json:"vdev_type"`
	Class          string                   `json:"class"`
	Path           string                   `json:"path"`
	State          string                   `json:"state"`
	ReadErrors     string                   `json:"read_errors"`
	WriteErrors    string                   `json:"write_errors"`
	ChecksumErrors string                   `json:"checksum_errors"`
	Vdevs          map[string]zpoolVdevJSON `json:"vdevs"`
}

type zpoolStatusPoolJSON struct {
	Name       string                   `json:"name"`
	State      string                   `json:"state"`
	ErrorCount string                   `json:"error_count"`
	Vdevs      map[string]zpoolVdevJSON `json:"vdevs"`
}

type zpoolStatusJSON struct {
	Pools map[string]zpoolStatusPoolJSON `json:"pools"`
}

type zfsPropertyJSON struct {
	Value string `json:"value"`
}

type zfsItemJSON struct {
	Name       string                     `json:"name"`
	Type       string                     `json:"type"`
	Pool       string                     `json:"pool"`
	Properties map[string]zfsPropertyJSON `json:"properties"`
}

type zfsListJSON struct {
	Datasets map[string]zfsItemJSON `json:"datasets"`
}

func buildVdevTree(vdevs map[string]zpoolVdevJSON) []domain.ZfsVdev {
	var res []domain.ZfsVdev
	for name, v := range vdevs {
		vType := v.Class
		if vType == "" || vType == "normal" {
			vType = "data"
		}
		vt := strings.ToLower(v.VdevType)
		if vt == "l2arc" {
			vType = "cache"
		} else if vt == "log" {
			vType = "log"
		} else if vt == "special" {
			vType = "special"
		} else if vt == "dedup" {
			vType = "dedup"
		} else if vt == "spare" {
			vType = "spare"
		}

		node := domain.ZfsVdev{
			Name:           name,
			Type:           vType,
			Layout:         v.VdevType,
			State:          v.State,
			Path:           v.Path,
			ReadErrors:     v.ReadErrors,
			WriteErrors:    v.WriteErrors,
			ChecksumErrors: v.ChecksumErrors,
		}
		if len(v.Vdevs) > 0 {
			node.Children = buildVdevTree(v.Vdevs)
		}
		res = append(res, node)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Name < res[j].Name
	})
	return res
}

func extractVdevInfo(vdevs map[string]zpoolVdevJSON) (layout string, devices []string) {
	for _, v := range vdevs {
		vt := strings.ToLower(v.VdevType)
		if vt == "mirror" || strings.HasPrefix(vt, "raidz") {
			layout = vt
		}
		if v.Path != "" {
			devices = append(devices, v.Path)
		} else if v.VdevType == "disk" || v.VdevType == "file" {
			devices = append(devices, v.Name)
		}
		if len(v.Vdevs) > 0 {
			subLayout, subDevs := extractVdevInfo(v.Vdevs)
			if layout == "" && subLayout != "" {
				layout = subLayout
			}
			devices = append(devices, subDevs...)
		}
	}
	if layout == "" {
		layout = "stripe"
	}
	return layout, devices
}

func formatRecordsize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return "128K"
	}
	if strings.HasSuffix(raw, "K") || strings.HasSuffix(raw, "k") ||
		strings.HasSuffix(raw, "M") || strings.HasSuffix(raw, "m") ||
		strings.HasSuffix(raw, "G") || strings.HasSuffix(raw, "g") {
		return strings.ToUpper(raw)
	}
	b, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	switch {
	case b >= 1024*1024*1024 && b%(1024*1024*1024) == 0:
		return fmt.Sprintf("%dG", b/(1024*1024*1024))
	case b >= 1024*1024 && b%(1024*1024) == 0:
		return fmt.Sprintf("%dM", b/(1024*1024))
	case b >= 1024 && b%1024 == 0:
		return fmt.Sprintf("%dK", b/1024)
	default:
		return raw
	}
}

func (a *Adapter) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	if a.canUseHostZfs() {
		if pools, err := a.listHostPools(ctx); err == nil && len(pools) > 0 {
			a.mu.Lock()
			for _, p := range pools {
				a.pools[p.ID] = p
			}
			a.mu.Unlock()
			return pools, nil
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.ZfsPool, 0, len(a.pools))
	for _, p := range a.pools {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (a *Adapter) listHostPools(ctx context.Context) ([]domain.ZfsPool, error) {
	cmd := exec.CommandContext(ctx, "zpool", "list", "-j", "-p")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var zpoolData zpoolListJSON
	if err := json.Unmarshal(out, &zpoolData); err != nil {
		return nil, err
	}

	var statusMap map[string]zpoolStatusPoolJSON
	statusCmd := exec.CommandContext(ctx, "zpool", "status", "-j")
	if stOut, stErr := statusCmd.Output(); stErr == nil {
		var stJSON zpoolStatusJSON
		if json.Unmarshal(stOut, &stJSON) == nil {
			statusMap = stJSON.Pools
		}
	}

	var pools []domain.ZfsPool
	for poolName, p := range zpoolData.Pools {
		size, _ := strconv.ParseInt(p.Properties["size"].Value, 10, 64)
		alloc, _ := strconv.ParseInt(p.Properties["allocated"].Value, 10, 64)
		free, _ := strconv.ParseInt(p.Properties["free"].Value, 10, 64)
		frag, _ := strconv.Atoi(strings.TrimSuffix(p.Properties["fragmentation"].Value, "%"))
		capPct, _ := strconv.Atoi(strings.TrimSuffix(p.Properties["capacity"].Value, "%"))
		dedup, _ := strconv.ParseFloat(strings.TrimSuffix(p.Properties["dedupratio"].Value, "x"), 64)
		state := p.State
		if state == "" {
			state = p.Properties["health"].Value
		}
		if state == "" {
			state = "ONLINE"
		}

		layout := "stripe"
		var devices []string
		var vdevTree []domain.ZfsVdev
		errCount := "0"
		if stPool, ok := statusMap[poolName]; ok {
			layout, devices = extractVdevInfo(stPool.Vdevs)
			errCount = stPool.ErrorCount
			if errCount == "" {
				errCount = "0"
			}
			if rootV, hasRoot := stPool.Vdevs[poolName]; hasRoot {
				vdevTree = buildVdevTree(rootV.Vdevs)
			} else {
				vdevTree = buildVdevTree(stPool.Vdevs)
			}
		}

		mountpoint := "/" + poolName
		compression := "lz4"
		syncMode := "standard"
		rootCmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-o", "name,mountpoint,compression,sync", poolName)
		if rootOut, rootErr := rootCmd.Output(); rootErr == nil {
			var rootJSON zfsListJSON
			if json.Unmarshal(rootOut, &rootJSON) == nil {
				if rds, ok := rootJSON.Datasets[poolName]; ok {
					if mp := rds.Properties["mountpoint"].Value; mp != "" && mp != "-" {
						mountpoint = mp
					}
					if comp := rds.Properties["compression"].Value; comp != "" && comp != "-" {
						compression = comp
					}
					if s := rds.Properties["sync"].Value; s != "" && s != "-" {
						syncMode = s
					}
				}
			}
		}

		autotrim := "off"
		trimCmd := exec.CommandContext(ctx, "zpool", "get", "-j", "-p", "autotrim", poolName)
		if trimOut, trimErr := trimCmd.Output(); trimErr == nil {
			var trimJSON zpoolListJSON
			if json.Unmarshal(trimOut, &trimJSON) == nil {
				if tp, ok := trimJSON.Pools[poolName]; ok {
					if at := tp.Properties["autotrim"].Value; at != "" && at != "-" {
						autotrim = at
					}
				}
			}
		}

		pools = append(pools, domain.ZfsPool{
			ID:                   poolName,
			PoolName:             poolName,
			State:                state,
			SizeBytes:            size,
			AllocatedBytes:       alloc,
			FreeBytes:            free,
			FragmentationPercent: int32(frag),
			CapacityPercent:      int32(capPct),
			DedupRatio:           dedup,
			VdevLayout:           layout,
			Devices:              devices,
			Mountpoint:           mountpoint,
			Compression:          compression,
			LastScrubStatus:      "No scrub errors recorded",
			HealthDetails:        fmt.Sprintf("All vdevs %s; %s pool errors", state, errCount),
			Sync:                 syncMode,
			Autotrim:             autotrim,
			Vdevs:                vdevTree,
		})
	}

	sort.Slice(pools, func(i, j int) bool {
		return pools[i].ID < pools[j].ID
	})
	return pools, nil
}

func (a *Adapter) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if a.canUseHostZfs() {
		pools, err := a.listHostPools(ctx)
		if err == nil {
			for _, p := range pools {
				if p.ID == cleanID {
					a.mu.Lock()
					a.pools[p.ID] = p
					a.mu.Unlock()
					return &p, nil
				}
			}
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	p, ok := a.pools[cleanID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
	}
	cp := p
	return &cp, nil
}

func (a *Adapter) CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.pools[pool.ID]; exists {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrAlreadyExists, pool.ID)
	}
	if a.canUseHostZfs() {
		args := []string{"create", "-m", pool.Mountpoint, pool.ID}
		if pool.VdevLayout != "" && pool.VdevLayout != "stripe" {
			args = append(args, pool.VdevLayout)
		}
		args = append(args, pool.Devices...)
		_ = exec.CommandContext(ctx, "zpool", args...).Run()
	}
	pool.State = "ONLINE"
	pool.SizeBytes = int64(len(pool.Devices)) * 2000000000000
	pool.AllocatedBytes = 1048576
	pool.FreeBytes = pool.SizeBytes - pool.AllocatedBytes
	pool.FragmentationPercent = 0
	pool.CapacityPercent = 0
	pool.DedupRatio = 1.00
	pool.LastScrubStatus = "Never scrubbed (newly created)"
	pool.HealthDetails = fmt.Sprintf("%s layout across %d device(s) ONLINE", strings.ToUpper(pool.VdevLayout), len(pool.Devices))
	a.pools[pool.ID] = pool
	if a.datasets[pool.ID] == nil {
		a.datasets[pool.ID] = make(map[string]domain.ZfsDataset)
	}
	if a.volumes[pool.ID] == nil {
		a.volumes[pool.ID] = make(map[string]domain.ZfsVolume)
	}
	cp := pool
	return &cp, nil
}

func (a *Adapter) DeletePool(ctx context.Context, id string, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if a.canUseHostZfs() {
		args := []string{"destroy"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, cleanID)
		_ = exec.CommandContext(ctx, "zpool", args...).Run()
	}
	delete(a.pools, cleanID)
	delete(a.datasets, cleanID)
	delete(a.volumes, cleanID)
	return nil
}

func (a *Adapter) ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if a.canUseHostZfs() {
		_ = exec.CommandContext(ctx, "zpool", "scrub", cleanID).Run()
		return a.GetPool(ctx, cleanID)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pool, exists := a.pools[cleanID]
	if !exists {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
	}
	pool.LastScrubStatus = fmt.Sprintf("Scrub completed at %s (0 errors)", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	a.pools[cleanID] = pool
	cp := pool
	return &cp, nil
}

func (a *Adapter) UpdatePool(ctx context.Context, id string, update domain.ZfsPoolUpdate) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if a.canUseHostZfs() {
		if update.Sync != "" {
			if out, err := exec.CommandContext(ctx, "zfs", "set", "sync="+update.Sync, cleanID).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("zfs set sync failed: %v (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		if update.Autotrim != "" {
			if out, err := exec.CommandContext(ctx, "zpool", "set", "autotrim="+update.Autotrim, cleanID).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("zpool set autotrim failed: %v (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		if update.Compression != "" {
			if out, err := exec.CommandContext(ctx, "zfs", "set", "compression="+update.Compression, cleanID).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("zfs set compression failed: %v (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		return a.GetPool(ctx, cleanID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	pool, ok := a.pools[cleanID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
	}
	if update.Sync != "" {
		pool.Sync = update.Sync
	}
	if update.Autotrim != "" {
		pool.Autotrim = update.Autotrim
	}
	if update.Compression != "" {
		pool.Compression = update.Compression
	}
	a.pools[cleanID] = pool
	cp := pool
	return &cp, nil
}

func (a *Adapter) AddVdev(ctx context.Context, poolID string, req domain.ZfsAddVdevRequest) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(poolID, "zfsPools/")
	if a.canUseHostZfs() {
		args := []string{"add"}
		vType := strings.ToLower(req.Type)
		switch vType {
		case "cache", "l2arc":
			args = append(args, cleanID, "cache")
			args = append(args, req.Devices...)
		case "log", "slog":
			args = append(args, cleanID, "log")
			if strings.ToLower(req.Layout) == "mirror" {
				args = append(args, "mirror")
			}
			args = append(args, req.Devices...)
		case "special", "metadata":
			args = append(args, cleanID, "special")
			if strings.ToLower(req.Layout) == "mirror" {
				args = append(args, "mirror")
			}
			args = append(args, req.Devices...)
		case "dedup":
			args = append(args, cleanID, "dedup")
			if strings.ToLower(req.Layout) == "mirror" {
				args = append(args, "mirror")
			}
			args = append(args, req.Devices...)
		case "spare":
			args = append(args, cleanID, "spare")
			args = append(args, req.Devices...)
		default: // data
			args = append(args, cleanID)
			layout := strings.ToLower(req.Layout)
			if layout != "" && layout != "stripe" {
				args = append(args, layout)
			}
			args = append(args, req.Devices...)
		}

		if out, err := exec.CommandContext(ctx, "zpool", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("zpool add failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return a.GetPool(ctx, cleanID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	pool, ok := a.pools[cleanID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
	}
	vdevNode := domain.ZfsVdev{
		Name:   fmt.Sprintf("%s-%d", req.Type, len(pool.Vdevs)+1),
		Type:   req.Type,
		Layout: req.Layout,
		State:  "ONLINE",
	}
	for _, dev := range req.Devices {
		vdevNode.Children = append(vdevNode.Children, domain.ZfsVdev{
			Name:  dev,
			Type:  req.Type,
			State: "ONLINE",
			Path:  dev,
		})
		pool.Devices = append(pool.Devices, dev)
	}
	pool.Vdevs = append(pool.Vdevs, vdevNode)
	a.pools[cleanID] = pool
	cp := pool
	return &cp, nil
}

func (a *Adapter) RemoveVdev(ctx context.Context, poolID string, device string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(poolID, "zfsPools/")
	if a.canUseHostZfs() {
		if out, err := exec.CommandContext(ctx, "zpool", "remove", cleanID, device).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("zpool remove failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return a.GetPool(ctx, cleanID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	pool, ok := a.pools[cleanID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
	}
	var newDevs []string
	for _, d := range pool.Devices {
		if d != device {
			newDevs = append(newDevs, d)
		}
	}
	pool.Devices = newDevs
	a.pools[cleanID] = pool
	cp := pool
	return &cp, nil
}

// =============================================================================
// ZFS Datasets
// =============================================================================

func (a *Adapter) getSnapshotsInfo(ctx context.Context, poolID string) (map[string]int32, map[string]string) {
	counts := make(map[string]int32)
	latest := make(map[string]string)

	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-t", "snapshot", "-r", poolID)
	out, err := cmd.Output()
	if err != nil {
		return counts, latest
	}
	var snapJSON zfsListJSON
	if err := json.Unmarshal(out, &snapJSON); err != nil {
		return counts, latest
	}
	for name := range snapJSON.Datasets {
		parts := strings.Split(name, "@")
		if len(parts) == 2 {
			dsName := parts[0]
			counts[dsName]++
			latest[dsName] = name
		}
	}
	return counts, latest
}

func (a *Adapter) ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	if a.canUseHostZfs() {
		datasets, err := a.listHostDatasets(ctx, cleanPool)
		if err == nil {
			a.mu.Lock()
			if a.datasets[cleanPool] == nil {
				a.datasets[cleanPool] = make(map[string]domain.ZfsDataset)
			}
			for _, d := range datasets {
				a.datasets[cleanPool][d.ID] = d
			}
			a.mu.Unlock()
			return datasets, nil
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, exists := a.pools[cleanPool]; !exists {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	dsMap := a.datasets[cleanPool]
	out := make([]domain.ZfsDataset, 0, len(dsMap))
	for _, ds := range dsMap {
		out = append(out, ds)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (a *Adapter) listHostDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-r", "-t", "filesystem",
		"-o", "name,type,used,available,referenced,mountpoint,compression,readonly,atime,recordsize,quota,sync,reservation,dedup", poolID)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var zfsData zfsListJSON
	if err := json.Unmarshal(out, &zfsData); err != nil {
		return nil, err
	}

	snapCounts, snapLatest := a.getSnapshotsInfo(ctx, poolID)

	var datasets []domain.ZfsDataset
	for _, item := range zfsData.Datasets {
		name := item.Name
		if name == poolID || !strings.HasPrefix(name, poolID+"/") {
			continue
		}
		cleanID := strings.TrimPrefix(name, poolID+"/")
		cleanID = strings.ReplaceAll(cleanID, "/", "-")

		used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
		avail, _ := strconv.ParseInt(item.Properties["available"].Value, 10, 64)
		ref, _ := strconv.ParseInt(item.Properties["referenced"].Value, 10, 64)
		quota, _ := strconv.ParseInt(item.Properties["quota"].Value, 10, 64)
		resv, _ := strconv.ParseInt(item.Properties["reservation"].Value, 10, 64)

		mountpoint := item.Properties["mountpoint"].Value
		compression := item.Properties["compression"].Value
		readonly := item.Properties["readonly"].Value == "on"
		atime := item.Properties["atime"].Value == "on"
		recSize := formatRecordsize(item.Properties["recordsize"].Value)
		sync := item.Properties["sync"].Value
		dedup := item.Properties["dedup"].Value == "on"

		datasets = append(datasets, domain.ZfsDataset{
			PoolID:           poolID,
			ID:               cleanID,
			DatasetPath:      name,
			Mountpoint:       mountpoint,
			UsedBytes:        used,
			AvailableBytes:   avail,
			ReferencedBytes:  ref,
			QuotaBytes:       quota,
			Compression:      compression,
			Readonly:         readonly,
			Atime:            atime,
			Recordsize:       recSize,
			SnapshotsCount:   snapCounts[name],
			LastSnapshot:     snapLatest[name],
			Sync:             sync,
			ReservationBytes: resv,
			Dedup:            dedup,
		})
	}

	sort.Slice(datasets, func(i, j int) bool {
		return datasets[i].ID < datasets[j].ID
	})
	return datasets, nil
}

func (a *Adapter) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	cleanDS = strings.ReplaceAll(cleanDS, "-", "/")

	if a.canUseHostZfs() {
		targetPath := fmt.Sprintf("%s/%s", cleanPool, cleanDS)
		if cleanDS == cleanPool || cleanDS == "root" {
			targetPath = cleanPool
		}
		cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-o",
			"name,type,used,available,referenced,mountpoint,compression,readonly,atime,recordsize,quota,sync,reservation,dedup", targetPath)
		if out, err := cmd.Output(); err == nil {
			var zfsData zfsListJSON
			if json.Unmarshal(out, &zfsData) == nil {
				if item, ok := zfsData.Datasets[targetPath]; ok {
					used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
					avail, _ := strconv.ParseInt(item.Properties["available"].Value, 10, 64)
					ref, _ := strconv.ParseInt(item.Properties["referenced"].Value, 10, 64)
					quota, _ := strconv.ParseInt(item.Properties["quota"].Value, 10, 64)
					resv, _ := strconv.ParseInt(item.Properties["reservation"].Value, 10, 64)

					snapCounts, snapLatest := a.getSnapshotsInfo(ctx, cleanPool)

					return &domain.ZfsDataset{
						PoolID:           cleanPool,
						ID:               strings.ReplaceAll(cleanDS, "/", "-"),
						DatasetPath:      targetPath,
						Mountpoint:       item.Properties["mountpoint"].Value,
						UsedBytes:        used,
						AvailableBytes:   avail,
						ReferencedBytes:  ref,
						QuotaBytes:       quota,
						Compression:      item.Properties["compression"].Value,
						Readonly:         item.Properties["readonly"].Value == "on",
						Atime:            item.Properties["atime"].Value == "on",
						Recordsize:       formatRecordsize(item.Properties["recordsize"].Value),
						SnapshotsCount:   snapCounts[targetPath],
						LastSnapshot:     snapLatest[targetPath],
						Sync:             item.Properties["sync"].Value,
						ReservationBytes: resv,
						Dedup:            item.Properties["dedup"].Value == "on",
					}, nil
				}
			}
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	dsMap, ok := a.datasets[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	ds, ok := dsMap[cleanDS]
	if !ok {
		ds, ok = dsMap[strings.ReplaceAll(cleanDS, "/", "-")]
	}
	if !ok {
		return nil, fmt.Errorf("%w: zfs dataset %s/%s", domain.ErrNotFound, cleanPool, cleanDS)
	}
	cp := ds
	return &cp, nil
}

func (a *Adapter) CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(dataset.PoolID, "zfsPools/")
	dataset.PoolID = cleanPool
	if dataset.DatasetPath == "" {
		dataset.DatasetPath = fmt.Sprintf("%s/%s", cleanPool, dataset.ID)
	}
	if dataset.Mountpoint == "" {
		dataset.Mountpoint = "/" + dataset.DatasetPath
	}

	if a.canUseHostZfs() {
		args := []string{"create"}
		if dataset.Mountpoint != "" {
			args = append(args, "-o", "mountpoint="+dataset.Mountpoint)
		}
		if dataset.Compression != "" {
			args = append(args, "-o", "compression="+dataset.Compression)
		}
		if dataset.Recordsize != "" {
			args = append(args, "-o", "recordsize="+dataset.Recordsize)
		}
		if dataset.QuotaBytes > 0 {
			args = append(args, "-o", fmt.Sprintf("quota=%d", dataset.QuotaBytes))
		}
		if dataset.ReservationBytes > 0 {
			args = append(args, "-o", fmt.Sprintf("reservation=%d", dataset.ReservationBytes))
		}
		if dataset.Readonly {
			args = append(args, "-o", "readonly=on")
		}
		if dataset.Sync != "" {
			args = append(args, "-o", "sync="+dataset.Sync)
		}
		if dataset.Dedup {
			args = append(args, "-o", "dedup=on")
		}
		if dataset.Atime {
			args = append(args, "-o", "atime=on")
		} else {
			args = append(args, "-o", "atime=off")
		}
		args = append(args, dataset.DatasetPath)
		if out, err := exec.CommandContext(ctx, "zfs", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("zfs create failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return a.GetDataset(ctx, cleanPool, dataset.ID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	pool, ok := a.pools[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	if a.datasets[cleanPool] == nil {
		a.datasets[cleanPool] = make(map[string]domain.ZfsDataset)
	}
	if _, exists := a.datasets[cleanPool][dataset.ID]; exists {
		return nil, fmt.Errorf("%w: dataset %s/%s", domain.ErrAlreadyExists, cleanPool, dataset.ID)
	}
	dataset.UsedBytes = 98304
	dataset.ReferencedBytes = 98304
	dataset.AvailableBytes = pool.FreeBytes
	if dataset.QuotaBytes > 0 && dataset.QuotaBytes < dataset.AvailableBytes {
		dataset.AvailableBytes = dataset.QuotaBytes
	}
	a.datasets[cleanPool][dataset.ID] = dataset
	cp := dataset
	return &cp, nil
}

func (a *Adapter) UpdateDataset(ctx context.Context, dataset domain.ZfsDataset, fields []string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(dataset.PoolID, "zfsPools/")
	targetPath := dataset.DatasetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, dataset.ID)
	}

	applyAll := len(fields) == 0
	hasField := func(name string) bool {
		if applyAll {
			return true
		}
		for _, f := range fields {
			if f == name {
				return true
			}
		}
		return false
	}

	if a.canUseHostZfs() {
		if hasField("compression") && dataset.Compression != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "compression="+dataset.Compression, targetPath).Run()
		}
		if hasField("quota_bytes") || hasField("quotaBytes") {
			_ = exec.CommandContext(ctx, "zfs", "set", fmt.Sprintf("quota=%d", dataset.QuotaBytes), targetPath).Run()
		}
		if hasField("readonly") {
			val := "off"
			if dataset.Readonly {
				val = "on"
			}
			_ = exec.CommandContext(ctx, "zfs", "set", "readonly="+val, targetPath).Run()
		}
		if hasField("atime") {
			val := "off"
			if dataset.Atime {
				val = "on"
			}
			_ = exec.CommandContext(ctx, "zfs", "set", "atime="+val, targetPath).Run()
		}
		if hasField("recordsize") && dataset.Recordsize != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "recordsize="+dataset.Recordsize, targetPath).Run()
		}
		if hasField("mountpoint") && dataset.Mountpoint != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "mountpoint="+dataset.Mountpoint, targetPath).Run()
		}
		if hasField("sync") && dataset.Sync != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "sync="+dataset.Sync, targetPath).Run()
		}
		if hasField("reservation_bytes") || hasField("reservationBytes") {
			_ = exec.CommandContext(ctx, "zfs", "set", fmt.Sprintf("reservation=%d", dataset.ReservationBytes), targetPath).Run()
		}
		if hasField("dedup") {
			val := "off"
			if dataset.Dedup {
				val = "on"
			}
			_ = exec.CommandContext(ctx, "zfs", "set", "dedup="+val, targetPath).Run()
		}
		return a.GetDataset(ctx, cleanPool, dataset.ID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	dsMap, ok := a.datasets[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	existing, ok := dsMap[dataset.ID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs dataset %s/%s", domain.ErrNotFound, cleanPool, dataset.ID)
	}
	if hasField("compression") && dataset.Compression != "" {
		existing.Compression = dataset.Compression
	}
	if hasField("quota_bytes") || hasField("quotaBytes") {
		existing.QuotaBytes = dataset.QuotaBytes
	}
	if hasField("readonly") {
		existing.Readonly = dataset.Readonly
	}
	if hasField("atime") {
		existing.Atime = dataset.Atime
	}
	if hasField("recordsize") && dataset.Recordsize != "" {
		existing.Recordsize = dataset.Recordsize
	}
	if hasField("mountpoint") && dataset.Mountpoint != "" {
		existing.Mountpoint = dataset.Mountpoint
	}
	if hasField("sync") && dataset.Sync != "" {
		existing.Sync = dataset.Sync
	}
	if hasField("reservation_bytes") || hasField("reservationBytes") {
		existing.ReservationBytes = dataset.ReservationBytes
	}
	if hasField("dedup") {
		existing.Dedup = dataset.Dedup
	}
	dsMap[dataset.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteDataset(ctx context.Context, poolID, datasetID string, recursive bool) error {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	cleanDS = strings.ReplaceAll(cleanDS, "-", "/")
	targetPath := fmt.Sprintf("%s/%s", cleanPool, cleanDS)

	if cleanDS == cleanPool || cleanDS == "" || cleanDS == "root" {
		return fmt.Errorf("cannot delete root dataset of pool %q; delete the pool instead", cleanPool)
	}

	if a.canUseHostZfs() {
		args := []string{"destroy"}
		if recursive {
			args = append(args, "-r")
		}
		args = append(args, targetPath)
		if out, err := exec.CommandContext(ctx, "zfs", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("zfs destroy failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if dsMap, ok := a.datasets[cleanPool]; ok {
		delete(dsMap, cleanDS)
		delete(dsMap, strings.ReplaceAll(cleanDS, "/", "-"))
	}
	return nil
}

func (a *Adapter) SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	cleanDS = strings.ReplaceAll(cleanDS, "-", "/")
	targetPath := fmt.Sprintf("%s/%s", cleanPool, cleanDS)

	if a.canUseHostZfs() {
		snapTarget := fmt.Sprintf("%s@%s", targetPath, snapName)
		if out, err := exec.CommandContext(ctx, "zfs", "snapshot", snapTarget).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("zfs snapshot failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return a.GetDataset(ctx, cleanPool, datasetID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	dsMap, ok := a.datasets[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	ds, ok := dsMap[cleanDS]
	if !ok {
		ds, ok = dsMap[strings.ReplaceAll(cleanDS, "/", "-")]
	}
	if !ok {
		return nil, fmt.Errorf("%w: zfs dataset %s/%s", domain.ErrNotFound, cleanPool, cleanDS)
	}
	ds.SnapshotsCount++
	ds.LastSnapshot = fmt.Sprintf("%s@%s", targetPath, snapName)
	dsMap[ds.ID] = ds
	cp := ds
	return &cp, nil
}

// =============================================================================
// ZFS Volumes (ZVols)
// =============================================================================

func (a *Adapter) ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	if a.canUseHostZfs() {
		volumes, err := a.listHostVolumes(ctx, cleanPool)
		if err == nil {
			a.mu.Lock()
			if a.volumes[cleanPool] == nil {
				a.volumes[cleanPool] = make(map[string]domain.ZfsVolume)
			}
			for _, v := range volumes {
				a.volumes[cleanPool][v.ID] = v
			}
			a.mu.Unlock()
			return volumes, nil
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, exists := a.pools[cleanPool]; !exists {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	volMap := a.volumes[cleanPool]
	out := make([]domain.ZfsVolume, 0, len(volMap))
	for _, v := range volMap {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (a *Adapter) listHostVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-r", "-t", "volume",
		"-o", "name,type,used,available,volsize,volblocksize,compression,refreservation,sync,readonly,dedup", poolID)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var zfsData zfsListJSON
	if err := json.Unmarshal(out, &zfsData); err != nil {
		return nil, err
	}

	var volumes []domain.ZfsVolume
	for _, item := range zfsData.Datasets {
		name := item.Name
		if !strings.HasPrefix(name, poolID+"/") {
			continue
		}
		cleanID := strings.TrimPrefix(name, poolID+"/")
		cleanID = strings.ReplaceAll(cleanID, "/", "-")

		volSize, _ := strconv.ParseInt(item.Properties["volsize"].Value, 10, 64)
		used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
		refres, _ := strconv.ParseInt(item.Properties["refreservation"].Value, 10, 64)
		thin := refres == 0 || refres < volSize

		volBlock := formatRecordsize(item.Properties["volblocksize"].Value)
		comp := item.Properties["compression"].Value
		sync := item.Properties["sync"].Value
		readonly := item.Properties["readonly"].Value == "on"
		dedup := item.Properties["dedup"].Value == "on"

		volumes = append(volumes, domain.ZfsVolume{
			PoolID:          poolID,
			ID:              cleanID,
			VolumePath:      name,
			DevicePath:      fmt.Sprintf("/dev/zvol/%s", name),
			SizeBytes:       volSize,
			UsedBytes:       used,
			Volblocksize:    volBlock,
			Compression:     comp,
			ThinProvisioned: thin,
			Sync:            sync,
			Readonly:        readonly,
			Dedup:           dedup,
		})
	}

	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].ID < volumes[j].ID
	})
	return volumes, nil
}

func (a *Adapter) GetVolume(ctx context.Context, poolID, volumeID string) (*domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanVol := strings.TrimPrefix(volumeID, "volumes/")
	cleanVol = strings.ReplaceAll(cleanVol, "-", "/")
	targetPath := fmt.Sprintf("%s/%s", cleanPool, cleanVol)

	if a.canUseHostZfs() {
		cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-o",
			"name,type,used,available,volsize,volblocksize,compression,refreservation,sync,readonly,dedup", targetPath)
		if out, err := cmd.Output(); err == nil {
			var zfsData zfsListJSON
			if json.Unmarshal(out, &zfsData) == nil {
				if item, ok := zfsData.Datasets[targetPath]; ok {
					volSize, _ := strconv.ParseInt(item.Properties["volsize"].Value, 10, 64)
					used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
					refres, _ := strconv.ParseInt(item.Properties["refreservation"].Value, 10, 64)
					thin := refres == 0 || refres < volSize

					return &domain.ZfsVolume{
						PoolID:          cleanPool,
						ID:              strings.ReplaceAll(cleanVol, "/", "-"),
						VolumePath:      targetPath,
						DevicePath:      fmt.Sprintf("/dev/zvol/%s", targetPath),
						SizeBytes:       volSize,
						UsedBytes:       used,
						Volblocksize:    formatRecordsize(item.Properties["volblocksize"].Value),
						Compression:     item.Properties["compression"].Value,
						ThinProvisioned: thin,
						Sync:            item.Properties["sync"].Value,
						Readonly:        item.Properties["readonly"].Value == "on",
						Dedup:           item.Properties["dedup"].Value == "on",
					}, nil
				}
			}
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	volMap, ok := a.volumes[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	vol, ok := volMap[cleanVol]
	if !ok {
		vol, ok = volMap[strings.ReplaceAll(cleanVol, "/", "-")]
	}
	if !ok {
		return nil, fmt.Errorf("%w: zfs volume %s/%s", domain.ErrNotFound, cleanPool, cleanVol)
	}
	cp := vol
	return &cp, nil
}

func (a *Adapter) CreateVolume(ctx context.Context, volume domain.ZfsVolume) (*domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(volume.PoolID, "zfsPools/")
	volume.PoolID = cleanPool
	if volume.VolumePath == "" {
		volume.VolumePath = fmt.Sprintf("%s/%s", cleanPool, volume.ID)
	}
	if volume.DevicePath == "" {
		volume.DevicePath = fmt.Sprintf("/dev/zvol/%s", volume.VolumePath)
	}

	if a.canUseHostZfs() {
		args := []string{"create"}
		if volume.ThinProvisioned {
			args = append(args, "-s")
		}
		args = append(args, "-V", fmt.Sprintf("%d", volume.SizeBytes))
		if volume.Volblocksize != "" {
			args = append(args, "-b", volume.Volblocksize)
		}
		if volume.Compression != "" {
			args = append(args, "-o", "compression="+volume.Compression)
		}
		if volume.Sync != "" {
			args = append(args, "-o", "sync="+volume.Sync)
		}
		if volume.Readonly {
			args = append(args, "-o", "readonly=on")
		}
		if volume.Dedup {
			args = append(args, "-o", "dedup=on")
		}
		args = append(args, volume.VolumePath)
		if out, err := exec.CommandContext(ctx, "zfs", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("zfs create -V failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return a.GetVolume(ctx, cleanPool, volume.ID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.pools[cleanPool]; !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	if a.volumes[cleanPool] == nil {
		a.volumes[cleanPool] = make(map[string]domain.ZfsVolume)
	}
	if _, exists := a.volumes[cleanPool][volume.ID]; exists {
		return nil, fmt.Errorf("%w: zfs volume %s/%s", domain.ErrAlreadyExists, cleanPool, volume.ID)
	}
	if volume.ThinProvisioned {
		volume.UsedBytes = 65536
	} else {
		volume.UsedBytes = volume.SizeBytes / 10
	}
	a.volumes[cleanPool][volume.ID] = volume
	cp := volume
	return &cp, nil
}

func (a *Adapter) UpdateVolume(ctx context.Context, volume domain.ZfsVolume, fields []string) (*domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(volume.PoolID, "zfsPools/")
	targetPath := volume.VolumePath
	if targetPath == "" {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, volume.ID)
	}

	applyAll := len(fields) == 0
	hasField := func(name string) bool {
		if applyAll {
			return true
		}
		for _, f := range fields {
			if f == name {
				return true
			}
		}
		return false
	}

	if a.canUseHostZfs() {
		if hasField("size_bytes") || hasField("sizeBytes") || (applyAll && volume.SizeBytes > 0) {
			if volume.SizeBytes > 0 {
				_ = exec.CommandContext(ctx, "zfs", "set", fmt.Sprintf("volsize=%d", volume.SizeBytes), targetPath).Run()
			}
		}
		if hasField("compression") && volume.Compression != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "compression="+volume.Compression, targetPath).Run()
		}
		if hasField("sync") && volume.Sync != "" {
			_ = exec.CommandContext(ctx, "zfs", "set", "sync="+volume.Sync, targetPath).Run()
		}
		if hasField("thin_provisioned") || hasField("thinProvisioned") {
			if volume.ThinProvisioned {
				_ = exec.CommandContext(ctx, "zfs", "set", "refreservation=none", targetPath).Run()
			} else {
				volSize := volume.SizeBytes
				if volSize <= 0 {
					if existing, err := a.GetVolume(ctx, cleanPool, volume.ID); err == nil && existing != nil {
						volSize = existing.SizeBytes
					}
				}
				if volSize > 0 {
					_ = exec.CommandContext(ctx, "zfs", "set", fmt.Sprintf("refreservation=%d", volSize), targetPath).Run()
				}
			}
		}
		if hasField("readonly") {
			val := "off"
			if volume.Readonly {
				val = "on"
			}
			_ = exec.CommandContext(ctx, "zfs", "set", "readonly="+val, targetPath).Run()
		}
		if hasField("dedup") {
			val := "off"
			if volume.Dedup {
				val = "on"
			}
			_ = exec.CommandContext(ctx, "zfs", "set", "dedup="+val, targetPath).Run()
		}
		return a.GetVolume(ctx, cleanPool, volume.ID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	volMap, ok := a.volumes[cleanPool]
	if !ok {
		return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanPool)
	}
	existing, ok := volMap[volume.ID]
	if !ok {
		return nil, fmt.Errorf("%w: zfs volume %s/%s", domain.ErrNotFound, cleanPool, volume.ID)
	}
	if hasField("size_bytes") || hasField("sizeBytes") || volume.SizeBytes > 0 {
		if volume.SizeBytes > 0 {
			existing.SizeBytes = volume.SizeBytes
		}
	}
	if hasField("compression") && volume.Compression != "" {
		existing.Compression = volume.Compression
	}
	if hasField("sync") && volume.Sync != "" {
		existing.Sync = volume.Sync
	}
	if hasField("thin_provisioned") || hasField("thinProvisioned") {
		existing.ThinProvisioned = volume.ThinProvisioned
	}
	if hasField("readonly") {
		existing.Readonly = volume.Readonly
	}
	if hasField("dedup") {
		existing.Dedup = volume.Dedup
	}
	volMap[volume.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteVolume(ctx context.Context, poolID, volumeID string) error {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanVol := strings.TrimPrefix(volumeID, "volumes/")
	cleanVol = strings.ReplaceAll(cleanVol, "-", "/")
	targetPath := fmt.Sprintf("%s/%s", cleanPool, cleanVol)

	if a.canUseHostZfs() {
		if out, err := exec.CommandContext(ctx, "zfs", "destroy", targetPath).CombinedOutput(); err != nil {
			return fmt.Errorf("zfs destroy volume failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if volMap, ok := a.volumes[cleanPool]; ok {
		delete(volMap, cleanVol)
		delete(volMap, strings.ReplaceAll(cleanVol, "/", "-"))
	}
	return nil
}
