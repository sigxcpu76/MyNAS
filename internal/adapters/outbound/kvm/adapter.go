package kvm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	logger    *slog.Logger
	stateDir  string
	vms       map[string]domain.VirtualMachine
	tickets   map[string]consoleTicket
	snapshots map[string][]domain.VirtualMachineSnapshot
	nextPort  int32
}

func NewAdapter() *Adapter {
	return NewAdapterWithStateDir("")
}

func NewAdapterWithStateDir(stateDir string) *Adapter {
	if stateDir == "" {
		if h := os.Getenv("HOME"); h != "" {
			stateDir = filepath.Join(h, ".local/share/nasctl/vms")
		} else {
			stateDir = "/tmp/nasctl/vms"
		}
	}
	_ = os.MkdirAll(stateDir, 0755)

	a := &Adapter{
		logger:    slog.Default().With("subsystem", "libvirt"),
		stateDir:  stateDir,
		vms:       make(map[string]domain.VirtualMachine),
		tickets:   make(map[string]consoleTicket),
		snapshots: make(map[string][]domain.VirtualMachineSnapshot),
		nextPort:  5903,
	}
	return a
}

func (a *Adapter) SetLogger(l *slog.Logger) {
	if l != nil {
		a.logger = l
	}
}

func (a *Adapter) logError(op string, vmID string, err error, output []byte) {
	if err == nil {
		return
	}
	l := a.logger
	if l == nil {
		l = slog.Default()
	}
	outStr := strings.TrimSpace(string(output))
	if outStr != "" {
		l.Error("libvirt operation failed", "subsystem", "libvirt", "operation", op, "vm_id", vmID, "error", err, "details", outStr)
	} else {
		l.Error("libvirt operation failed", "subsystem", "libvirt", "operation", op, "vm_id", vmID, "error", err)
	}
}

func (a *Adapter) hasVirsh() bool {
	_, err := exec.LookPath("virsh")
	return err == nil
}

func (a *Adapter) isRealDomain(ctx context.Context, id string) bool {
	if !a.hasVirsh() {
		return false
	}
	_, err := a.getDomainFromVirsh(ctx, id)
	return err == nil
}


// Libvirt XML structures for parsing dumpxml output
type domainXML struct {
	XMLName       xml.Name  `xml:"domain"`
	Type          string    `xml:"type,attr"`
	Name          string    `xml:"name"`
	UUID          string    `xml:"uuid"`
	Description   string    `xml:"description"`
	Memory        memoryXML `xml:"memory"`
	CurrentMemory memoryXML `xml:"currentMemory"`
	VCPU          struct {
		Value int32 `xml:",chardata"`
	} `xml:"vcpu"`
	CPU struct {
		Mode     string `xml:"mode,attr"`
		Topology struct {
			Sockets int32 `xml:"sockets,attr"`
			Cores   int32 `xml:"cores,attr"`
			Threads int32 `xml:"threads,attr"`
		} `xml:"topology"`
		Model string `xml:"model"`
	} `xml:"cpu"`
	OS struct {
		Firmware string `xml:"firmware,attr"`
		Type     struct {
			Arch    string `xml:"arch,attr"`
			Machine string `xml:"machine,attr"`
			Value   string `xml:",chardata"`
		} `xml:"type"`
		Loader struct {
			Value string `xml:",chardata"`
		} `xml:"loader"`
	} `xml:"os"`
	Devices struct {
		Disks []struct {
			Type   string `xml:"type,attr"`
			Device string `xml:"device,attr"`
			Driver struct {
				Name    string `xml:"name,attr"`
				Type    string `xml:"type,attr"`
				Cache   string `xml:"cache,attr"`
				Discard string `xml:"discard,attr"`
			} `xml:"driver"`
			Source struct {
				File string `xml:"file,attr"`
				Dev  string `xml:"dev,attr"`
			} `xml:"source"`
			Target struct {
				Dev string `xml:"dev,attr"`
				Bus string `xml:"bus,attr"`
			} `xml:"target"`
		} `xml:"disk"`
		Interfaces []struct {
			Type   string `xml:"type,attr"`
			Source struct {
				Bridge  string `xml:"bridge,attr"`
				Network string `xml:"network,attr"`
			} `xml:"source"`
			Mac struct {
				Address string `xml:"address,attr"`
			} `xml:"mac"`
			Model struct {
				Type string `xml:"type,attr"`
			} `xml:"model"`
			Vlan struct {
				Tag struct {
					ID int32 `xml:"id,attr"`
				} `xml:"tag"`
			} `xml:"vlan"`
		} `xml:"interface"`
		Graphics []struct {
			Type string `xml:"type,attr"`
			Port int32  `xml:"port,attr"`
		} `xml:"graphics"`
	} `xml:"devices"`
}

