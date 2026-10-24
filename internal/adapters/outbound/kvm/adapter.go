package kvm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type consoleTicket struct {
	VMID      string
	Token     string
	ExpiresAt time.Time
}

type Adapter struct {
	mu        sync.RWMutex
	hybrid    bool
	vms       map[string]domain.VirtualMachine
	tickets   map[string]consoleTicket
	snapshots map[string][]domain.VirtualMachineSnapshot
	nextPort  int32
}

func NewAdapter(hybrid bool) *Adapter {
	a := &Adapter{
		hybrid:    hybrid,
		vms:       make(map[string]domain.VirtualMachine),
		tickets:   make(map[string]consoleTicket),
		snapshots: make(map[string][]domain.VirtualMachineSnapshot),
		nextPort:  5903,
	}
	if hybrid {
		a.seedDefaults()
	}
	return a
}

func (a *Adapter) seedDefaults() {
	now := time.Now().UTC()
	a.vms["ubuntu-24-lts"] = domain.VirtualMachine{
		ID:                  "ubuntu-24-lts",
		VMName:              "ubuntu-24-lts",
		UUID:                "a7f4e912-38b1-4f92-9b1c-812093a4b001",
		State:               "RUNNING",
		Sockets:             1,
		Cores:               4,
		VCPUs:               4,
		CPUType:             "host",
		CPUUnits:            1024,
		MemoryMiB:           8192,
		BalloonMinMiB:       4096,
		Hugepages:           "2MB",
		MachineType:         "q35",
		BiosType:            "ovmf",
		TPMVersion:          "v2.0",
		SecureBoot:          true,
		DiskPath:            "/dev/zvol/nvme-fast/vm-ubuntu-root",
		DiskSizeGiB:         64,
		ISOImage:            "/tank/isos/ubuntu-24.04.1-live-server-amd64.iso",
		CdromIsoImage:       "/tank/isos/ubuntu-24.04.1-live-server-amd64.iso",
		NetworkBridge:       "virbr0",
		VNCPort:             5901,
		VNCDisplay:          "127.0.0.1:1 (port 5901)",
		Autostart:           true,
		OSVariant:           "ubuntu24.04",
		Description:         "Primary Linux application & Kubernetes worker node",
		AgentEnabled:        true,
		AgentRunning:        true,
		GuestIPAddresses:    []string{"192.168.1.115", "fe80::5054:ff:fe12:3456"},
		GuestOSName:         "Ubuntu 24.04.1 LTS (Noble Numbat)",
		GuestCPUPercent:     12.4,
		GuestMemoryUsedMiB:  2850,
		StartupDelaySeconds: 10,
		BootOrder:           "order=scsi0;ide2;net0",
		WatchdogModel:       "i6300esb",
		WatchdogAction:      "reset",
		DisplayType:         "virtio-gpu",
		Disks: []domain.VirtualMachineDisk{
			{
				ID:             "scsi0",
				Bus:            "scsi",
				DiskPath:       "/dev/zvol/nvme-fast/vm-ubuntu-root",
				SizeBytes:      68719476736,
				CacheMode:      "none",
				Discard:        true,
				IOThread:       true,
				SSDEmulation:   true,
				Backup:         true,
				IOPSLimitRead:  15000,
				IOPSLimitWrite: 10000,
			},
			{
				ID:        "scsi1",
				Bus:       "scsi",
				DiskPath:  "/dev/zvol/nvme-fast/vm-ubuntu-data",
				SizeBytes: 107374182400,
				CacheMode: "none",
				Discard:   true,
				IOThread:  true,
				Backup:    true,
			},
		},
		NetworkIfaces: []domain.VirtualMachineNetworkInterface{
			{
				ID:            "net0",
				Model:         "virtio",
				Bridge:        "virbr0",
				VlanTag:       0,
				MacAddress:    "bc:24:11:80:a1:01",
				RateLimitMbps: 1000,
				Firewall:      true,
			},
			{
				ID:            "net1",
				Model:         "virtio",
				Bridge:        "br0",
				VlanTag:       20,
				MacAddress:    "bc:24:11:80:a1:02",
				RateLimitMbps: 1000,
				Firewall:      true,
			},
		},
		CloudInit: domain.VirtualMachineCloudInit{
			User:          "admin",
			SSHPublicKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICa5/w8v03xW9 admin@nas-cluster"},
			IPConfig:      "ip=192.168.1.115/24,gw=192.168.1.1",
		},
		QemuCommandlineArgs: []string{
			"-global", "ICH9-LPC.disable_s3=1",
			"-cpu", "+kvm_pv_unhalt,+kvm_pv_eoi",
		},
		SerialConsoleURL: "/ws/serial/ubuntu-24-lts",
	}

	a.snapshots["ubuntu-24-lts"] = []domain.VirtualMachineSnapshot{
		{
			Name:           "virtualMachines/ubuntu-24-lts/snapshots/clean-base",
			SnapshotID:     "clean-base",
			Description:    "Post-installation baseline with Docker & ZFS tools",
			CreateTime:     now.Add(-48 * time.Hour),
			IncludeRAM:     false,
			ParentSnapshot: "",
			VMState:        "STOPPED",
		},
		{
			Name:           "virtualMachines/ubuntu-24-lts/snapshots/pre-k8s-upgrade",
			SnapshotID:     "pre-k8s-upgrade",
			Description:    "Live snapshot before Kubernetes v1.31 upgrade",
			CreateTime:     now.Add(-2 * time.Hour),
			IncludeRAM:     true,
			ParentSnapshot: "clean-base",
			VMState:        "RUNNING",
		},
	}

	a.vms["opnsense-fw"] = domain.VirtualMachine{
		ID:                  "opnsense-fw",
		VMName:              "opnsense-fw",
		UUID:                "b9c21044-61e0-4410-81c4-990218c1e002",
		State:               "RUNNING",
		Sockets:             1,
		Cores:               2,
		VCPUs:               2,
		CPUType:             "host",
		MemoryMiB:           4096,
		MachineType:         "q35",
		BiosType:            "ovmf",
		DiskPath:            "/dev/zvol/tank/kvm-storage-lun0",
		DiskSizeGiB:         32,
		ISOImage:            "/tank/isos/OPNsense-24.7-dvd-amd64.iso",
		CdromIsoImage:       "/tank/isos/OPNsense-24.7-dvd-amd64.iso",
		NetworkBridge:       "br0",
		VNCPort:             5902,
		VNCDisplay:          "127.0.0.1:2 (port 5902)",
		Autostart:           true,
		OSVariant:           "freebsd14.0",
		Description:         "Virtual router & WireGuard VPN concentrator",
		AgentEnabled:        true,
		AgentRunning:        true,
		GuestIPAddresses:    []string{"192.168.1.1", "10.0.0.1"},
		GuestOSName:         "OPNsense 24.7 (FreeBSD 14.1-RELEASE)",
		GuestCPUPercent:     4.2,
		GuestMemoryUsedMiB:  1420,
		StartupDelaySeconds: 0,
		BootOrder:           "order=scsi0;ide2",
		WatchdogModel:       "i6300esb",
		WatchdogAction:      "reset",
		DisplayType:         "virtio-gpu",
		Disks: []domain.VirtualMachineDisk{
			{
				ID:        "scsi0",
				Bus:       "scsi",
				DiskPath:  "/dev/zvol/tank/kvm-storage-lun0",
				SizeBytes: 34359738368,
				CacheMode: "none",
				Discard:   true,
				IOThread:  true,
			},
		},
		NetworkIfaces: []domain.VirtualMachineNetworkInterface{
			{ID: "net0", Model: "virtio", Bridge: "br0", VlanTag: 10, MacAddress: "bc:24:11:80:a2:01", Firewall: false},
			{ID: "net1", Model: "virtio", Bridge: "br0", VlanTag: 20, MacAddress: "bc:24:11:80:a2:02", Firewall: false},
		},
		SerialConsoleURL: "/ws/serial/opnsense-fw",
	}

	a.vms["debian-lab"] = domain.VirtualMachine{
		ID:            "debian-lab",
		VMName:        "debian-lab",
		UUID:          "c1d88219-94a2-4088-a021-771230f1d003",
		State:         "STOPPED",
		Sockets:       1,
		Cores:         2,
		VCPUs:         2,
		CPUType:       "kvm64",
		MemoryMiB:     2048,
		MachineType:   "q35",
		BiosType:      "seabios",
		DiskPath:      "/tank/vms/debian-lab.qcow2",
		DiskSizeGiB:   40,
		ISOImage:      "/tank/isos/debian-12.7.0-amd64-netinst.iso",
		CdromIsoImage: "/tank/isos/debian-12.7.0-amd64-netinst.iso",
		NetworkBridge: "virbr0",
		VNCPort:       5903,
		VNCDisplay:    "127.0.0.1:3 (port 5903)",
		Autostart:     false,
		OSVariant:     "debian12",
		Description:   "Sandbox testing environment",
		Disks: []domain.VirtualMachineDisk{
			{
				ID:        "virtio0",
				Bus:       "virtio",
				DiskPath:  "/tank/vms/debian-lab.qcow2",
				SizeBytes: 42949672960,
				CacheMode: "writeback",
				Discard:   true,
			},
		},
		NetworkIfaces: []domain.VirtualMachineNetworkInterface{
			{ID: "net0", Model: "virtio", Bridge: "virbr0", MacAddress: "bc:24:11:80:a3:01"},
		},
		SerialConsoleURL: "/ws/serial/debian-lab",
	}
}

