package lxc_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

func TestLxcAdapter_MainPoolEnforcement(t *testing.T) {
	ctx := context.Background()
	adapter := lxc.NewAdapter()
	adapter.SetMainPool("tank")

	// 1. Rootfs defaults to main pool if empty
	c1, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:              "ct-default",
		Distribution:    "debian",
		Release:         "bookworm",
		TemplateArchive: "/anypool/archives/debian.tar.xz",
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	expectedRootfs := "/mnt/tank/containers/ct-default/rootfs"
	if c1.RootfsPath != expectedRootfs {
		t.Fatalf("expected RootfsPath %q, got %q", expectedRootfs, c1.RootfsPath)
	}
	if c1.TemplateArchive != "/anypool/archives/debian.tar.xz" {
		t.Fatalf("expected TemplateArchive preserved, got %q", c1.TemplateArchive)
	}

	// 2. Explicit Rootfs on main pool succeeds
	c2, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           "ct-custom",
		Distribution: "alpine",
		RootfsPath:   "/tank/custom/lxc/rootfs",
	})
	if err != nil {
		t.Fatalf("expected success with rootfs on tank, got error: %v", err)
	}
	if c2.RootfsPath != "/tank/custom/lxc/rootfs" {
		t.Fatalf("unexpected rootfs path: %s", c2.RootfsPath)
	}

	// 3. Rootfs on another pool is rejected
	_, err = adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           "ct-invalid",
		Distribution: "ubuntu",
		RootfsPath:   "/otherpool/lxc/invalid/rootfs",
	})
	if err == nil {
		t.Fatalf("expected error when rootfs is not on main pool tank, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got: %v", err)
	}
}

func TestLxcAdapter_Lifecycle(t *testing.T) {
	ctx := context.Background()
	adapter := lxc.NewAdapter()

	created, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           "ct-test-1",
		Distribution: "debian",
		Release:      "bookworm",
		RootfsPath:   "/mnt/mypool/lxc/ct-test-1/rootfs",
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	list, err := adapter.ListContainers(ctx)
	if err != nil || len(list) == 0 {
		t.Fatalf("ListContainers failed or empty: %v", err)
	}

	// GetContainer
	got, err := adapter.GetContainer(ctx, created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("GetContainer failed: %v", err)
	}

	// Stop container
	stopped, err := adapter.StopContainer(ctx, created.ID)
	if err != nil || stopped.State != "STOPPED" {
		t.Fatalf("StopContainer failed: %v, state: %s", err, stopped.State)
	}

	// Freeze container
	frozen, err := adapter.FreezeContainer(ctx, created.ID)
	if err != nil || frozen.State != "FROZEN" {
		t.Fatalf("FreezeContainer failed: %v, state: %s", err, frozen.State)
	}

	// Start container
	started, err := adapter.StartContainer(ctx, created.ID)
	if err != nil || started.State != "RUNNING" {
		t.Fatalf("StartContainer failed: %v, state: %s", err, started.State)
	}

	// Delete running container without force fails
	err = adapter.DeleteContainer(ctx, created.ID, false)
	if err == nil {
		t.Fatalf("expected error deleting running container without force")
	}

	// Delete running container with force succeeds
	err = adapter.DeleteContainer(ctx, created.ID, true)
	if err != nil {
		t.Fatalf("DeleteContainer with force failed: %v", err)
	}
}

