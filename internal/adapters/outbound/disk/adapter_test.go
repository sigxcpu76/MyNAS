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
	t.Logf("Disk %s has %d SMART parameters, powerOnHours=%d, writtenBytes=%d, owningPool=%q, aliases=%v",
		found.DiskID, len(found.SmartParameters), found.PowerOnHours, found.TotalBytesWritten, found.OwningZfsPool, found.Aliases)
	if len(found.Aliases) == 0 {
		t.Errorf("expected aliases for disk %q, got empty", found.DiskID)
	}
	if found.DiskID == "nvme0n1" && found.OwningZfsPool == "" {
		t.Logf("Note: disk nvme0n1 owningPool is empty if pools are not backed by this device")
	}
}