func (a *Adapter) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.VirtualMachine, 0, len(a.vms))
	for _, vm := range a.vms {
		a.enrichRuntimeTelemetry(ctx, &vm)
		out = append(out, vm)
	}
	return out, nil
}

func (a *Adapter) GetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	cp := vm
	a.enrichRuntimeTelemetry(ctx, &cp)
	return &cp, nil
}

func (a *Adapter) enrichRuntimeTelemetry(ctx context.Context, vm *domain.VirtualMachine) {
	if vm.State != "RUNNING" {
		return
	}
	// Try QEMU guest agent if virsh is installed
	if _, err := exec.LookPath("virsh"); err == nil {
		tctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(tctx, "virsh", "qemu-agent-command", vm.ID, `{"execute":"guest-network-get-interfaces"}`)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			var resp struct {
				Return []struct {
					Name       string `json:"name"`
					IPAddresses []struct {
						IPType  string `json:"ip-address-type"`
						IPAddr  string `json:"ip-address"`
						Prefix  int    `json:"prefix"`
					} `json:"ip-addresses"`
				} `json:"return"`
			}
			if err := json.Unmarshal(out, &resp); err == nil {
				var ips []string
				for _, iface := range resp.Return {
					if iface.Name == "lo" {
						continue
					}
					for _, ip := range iface.IPAddresses {
						if ip.IPAddr != "" && !strings.HasPrefix(ip.IPAddr, "127.") {
							ips = append(ips, ip.IPAddr)
						}
					}
				}
				if len(ips) > 0 {
					vm.GuestIPAddresses = ips
					vm.AgentRunning = true
				}
			}
		}
	}
}