type memoryXML struct {
	Unit  string `xml:"unit,attr"`
	Value int64  `xml:",chardata"`
}

func (m memoryXML) ToMiB() int64 {
	switch strings.ToLower(m.Unit) {
	case "bytes", "b":
		return m.Value / (1024 * 1024)
	case "kib", "k":
		return m.Value / 1024
	case "mib", "m":
		return m.Value
	case "gib", "g":
		return m.Value * 1024
	default:
		return m.Value / 1024
	}
}

func ensureDiskAllocated(diskPath string, sizeBytes int64) error {
	if _, err := os.Stat(diskPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for disk %s: %w", diskPath, err)
	}
	if sizeBytes <= 0 {
		sizeBytes = 10 * 1024 * 1024 * 1024
	}
	if strings.HasSuffix(diskPath, ".qcow2") {
		if _, err := exec.LookPath("qemu-img"); err == nil {
			sizeG := sizeBytes / (1024 * 1024 * 1024)
			if sizeG <= 0 {
				sizeG = 1
			}
			cmd := exec.Command("qemu-img", "create", "-f", "qcow2", diskPath, fmt.Sprintf("%dG", sizeG))
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("qemu-img create failed: %s: %w", strings.TrimSpace(string(out)), err)
			}
			return nil
		}
	}
	f, err := os.Create(diskPath)
	if err != nil {
		return fmt.Errorf("failed to create disk file %s: %w", diskPath, err)
	}
	defer f.Close()
	if err := f.Truncate(sizeBytes); err != nil {
		return fmt.Errorf("failed to allocate sparse disk %s (%d bytes): %w", diskPath, sizeBytes, err)
	}
	return nil
}

