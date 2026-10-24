package disk_test

import (
	"context"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
)

func TestDiskAdapter(t *testing.T) {
	ctx := context.Background()
	zfsAdapter := zfs.NewAdapter(true)
	adapter := disk.NewAdapter(zfsAdapter, true)

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
}