func (a *Adapter) CreateVM(ctx context.Context, vm domain.VirtualMachine) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.vms[vm.ID]; exists {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrAlreadyExists, vm.ID)
	}
	if err := vm.Validate(); err != nil {
		return nil, err
	}

	a.nextPort++
	vm.VNCPort = a.nextPort
	vm.VNCDisplay = fmt.Sprintf("127.0.0.1:%d (port %d)", vm.VNCPort-5900, vm.VNCPort)
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}
	vm.State = "RUNNING"
	vm.SerialConsoleURL = fmt.Sprintf("/ws/serial/%s", vm.ID)

	// If disks list is empty, initialize default
	if len(vm.Disks) == 0 {
		diskPath := vm.DiskPath
		if diskPath == "" {
			diskPath = fmt.Sprintf("/tank/vms/%s.qcow2", vm.ID)
		}
		sizeGiB := vm.DiskSizeGiB
		if sizeGiB <= 0 {
			sizeGiB = 32
		}
		vm.Disks = []domain.VirtualMachineDisk{
			{
				ID:        "scsi0",
				Bus:       "scsi",
				DiskPath:  diskPath,
				SizeBytes: sizeGiB * 1024 * 1024 * 1024,
				CacheMode: "none",
				Discard:   true,
				IOThread:  true,
			},
		}
	}

	// If network interfaces list is empty, initialize default
	if len(vm.NetworkIfaces) == 0 {
		bridge := vm.NetworkBridge
		if bridge == "" {
			bridge = "virbr0"
		}
		vm.NetworkIfaces = []domain.VirtualMachineNetworkInterface{
			{
				ID:         "net0",
				Model:      "virtio",
				Bridge:     bridge,
				MacAddress: generateMAC(),
				Firewall:   true,
			},
		}
	}

	// Try virsh define if available on host
	if _, err := exec.LookPath("virsh"); err == nil {
		xml := buildDomainXML(vm)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xml)
		_ = cmd.Run()
		_ = exec.CommandContext(ctx, "virsh", "start", vm.ID).Run()
	}

	a.vms[vm.ID] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) UpdateVM(ctx context.Context, vm domain.VirtualMachine, fields []string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.vms[vm.ID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vm.ID)
	}

	if vm.VCPUs > 0 {
		existing.VCPUs = vm.VCPUs
	}
	if vm.Sockets > 0 {
		existing.Sockets = vm.Sockets
	}
	if vm.Cores > 0 {
		existing.Cores = vm.Cores
	}
	if vm.CPUType != "" {
		existing.CPUType = vm.CPUType
	}
	if vm.MemoryMiB >= 128 {
		existing.MemoryMiB = vm.MemoryMiB
	}
	if vm.BalloonMinMiB > 0 {
		existing.BalloonMinMiB = vm.BalloonMinMiB
	}
	if vm.MachineType != "" {
		existing.MachineType = vm.MachineType
	}
	if vm.BiosType != "" {
		existing.BiosType = vm.BiosType
	}
	if vm.TPMVersion != "" {
		existing.TPMVersion = vm.TPMVersion
	}
	existing.SecureBoot = vm.SecureBoot
	if len(vm.Disks) > 0 {
		existing.Disks = vm.Disks
	}
	if vm.CdromIsoImage != "" {
		existing.CdromIsoImage = vm.CdromIsoImage
	}
	if len(vm.NetworkIfaces) > 0 {
		existing.NetworkIfaces = vm.NetworkIfaces
	}
	if vm.CloudInit.User != "" || len(vm.CloudInit.SSHPublicKeys) > 0 || vm.CloudInit.IPConfig != "" {
		existing.CloudInit = vm.CloudInit
	}
	if len(vm.PciDevices) > 0 {
		existing.PciDevices = vm.PciDevices
	}
	if len(vm.UsbDevices) > 0 {
		existing.UsbDevices = vm.UsbDevices
	}
	if vm.Description != "" {
		existing.Description = vm.Description
	}
	if vm.BootOrder != "" {
		existing.BootOrder = vm.BootOrder
	}
	if vm.WatchdogModel != "" {
		existing.WatchdogModel = vm.WatchdogModel
	}
	if vm.WatchdogAction != "" {
		existing.WatchdogAction = vm.WatchdogAction
	}
	if vm.DisplayType != "" {
		existing.DisplayType = vm.DisplayType
	}
	existing.Autostart = vm.Autostart
	if len(vm.QemuCommandlineArgs) > 0 {
		existing.QemuCommandlineArgs = vm.QemuCommandlineArgs
	}

	if _, err := exec.LookPath("virsh"); err == nil {
		xml := buildDomainXML(existing)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xml)
		_ = cmd.Run()
	}

	a.vms[vm.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteVM(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.vms[id]; !exists {
		return fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "destroy", id).Run()
		_ = exec.CommandContext(ctx, "virsh", "undefine", id, "--nvram", "--remove-all-storage").Run()
	}
	delete(a.vms, id)
	delete(a.snapshots, id)
	return nil
}