func (a *Adapter) getDomainFromVirsh(ctx context.Context, name string) (*domain.VirtualMachine, error) {
	cmd := exec.CommandContext(ctx, "virsh", "dumpxml", name)
	xmlBytes, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: domain %q: %s", domain.ErrNotFound, name, strings.TrimSpace(string(xmlBytes)))
	}
	var doc domainXML
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse domain XML for %q: %w", name, err)
	}

	vm := domain.VirtualMachine{
		ID:               doc.Name,
		VMName:           doc.Name,
		UUID:             doc.UUID,
		Description:      doc.Description,
		MemoryMiB:        doc.Memory.ToMiB(),
		BalloonMinMiB:    doc.CurrentMemory.ToMiB(),
		VCPUs:            doc.VCPU.Value,
		Sockets:          doc.CPU.Topology.Sockets,
		Cores:            doc.CPU.Topology.Cores,
		CPUType:          doc.CPU.Model,
		MachineType:      doc.OS.Type.Machine,
		SerialConsoleURL: fmt.Sprintf("/ws/serial/%s", doc.Name),
	}
	if vm.Sockets <= 0 {
		vm.Sockets = 1
	}
	if vm.Cores <= 0 {
		vm.Cores = vm.VCPUs
	}
	if doc.OS.Firmware == "efi" || strings.Contains(doc.OS.Loader.Value, "OVMF") || strings.Contains(doc.OS.Loader.Value, "edk2") {
		vm.BiosType = "ovmf"
	} else {
		vm.BiosType = "seabios"
	}

	for _, d := range doc.Devices.Disks {
		path := d.Source.File
		if path == "" {
			path = d.Source.Dev
		}
		if d.Device == "cdrom" {
			vm.CdromIsoImage = path
			continue
		}
		disk := domain.VirtualMachineDisk{
			ID:        d.Target.Dev,
			Bus:       d.Target.Bus,
			DiskPath:  path,
			CacheMode: d.Driver.Cache,
			Discard:   d.Driver.Discard == "unmap",
		}
		if fi, err := os.Stat(path); err == nil {
			disk.SizeBytes = fi.Size()
		}
		vm.Disks = append(vm.Disks, disk)
	}

	for idx, iface := range doc.Devices.Interfaces {
		nic := domain.VirtualMachineNetworkInterface{
			ID:         fmt.Sprintf("net%d", idx),
			Model:      iface.Model.Type,
			Bridge:     iface.Source.Bridge,
			MacAddress: iface.Mac.Address,
			VlanTag:    iface.Vlan.Tag.ID,
			Firewall:   true,
		}
		vm.NetworkIfaces = append(vm.NetworkIfaces, nic)
	}

	stateOut, _ := exec.CommandContext(ctx, "virsh", "domstate", name).Output()
	st := strings.TrimSpace(string(stateOut))
	switch st {
	case "running":
		vm.State = "RUNNING"
	case "paused", "pmsuspended":
		vm.State = "PAUSED"
	default:
		vm.State = "STOPPED"
	}

	if vm.State == "RUNNING" {
		vncOut, _ := exec.CommandContext(ctx, "virsh", "vncdisplay", name).Output()
		vncStr := strings.TrimSpace(string(vncOut))
		if strings.Contains(vncStr, ":") {
			parts := strings.Split(vncStr, ":")
			var dispNum int
			fmt.Sscanf(parts[len(parts)-1], "%d", &dispNum)
			port := int32(5900 + dispNum)
			vm.VNCPort = port
			vm.VNCDisplay = fmt.Sprintf("127.0.0.1:%d (port %d)", dispNum, port)
		}
		a.enrichRuntimeTelemetry(ctx, &vm)
	}

	infoOut, _ := exec.CommandContext(ctx, "virsh", "dominfo", name).Output()
	if strings.Contains(string(infoOut), "Autostart:      enable") {
		vm.Autostart = true
	}

	a.mu.Lock()
	a.vms[name] = vm
	a.mu.Unlock()

	return &vm, nil
}

func (a *Adapter) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	if a.hasVirsh() {
		cmd := exec.CommandContext(ctx, "virsh", "list", "--all", "--name")
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			var vms []domain.VirtualMachine
			for _, line := range lines {
				name := strings.TrimSpace(line)
				if name == "" {
					continue
				}
				vm, err := a.getDomainFromVirsh(ctx, name)
				if err == nil && vm != nil {
					vms = append(vms, *vm)
				}
			}
			return vms, nil
		}
	}

	return nil, fmt.Errorf("virsh/libvirt is not available on this host")
}

func (a *Adapter) GetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if !a.hasVirsh() {
		return nil, fmt.Errorf("virsh/libvirt is not available on this host")
	}
	vm, err := a.getDomainFromVirsh(ctx, id)
	if err == nil {
		return vm, nil
	}
	return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
}

