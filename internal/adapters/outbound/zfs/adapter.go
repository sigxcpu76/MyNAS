package zfs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
)

type Adapter struct {
	mu sync.RWMutex
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) canUseHostZfs() bool {
	if _, err := exec.LookPath("zpool"); err != nil {
		return false
	}
	if _, err := exec.LookPath("zfs"); err != nil {
		return false
	}
	return true
}

type zpoolPoolJSON struct {
	Name       string                     `json:"name"`
	State      string                     `json:"state"`
	Properties map[string]zfsPropertyJSON `json:"properties"`
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
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	return a.listHostPools(ctx)
}

func (a *Adapter) listHostPools(ctx context.Context) ([]domain.ZfsPool, error) {
	cmd := exec.CommandContext(ctx, "zpool", "list", "-j", "-p")
	out, err := cmd.Output()
	if err != nil {
		// If zpool returns empty or no pools exist
		return []domain.ZfsPool{}, nil
	}
	var zpoolData zpoolListJSON
	if err := json.Unmarshal(out, &zpoolData); err != nil {
		return nil, err
	}

	// Also inspect zpool status -j for vdev topology and health details
	statusCmd := exec.CommandContext(ctx, "zpool", "status", "-j")
	statusOut, _ := statusCmd.Output()
	var statusData zpoolStatusJSON
	_ = json.Unmarshal(statusOut, &statusData)

	var pools []domain.ZfsPool
	for poolName, p := range zpoolData.Pools {
		size, _ := strconv.ParseInt(p.Properties["size"].Value, 10, 64)
		alloc, _ := strconv.ParseInt(p.Properties["allocated"].Value, 10, 64)
		free, _ := strconv.ParseInt(p.Properties["free"].Value, 10, 64)
		frag, _ := strconv.ParseInt(p.Properties["fragmentation"].Value, 10, 64)
		capPct, _ := strconv.ParseInt(p.Properties["capacity"].Value, 10, 64)
		dedupStr := p.Properties["dedupratio"].Value
		dedup, _ := strconv.ParseFloat(strings.TrimSuffix(dedupStr, "x"), 64)
		if dedup == 0 {
			dedup = 1.00
		}

		mountpoint := "/nas/" + poolName
		compression := "lz4"
		syncProp := "standard"
		autotrim := "off"

		// Fetch pool dataset properties (mountpoint, compression)
		zfsGetCmd := exec.CommandContext(ctx, "zfs", "get", "-Hp", "-o", "property,value", "mountpoint,compression,sync", poolName)
		if zfsGetOut, err := zfsGetCmd.Output(); err == nil {
			for _, line := range strings.Split(string(zfsGetOut), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					switch fields[0] {
					case "mountpoint":
						if fields[1] != "-" && fields[1] != "none" && fields[1] != "legacy" {
							mountpoint = fields[1]
						}
					case "compression":
						compression = fields[1]
					case "sync":
						syncProp = fields[1]
					}
				}
			}
		}

		zpoolGetCmd := exec.CommandContext(ctx, "zpool", "get", "-Hp", "-o", "property,value", "autotrim", poolName)
		if zpoolGetOut, err := zpoolGetCmd.Output(); err == nil {
			for _, line := range strings.Split(string(zpoolGetOut), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "autotrim" {
					autotrim = fields[1]
				}
			}
		}

		var vdevs []domain.ZfsVdev
		var devices []string
		layout := "stripe"
		health := "All vdevs ONLINE; 0 pool errors"
		scrubStatus := "No scrub errors recorded"

		if stPool, ok := statusData.Pools[poolName]; ok {
			vdevs = buildVdevTree(stPool.Vdevs)
			layout, devices = extractVdevInfo(stPool.Vdevs)
			if stPool.ErrorCount != "" && stPool.ErrorCount != "0" {
				health = fmt.Sprintf("Pool reported %s errors", stPool.ErrorCount)
			}
		}

		pools = append(pools, domain.ZfsPool{
			ID:                   poolName,
			PoolName:             poolName,
			State:                p.State,
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
			LastScrubStatus:      scrubStatus,
			HealthDetails:        health,
			Autotrim:             autotrim,
			Sync:                 syncProp,
			Vdevs:                vdevs,
		})
	}

	sort.Slice(pools, func(i, j int) bool {
		return pools[i].ID < pools[j].ID
	})
	return pools, nil
}

