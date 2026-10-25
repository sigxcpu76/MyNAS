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
	adapter := kvm.NewAdapter(true)

	// 1. List VMs and check multi-disk, multi-nic, UEFI and QGA attributes
	vms, err := adapter.ListVMs(ctx)
	if err != nil {
		t.Fatalf("ListVMs failed: %v", err)
	}
	if len(vms) < 3 {
		t.Fatalf("expected at least 3 default VMs, got %d", len(vms))
	}

	ubuntu, err := adapter.GetVM(ctx, "ubuntu-24-lts")
	if err != nil {
		t.Fatalf("GetVM(ubuntu-24-lts) failed: %v", err)
	}
	if len(ubuntu.Disks) < 2 {
		t.Errorf("expected multi-disk for ubuntu-24-lts, got %d", len(ubuntu.Disks))
	}
	if len(ubuntu.NetworkIfaces) < 2 {
		t.Errorf("expected multi-NIC for ubuntu-24-lts, got %d", len(ubuntu.NetworkIfaces))
	}
	if ubuntu.BiosType != "ovmf" || ubuntu.TPMVersion != "v2.0" {
		t.Errorf("expected OVMF UEFI and TPM v2.0, got %s / %s", ubuntu.BiosType, ubuntu.TPMVersion)
	}
	if len(ubuntu.GuestIPAddresses) == 0 {
		t.Errorf("expected QGA guest IP addresses, got 0")
	}

	// 2. Test Snapshots
	snaps, err := adapter.ListSnapshots(ctx, "ubuntu-24-lts")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snaps) < 2 {
		t.Errorf("expected at least 2 default snapshots, got %d", len(snaps))
	}

	newSnap, err := adapter.CreateSnapshot(ctx, "ubuntu-24-lts", domain.VirtualMachineSnapshot{
		SnapshotID:  "test-snap-1",
		Description: "Unit test snapshot",
		IncludeRAM:  false,
	})
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if newSnap.SnapshotID != "test-snap-1" {
		t.Errorf("expected test-snap-1, got %s", newSnap.SnapshotID)
	}

	// Rollback snapshot
	_, err = adapter.RollbackSnapshot(ctx, "ubuntu-24-lts", "test-snap-1")
	if err != nil {
		t.Fatalf("RollbackSnapshot failed: %v", err)
	}

	// Delete snapshot
	if err := adapter.DeleteSnapshot(ctx, "ubuntu-24-lts", "test-snap-1"); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// 3. Test Lifecycle: Pause, Resume, Reset
	paused, err := adapter.PauseVM(ctx, "ubuntu-24-lts")
	if err != nil || paused.State != "PAUSED" {
		t.Errorf("PauseVM failed: state=%s, err=%v", paused.State, err)
	}
	resumed, err := adapter.ResumeVM(ctx, "ubuntu-24-lts")
	if err != nil || resumed.State != "RUNNING" {
		t.Errorf("ResumeVM failed: state=%s, err=%v", resumed.State, err)
	}
	reset, err := adapter.ResetVM(ctx, "ubuntu-24-lts")
	if err != nil || reset.State != "RUNNING" {
		t.Errorf("ResetVM failed: state=%s, err=%v", reset.State, err)
	}

	// 4. Test Cloning
	cloned, err := adapter.CloneVM(ctx, "ubuntu-24-lts", "ubuntu-clone-99", "Ubuntu Cloned Worker", "tank", true)
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}
	if cloned.ID != "ubuntu-clone-99" || len(cloned.Disks) != len(ubuntu.Disks) {
		t.Errorf("CloneVM returned invalid clone: %+v", cloned)
	}

	// 5. Test Media Attach & Eject
	attached, err := adapter.AttachMedia(ctx, "ubuntu-24-lts", "hdc", "/tank/isos/debian-netinst.iso")
	if err != nil || attached.CdromIsoImage != "/tank/isos/debian-netinst.iso" {
		t.Errorf("AttachMedia failed: %v", err)
	}
	ejected, err := adapter.EjectMedia(ctx, "ubuntu-24-lts", "hdc")
	if err != nil || ejected.CdromIsoImage != "" {
		t.Errorf("EjectMedia failed: %v", err)
	}
}

func TestRealLibvirtDomainLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := kvm.NewAdapterWithStateDir(false, tempDir)

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
