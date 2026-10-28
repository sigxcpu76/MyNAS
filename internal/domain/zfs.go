package domain

import (
	"fmt"
	"path/filepath"
	"strings"
)

type ZfsPool struct {
	ID                   string
	PoolName             string
	State                string // ONLINE, DEGRADED, FAULTED
	SizeBytes            int64
	AllocatedBytes       int64
	FreeBytes            int64
	FragmentationPercent int32
	CapacityPercent      int32
	DedupRatio           float64
	VdevLayout           string // stripe, mirror, raidz1, raidz2
	Devices              []string
	Mountpoint           string
	Compression          string
	LastScrubStatus      string
	HealthDetails        string
	Sync                 string // standard, always, disabled
	Autotrim             string // on, off
	Vdevs                []ZfsVdev
}

type ZfsVdev struct {
	Name           string    `json:"name"`
	Type           string    `json:"type"`   // data, cache, log, special, dedup, spare
	Layout         string    `json:"layout"` // stripe, mirror, raidz1, raidz2, file, disk
	State          string    `json:"state"`
	Path           string    `json:"path"`
	ReadErrors     string    `json:"readErrors"`
	WriteErrors    string    `json:"writeErrors"`
	ChecksumErrors string    `json:"checksumErrors"`
	Children       []ZfsVdev `json:"children,omitempty"`
}

type ZfsPoolUpdate struct {
	Sync        string `json:"sync,omitempty"`
	Autotrim    string `json:"autotrim,omitempty"`
	Compression string `json:"compression,omitempty"`
}

type ZfsAddVdevRequest struct {
	Type    string   `json:"type"`    // data, cache, log, special, dedup, spare
	Layout  string   `json:"layout"`  // stripe, mirror, raidz1, raidz2
	Devices []string `json:"devices"` // device paths
}

func (p *ZfsPool) Validate() error {
	if err := ValidateResourceID(p.ID, "zfs_pool_id"); err != nil {
		return err
	}
	if len(p.Devices) == 0 && len(p.Vdevs) == 0 {
		return fmt.Errorf("%w: at least one backing device or vdev is required to create ZFS pool %q", ErrInvalidArgument, p.ID)
	}
	switch p.VdevLayout {
	case "", "stripe", "mirror", "raidz1", "raidz2", "raidz3":
	default:
		return fmt.Errorf("%w: unsupported vdev_layout %q", ErrInvalidArgument, p.VdevLayout)
	}
	return nil
}

type ZfsDataset struct {
	PoolID          string
	ID              string
	DatasetPath     string
	Mountpoint      string
	UsedBytes       int64
	AvailableBytes  int64
	ReferencedBytes int64
	QuotaBytes      int64
	Compression     string // lz4, zstd, off, gzip
	Readonly        bool
	Atime           bool
	Recordsize      string
	SnapshotsCount  int32
	LastSnapshot    string
	Sync            string
	ReservationBytes int64
	Dedup           bool
}

func (d *ZfsDataset) Validate() error {
	if err := ValidateResourceID(d.PoolID, "pool_id"); err != nil {
		return err
	}
	if err := ValidateResourceID(d.ID, "zfs_dataset_id"); err != nil {
		return err
	}
	if d.QuotaBytes < 0 {
		return fmt.Errorf("%w: quota_bytes cannot be negative", ErrInvalidArgument)
	}
	return nil
}

type ZfsVolume struct {
	PoolID          string
	ID              string
	VolumePath      string
	DevicePath      string
	SizeBytes       int64
	UsedBytes       int64
	Volblocksize    string
	Compression     string
	ThinProvisioned bool
	Sync            string
	Readonly        bool
	Dedup           bool
}

func (v *ZfsVolume) Validate() error {
	if err := ValidateResourceID(v.PoolID, "pool_id"); err != nil {
		return err
	}
	if err := ValidateResourceID(v.ID, "zfs_volume_id"); err != nil {
		return err
	}
	if v.SizeBytes <= 0 {
		return fmt.Errorf("%w: size_bytes must be greater than 0", ErrInvalidArgument)
	}
	if v.Volblocksize == "" {
		v.Volblocksize = "16K"
	}
	if v.Compression == "" {
		v.Compression = "lz4"
	}
	if !strings.HasPrefix(v.DevicePath, "/dev/zvol/") && v.DevicePath == "" {
		v.DevicePath = fmt.Sprintf("/dev/zvol/%s/%s", v.PoolID, v.ID)
	}
	return nil
}

// IsZvolDevice reports whether a given device path or name is a ZFS block volume (zvol, e.g. zd0, zd64p2, /dev/zd*).
// Zvols must never be targeted by zpool commands (create, add, import).
func IsZvolDevice(dev string) bool {
	trimmed := strings.TrimSpace(dev)
	if trimmed == "" {
		return false
	}
	clean := filepath.Base(trimmed)
	// zd followed by digits (e.g. zd0, zd16, zd64p2)
	if strings.HasPrefix(clean, "zd") && len(clean) > 2 && clean[2] >= '0' && clean[2] <= '9' {
		return true
	}
	if strings.Contains(trimmed, "/zvol/") || strings.Contains(trimmed, "/zd") {
		return true
	}
	return false
}
