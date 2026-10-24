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

type SmartParameter struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Worst       string `json:"worst"`
	Threshold   string `json:"threshold"`
	RawValue    string `json:"raw_value"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Disk struct {
	Name               string           `json:"name"`        // "disks/{disk}"
	DiskID             string           `json:"disk_id"`     // "nvme0n1", "sda"
	DevicePath         string           `json:"device_path"` // "/dev/nvme0n1"
	SizeBytes          int64            `json:"size_bytes"`
	Model              string           `json:"model"`
	Serial             string           `json:"serial"`
	Vendor             string           `json:"vendor"`
	Transport          string           `json:"transport"` // "nvme", "sata", "sas", "usb", "virtio"
	Rotational         bool             `json:"rotational"`
	Type               string           `json:"type"` // "NVME", "SSD", "HDD", "VIRTUAL"
	FirmwareRev        string           `json:"firmware_rev"`
	State              string           `json:"state"` // "live", "running"
	Removable          bool             `json:"removable"`
	LogicalBlockSize   int64            `json:"logical_block_size"`
	PhysicalBlockSize  int64            `json:"physical_block_size"`
	TemperatureCelsius float64          `json:"temperature_celsius"`
	SmartStatus        string           `json:"smart_status"` // "PASSED", "FAILED", "UNKNOWN"
	HealthPercentage   int32            `json:"health_percentage"`
	OwningZfsPool      string           `json:"owning_zfs_pool"`
	Partitions         []DiskPartition  `json:"partitions"`
	SmartParameters    []SmartParameter `json:"smart_parameters"`
	PowerOnHours       int64            `json:"power_on_hours"`
	PowerCycleCount    int64            `json:"power_cycle_count"`
	UnsafeShutdowns    int64            `json:"unsafe_shutdowns"`
	MediaErrors        int64            `json:"media_errors"`
	TotalBytesRead     int64            `json:"total_bytes_read"`
	TotalBytesWritten  int64            `json:"total_bytes_written"`
	ReallocatedSectors int64            `json:"reallocated_sectors"`
	PendingSectors     int64            `json:"pending_sectors"`
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