func TestLxcAdapter_ParityFeatures(t *testing.T) {
	ctx := context.Background()
	adapter := lxc.NewAdapter()

	created, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           "ct-parity-1",
		Distribution: "debian",
		Release:      "bookworm",
		RootfsPath:   "/mnt/tank/lxc/ct-parity-1/rootfs",
		CPUCores:     2,
		MemoryLimitMiB: 1024,
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	// 1. Restart
	restarted, err := adapter.RestartContainer(ctx, created.ID)
	if err != nil {
		t.Fatalf("RestartContainer failed: %v", err)
	}
	if restarted.State != "RUNNING" {
		t.Fatalf("expected state RUNNING after restart, got %s", restarted.State)
	}

	// 2. Update
	updated, err := adapter.UpdateContainer(ctx, domain.LxcContainer{
		ID:             created.ID,
		ContainerName:  "renamed-ct",
		Autostart:      true,
		Unprivileged:   true,
		CPUCores:       4,
		MemoryLimitMiB: 2048,
		GpuEnabled:     true,
		Description:    "Test LXC description",
		NetworkBridge:  "br-lan",
		MountPoints: []domain.LxcMountPoint{
			{
				HostPath:      "/tank/media",
				ContainerPath: "/media",
				ReadOnly:      true,
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("UpdateContainer failed: %v", err)
	}
	if !updated.Autostart || !updated.GpuEnabled || updated.CPUCores != 4 || updated.MemoryLimitMiB != 2048 {
		t.Fatalf("UpdateContainer values mismatch: %+v", updated)
	}
	if len(updated.MountPoints) != 1 || updated.MountPoints[0].ContainerPath != "/media" {
		t.Fatalf("Mount points mismatch: %+v", updated.MountPoints)
	}

	// 3. Container Logs
	logs, err := adapter.GetContainerLogs(ctx, created.ID, 50)
	if err != nil {
		t.Fatalf("GetContainerLogs failed: %v", err)
	}
	if logs == "" {
		t.Fatalf("expected non-empty container logs")
	}

	// 4. ExecAttach
	conn, execID, err := adapter.ExecAttach(ctx, created.ID, []string{"/bin/sh"})
	if err != nil {
		t.Fatalf("ExecAttach failed: %v", err)
	}
	if execID == "" || conn == nil {
		t.Fatalf("ExecAttach returned nil conn or empty execID")
	}
	_ = conn.Close()
}

type mockZfsEngine struct {
	ports.ZfsEnginePort
	createdDatasets []domain.ZfsDataset
	deletedDatasets []string
}

func (m *mockZfsEngine) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	for _, ds := range m.createdDatasets {
		if ds.ID == datasetID || ds.DatasetPath == poolID+"/"+datasetID {
			return &ds, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockZfsEngine) CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error) {
	m.createdDatasets = append(m.createdDatasets, dataset)
	return &dataset, nil
}
func (m *mockZfsEngine) DeleteDataset(ctx context.Context, poolID, datasetID string, recursive bool) error {
	m.deletedDatasets = append(m.deletedDatasets, datasetID)
	return nil
}

func TestLxcAdapter_DedicatedZfsDataset(t *testing.T) {
	ctx := context.Background()
	adapter := lxc.NewAdapter()
	adapter.SetMainPool("fast")

	zfsMock := &mockZfsEngine{}
	adapter.SetZfsPort(zfsMock)

	// Create container on main pool fast
	created, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           "alpine-vpn",
		Distribution: "alpine",
		Release:      "3.20",
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	// Verify that dedicated ZFS dataset was created
	foundChild := false
	for _, ds := range zfsMock.createdDatasets {
		if ds.DatasetPath == "fast/containers/alpine-vpn" {
			foundChild = true
			if ds.Mountpoint != "/mnt/fast/containers/alpine-vpn" {
				t.Errorf("expected mountpoint /mnt/fast/containers/alpine-vpn, got %s", ds.Mountpoint)
			}
			if ds.Compression != "lz4" {
				t.Errorf("expected lz4 compression, got %s", ds.Compression)
			}
		}
	}
	if !foundChild {
		t.Fatalf("expected dedicated dataset fast/containers/alpine-vpn to be created, got: %+v", zfsMock.createdDatasets)
	}

	// Delete container
	if err := adapter.DeleteContainer(ctx, created.ID, true); err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}

	// Verify that dedicated ZFS dataset was destroyed
	foundDeleted := false
	for _, d := range zfsMock.deletedDatasets {
		if d == "containers/alpine-vpn" || d == "fast/containers/alpine-vpn" {
			foundDeleted = true
			break
		}
	}
	if !foundDeleted {
		t.Fatalf("expected dataset containers/alpine-vpn to be deleted, got: %+v", zfsMock.deletedDatasets)
	}
}

func TestLxcAdapter_MetricsSampling_CgroupV2(t *testing.T) {
	ctx := context.Background()
	fakeCgroupRoot := t.TempDir()

	adapter := lxc.NewAdapter()
	adapter.SetCgroupRoot(fakeCgroupRoot)

	ctID := "ct-metrics-v2"
	cgDir := filepath.Join(fakeCgroupRoot, "lxc.payload."+ctID)
	if err := os.MkdirAll(cgDir, 0755); err != nil {
		t.Fatalf("failed to create fake cgroup dir: %v", err)
	}

	// 16 MiB in bytes
	memBytes := 16 * 1024 * 1024
	if err := os.WriteFile(filepath.Join(cgDir, "memory.current"), []byte("16777216\n"), 0644); err != nil {
		t.Fatalf("failed to write memory.current: %v", err)
	}

	cpuStat := "usage_usec 1000000\nuser_usec 400000\nsystem_usec 600000\n"
	if err := os.WriteFile(filepath.Join(cgDir, "cpu.stat"), []byte(cpuStat), 0644); err != nil {
		t.Fatalf("failed to write cpu.stat: %v", err)
	}

	// Create running container
	_, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           ctID,
		Distribution: "alpine",
		Release:      "3.20",
		RootfsPath:   filepath.Join(t.TempDir(), "rootfs"),
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	// First sample should immediately read memory accurately
	c1, err := adapter.GetContainer(ctx, ctID)
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if c1.MemoryUsedMiB != int64(memBytes/(1024*1024)) {
		t.Errorf("expected MemoryUsedMiB == 16, got %d", c1.MemoryUsedMiB)
	}

	// Simulate CPU time advancement
	time.Sleep(200 * time.Millisecond)
	// Add 100,000 usec (0.1s CPU time over ~0.2s elapsed ~= ~50% CPU)
	updatedCpuStat := "usage_usec 1100000\nuser_usec 450000\nsystem_usec 650000\n"
	if err := os.WriteFile(filepath.Join(cgDir, "cpu.stat"), []byte(updatedCpuStat), 0644); err != nil {
		t.Fatalf("failed to update cpu.stat: %v", err)
	}

	c2, err := adapter.GetContainer(ctx, ctID)
	if err != nil {
		t.Fatalf("GetContainer 2 failed: %v", err)
	}
	if c2.CPUPercent <= 0 {
		t.Errorf("expected positive CPUPercent after tick, got %f", c2.CPUPercent)
	}
	if c2.MemoryUsedMiB != 16 {
		t.Errorf("expected MemoryUsedMiB == 16, got %d", c2.MemoryUsedMiB)
	}

	// Test ListContainers also includes the metrics
	list, err := adapter.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	var found *domain.LxcContainer
	for i := range list {
		if list[i].ID == ctID {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("container %q not in ListContainers", ctID)
	}
	if found.MemoryUsedMiB != 16 {
		t.Errorf("expected found.MemoryUsedMiB == 16, got %d", found.MemoryUsedMiB)
	}
	if found.CPUPercent <= 0 {
		t.Errorf("expected found.CPUPercent > 0, got %f", found.CPUPercent)
	}

	// When container is stopped, metrics should reset to zero
	stopped, err := adapter.StopContainer(ctx, ctID)
	if err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}
	if stopped.CPUPercent != 0 || stopped.MemoryUsedMiB != 0 {
		t.Errorf("expected zero metrics for stopped container, got CPU=%f, Mem=%d", stopped.CPUPercent, stopped.MemoryUsedMiB)
	}
}

func TestLxcAdapter_MetricsSampling_CgroupV1(t *testing.T) {
	ctx := context.Background()
	fakeCgroupRoot := t.TempDir()

	adapter := lxc.NewAdapter()
	adapter.SetCgroupRoot(fakeCgroupRoot)

	ctID := "ct-metrics-v1"
	cgDir := filepath.Join(fakeCgroupRoot, "lxc", ctID)
	if err := os.MkdirAll(cgDir, 0755); err != nil {
		t.Fatalf("failed to create fake cgroup v1 dir: %v", err)
	}

	// 32 MiB in bytes
	if err := os.WriteFile(filepath.Join(cgDir, "memory.usage_in_bytes"), []byte("33554432\n"), 0644); err != nil {
		t.Fatalf("failed to write memory.usage_in_bytes: %v", err)
	}

	// 1 second in nanoseconds = 1,000,000 usec
	if err := os.WriteFile(filepath.Join(cgDir, "cpuacct.usage"), []byte("1000000000\n"), 0644); err != nil {
		t.Fatalf("failed to write cpuacct.usage: %v", err)
	}

	_, err := adapter.CreateContainer(ctx, domain.LxcContainer{
		ID:           ctID,
		Distribution: "debian",
		Release:      "bookworm",
		RootfsPath:   filepath.Join(t.TempDir(), "rootfs"),
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	c, err := adapter.GetContainer(ctx, ctID)
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if c.MemoryUsedMiB != 32 {
		t.Errorf("expected MemoryUsedMiB == 32 for cgroup v1, got %d", c.MemoryUsedMiB)
	}
}

