package zfs_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestZfsLiveHostDiscovery(t *testing.T) {
	if _, err := exec.LookPath("zpool"); err != nil {
		t.Skip("zpool not installed, skipping host ZFS test")
	}
	if _, err := exec.LookPath("zfs"); err != nil {
		t.Skip("zfs not installed, skipping host ZFS test")
	}

	ctx := context.Background()
	adapter := zfs.NewAdapter()

	pools, err := adapter.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools failed: %v", err)
	}
	if len(pools) == 0 {
		t.Skip("no host ZFS pools present")
	}

	hasPool := false
	for _, p := range pools {
		if p.ID == "mypool" || p.ID == "f1" || p.ID == "tank" {
			hasPool = true
			if p.State != "ONLINE" {
				t.Fatalf("expected pool state ONLINE, got %s", p.State)
			}
			t.Logf("Discovered host pool %s: size=%d bytes, alloc=%d, mount=%s", p.ID, p.SizeBytes, p.AllocatedBytes, p.Mountpoint)
			break
		}
	}
	if !hasPool {
		t.Logf("Discovered %d pools on host: %v", len(pools), pools)
	}
}

func TestZfsCreateAndExportPoolMultiVdev(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping zpool create/export test requiring root privileges")
	}
	ctx := context.Background()
	adapter := zfs.NewAdapter()

	// Create pool with Data, Cache, Log, and Spare vdevs
	pool := domain.ZfsPool{
		ID:          "vault",
		PoolName:    "vault",
		Mountpoint:  "/mnt/vault",
		Compression: "lz4",
		Vdevs: []domain.ZfsVdev{
			{
				Type:   "data",
				Layout: "mirror",
				Children: []domain.ZfsVdev{
					{Path: "/dev/sdb"},
					{Path: "/dev/sdc"},
				},
			},
			{
				Type:   "cache",
				Layout: "stripe",
				Children: []domain.ZfsVdev{
					{Path: "/dev/nvme1n1"},
				},
			},
			{
				Type:   "log",
				Layout: "mirror",
				Children: []domain.ZfsVdev{
					{Path: "/dev/nvme2n1p1"},
					{Path: "/dev/nvme2n1p2"},
				},
			},
			{
				Type:   "spare",
				Layout: "stripe",
				Children: []domain.ZfsVdev{
					{Path: "/dev/sdd"},
				},
			},
		},
	}

	created, err := adapter.CreatePool(ctx, pool)
	if err != nil {
		t.Fatalf("CreatePool failed: %v", err)
	}
	if created.ID != "vault" {
		t.Fatalf("expected vault ID, got %s", created.ID)
	}
	if len(created.Devices) < 5 {
		t.Fatalf("expected at least 5 aggregate devices, got %d: %v", len(created.Devices), created.Devices)
	}

	p, err := adapter.GetPool(ctx, "vault")
	if err != nil {
		t.Fatalf("GetPool vault failed: %v", err)
	}
	if p.ID != "vault" {
		t.Fatalf("expected vault, got %s", p.ID)
	}

	// Export pool
	if err := adapter.ExportPool(ctx, "vault", false); err != nil {
		t.Fatalf("ExportPool failed: %v", err)
	}

	// Should not exist after export
	if _, err := adapter.GetPool(ctx, "vault"); err == nil {
		t.Fatalf("expected GetPool vault to fail after export, but succeeded")
	}
}

