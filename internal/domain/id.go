package domain

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var (
	lastNanosecondID  int64
	nonSlugCharRegexp = regexp.MustCompile(`[^a-z0-9_-]+`)
	multiHyphenRegexp = regexp.MustCompile(`-+`)
)

// ToZfsSafeSlug computes a ZFS-safe slug identifier from an arbitrary entity name.
// ZFS dataset components can only contain alphanumeric characters, hyphens, underscores, colons, and periods,
// and must NOT begin with a period or hyphen.
// The slug is converted to lowercase, non-alphanumeric characters replaced by hyphens,
// consecutive hyphens collapsed, leading/trailing punctuation trimmed, and length truncated to 63 chars.
func ToZfsSafeSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	s = nonSlugCharRegexp.ReplaceAllString(s, "-")
	s = multiHyphenRegexp.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_.")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-_.")
	}
	return s
}

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
// E.g. "vm-my-vm-01"
func GenerateVMDiskName(vmID string, diskIndex int) string {
	cleanID := ToZfsSafeSlug(strings.TrimPrefix(vmID, "virtualMachines/"))
	if cleanID == "" {
		cleanID = "vm"
	}
	return fmt.Sprintf("vm-%s-%02d", cleanID, diskIndex+1)
}

// GenerateZvolDiskPath generates a standard ZVol block device path for a VM on a storage pool.
// All VMs are rooted to the main pool under /vms/<vm_id>-<diskIndex>.
// E.g. "/dev/zvol/mypool/vms/my-vm-01" or "/dev/zvol/mypool/vms/my-vm-02"
func GenerateZvolDiskPath(pool, vmID string, diskIndex int) string {
	cleanPool := strings.TrimPrefix(pool, "zfsPools/")
	cleanID := ToZfsSafeSlug(strings.TrimPrefix(vmID, "virtualMachines/"))
	if cleanID == "" {
		cleanID = "vm"
	}
	return fmt.Sprintf("/dev/zvol/%s/vms/%s-%02d", cleanPool, cleanID, diskIndex+1)
}

// GenerateVMPath returns the root directory for a virtual machine on the main pool.
// E.g. "/mnt/mypool/vms/my-vm"
func GenerateVMPath(mainPool, vmID string) string {
	cleanPool := strings.TrimPrefix(mainPool, "zfsPools/")
	cleanID := ToZfsSafeSlug(strings.TrimPrefix(vmID, "virtualMachines/"))
	if cleanID == "" {
		cleanID = "vm"
	}
	return fmt.Sprintf("/mnt/%s/vms/%s", cleanPool, cleanID)
}

// GenerateVMDiskFilePath returns the standard disk image path on the main pool for a VM.
// All VMs are rooted to the main pool under /vms/<vm_id>.
// E.g. "/mnt/mypool/vms/my-vm/disk-01.raw"
func GenerateVMDiskFilePath(mainPool, vmID string, diskIndex int) string {
	return fmt.Sprintf("%s/disk-%02d.raw", GenerateVMPath(mainPool, vmID), diskIndex+1)
}

// GenerateLxcRootfsPath returns the standard rootfs path for an LXC container on the main pool.
// All LXC containers are rooted to the main pool under /containers/<container_id>.
// E.g. "/mnt/mypool/containers/my-container/rootfs"
func GenerateLxcRootfsPath(mainPool, containerID string) string {
	cleanPool := strings.TrimPrefix(mainPool, "zfsPools/")
	cleanID := ToZfsSafeSlug(strings.TrimPrefix(containerID, "lxcContainers/"))
	if cleanID == "" {
		cleanID = "container"
	}
	return fmt.Sprintf("/mnt/%s/containers/%s/rootfs", cleanPool, cleanID)
}

// GenerateAppdataPath returns the standard application persistent storage path on the main pool.
// All applications are rooted to the main pool under /apps/<app_id>.
// E.g. "/mnt/mypool/apps/nextcloud"
func GenerateAppdataPath(mainPool, appID string) string {
	cleanPool := strings.TrimPrefix(mainPool, "zfsPools/")
	cleanID := ToZfsSafeSlug(strings.TrimPrefix(appID, "dockerApps/"))
	if cleanID == "" {
		cleanID = "app"
	}
	return fmt.Sprintf("/mnt/%s/apps/%s", cleanPool, cleanID)
}
