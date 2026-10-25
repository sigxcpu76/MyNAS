package domain

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var lastNanosecondID int64

// GenerateNanosecondID generates a strictly increasing unique numeric ID based on the creation timestamp at nanosecond level.
// E.g. "1761411800123456789"
func GenerateNanosecondID() string {
	for {
		now := time.Now().UnixNano()
		last := atomic.LoadInt64(&lastNanosecondID)
		if now <= last {
			now = last + 1
		}
		if atomic.CompareAndSwapInt64(&lastNanosecondID, last, now) {
			return fmt.Sprintf("%d", now)
		}
	}
}

// GenerateVMDiskName generates a standardized disk name based on the VM ID and disk index.
// E.g. "vm-1761411800123456789-disk-0"
func GenerateVMDiskName(vmID string, diskIndex int) string {
	cleanID := strings.TrimPrefix(vmID, "virtualMachines/")
	return fmt.Sprintf("vm-%s-disk-%d", cleanID, diskIndex)
}

// GenerateZvolDiskPath generates a standard ZVol block device path for a VM disk on a storage pool.
// E.g. "/dev/zvol/mypool/vm-1761411800123456789-disk-0"
func GenerateZvolDiskPath(pool, vmID string, diskIndex int) string {
	cleanPool := strings.TrimPrefix(pool, "zfsPools/")
	return fmt.Sprintf("/dev/zvol/%s/%s", cleanPool, GenerateVMDiskName(vmID, diskIndex))
}

// GenerateLxcRootfsPath returns the standard rootfs path for an LXC container on the main pool.
// E.g. "/nas/mypool/lxc/1761411800123456789/rootfs"
func GenerateLxcRootfsPath(mainPool, containerID string) string {
	cleanPool := strings.TrimPrefix(mainPool, "zfsPools/")
	cleanID := strings.TrimPrefix(containerID, "lxcContainers/")
	return fmt.Sprintf("/nas/%s/lxc/%s/rootfs", cleanPool, cleanID)
}

// GenerateAppdataPath returns the standard application persistent storage path on the main pool.
// E.g. "/nas/mypool/apps/appdata/1761411800123456789"
func GenerateAppdataPath(mainPool, appID string) string {
	cleanPool := strings.TrimPrefix(mainPool, "zfsPools/")
	cleanID := strings.TrimPrefix(appID, "dockerApps/")
	return fmt.Sprintf("/nas/%s/apps/appdata/%s", cleanPool, cleanID)
}