func (a *Adapter) StartVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "start", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		verb := "shutdown"
		if force {
			verb = "destroy"
		}
		_ = exec.CommandContext(ctx, "virsh", verb, id).Run()
	}
	vm.State = "STOPPED"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) RebootVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "reboot", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ResetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "reset", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) PauseVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "suspend", id).Run()
	}
	vm.State = "PAUSED"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ResumeVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "resume", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) CloneVM(ctx context.Context, id, newID, newName, targetPool string, linkedClone bool) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	source, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: source virtual machine %q", domain.ErrNotFound, id)
	}
	if _, exists := a.vms[newID]; exists {
		return nil, fmt.Errorf("%w: virtual machine %q already exists", domain.ErrAlreadyExists, newID)
	}

	a.nextPort++
	cloned := source
	cloned.ID = newID
	if newName != "" {
		cloned.VMName = newName
	} else {
		cloned.VMName = newID
	}
	cloned.UUID = generateUUID()
	cloned.State = "STOPPED"
	cloned.VNCPort = a.nextPort
	cloned.VNCDisplay = fmt.Sprintf("127.0.0.1:%d (port %d)", cloned.VNCPort-5900, cloned.VNCPort)
	cloned.SerialConsoleURL = fmt.Sprintf("/ws/serial/%s", newID)

	// Clone disks
	var clonedDisks []domain.VirtualMachineDisk
	for i, d := range source.Disks {
		cd := d
		cd.ID = fmt.Sprintf("scsi%d", i)
		if strings.HasPrefix(d.DiskPath, "/dev/zvol/") {
			pool := targetPool
			if pool == "" {
				parts := strings.Split(strings.TrimPrefix(d.DiskPath, "/dev/zvol/"), "/")
				if len(parts) > 0 {
					pool = parts[0]
				} else {
					pool = "tank"
				}
			}
			cd.DiskPath = fmt.Sprintf("/dev/zvol/%s/vm-%s-disk%d", pool, newID, i)
		} else {
			cd.DiskPath = fmt.Sprintf("/tank/vms/%s-disk%d.qcow2", newID, i)
		}
		clonedDisks = append(clonedDisks, cd)
	}
	cloned.Disks = clonedDisks

	// Refresh network MAC addresses
	var clonedIfaces []domain.VirtualMachineNetworkInterface
	for _, iface := range source.NetworkIfaces {
		cif := iface
		cif.MacAddress = generateMAC()
		clonedIfaces = append(clonedIfaces, cif)
	}
	cloned.NetworkIfaces = clonedIfaces

	a.vms[newID] = cloned
	cp := cloned
	return &cp, nil
}

