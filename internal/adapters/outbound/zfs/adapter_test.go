package zfs_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestZfsHybridSimulation(t *testing.T) {
	ctx := context.Background()
	adapter := zfs.NewAdapter(true)

	pools, err := adapter.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools failed: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("expected 2 seeded pools, got %d", len(pools))
	}

	p, err := adapter.GetPool(ctx, "tank")
	if err != nil {
		t.Fatalf("GetPool failed: %v", err)
	}
	if p.ID != "tank" {
		t.Fatalf("expected tank pool, got %s", p.ID)
	}

	datasets, err := adapter.ListDatasets(ctx, "tank")
	if err != nil {
		t.Fatalf("ListDatasets failed: %v", err)
	}
	if len(datasets) < 2 {
		t.Fatalf("expected >= 2 datasets in tank, got %d", len(datasets))
	}
	hasTankRoot := false
	for _, d := range datasets {
		if d.DatasetPath == "tank" {
			hasTankRoot = true
			break
		}
	}
	if !hasTankRoot {
		t.Fatalf("expected root dataset 'tank' to be present in ListDatasets, got: %v", datasets)
	}

	volumes, err := adapter.ListVolumes(ctx, "tank")
	if err != nil {
		t.Fatalf("ListVolumes failed: %v", err)
	}
	if len(volumes) < 1 {
		t.Fatalf("expected >= 1 volumes in tank, got %d", len(volumes))
	}
}

func TestZfsLiveHostDiscovery(t *testing.T) {
	if _, err := exec.LookPath("zpool"); err != nil {
		t.Skip("zpool not installed, skipping host ZFS test")
	}
	if _, err := exec.LookPath("zfs"); err != nil {
		t.Skip("zfs not installed, skipping host ZFS test")
	}

	ctx := context.Background()
	adapter := zfs.NewAdapter(false)

	pools, err := adapter.ListPools(ctx)
	if err != nil {
		t.Fatalf("ListPools failed: %v", err)
	}
	if len(pools) == 0 {
		t.Skip("no host ZFS pools present")
	}

	hasF1 := false
	for _, p := range pools {
		if p.ID == "f1" {
			hasF1 = true
			if p.State != "ONLINE" {
				t.Fatalf("expected f1 pool state ONLINE, got %s", p.State)
			}
			if p.Mountpoint != "/f1" {
				t.Fatalf("expected mountpoint /f1, got %s", p.Mountpoint)
			}
		}
	}
	if !hasF1 {
		t.Logf("f1 not found among pools: %v", pools)
		return
	}

	datasets, err := adapter.ListDatasets(ctx, "f1")
	if err != nil {
		t.Fatalf("ListDatasets f1 failed: %v", err)
	}
	dsMap := make(map[string]domain.ZfsDataset)
	for _, d := range datasets {
		dsMap[d.ID] = d
	}
	if _, ok := dsMap["ds1"]; !ok {
		t.Fatalf("expected ds1 dataset in f1, got: %v", datasets)
	}
	if _, ok := dsMap["ds2"]; !ok {
		t.Fatalf("expected ds2 dataset in f1, got: %v", datasets)
	}
	if _, ok := dsMap["root"]; !ok {
		t.Fatalf("expected root dataset in f1, got: %v", datasets)
	}
	if dsMap["root"].DatasetPath != "f1" {
		t.Fatalf("expected root dataset path 'f1', got %s", dsMap["root"].DatasetPath)
	}
	if dsMap["ds1"].Mountpoint != "/f1/ds1" {
		t.Fatalf("expected /f1/ds1 mountpoint, got %s", dsMap["ds1"].Mountpoint)
	}

	volumes, err := adapter.ListVolumes(ctx, "f1")
	if err != nil {
		t.Fatalf("ListVolumes f1 failed: %v", err)
	}
	volMap := make(map[string]domain.ZfsVolume)
	for _, v := range volumes {
		volMap[v.ID] = v
	}
	if _, ok := volMap["ds3"]; !ok {
		t.Fatalf("expected ds3 volume in f1, got: %v", volumes)
	}
	if volMap["ds3"].DevicePath != "/dev/zvol/f1/ds3" {
		t.Fatalf("expected /dev/zvol/f1/ds3 device path, got %s", volMap["ds3"].DevicePath)
	}
}