func (a *Adapter) enrichRuntimeTelemetry(ctx context.Context, vm *domain.VirtualMachine) {
	if vm.State != "RUNNING" {
		return
	}
	if a.hasVirsh() {
		tctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(tctx, "virsh", "qemu-agent-command", vm.ID, `{"execute":"guest-network-get-interfaces"}`)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			var resp struct {
				Return []struct {
					Name        string `json:"name"`
					IPAddresses []struct {
						IPType string `json:"ip-address-type"`
						IPAddr string `json:"ip-address"`
						Prefix int    `json:"prefix"`
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
	if err := vm.Validate(); err != nil {
		return nil, err
	}

	if a.hasVirsh() {
		if _, err := a.getDomainFromVirsh(ctx, vm.ID); err == nil {
			return nil, fmt.Errorf("%w: virtual machine %q already exists in libvirt", domain.ErrAlreadyExists, vm.ID)
		}

		if len(vm.Disks) == 0 {
			diskDir := filepath.Join(a.stateDir, "disks")
			_ = os.MkdirAll(diskDir, 0755)
			diskPath := filepath.Join(diskDir, fmt.Sprintf("%s.raw", domain.GenerateVMDiskName(vm.ID, 0)))
			sizeB := int64(vm.DiskSizeGiB) * 1024 * 1024 * 1024
			if sizeB <= 0 {
				sizeB = 32 * 1024 * 1024 * 1024
			}
			vm.Disks = []domain.VirtualMachineDisk{
				{
					ID:        domain.GenerateVMDiskName(vm.ID, 0),
					Bus:       "scsi",
					DiskPath:  diskPath,
					SizeBytes: sizeB,
					CacheMode: "none",
					Discard:   true,
					IOThread:  true,
				},
			}
		}

		for i := range vm.Disks {
			d := &vm.Disks[i]
			if d.ID == "" {
				d.ID = domain.GenerateVMDiskName(vm.ID, i)
			}
			if d.DiskPath == "" {
				diskDir := filepath.Join(a.stateDir, "disks")
				_ = os.MkdirAll(diskDir, 0755)
				d.DiskPath = filepath.Join(diskDir, fmt.Sprintf("%s.raw", domain.GenerateVMDiskName(vm.ID, i)))
			}
			if err := ensureDiskAllocated(d.DiskPath, d.SizeBytes); err != nil {
				return nil, fmt.Errorf("failed to allocate disk %s: %w", d.DiskPath, err)
			}
		}

		if len(vm.NetworkIfaces) == 0 {
			vm.NetworkIfaces = []domain.VirtualMachineNetworkInterface{
				{
					ID:         "net0",
					Model:      "virtio",
					Bridge:     "virbr0",
					MacAddress: generateMAC(),
					Firewall:   true,
				},
			}
		}

		xmlStr := buildDomainXML(vm)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xmlStr)
		out, err := cmd.CombinedOutput()
		if err != nil {
			a.logError("define", vm.ID, err, out)
			return nil, fmt.Errorf("libvirt define failed: %s: %w", strings.TrimSpace(string(out)), err)
		}

		if vm.Autostart {
			_ = exec.CommandContext(ctx, "virsh", "autostart", vm.ID).Run()
		}

		startCmd := exec.CommandContext(ctx, "virsh", "start", vm.ID)
		startOut, err := startCmd.CombinedOutput()
		if err != nil {
			a.logError("start", vm.ID, err, startOut)
			return nil, fmt.Errorf("libvirt start failed: %s: %w", strings.TrimSpace(string(startOut)), err)
		}

		created, err := a.getDomainFromVirsh(ctx, vm.ID)
		if err != nil {
			return nil, err
		}
		return created, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.vms[vm.ID]; exists {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrAlreadyExists, vm.ID)
	}

	a.nextPort++
	vm.VNCPort = a.nextPort
	vm.VNCDisplay = fmt.Sprintf("127.0.0.1:%d (port %d)", vm.VNCPort-5900, vm.VNCPort)
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}
	vm.State = "RUNNING"
	vm.SerialConsoleURL = fmt.Sprintf("/ws/serial/%s", vm.ID)

	if len(vm.Disks) == 0 {
		diskPath := vm.DiskPath
		if diskPath == "" {
			diskPath = fmt.Sprintf("/nas/tank/vms/%s.raw", domain.GenerateVMDiskName(vm.ID, 0))
		}
		sizeGiB := vm.DiskSizeGiB
		if sizeGiB <= 0 {
			sizeGiB = 32
		}
		vm.Disks = []domain.VirtualMachineDisk{
			{
				ID:        domain.GenerateVMDiskName(vm.ID, 0),
				Bus:       "scsi",
				DiskPath:  diskPath,
				SizeBytes: sizeGiB * 1024 * 1024 * 1024,
				CacheMode: "none",
				Discard:   true,
				IOThread:  true,
			},
		}
	}

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

	if a.hasVirsh() {
		xmlStr := buildDomainXML(existing)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xmlStr)
		_ = cmd.Run()
	}

	a.vms[vm.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteVM(ctx context.Context, id string) error {
	if a.isRealDomain(ctx, id) {
		_ = exec.CommandContext(ctx, "virsh", "destroy", id).Run()
		out, err := exec.CommandContext(ctx, "virsh", "undefine", "--nvram", "--snapshots-metadata", id).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "Domain not found") {
			a.logError("undefine", id, err, out)
			return fmt.Errorf("virsh undefine failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.vms, id)
	delete(a.snapshots, id)
	return nil
}

func (a *Adapter) StartVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		out, err := exec.CommandContext(ctx, "virsh", "start", id).CombinedOutput()
		if err != nil {
			a.logError("start", id, err, out)
			return nil, fmt.Errorf("virsh start failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		verb := "shutdown"
		if force {
			verb = "destroy"
		}
		out, err := exec.CommandContext(ctx, "virsh", verb, id).CombinedOutput()
		if err != nil {
			a.logError(verb, id, err, out)
			return nil, fmt.Errorf("virsh %s failed: %s: %w", verb, strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "STOPPED"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) RebootVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		out, err := exec.CommandContext(ctx, "virsh", "reboot", id).CombinedOutput()
		if err != nil {
			a.logError("reboot", id, err, out)
			return nil, fmt.Errorf("virsh reboot failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ResetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		out, err := exec.CommandContext(ctx, "virsh", "reset", id).CombinedOutput()
		if err != nil {
			a.logError("reset", id, err, out)
			return nil, fmt.Errorf("virsh reset failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) PauseVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		out, err := exec.CommandContext(ctx, "virsh", "suspend", id).CombinedOutput()
		if err != nil {
			a.logError("suspend", id, err, out)
			return nil, fmt.Errorf("virsh suspend failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "PAUSED"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ResumeVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		out, err := exec.CommandContext(ctx, "virsh", "resume", id).CombinedOutput()
		if err != nil {
			a.logError("resume", id, err, out)
			return nil, fmt.Errorf("virsh resume failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) CloneVM(ctx context.Context, id, newID, newName, targetPool string, linkedClone bool) (*domain.VirtualMachine, error) {
	source, err := a.GetVM(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: source virtual machine %q", domain.ErrNotFound, id)
	}

	targetName := newName
	if targetName == "" {
		targetName = newID
	}

	cloned := *source
	cloned.ID = newID
	cloned.VMName = targetName
	cloned.UUID = generateUUID()
	cloned.State = "STOPPED"
	cloned.SerialConsoleURL = fmt.Sprintf("/ws/serial/%s", newID)

	clonedDisks := make([]domain.VirtualMachineDisk, 0, len(source.Disks))
	for i, d := range source.Disks {
		cDisk := d
		cDisk.ID = domain.GenerateVMDiskName(newID, i)
		diskDir := filepath.Join(a.stateDir, "disks")
		_ = os.MkdirAll(diskDir, 0755)
		cDisk.DiskPath = filepath.Join(diskDir, fmt.Sprintf("%s.raw", domain.GenerateVMDiskName(newID, i)))
		_ = ensureDiskAllocated(cDisk.DiskPath, cDisk.SizeBytes)
		clonedDisks = append(clonedDisks, cDisk)
	}
	cloned.Disks = clonedDisks

	clonedNics := make([]domain.VirtualMachineNetworkInterface, 0, len(source.NetworkIfaces))
	for i, nic := range source.NetworkIfaces {
		cNic := nic
		cNic.ID = fmt.Sprintf("net%d", i)
		cNic.MacAddress = generateMAC()
		clonedNics = append(clonedNics, cNic)
	}
	cloned.NetworkIfaces = clonedNics

	if a.isRealDomain(ctx, id) {
		xmlStr := buildDomainXML(cloned)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xmlStr)
		if out, err := cmd.CombinedOutput(); err != nil {
			a.logError("clone_define", newID, err, out)
			return nil, fmt.Errorf("virsh define clone failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, newID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.vms[newID] = cloned
	cp := cloned
	return &cp, nil
}

func (a *Adapter) AttachMedia(ctx context.Context, id, deviceTarget, isoPath string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		dev := deviceTarget
		if dev == "" {
			dev = "sda"
		}
		out, err := exec.CommandContext(ctx, "virsh", "change-media", id, dev, isoPath, "--insert").CombinedOutput()
		if err != nil {
			a.logError("change_media_insert", id, err, out)
			return nil, fmt.Errorf("virsh change-media failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.CdromIsoImage = isoPath
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) EjectMedia(ctx context.Context, id, deviceTarget string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		dev := deviceTarget
		if dev == "" {
			dev = "sda"
		}
		out, err := exec.CommandContext(ctx, "virsh", "change-media", id, dev, "--eject").CombinedOutput()
		if err != nil {
			a.logError("change_media_eject", id, err, out)
			return nil, fmt.Errorf("virsh change-media eject failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, id)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	vm.CdromIsoImage = ""
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) ListSnapshots(ctx context.Context, vmID string) ([]domain.VirtualMachineSnapshot, error) {
	if a.isRealDomain(ctx, vmID) {
		out, err := exec.CommandContext(ctx, "virsh", "snapshot-list", vmID, "--name").Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			var list []domain.VirtualMachineSnapshot
			for _, line := range lines {
				sName := strings.TrimSpace(line)
				if sName == "" {
					continue
				}
				list = append(list, domain.VirtualMachineSnapshot{
					Name:        fmt.Sprintf("%s/snapshots/%s", vmID, sName),
					SnapshotID:  sName,
					Description: sName,
					CreateTime:  time.Now().UTC(),
				})
			}
			return list, nil
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	snaps, ok := a.snapshots[vmID]
	if !ok {
		return []domain.VirtualMachineSnapshot{}, nil
	}
	out := make([]domain.VirtualMachineSnapshot, len(snaps))
	copy(out, snaps)
	return out, nil
}

func (a *Adapter) CreateSnapshot(ctx context.Context, vmID string, snapshot domain.VirtualMachineSnapshot) (*domain.VirtualMachineSnapshot, error) {
	if snapshot.SnapshotID == "" {
		snapshot.SnapshotID = domain.GenerateNanosecondID()
	}

	if a.isRealDomain(ctx, vmID) {
		args := []string{"snapshot-create-as", vmID, snapshot.SnapshotID, snapshot.Description}
		if !snapshot.IncludeRAM {
			args = append(args, "--disk-only")
		}
		out, err := exec.CommandContext(ctx, "virsh", args...).CombinedOutput()
		if err != nil {
			a.logError("snapshot_create", vmID, err, out)
			return nil, fmt.Errorf("virsh snapshot-create-as failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		snap := domain.VirtualMachineSnapshot{
			Name:        fmt.Sprintf("%s/snapshots/%s", vmID, snapshot.SnapshotID),
			SnapshotID:  snapshot.SnapshotID,
			Description: snapshot.Description,
			IncludeRAM:  snapshot.IncludeRAM,
			CreateTime:  time.Now().UTC(),
		}
		a.mu.Lock()
		a.snapshots[vmID] = append(a.snapshots[vmID], snap)
		a.mu.Unlock()
		return &snap, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	snap := domain.VirtualMachineSnapshot{
		Name:        fmt.Sprintf("%s/snapshots/%s", vmID, snapshot.SnapshotID),
		SnapshotID:  snapshot.SnapshotID,
		Description: snapshot.Description,
		IncludeRAM:  snapshot.IncludeRAM,
		CreateTime:  time.Now().UTC(),
	}
	a.snapshots[vmID] = append(a.snapshots[vmID], snap)
	return &snap, nil
}

func (a *Adapter) RollbackSnapshot(ctx context.Context, vmID, snapID string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, vmID) {
		out, err := exec.CommandContext(ctx, "virsh", "snapshot-revert", vmID, snapID).CombinedOutput()
		if err != nil {
			a.logError("snapshot_revert", vmID, err, out)
			return nil, fmt.Errorf("virsh snapshot-revert failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return a.getDomainFromVirsh(ctx, vmID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[vmID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	cp := vm
	return &cp, nil
}

func (a *Adapter) DeleteSnapshot(ctx context.Context, vmID, snapID string) error {
	if a.isRealDomain(ctx, vmID) {
		out, err := exec.CommandContext(ctx, "virsh", "snapshot-delete", vmID, snapID).CombinedOutput()
		if err != nil {
			a.logError("snapshot_delete", vmID, err, out)
			return fmt.Errorf("virsh snapshot-delete failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	snaps := a.snapshots[vmID]
	idx := -1
	for i, s := range snaps {
		if s.SnapshotID == snapID || s.Name == snapID {
			idx = i
			break
		}
	}
	if idx >= 0 {
		a.snapshots[vmID] = append(snaps[:idx], snaps[idx+1:]...)
	}
	return nil
}

func buildDomainXML(vm domain.VirtualMachine) string {
	var sb strings.Builder
	domainType := "kvm"
	if _, err := os.Stat("/dev/kvm"); err != nil {
		domainType = "qemu"
	}

	sb.WriteString(fmt.Sprintf(`<domain type='%s' xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>
  <name>%s</name>
`, domainType, vm.ID))
	if vm.UUID != "" {
		sb.WriteString(fmt.Sprintf(`  <uuid>%s</uuid>
`, vm.UUID))
	}
	if vm.Description != "" {
		sb.WriteString(fmt.Sprintf(`  <description>%s</description>
`, vm.Description))
	}
	sb.WriteString(fmt.Sprintf(`  <memory unit='MiB'>%d</memory>
`, vm.MemoryMiB))

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

	mach := vm.MachineType
	if mach == "" {
		mach = "q35"
	}
	sb.WriteString("  <os")
	if vm.BiosType == "ovmf" {
		sb.WriteString(" firmware='efi'")
	}
	sb.WriteString(">\n")
	sb.WriteString(fmt.Sprintf(`    <type arch='x86_64' machine='%s'>hvm</type>
    <boot dev='hd'/>
`, mach))
	if vm.CdromIsoImage != "" {
		sb.WriteString(`    <boot dev='cdrom'/>
`)
	}
	sb.WriteString(`  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <devices>
    <emulator>/usr/sbin/qemu-system-x86_64</emulator>
    <controller type='scsi' model='virtio-scsi'/>
`)

	diskLetter := 'a'
	for _, d := range vm.Disks {
		if d.DiskPath == "" {
			continue
		}
		diskKind := "file"
		sourceAttr := "file"
		driverType := "raw"
		if strings.HasPrefix(d.DiskPath, "/dev/") {
			diskKind = "block"
			sourceAttr = "dev"
		}
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
		targetPrefix := "vd"
		if bus == "scsi" || bus == "sata" {
			targetPrefix = "sd"
		} else if bus == "ide" {
			targetPrefix = "hd"
		}

		sb.WriteString(fmt.Sprintf(`    <disk type='%s' device='disk'>
      <driver name='qemu' type='%s' cache='%s' discard='%s'/>
      <source %s='%s'/>
      <target dev='%s%c' bus='%s'/>
    </disk>
`, diskKind, driverType, cacheMode, discardMode, sourceAttr, d.DiskPath, targetPrefix, diskLetter, bus))
		diskLetter++
	}

	if vm.CdromIsoImage != "" {
		sb.WriteString(fmt.Sprintf(`    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='%s'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
`, vm.CdromIsoImage))
	}

	for _, iface := range vm.NetworkIfaces {
		bridge := iface.Bridge
		model := iface.Model
		if model == "" {
			model = "virtio"
		}
		hasBridge := false
		if bridge != "" {
			if _, err := net.InterfaceByName(bridge); err == nil {
				hasBridge = true
			}
		}
		if hasBridge {
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
		} else {
			sb.WriteString(fmt.Sprintf(`    <interface type='user'>
      <model type='%s'/>
`, model))
			if iface.MacAddress != "" {
				sb.WriteString(fmt.Sprintf(`      <mac address='%s'/>
`, iface.MacAddress))
			}
			sb.WriteString(`    </interface>
`)
		}
	}

	if vm.TPMVersion == "v2.0" {
		if _, err := exec.LookPath("swtpm"); err == nil {
			sb.WriteString(`    <tpm model='tpm-tis'>
      <backend type='emulator' version='2.0'/>
    </tpm>
`)
		}
	}

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

	sb.WriteString(`    <channel type='unix'>
      <target type='virtio' name='org.qemu.guest_agent.0'/>
    </channel>
    <memballoon model='virtio'/>
    <graphics type='vnc' port='-1' autoport='yes' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>
    <video>
      <model type='vga'/>
    </video>
    <serial type='pty'>
      <target type='isa-serial' port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
  </devices>
`)

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
	vm, err := a.GetVM(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if vm.State != "RUNNING" {
		return nil, fmt.Errorf("%w: virtual machine %q must be RUNNING to open noVNC console", domain.ErrFailedPrecondition, id)
	}

	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	exp := time.Now().UTC().Add(30 * time.Minute)

	a.mu.Lock()
	a.tickets[token] = consoleTicket{
		VMID:      id,
		Token:     token,
		ExpiresAt: exp,
	}
	a.mu.Unlock()

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
	return nil, fmt.Errorf("%w: invalid or expired console token", domain.ErrUnauthorized)
}

func (a *Adapter) DialVNC(ctx context.Context, vmID string) (net.Conn, error) {
	if a.isRealDomain(ctx, vmID) {
		vncOut, err := exec.CommandContext(ctx, "virsh", "vncdisplay", vmID).CombinedOutput()
		if err != nil {
			a.logError("vnc_lookup", vmID, err, vncOut)
			return nil, fmt.Errorf("domain %q is not running: %s", vmID, strings.TrimSpace(string(vncOut)))
		}
		vncStr := strings.TrimSpace(string(vncOut))
		if !strings.Contains(vncStr, ":") {
			err := fmt.Errorf("no VNC display allocated for domain %q (got %q)", vmID, vncStr)
			a.logError("vnc_display_missing", vmID, err, []byte(vncStr))
			return nil, err
		}
		parts := strings.Split(vncStr, ":")
		var dispNum int
		fmt.Sscanf(parts[len(parts)-1], "%d", &dispNum)
		port := 5900 + dispNum

		dialer := net.Dialer{Timeout: 2 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			a.logError("vnc_dial", vmID, err, nil)
			return nil, fmt.Errorf("failed to connect to QEMU VNC server on port %d: %w", port, err)
		}
		return conn, nil
	}

	return nil, fmt.Errorf("domain %q is not running under virsh", vmID)
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
