package disk_test

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
)

func TestDiskAdapter(t *testing.T) {
	ctx := context.Background()
	zfsAdapter := zfs.NewAdapter()
	adapter := disk.NewAdapter(zfsAdapter)

	disks, err := adapter.ListDisks(ctx)
	if err != nil {
		t.Fatalf("ListDisks failed: %v", err)
	}

	if len(disks) == 0 {
		t.Fatalf("expected at least one disk")
	}

	first := disks[0]
	if first.DiskID == "" {
		t.Errorf("expected non-empty DiskID")
	}

	// Test GetDisk
	found, err := adapter.GetDisk(ctx, first.DiskID)
	if err != nil {
		t.Fatalf("GetDisk(%q) failed: %v", first.DiskID, err)
	}
	if found.DiskID != first.DiskID {
		t.Errorf("expected %q, got %q", first.DiskID, found.DiskID)
	}
	if len(found.SmartParameters) == 0 {
		t.Errorf("expected SMART parameters for disk %q, got 0", found.DiskID)
	}
	t.Logf("Disk %s has %d SMART parameters, powerOnHours=%d, writtenBytes=%d, owningPool=%q, isSystem=%v, aliases=%v",
		found.DiskID, len(found.SmartParameters), found.PowerOnHours, found.TotalBytesWritten, found.OwningZfsPool, found.IsSystem, found.Aliases)
	for _, p := range found.Partitions {
		t.Logf("  Partition %s: mountpoint=%q, isSystem=%v, owningPool=%q", p.Name, p.Mountpoint, p.IsSystem, p.OwningZfsPool)
	}
	if found.DiskID == "nvme0n1" {
		if !found.IsSystem {
			t.Errorf("expected nvme0n1 to be marked as system disk (hosting /)")
		}
	}
}
