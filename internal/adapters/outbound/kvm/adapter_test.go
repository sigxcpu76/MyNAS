package kvm_test

import (
	"context"
	"os"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/kvm"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestKvmProxmoxParity(t *testing.T) {
	ctx := context.Background()
	adapter := kvm.NewAdapter()

	vms, err := adapter.ListVMs(ctx)
	if err != nil {
		t.Skipf("ListVMs failed (virsh might not be configured or available): %v", err)
	}
	t.Logf("Discovered %d VMs via virsh on host", len(vms))
	for _, vm := range vms {
		t.Logf("VM: %s (%s) State: %s", vm.ID, vm.VMName, vm.State)
	}
}

func TestRealLibvirtDomainLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := kvm.NewAdapterWithStateDir(tempDir)

	if _, err := adapter.ListVMs(ctx); err != nil {
		t.Skipf("virsh/libvirt not available on this host: %v", err)
	}

	vmID := "nasctl-ci-test-vm"
	diskPath := tempDir + "/ci-test.raw"

	// Create real VM
	vm, err := adapter.CreateVM(ctx, domain.VirtualMachine{
		ID:        vmID,
		VMName:    "NASCTL Real Test VM",
		MemoryMiB: 256,
		VCPUs:     1,
		Disks: []domain.VirtualMachineDisk{
			{
				ID:        "sda",
				Bus:       "scsi",
				DiskPath:  diskPath,
				SizeBytes: 10 * 1024 * 1024,
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create real VM: %v", err)
	}
	defer func() {
		_ = adapter.DeleteVM(ctx, vmID)
		_ = os.Remove(diskPath)
	}()

	if vm.ID != vmID {
		t.Errorf("expected ID %s, got %s", vmID, vm.ID)
	}
	if vm.State != "RUNNING" {
		t.Errorf("expected RUNNING state, got %s", vm.State)
	}

	// Verify it shows up in ListVMs from virsh
	list, err := adapter.ListVMs(ctx)
	if err != nil {
		t.Fatalf("ListVMs failed: %v", err)
	}
	found := false
	for _, v := range list {
		if v.ID == vmID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("real VM %s was not found in ListVMs", vmID)
	}

	// Verify Stop
	stopped, err := adapter.StopVM(ctx, vmID, true)
	if err != nil || stopped.State != "STOPPED" {
		t.Errorf("StopVM failed: state=%v, err=%v", stopped, err)
	}

	// Verify Delete
	if err := adapter.DeleteVM(ctx, vmID); err != nil {
		t.Errorf("DeleteVM failed: %v", err)
	}
}
