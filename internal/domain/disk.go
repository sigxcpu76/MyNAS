package domain

import (
	"fmt"
	"strings"
)

type DiskPartition struct {
	Name          string `json:"name"`
	DevicePath    string `json:"device_path"`
	SizeBytes     int64  `json:"size_bytes"`
	FSType        string `json:"fstype"`
	Mountpoint    string `json:"mountpoint"`
	Label         string `json:"label"`
	OwningZfsPool string `json:"owning_zfs_pool"`
}

type Disk struct {
	Name               string          `json:"name"` // "disks/{disk}"
	DiskID             string          `json:"disk_id"` // "nvme0n1", "sda"
	DevicePath         string          `json:"device_path"` // "/dev/nvme0n1"
	SizeBytes          int64           `json:"size_bytes"`
	Model              string          `json:"model"`
	Serial             string          `json:"serial"`
	Vendor             string          `json:"vendor"`
	Transport          string          `json:"transport"` // "nvme", "sata", "sas", "usb", "virtio"
	Rotational         bool            `json:"rotational"`
	Type               string          `json:"type"` // "NVME", "SSD", "HDD", "VIRTUAL"
	FirmwareRev        string          `json:"firmware_rev"`
	State              string          `json:"state"` // "live", "running"
	Removable          bool            `json:"removable"`
	LogicalBlockSize   int64           `json:"logical_block_size"`
	PhysicalBlockSize  int64           `json:"physical_block_size"`
	TemperatureCelsius float64         `json:"temperature_celsius"`
	SmartStatus        string          `json:"smart_status"` // "PASSED", "FAILED", "UNKNOWN"
	HealthPercentage   int32           `json:"health_percentage"`
	OwningZfsPool      string          `json:"owning_zfs_pool"`
	Partitions         []DiskPartition `json:"partitions"`
}

func (d *Disk) Validate() error {
	if strings.TrimSpace(d.DiskID) == "" {
		return fmt.Errorf("disk_id cannot be empty")
	}
	if !strings.HasPrefix(d.Name, "disks/") {
		d.Name = fmt.Sprintf("disks/%s", d.DiskID)
	}
	return nil
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