func (a *Adapter) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	pools, err := a.listHostPools(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pools {
		if p.ID == cleanID {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("%w: zfs pool %q", domain.ErrNotFound, cleanID)
}

func (a *Adapter) CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	if pool.Mountpoint == "" {
		pool.Mountpoint = fmt.Sprintf("/nas/%s", pool.ID)
	}

	_ = os.MkdirAll("/nas", 0755)
	args := []string{"create", "-m", pool.Mountpoint}
	if pool.Compression != "" && pool.Compression != "off" {
		args = append(args, "-O", fmt.Sprintf("compression=%s", pool.Compression))
	}
	args = append(args, pool.ID)

	if len(pool.Vdevs) > 0 {
		for _, v := range pool.Vdevs {
			vType := strings.ToLower(v.Type)
			vLayout := strings.ToLower(v.Layout)
			var devPaths []string
			for _, c := range v.Children {
				p := c.Path
				if p == "" {
					p = c.Name
				}
				if p != "" {
					devPaths = append(devPaths, p)
				}
			}
			if len(devPaths) == 0 && v.Path != "" {
				devPaths = append(devPaths, v.Path)
			}
			if len(devPaths) == 0 {
				continue
			}

			switch vType {
			case "cache", "l2arc":
				args = append(args, "cache")
				args = append(args, devPaths...)
			case "log", "slog":
				args = append(args, "log")
				if vLayout == "mirror" {
					args = append(args, "mirror")
				}
				args = append(args, devPaths...)
			case "spare", "spares":
				args = append(args, "spare")
				args = append(args, devPaths...)
			case "special", "metadata":
				args = append(args, "special")
				if vLayout == "mirror" {
					args = append(args, "mirror")
				}
				args = append(args, devPaths...)
			case "dedup":
				args = append(args, "dedup")
				if vLayout == "mirror" {
					args = append(args, "mirror")
				}
				args = append(args, devPaths...)
			default: // data
				if vLayout != "" && vLayout != "stripe" {
					args = append(args, vLayout)
				}
				args = append(args, devPaths...)
			}
		}
	} else {
		if pool.VdevLayout != "" && pool.VdevLayout != "stripe" {
			args = append(args, pool.VdevLayout)
		}
		args = append(args, pool.Devices...)
	}

	out, err := exec.CommandContext(ctx, "zpool", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("zpool create failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	return a.GetPool(ctx, pool.ID)
}

func (a *Adapter) ExportPool(ctx context.Context, id string, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if !a.canUseHostZfs() {
		return fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	args := []string{"export"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, cleanID)
	out, err := exec.CommandContext(ctx, "zpool", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("zpool export failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) DeletePool(ctx context.Context, id string, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if !a.canUseHostZfs() {
		return fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	args := []string{"destroy"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, cleanID)
	out, err := exec.CommandContext(ctx, "zpool", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("zpool destroy failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	if out, err := exec.CommandContext(ctx, "zpool", "scrub", cleanID).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("zpool scrub failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return a.GetPool(ctx, cleanID)
}

func (a *Adapter) UpdatePool(ctx context.Context, id string, update domain.ZfsPoolUpdate) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(id, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
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

func (a *Adapter) AddVdev(ctx context.Context, poolID string, req domain.ZfsAddVdevRequest) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(poolID, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
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

func (a *Adapter) RemoveVdev(ctx context.Context, poolID string, device string) (*domain.ZfsPool, error) {
	cleanID := strings.TrimPrefix(poolID, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	if out, err := exec.CommandContext(ctx, "zpool", "remove", cleanID, device).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("zpool remove failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return a.GetPool(ctx, cleanID)
}

// =============================================================================
// ZFS Datasets
// =============================================================================

func (a *Adapter) getSnapshotsInfo(ctx context.Context, poolID string) (map[string]int32, map[string]string) {
	snapCounts := make(map[string]int32)
	snapLatest := make(map[string]string)
	cmd := exec.CommandContext(ctx, "zfs", "list", "-Hp", "-t", "snapshot", "-o", "name,creation", "-s", "creation", "-r", poolID)
	out, err := cmd.Output()
	if err != nil {
		return snapCounts, snapLatest
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		snapFullName := fields[0]
		parts := strings.Split(snapFullName, "@")
		if len(parts) == 2 {
			parentDS := parts[0]
			snapCounts[parentDS]++
			snapLatest[parentDS] = snapFullName
		}
	}
	return snapCounts, snapLatest
}

func (a *Adapter) ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	return a.listHostDatasets(ctx, cleanPool)
}

func (a *Adapter) listHostDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-r", "-t", "filesystem",
		"-o", "name,type,used,available,referenced,mountpoint,compression,readonly,atime,recordsize,sync,quota,reservation,dedup", poolID)
	out, err := cmd.Output()
	if err != nil {
		return []domain.ZfsDataset{}, nil
	}
	var zfsData zfsListJSON
	if err := json.Unmarshal(out, &zfsData); err != nil {
		return nil, err
	}

	snapCounts, snapLatest := a.getSnapshotsInfo(ctx, poolID)

	var datasets []domain.ZfsDataset
	for _, item := range zfsData.Datasets {
		name := item.Name
		if name != poolID && !strings.HasPrefix(name, poolID+"/") {
			continue
		}
		cleanID := strings.TrimPrefix(name, poolID+"/")
		if name == poolID {
			cleanID = "root"
		} else {
			cleanID = strings.ReplaceAll(cleanID, "/", "-")
		}

		used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
		avail, _ := strconv.ParseInt(item.Properties["available"].Value, 10, 64)
		ref, _ := strconv.ParseInt(item.Properties["referenced"].Value, 10, 64)
		quota, _ := strconv.ParseInt(item.Properties["quota"].Value, 10, 64)
		resv, _ := strconv.ParseInt(item.Properties["reservation"].Value, 10, 64)

		mp := item.Properties["mountpoint"].Value
		if mp == "-" {
			mp = ""
		}

		comp := item.Properties["compression"].Value
		rec := formatRecordsize(item.Properties["recordsize"].Value)
		sync := item.Properties["sync"].Value
		readonly := item.Properties["readonly"].Value == "on"
		atime := item.Properties["atime"].Value == "on"
		dedup := item.Properties["dedup"].Value == "on"

		datasets = append(datasets, domain.ZfsDataset{
			PoolID:           poolID,
			ID:               cleanID,
			DatasetPath:      name,
			Mountpoint:       mp,
			UsedBytes:        used,
			AvailableBytes:   avail,
			ReferencedBytes:  ref,
			QuotaBytes:       quota,
			ReservationBytes: resv,
			Compression:      comp,
			Readonly:         readonly,
			Atime:            atime,
			Recordsize:       rec,
			Sync:             sync,
			Dedup:            dedup,
			SnapshotsCount:   snapCounts[name],
			LastSnapshot:     snapLatest[name],
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
	cleanDS = strings.TrimPrefix(cleanDS, cleanPool+"/")

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	targetPath := datasetID
	if !strings.Contains(targetPath, "/") {
		if cleanDS == "root" || cleanDS == "" {
			targetPath = cleanPool
		} else {
			targetPath = fmt.Sprintf("%s/%s", cleanPool, cleanDS)
		}
	}

	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-o",
		"name,type,used,available,referenced,mountpoint,compression,readonly,atime,recordsize,sync,quota,reservation,dedup", targetPath)
	if out, err := cmd.Output(); err == nil {
		var zfsData zfsListJSON
		if json.Unmarshal(out, &zfsData) == nil {
			if item, ok := zfsData.Datasets[targetPath]; ok {
				snapCounts, snapLatest := a.getSnapshotsInfo(ctx, cleanPool)
				used, _ := strconv.ParseInt(item.Properties["used"].Value, 10, 64)
				avail, _ := strconv.ParseInt(item.Properties["available"].Value, 10, 64)
				ref, _ := strconv.ParseInt(item.Properties["referenced"].Value, 10, 64)
				quota, _ := strconv.ParseInt(item.Properties["quota"].Value, 10, 64)
				resv, _ := strconv.ParseInt(item.Properties["reservation"].Value, 10, 64)

				mp := item.Properties["mountpoint"].Value
				if mp == "-" {
					mp = ""
				}

				return &domain.ZfsDataset{
					PoolID:           cleanPool,
					ID:               cleanDS,
					DatasetPath:      targetPath,
					Mountpoint:       mp,
					UsedBytes:        used,
					AvailableBytes:   avail,
					ReferencedBytes:  ref,
					QuotaBytes:       quota,
					ReservationBytes: resv,
					Compression:      item.Properties["compression"].Value,
					Readonly:         item.Properties["readonly"].Value == "on",
					Atime:            item.Properties["atime"].Value == "on",
					Recordsize:       formatRecordsize(item.Properties["recordsize"].Value),
					Sync:             item.Properties["sync"].Value,
					Dedup:            item.Properties["dedup"].Value == "on",
					SnapshotsCount:   snapCounts[targetPath],
					LastSnapshot:     snapLatest[targetPath],
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("%w: zfs dataset %s/%s", domain.ErrNotFound, cleanPool, cleanDS)
}

func (a *Adapter) CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(dataset.PoolID, "zfsPools/")
	dataset.PoolID = cleanPool
	if dataset.DatasetPath == "" {
		dataset.DatasetPath = fmt.Sprintf("%s/%s", cleanPool, dataset.ID)
	}

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	args := []string{"create"}
	if dataset.Compression != "" {
		args = append(args, "-o", "compression="+dataset.Compression)
	}
	if dataset.Mountpoint != "" {
		args = append(args, "-o", "mountpoint="+dataset.Mountpoint)
	}
	if dataset.Recordsize != "" {
		args = append(args, "-o", "recordsize="+dataset.Recordsize)
	}
	if dataset.QuotaBytes > 0 {
		args = append(args, "-o", fmt.Sprintf("quota=%d", dataset.QuotaBytes))
	}
	if dataset.Sync != "" {
		args = append(args, "-o", "sync="+dataset.Sync)
	}
	if dataset.Readonly {
		args = append(args, "-o", "readonly=on")
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

func (a *Adapter) UpdateDataset(ctx context.Context, dataset domain.ZfsDataset, fields []string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(dataset.PoolID, "zfsPools/")
	targetPath := dataset.DatasetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, dataset.ID)
	}

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
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

func (a *Adapter) DeleteDataset(ctx context.Context, poolID, datasetID string, recursive bool) error {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	if !a.canUseHostZfs() {
		return fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	targetPath := datasetID
	if !strings.Contains(targetPath, "/") {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, cleanDS)
	}

	args := []string{"destroy"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, targetPath)
	if out, err := exec.CommandContext(ctx, "zfs", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("zfs destroy failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	targetPath := datasetID
	if !strings.Contains(targetPath, "/") {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, cleanDS)
	}

	snapTarget := fmt.Sprintf("%s@%s", targetPath, snapName)
	if out, err := exec.CommandContext(ctx, "zfs", "snapshot", snapTarget).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("zfs snapshot failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return a.GetDataset(ctx, cleanPool, cleanDS)
}

func (a *Adapter) RollbackDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanDS := strings.TrimPrefix(datasetID, "datasets/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	targetPath := datasetID
	if !strings.Contains(targetPath, "/") {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, cleanDS)
	}

	snapTarget := fmt.Sprintf("%s@%s", targetPath, snapName)
	if out, err := exec.CommandContext(ctx, "zfs", "rollback", "-r", snapTarget).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("zfs rollback failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return a.GetDataset(ctx, cleanPool, cleanDS)
}

// =============================================================================
// ZFS Volumes (ZVols)
// =============================================================================

func (a *Adapter) resolveVolumePath(cleanPool, cleanVol string) string {
	if strings.Contains(cleanVol, "/") {
		if strings.HasPrefix(cleanVol, cleanPool+"/") {
			return cleanVol
		}
		return fmt.Sprintf("%s/%s", cleanPool, cleanVol)
	}
	// Try slash-separated if cleanVol has hyphens
	slashCandidate := fmt.Sprintf("%s/%s", cleanPool, strings.ReplaceAll(cleanVol, "-", "/"))
	if _, err := exec.Command("zfs", "list", "-t", "volume", slashCandidate).Output(); err == nil {
		return slashCandidate
	}
	return fmt.Sprintf("%s/%s", cleanPool, cleanVol)
}

func (a *Adapter) ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}
	return a.listHostVolumes(ctx, cleanPool)
}

func (a *Adapter) listHostVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	cmd := exec.CommandContext(ctx, "zfs", "list", "-j", "-p", "-r", "-t", "volume",
		"-o", "name,type,used,available,volsize,volblocksize,compression,refreservation,sync,readonly,dedup", poolID)
	out, err := cmd.Output()
	if err != nil {
		return []domain.ZfsVolume{}, nil
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

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	targetPath := a.resolveVolumePath(cleanPool, cleanVol)
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
					ID:              cleanVol,
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

	return nil, fmt.Errorf("%w: zfs volume %s/%s", domain.ErrNotFound, cleanPool, cleanVol)
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

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

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

func (a *Adapter) UpdateVolume(ctx context.Context, volume domain.ZfsVolume, fields []string) (*domain.ZfsVolume, error) {
	cleanPool := strings.TrimPrefix(volume.PoolID, "zfsPools/")
	targetPath := volume.VolumePath
	if targetPath == "" {
		targetPath = fmt.Sprintf("%s/%s", cleanPool, volume.ID)
	}

	if !a.canUseHostZfs() {
		return nil, fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
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

func (a *Adapter) DeleteVolume(ctx context.Context, poolID, volumeID string) error {
	cleanPool := strings.TrimPrefix(poolID, "zfsPools/")
	cleanVol := strings.TrimPrefix(volumeID, "volumes/")
	targetPath := a.resolveVolumePath(cleanPool, cleanVol)

	if !a.canUseHostZfs() {
		return fmt.Errorf("zfs tools (zpool/zfs) are not available on this host")
	}

	if out, err := exec.CommandContext(ctx, "zfs", "destroy", targetPath).CombinedOutput(); err != nil {
		return fmt.Errorf("zfs destroy volume failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