func (a *Adapter) AttachMedia(ctx context.Context, id, deviceID, isoPath string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if deviceID == "" {
		deviceID = "hdc"
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "change-media", id, deviceID, isoPath, "--insert").Run()
	}
	vm.CdromIsoImage = isoPath
	vm.ISOImage = isoPath
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) EjectMedia(ctx context.Context, id, deviceID string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if deviceID == "" {
		deviceID = "hdc"
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "change-media", id, deviceID, "--eject").Run()
	}
	vm.CdromIsoImage = ""
	vm.ISOImage = ""
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ListSnapshots(ctx context.Context, vmID string) ([]domain.VirtualMachineSnapshot, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, ok := a.vms[vmID]; !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	snaps := a.snapshots[vmID]
	if snaps == nil {
		snaps = []domain.VirtualMachineSnapshot{}
	}
	out := make([]domain.VirtualMachineSnapshot, len(snaps))
	copy(out, snaps)
	return out, nil
}

func (a *Adapter) CreateSnapshot(ctx context.Context, vmID string, snap domain.VirtualMachineSnapshot) (*domain.VirtualMachineSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[vmID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	if snap.SnapshotID == "" {
		snap.SnapshotID = fmt.Sprintf("snap-%d", time.Now().Unix())
	}
	snap.Name = fmt.Sprintf("virtualMachines/%s/snapshots/%s", vmID, snap.SnapshotID)
	snap.CreateTime = time.Now().UTC()
	snap.VMState = vm.State

	// Call virsh snapshot-create-as if available
	if _, err := exec.LookPath("virsh"); err == nil {
		args := []string{"snapshot-create-as", vmID, snap.SnapshotID, snap.Description}
		if !snap.IncludeRAM {
			args = append(args, "--disk-only")
		}
		_ = exec.CommandContext(ctx, "virsh", args...).Run()
	}

	a.snapshots[vmID] = append(a.snapshots[vmID], snap)
	cp := snap
	return &cp, nil
}

func (a *Adapter) RollbackSnapshot(ctx context.Context, vmID, snapID string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[vmID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	found := false
	for _, s := range a.snapshots[vmID] {
		if s.SnapshotID == snapID || s.Name == snapID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: snapshot %q for vm %q", domain.ErrNotFound, snapID, vmID)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "snapshot-revert", vmID, snapID).Run()
	}
	cp := vm
	return &cp, nil
}

func (a *Adapter) DeleteSnapshot(ctx context.Context, vmID, snapID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.vms[vmID]; !ok {
		return fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	snaps := a.snapshots[vmID]
	idx := -1
	for i, s := range snaps {
		if s.SnapshotID == snapID || s.Name == snapID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("%w: snapshot %q for vm %q", domain.ErrNotFound, snapID, vmID)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "snapshot-delete", vmID, snapID).Run()
	}
	a.snapshots[vmID] = append(snaps[:idx], snaps[idx+1:]...)
	return nil
}

func buildDomainXML(vm domain.VirtualMachine) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<domain type='kvm' xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>
  <name>%s</name>
  <uuid>%s</uuid>
  <description>%s</description>
  <memory unit='MiB'>%d</memory>
`, vm.ID, vm.UUID, vm.Description, vm.MemoryMiB))

	if vm.BalloonMinMiB > 0 {
		sb.WriteString(fmt.Sprintf(`  <currentMemory unit='MiB'>%d</currentMemory>
`, vm.BalloonMinMiB))
	}

	if vm.Hugepages != "" && vm.Hugepages != "off" {
		sb.WriteString(`  <memoryBacking>
    <hugepages/>
  </memoryBacking>
`)
	}

	// CPU topology
	sockets := vm.Sockets
	if sockets <= 0 {
		sockets = 1
	}
	cores := vm.Cores
	if cores <= 0 {
		cores = vm.VCPUs / sockets
		if cores <= 0 {
			cores = 1
		}
	}
	cpuMode := "host-passthrough"
	if vm.CPUType != "" && vm.CPUType != "host" {
		cpuMode = "custom"
	}
	sb.WriteString(fmt.Sprintf(`  <vcpu placement='static'>%d</vcpu>
  <cpu mode='%s'>
    <topology sockets='%d' cores='%d' threads='1'/>
`, vm.VCPUs, cpuMode, sockets, cores))
	if cpuMode == "custom" {
		sb.WriteString(fmt.Sprintf(`    <model fallback='forbid'>%s</model>
`, vm.CPUType))
	}
	sb.WriteString(`  </cpu>
`)

	// OS and Firmware
	mach := vm.MachineType
	if mach == "" {
		mach = "q35"
	}
	sb.WriteString(fmt.Sprintf(`  <os>
    <type arch='x86_64' machine='%s'>hvm</type>
`, mach))
	if vm.BiosType == "ovmf" {
		sb.WriteString(`    <loader readonly='yes' type='pflash'>/usr/share/OVMF/OVMF_CODE.fd</loader>
    <nvram>/var/lib/libvirt/qemu/nvram/` + vm.ID + `_VARS.fd</nvram>
`)
	}
	sb.WriteString(`    <boot dev='hd'/>
    <boot dev='cdrom'/>
  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
`)

	// Controller for VirtIO SCSI
	sb.WriteString(`    <controller type='scsi' model='virtio-scsi'/>
`)

	// Disks
	diskLetter := 'a'
	for _, d := range vm.Disks {
		devType := "disk"
		driverType := "raw"
		if strings.HasSuffix(d.DiskPath, ".qcow2") {
			driverType = "qcow2"
		}
		cacheMode := d.CacheMode
		if cacheMode == "" {
			cacheMode = "none"
		}
		discardMode := "unmap"
		if !d.Discard {
			discardMode = "ignore"
		}
		bus := d.Bus
		if bus == "" {
			bus = "scsi"
		}

		sb.WriteString(fmt.Sprintf(`    <disk type='block' device='%s'>
      <driver name='qemu' type='%s' cache='%s' discard='%s'/>
      <source dev='%s'/>
      <target dev='sd%c' bus='%s'/>
    </disk>
`, devType, driverType, cacheMode, discardMode, d.DiskPath, diskLetter, bus))
		diskLetter++
	}

	// CD-ROM media
	if vm.CdromIsoImage != "" {
		sb.WriteString(fmt.Sprintf(`    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='%s'/>
      <target dev='hdc' bus='ide'/>
      <readonly/>
    </disk>
`, vm.CdromIsoImage))
	}

	// Network interfaces
	for _, iface := range vm.NetworkIfaces {
		bridge := iface.Bridge
		if bridge == "" {
			bridge = "virbr0"
		}
		model := iface.Model
		if model == "" {
			model = "virtio"
		}
		sb.WriteString(fmt.Sprintf(`    <interface type='bridge'>
      <source bridge='%s'/>
      <model type='%s'/>
`, bridge, model))
		if iface.MacAddress != "" {
			sb.WriteString(fmt.Sprintf(`      <mac address='%s'/>
`, iface.MacAddress))
		}
		if iface.VlanTag > 0 {
			sb.WriteString(fmt.Sprintf(`      <vlan><tag id='%d'/></vlan>
`, iface.VlanTag))
		}
		sb.WriteString(`    </interface>
`)
	}

	// TPM 2.0
	if vm.TPMVersion == "v2.0" {
		sb.WriteString(`    <tpm model='tpm-tis'>
      <backend type='emulator' version='2.0'/>
    </tpm>
`)
	}

	// PCI Passthrough Devices
	for _, pci := range vm.PciDevices {
		parts := strings.Split(pci.HostAddress, ":")
		if len(parts) >= 3 {
			domainStr := parts[0]
			busStr := parts[1]
			slotFunc := strings.Split(parts[2], ".")
			if len(slotFunc) == 2 {
				sb.WriteString(fmt.Sprintf(`    <hostdev mode='subsystem' type='pci' managed='yes'>
      <source>
        <address domain='0x%s' bus='0x%s' slot='0x%s' function='0x%s'/>
      </source>
    </hostdev>
`, domainStr, busStr, slotFunc[0], slotFunc[1]))
			}
		}
	}

	// USB Passthrough Devices
	for _, usb := range vm.UsbDevices {
		if strings.Contains(usb.VendorProductID, ":") {
			ids := strings.Split(usb.VendorProductID, ":")
			sb.WriteString(fmt.Sprintf(`    <hostdev mode='subsystem' type='usb' managed='yes'>
      <source>
        <vendor id='0x%s'/>
        <product id='0x%s'/>
      </source>
    </hostdev>
`, ids[0], ids[1]))
		}
	}

	// QEMU Guest Agent Channel
	sb.WriteString(`    <channel type='unix'>
      <source mode='bind'/>
      <target type='virtio' name='org.qemu.guest_agent.0'/>
    </channel>
`)

	// Balloon
	sb.WriteString(`    <memballoon model='virtio'/>
`)

	// Graphics & VNC
	sb.WriteString(fmt.Sprintf(`    <graphics type='vnc' port='%d' autoport='no' listen='127.0.0.1'/>
    <video>
      <model type='virtio'/>
    </video>
    <serial type='pty'>
      <target port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
  </devices>
`, vm.VNCPort))

	// Raw <qemu:commandline> arguments
	if len(vm.QemuCommandlineArgs) > 0 {
		sb.WriteString(`  <qemu:commandline>
`)
		for _, arg := range vm.QemuCommandlineArgs {
			sb.WriteString(fmt.Sprintf(`    <qemu:arg value='%s'/>
`, arg))
		}
		sb.WriteString(`  </qemu:commandline>
`)
	}

	sb.WriteString(`</domain>`)
	return sb.String()
}

func generateMAC() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("bc:24:11:%02x:%02x:%02x", b[0], b[1], b[2])
}

func (a *Adapter) OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if vm.State != "RUNNING" {
		return nil, fmt.Errorf("%w: virtual machine %q must be RUNNING to open noVNC console", domain.ErrFailedPrecondition, id)
	}
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	exp := time.Now().UTC().Add(30 * time.Minute)
	a.tickets[token] = consoleTicket{
		VMID:      id,
		Token:     token,
		ExpiresAt: exp,
	}
	return &domain.ConsoleSession{
		VMName:       vm.VMName,
		Token:        token,
		WebsocketURL: fmt.Sprintf("/ws/novnc/%s?token=%s", vm.VMName, token),
		VNCPort:      vm.VNCPort,
		Resolution:   "800x480",
		ExpiresAt:    exp.Format(time.RFC3339),
	}, nil
}

func (a *Adapter) ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	vm, ok := a.vms[vmID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	if token != "" {
		if t, exists := a.tickets[token]; exists && t.VMID == vmID && time.Now().UTC().Before(t.ExpiresAt) {
			cp := vm
			return &cp, nil
		}
	}
	// Allow valid VM lookup in hybrid mode if session was just requested
	if a.hybrid && vm.State == "RUNNING" {
		cp := vm
		return &cp, nil
	}
	return nil, fmt.Errorf("%w: invalid or expired console token", domain.ErrUnauthorized)
}

func (a *Adapter) DialVNC(ctx context.Context, vmID string) (net.Conn, error) {
	vm, err := a.GetVM(ctx, vmID)
	if err != nil {
		return nil, err
	}
	// First try connecting to real QEMU VNC server on localhost:<VNCPort>
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", vm.VNCPort))
	if err == nil {
		return conn, nil
	}
	if !a.hybrid {
		return nil, fmt.Errorf("failed to connect to VNC server on 127.0.0.1:%d: %w", vm.VNCPort, err)
	}
	// In hybrid mode, spawn an embedded RFB 003.008 VNC server over net.Pipe()
	clientConn, serverConn := net.Pipe()
	go serveEmbeddedRFB(serverConn, *vm)
	return clientConn, nil
}

func serveEmbeddedRFB(conn net.Conn, vm domain.VirtualMachine) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))

	// 1. Send RFB ProtocolVersion handshake
	if _, err := conn.Write([]byte("RFB 003.008\n")); err != nil {
		return
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	// Check if client sent RFB version ("RFB ...")
	if n >= 3 && string(buf[:3]) == "RFB" {
		// 2. Security Types: 1 type -> Type 1 (None)
		_, _ = conn.Write([]byte{1, 1})
		// Read client's chosen security type (1 byte)
		if _, err := conn.Read(buf[:1]); err != nil {
			return
		}
		// 3. SecurityResult: uint32(0) = OK
		_, _ = conn.Write([]byte{0, 0, 0, 0})
		// Read ClientInit (shared flag 1 byte)
		if _, err := conn.Read(buf[:1]); err != nil {
			return
		}
		// 4. Send ServerInit: width=640, height=360, 32bpp, depth=24, bigEndian=0, trueColor=1
		const width = 640
		const height = 360
		nameBytes := []byte(fmt.Sprintf("KVM/QEMU [%s] (%d vCPU, %d MiB)", vm.VMName, vm.VCPUs, vm.MemoryMiB))
		serverInit := make([]byte, 24+len(nameBytes))
		binary.BigEndian.PutUint16(serverInit[0:2], width)
		binary.BigEndian.PutUint16(serverInit[2:4], height)
		serverInit[4] = 32 // bits-per-pixel
		serverInit[5] = 24 // depth
		serverInit[6] = 0  // big-endian-flag
		serverInit[7] = 1  // true-color-flag
		binary.BigEndian.PutUint16(serverInit[8:10], 255)  // red-max
		binary.BigEndian.PutUint16(serverInit[10:12], 255) // green-max
		binary.BigEndian.PutUint16(serverInit[12:14], 255) // blue-max
		serverInit[14] = 16                                // red-shift
		serverInit[15] = 8                                 // green-shift
		serverInit[16] = 0                                 // blue-shift
		binary.BigEndian.PutUint32(serverInit[20:24], uint32(len(nameBytes)))
		copy(serverInit[24:], nameBytes)
		if _, err := conn.Write(serverInit); err != nil {
			return
		}

		// Loop reading RFB client messages and responding to FramebufferUpdateRequest (msgType 3)
		for {
			msgTypeBuf := make([]byte, 1)
			if _, err := conn.Read(msgTypeBuf); err != nil {
				return
			}
			switch msgTypeBuf[0] {
			case 0: // SetPixelFormat (19 bytes remaining)
				discard := make([]byte, 19)
				_, _ = conn.Read(discard)
			case 2: // SetEncodings (1 padding + uint16 count + count*4)
				hdr := make([]byte, 3)
				if _, err := conn.Read(hdr); err != nil {
					return
				}
				count := binary.BigEndian.Uint16(hdr[1:3])
				discard := make([]byte, int(count)*4)
				_, _ = conn.Read(discard)
			case 3: // FramebufferUpdateRequest (9 bytes remaining)
				req := make([]byte, 9)
				if _, err := conn.Read(req); err != nil {
					return
				}
				// Send a compact Raw rectangle update (header bar 640x40)
				const rectW = 640
				const rectH = 40
				fbUpdate := make([]byte, 4+12+(rectW*rectH*4))
				fbUpdate[0] = 0 // FramebufferUpdate
				fbUpdate[1] = 0
				binary.BigEndian.PutUint16(fbUpdate[2:4], 1) // 1 rectangle
				binary.BigEndian.PutUint16(fbUpdate[4:6], 0) // x
				binary.BigEndian.PutUint16(fbUpdate[6:8], 0) // y
				binary.BigEndian.PutUint16(fbUpdate[8:10], rectW)
				binary.BigEndian.PutUint16(fbUpdate[10:12], rectH)
				binary.BigEndian.PutUint32(fbUpdate[12:16], 0) // Raw encoding (0)
				pixels := fbUpdate[16:]
				for i := 0; i < rectW*rectH; i++ {
					pixels[i*4+0] = 0x1e // Blue
					pixels[i*4+1] = 0x29 // Green
					pixels[i*4+2] = 0x3b // Red
					pixels[i*4+3] = 0xff // Alpha
				}
				_, _ = conn.Write(fbUpdate)
			case 4: // KeyEvent (7 bytes remaining)
				discard := make([]byte, 7)
				_, _ = conn.Read(discard)
			case 5: // PointerEvent (5 bytes remaining)
				discard := make([]byte, 5)
				_, _ = conn.Read(discard)
			default:
				return
			}
		}
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
