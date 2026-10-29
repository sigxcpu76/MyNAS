package kvm

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestNormalizeMachineTypeForLibvirt(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"i440fx", "pc"},
		{"pc-i440fx", "pc"},
		{"pc", "pc"},
		{"q35", "q35"},
		{"pc-q35", "q35"},
		{"pc-q35-7.2", "q35"},
		{"", "pc"},
		{"unknown-custom", "unknown-custom"},
	}

	for _, tc := range tests {
		got := normalizeMachineTypeForLibvirt(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeMachineTypeForLibvirt(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestMemoryXMLToMiB(t *testing.T) {
	tests := []struct {
		m        memoryXML
		expected int64
	}{
		{memoryXML{Unit: "MiB", Value: 2048}, 2048},
		{memoryXML{Unit: "mib", Value: 1024}, 1024},
		{memoryXML{Unit: "GiB", Value: 4}, 4096},
		{memoryXML{Unit: "gib", Value: 2}, 2048},
		{memoryXML{Unit: "KiB", Value: 1048576}, 1024},
		{memoryXML{Unit: "bytes", Value: 1073741824}, 1024},
		{memoryXML{Unit: "", Value: 2097152}, 2048}, // Libvirt default is KiB
	}

	for _, tc := range tests {
		got := tc.m.ToMiB()
		if got != tc.expected {
			t.Errorf("memoryXML{%s, %d}.ToMiB() = %d, expected %d", tc.m.Unit, tc.m.Value, got, tc.expected)
		}
	}
}

func TestBuildDomainXML_BasicAndDisks(t *testing.T) {
	vm := domain.VirtualMachine{
		ID:          "vm-test-101",
		VMName:      "Production Database",
		Description: "Main Postgres Instance",
		UUID:        "550e8400-e29b-41d4-a716-446655440000",
		MemoryMiB:   4096,
		VCPUs:       4,
		Sockets:     2,
		Cores:       2,
		MachineType: "q35",
		BiosType:    "ovmf",
		Disks: []domain.VirtualMachineDisk{
			{
				ID:        "sda",
				Bus:       "scsi",
				DiskPath:  "/dev/zvol/tank/vms/vm-test-101-os",
				CacheMode: "none",
				Discard:   true,
			},
			{
				ID:        "vda",
				Bus:       "virtio",
				DiskPath:  "/mnt/tank/vms/vm-test-101/data.qcow2",
				CacheMode: "writeback",
				Discard:   false,
			},
		},
		CdromIsoImage: "/mnt/tank/isos/debian-12.iso",
	}

	xmlStr := buildDomainXML(vm)

	if !strings.Contains(xmlStr, "<name>vm-test-101</name>") {
		t.Errorf("expected VM name in XML, got:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, "<uuid>550e8400-e29b-41d4-a716-446655440000</uuid>") {
		t.Errorf("expected UUID in XML")
	}
	if !strings.Contains(xmlStr, "<description>Main Postgres Instance</description>") {
		t.Errorf("expected description in XML")
	}
	if !strings.Contains(xmlStr, "<memory unit='MiB'>4096</memory>") {
		t.Errorf("expected memory 4096 in XML")
	}
	if !strings.Contains(xmlStr, "<vcpu placement='static'>4</vcpu>") {
		t.Errorf("expected vcpu 4 in XML")
	}
	if !strings.Contains(xmlStr, "topology sockets='2' cores='2' threads='1'") {
		t.Errorf("expected CPU topology sockets=2 cores=2 in XML")
	}
	if !strings.Contains(xmlStr, "firmware='efi'") {
		t.Errorf("expected EFI firmware for ovmf bios in XML")
	}
	if !strings.Contains(xmlStr, "machine='q35'") {
		t.Errorf("expected machine type q35 in XML")
	}
	if !strings.Contains(xmlStr, "<boot dev='hd'/>") {
		t.Errorf("expected hd boot dev in XML")
	}
	if !strings.Contains(xmlStr, "<boot dev='cdrom'/>") {
		t.Errorf("expected cdrom boot dev in XML")
	}
	hdIdx := strings.Index(xmlStr, "<boot dev='hd'/>")
	cdIdx := strings.Index(xmlStr, "<boot dev='cdrom'/>")
	if hdIdx == -1 || cdIdx == -1 || hdIdx >= cdIdx {
		t.Errorf("expected hd boot device before cdrom in XML: hdIdx=%d, cdIdx=%d", hdIdx, cdIdx)
	}
	if !strings.Contains(xmlStr, "bus='scsi'") {
		t.Errorf("expected scsi bus for disk in XML")
	}
	if !strings.Contains(xmlStr, "discard='unmap'") {
		t.Errorf("expected discard=unmap for sda")
	}
	if !strings.Contains(xmlStr, "type='qcow2'") {
		t.Errorf("expected driver type qcow2 for data disk")
	}
	if !strings.Contains(xmlStr, "<input type='tablet' bus='usb'/>") {
		t.Errorf("expected usb tablet input device for absolute cursor tracking in XML")
	}
}

func TestBuildDomainXML_MultipleCdroms(t *testing.T) {
	vm := domain.VirtualMachine{
		ID:     "win-vm",
		VMName: "Windows VM with VirtIO Driver CD",
		VCPUs:  4,
		Cdroms: []domain.VirtualMachineCdrom{
			{
				ID:      "sdz",
				Bus:     "sata",
				IsoPath: "/mnt/fast/isos/win10.iso",
			},
			{
				ID:      "sdy",
				Bus:     "sata",
				IsoPath: "/mnt/fast/isos/virtio-win.iso",
			},
			{
				ID:      "sdx",
				Bus:     "sata",
				IsoPath: "", // empty tray
			},
		},
	}

	xmlStr := buildDomainXML(vm)

	if !strings.Contains(xmlStr, "<source file='/mnt/fast/isos/win10.iso'/>") {
		t.Errorf("expected win10.iso in XML")
	}
	if !strings.Contains(xmlStr, "<target dev='sdz' bus='sata'/>") {
		t.Errorf("expected sdz target in XML")
	}
	if !strings.Contains(xmlStr, "<source file='/mnt/fast/isos/virtio-win.iso'/>") {
		t.Errorf("expected virtio-win.iso in XML")
	}
	if !strings.Contains(xmlStr, "<target dev='sdy' bus='sata'/>") {
		t.Errorf("expected sdy target in XML")
	}
	if !strings.Contains(xmlStr, "<target dev='sdx' bus='sata'/>") {
		t.Errorf("expected sdx target in XML")
	}
}

func TestParseBootOrder(t *testing.T) {
	tests := []struct {
		input    string
		hasCdrom bool
		expected []string
	}{
		{"", false, []string{"hd"}},
		{"", true, []string{"hd", "cdrom"}},
		{"cdrom,hd", true, []string{"cdrom", "hd"}},
		{"hd,cdrom,network", true, []string{"hd", "cdrom", "network"}},
		{"network,hd", false, []string{"network", "hd"}},
		{"order=scsi0;ide2;net0", true, []string{"hd", "cdrom", "network"}},
	}

	for _, tc := range tests {
		got := parseBootOrder(tc.input, tc.hasCdrom)
		if len(got) != len(tc.expected) {
			t.Errorf("parseBootOrder(%q, %v) = %v, expected %v", tc.input, tc.hasCdrom, got, tc.expected)
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("parseBootOrder(%q, %v)[%d] = %q, expected %q", tc.input, tc.hasCdrom, i, got[i], tc.expected[i])
			}
		}
	}
}

func TestBuildDomainXML_BootOrder(t *testing.T) {
	vm := domain.VirtualMachine{
		ID:        "vm-bootorder",
		MemoryMiB: 1024,
		VCPUs:     1,
		BootOrder: "cdrom,hd,network",
	}
	xmlStr := buildDomainXML(vm)
	cdIdx := strings.Index(xmlStr, "<boot dev='cdrom'/>")
	hdIdx := strings.Index(xmlStr, "<boot dev='hd'/>")
	netIdx := strings.Index(xmlStr, "<boot dev='network'/>")

	if cdIdx == -1 || hdIdx == -1 || netIdx == -1 {
		t.Fatalf("expected all boot devices in XML, got: %s", xmlStr)
	}
	if cdIdx >= hdIdx || hdIdx >= netIdx {
		t.Errorf("expected order cdrom -> hd -> network, got indices: cdrom=%d, hd=%d, net=%d", cdIdx, hdIdx, netIdx)
	}
}

func TestBootOrderToHMP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "cd"},
		{"hd,cdrom", "cd"},
		{"cdrom,hd", "dc"},
		{"hd", "c"},
		{"cdrom", "d"},
		{"network,hd", "nc"},
		{"network,hd,cdrom", "ncd"},
		{"order=scsi0;ide2;net0", "cdn"},
	}

	for _, tc := range tests {
		actual := bootOrderToHMP(tc.input)
		if actual != tc.expected {
			t.Errorf("bootOrderToHMP(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestParseVirshEventLine(t *testing.T) {
	tests := []struct {
		line             string
		expectNil        bool
		expectedTopic    string
		expectedAction   string
		expectedResource string
		expectedMessage  string
	}{
		{
			line:             "event 'lifecycle' for domain mynas: Started Booted",
			expectNil:        false,
			expectedTopic:    "kvm",
			expectedAction:   "lifecycle",
			expectedResource: "virtualMachines/mynas",
			expectedMessage:  "Started Booted",
		},
		{
			line:             "event 'reboot' for domain mynas",
			expectNil:        false,
			expectedTopic:    "kvm",
			expectedAction:   "reboot",
			expectedResource: "virtualMachines/mynas",
			expectedMessage:  "",
		},
		{
			line:             "event 'agent-lifecycle' for domain win10: State changed to 1",
			expectNil:        false,
			expectedTopic:    "kvm",
			expectedAction:   "agent-lifecycle",
			expectedResource: "virtualMachines/win10",
			expectedMessage:  "State changed to 1",
		},
		{
			line:      "random unformatted log output",
			expectNil: true,
		},
	}

	for _, tc := range tests {
		evt := parseVirshEventLine(tc.line)
		if tc.expectNil {
			if evt != nil {
				t.Errorf("expected nil for line %q, got %+v", tc.line, evt)
			}
			continue
		}
		if evt == nil {
			t.Fatalf("expected event for line %q, got nil", tc.line)
		}
		if evt.Topic != tc.expectedTopic {
			t.Errorf("expected Topic %q, got %q", tc.expectedTopic, evt.Topic)
		}
		if evt.Action != tc.expectedAction {
			t.Errorf("expected Action %q, got %q", tc.expectedAction, evt.Action)
		}
		if evt.Resource != tc.expectedResource {
			t.Errorf("expected Resource %q, got %q", tc.expectedResource, evt.Resource)
		}
		if evt.Message != tc.expectedMessage {
			t.Errorf("expected Message %q, got %q", tc.expectedMessage, evt.Message)
		}
	}
}

func TestBuildDomainXML_MachineTypeAndCPUMode(t *testing.T) {
	vm := domain.VirtualMachine{
		ID:          "vm-test-i440fx",
		MemoryMiB:   1024,
		VCPUs:       1,
		MachineType: "i440fx",
		CPUType:     "Nehalem",
	}

	xmlStr := buildDomainXML(vm)
	if !strings.Contains(xmlStr, "machine='pc'") {
		t.Errorf("expected machine='pc', got:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, "cpu mode='custom'") {
		t.Errorf("expected cpu mode='custom', got:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, "<model fallback='allow'>Nehalem</model>") {
		t.Errorf("expected model Nehalem, got:\n%s", xmlStr)
	}
}

func TestDomainXML_UnmarshalParsing(t *testing.T) {
	rawXML := `
<domain type='kvm' id='42'>
  <name>nasctl-test-vm</name>
  <uuid>b32f228b-b8f4-4dfc-9b55-667dbf5d88f1</uuid>
  <description>Test VM</description>
  <memory unit='KiB'>2097152</memory>
  <currentMemory unit='KiB'>2097152</currentMemory>
  <vcpu placement='static'>2</vcpu>
  <os firmware='efi'>
    <type arch='x86_64' machine='pc-q35-7.2'>hvm</type>
  </os>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='raw' cache='none' discard='unmap'/>
      <source file='/mnt/tank/vms/test.raw'/>
      <target dev='sda' bus='scsi'/>
    </disk>
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='/mnt/tank/isos/alpine.iso'/>
      <target dev='sdz' bus='sata'/>
    </disk>
    <interface type='bridge'>
      <mac address='52:54:00:12:34:56'/>
      <source bridge='br0'/>
      <model type='virtio'/>
      <vlan>
        <tag id='20'/>
      </vlan>
    </interface>
    <graphics type='vnc' port='5901'/>
  </devices>
</domain>
`
	var d domainXML
	err := xml.Unmarshal([]byte(rawXML), &d)
	if err != nil {
		t.Fatalf("xml.Unmarshal failed: %v", err)
	}

	if d.Name != "nasctl-test-vm" {
		t.Errorf("expected name nasctl-test-vm, got %s", d.Name)
	}
	if d.Memory.ToMiB() != 2048 {
		t.Errorf("expected memory 2048 MiB, got %d", d.Memory.ToMiB())
	}
	if d.OS.Firmware != "efi" {
		t.Errorf("expected firmware efi, got %s", d.OS.Firmware)
	}
	if len(d.Devices.Disks) != 2 {
		t.Fatalf("expected 2 disks, got %d", len(d.Devices.Disks))
	}
	if d.Devices.Disks[0].Target.Bus != "scsi" {
		t.Errorf("expected bus scsi, got %s", d.Devices.Disks[0].Target.Bus)
	}
	if len(d.Devices.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(d.Devices.Interfaces))
	}
	if d.Devices.Interfaces[0].Vlan.Tag.ID != 20 {
		t.Errorf("expected vlan tag 20, got %d", d.Devices.Interfaces[0].Vlan.Tag.ID)
	}
	if d.Devices.Interfaces[0].Mac.Address != "52:54:00:12:34:56" {
		t.Errorf("expected mac address 52:54:00:12:34:56, got %s", d.Devices.Interfaces[0].Mac.Address)
	}
}

func TestResolveHostNetworkTarget_NonRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping non-root check when running as root")
	}
	netType, dev := resolveHostNetworkTarget("br0")
	if netType != "user" || dev != "" {
		t.Errorf("expected (user, ''), got (%s, %s)", netType, dev)
	}
}

// Live integration test: strictly opt-in and root-only to prevent polkit authentication popups.
func TestLiveLibvirtDomainLifecycle_OptInOnly(t *testing.T) {
	if os.Getenv("NASCTL_TEST_REAL_LIBVIRT") != "1" || os.Geteuid() != 0 {
		t.Skip("skipping live libvirt daemon test (requires root privileges and NASCTL_TEST_REAL_LIBVIRT=1)")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	adapter := NewAdapterWithStateDir(tempDir)

	if _, err := adapter.ListVMs(ctx); err != nil {
		t.Skipf("virsh/libvirt not available on this host: %v", err)
	}

	vmID := "nasctl-ci-test-vm"
	diskPath := tempDir + "/ci-test.raw"

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
	stopped, err := adapter.StopVM(ctx, vmID, true)
	if err != nil || stopped.State != "STOPPED" {
		t.Errorf("StopVM failed: state=%v, err=%v", stopped, err)
	}
}
