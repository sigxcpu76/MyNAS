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

func TestRealLibvirtDomainWithCdromLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := kvm.NewAdapterWithStateDir(tempDir)

	if _, err := adapter.ListVMs(ctx); err != nil {
		t.Skipf("virsh/libvirt not available on this host: %v", err)
	}

	vmID := "nasctl-ci-test-cdrom"
	diskPath := tempDir + "/ci-disk.raw"
	isoPath := tempDir + "/ci-boot.iso"
	if err := os.WriteFile(isoPath, []byte("fake-iso-boot-content"), 0644); err != nil {
		t.Fatalf("failed to write dummy iso: %v", err)
	}

	// Create real VM with both hard disk and boot ISO
	vm, err := adapter.CreateVM(ctx, domain.VirtualMachine{
		ID:            vmID,
		VMName:        "NASCTL Real Test CDROM VM",
		MemoryMiB:     256,
		VCPUs:         1,
		CdromIsoImage: isoPath,
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
		t.Fatalf("Failed to create real VM with CDROM: %v", err)
	}
	defer func() {
		_ = adapter.DeleteVM(ctx, vmID)
		_ = os.Remove(diskPath)
		_ = os.Remove(isoPath)
	}()

	if vm.CdromIsoImage != isoPath {
		t.Errorf("expected CdromIsoImage %q, got %q", isoPath, vm.CdromIsoImage)
	}

	// Test AttachMedia and EjectMedia
	newIsoPath := tempDir + "/ci-new.iso"
	if err := os.WriteFile(newIsoPath, []byte("fake-new-iso"), 0644); err != nil {
		t.Fatalf("failed to write new dummy iso: %v", err)
	}
	defer os.Remove(newIsoPath)

	attached, err := adapter.AttachMedia(ctx, vmID, "", newIsoPath)
	if err != nil {
		t.Errorf("AttachMedia failed: %v", err)
	} else if attached.CdromIsoImage != newIsoPath {
		t.Errorf("expected attached ISO %q, got %q", newIsoPath, attached.CdromIsoImage)
	}

	ejected, err := adapter.EjectMedia(ctx, vmID, "")
	if err != nil {
		t.Errorf("EjectMedia failed: %v", err)
	} else if ejected.CdromIsoImage != "" {
		t.Errorf("expected empty CdromIsoImage after eject, got %q", ejected.CdromIsoImage)
	}

	_, _ = adapter.StopVM(ctx, vmID, true)
	_ = adapter.DeleteVM(ctx, vmID)
}

func TestQemuSoftwareEmulationCpuCompatibility(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := kvm.NewAdapterWithStateDir(tempDir)

	if _, err := adapter.ListVMs(ctx); err != nil {
		t.Skipf("virsh/libvirt not available on this host: %v", err)
	}

	vmID := "nasctl-ci-test-qemu-emu"
	diskPath := tempDir + "/qemu-emu.raw"

	// Define VM with CPUType "host"
	vm, err := adapter.CreateVM(ctx, domain.VirtualMachine{
		ID:        vmID,
		VMName:    "NASCTL QEMU Emulation Test",
		MemoryMiB: 256,
		VCPUs:     1,
		CPUType:   "host",
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
		t.Fatalf("Failed to create VM: %v", err)
	}
	defer func() {
		_, _ = adapter.StopVM(ctx, vmID, true)
		_ = adapter.DeleteVM(ctx, vmID)
		_ = os.Remove(diskPath)
	}()

	if vm.State != "RUNNING" {
		t.Errorf("expected RUNNING, got %s", vm.State)
	}
}

func TestVmRebootAndStopForcedOptions(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := kvm.NewAdapterWithStateDir(tempDir)

	if _, err := adapter.ListVMs(ctx); err != nil {
		t.Skipf("virsh/libvirt not available on this host: %v", err)
	}

	vmID := "nasctl-ci-test-force-opts"
	diskPath := tempDir + "/ci-force.raw"

	vm, err := adapter.CreateVM(ctx, domain.VirtualMachine{
		ID:        vmID,
		VMName:    "Test Forced Options VM",
		MemoryMiB: 256,
		VCPUs:     1,
		CPUType:   "host",
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
		t.Fatalf("CreateVM failed: %v", err)
	}
	defer func() {
		_, _ = adapter.StopVM(ctx, vm.ID, true)
		_ = adapter.DeleteVM(ctx, vm.ID)
		_ = os.Remove(diskPath)
	}()

	// Test graceful reboot
	rebooted, err := adapter.RebootVM(ctx, vm.ID, false)
	if err != nil || rebooted.State != "RUNNING" {
		t.Fatalf("RebootVM (graceful) failed: %v", err)
	}

	// Test forced reboot (reset)
	resetVM, err := adapter.RebootVM(ctx, vm.ID, true)
	if err != nil || resetVM.State != "RUNNING" {
		t.Fatalf("RebootVM (forced) failed: %v", err)
	}

	// Test forced stop
	stoppedForce, err := adapter.StopVM(ctx, vm.ID, true)
	if err != nil || stoppedForce.State != "STOPPED" {
		t.Fatalf("StopVM (forced) failed: %v", err)
	}
}
