package lxc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/domain"
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
