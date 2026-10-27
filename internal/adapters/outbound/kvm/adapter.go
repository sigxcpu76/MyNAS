package kvm

import (
	"bufio"
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

	"github.com/sigxcpu/nas/internal/config"
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
	mainPool  string
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

	go func() {
		_ = EnsureHostPermissions()
		_ = EnsureHostBridging()
	}()

	return a
}

// EnsureHostPermissions ensures the libvirt/qemu user has group access to block devices (disk/kvm)
// and that AppArmor profiles allow access to ZFS zvols (/dev/zvol/**, /dev/zd*) and NAS storage (/mnt/**, /nas/**).
func EnsureHostPermissions() error {
	// 1. Add libvirt-qemu / qemu user to disk and kvm groups
	for _, user := range []string{"libvirt-qemu", "qemu"} {
		if _, err := exec.Command("id", user).Output(); err == nil {
			_ = exec.Command("usermod", "-a", "-G", "disk", user).Run()
			_ = exec.Command("usermod", "-a", "-G", "kvm", user).Run()
		}
	}

	// 2. Configure AppArmor overrides if AppArmor is installed and active
	if _, err := exec.LookPath("apparmor_parser"); err == nil {
		aaDirs := []string{
			"/etc/apparmor.d/local/abstractions",
			"/etc/apparmor.d/abstractions/libvirt-qemu.d",
			"/etc/apparmor.d/local",
		}
		for _, d := range aaDirs {
			_ = os.MkdirAll(d, 0755)
		}

		ruleBlock := "# Allow QEMU access to ZFS zvols and NAS storage\n/dev/zvol/** rwk,\n/dev/zd* rwk,\n/mnt/** rwk,\n/nas/** rwk,\n"
		helperRule := "# Allow virt-aa-helper access to ZFS zvols and NAS storage\n/dev/zvol/** rw,\n/dev/zd* rw,\n/mnt/** rw,\n/nas/** rw,\n"

		_ = os.WriteFile("/etc/apparmor.d/local/abstractions/libvirt-qemu", []byte(ruleBlock), 0644)
		_ = os.WriteFile("/etc/apparmor.d/abstractions/libvirt-qemu.d/mynas", []byte(ruleBlock), 0644)
		_ = os.WriteFile("/etc/apparmor.d/local/usr.lib.libvirt.virt-aa-helper", []byte(helperRule), 0644)

		// On modern kernels (Linux 6.8+ / Debian 13 / Proxmox PVE), the legacy host libvirtd daemon AppArmor profile
		// breaks UNIX domain socket creation due to fine-grained network unix mediation mismatch.
		// Confining the libvirtd daemon itself is deprecated and unnecessary because guest VMs are fully confined
		// by virt-aa-helper and libvirt-qemu profiles. Disable the host daemon profile to allow libvirtd sockets to function.
		_ = os.MkdirAll("/etc/apparmor.d/disable", 0755)
		if _, err := os.Stat("/etc/apparmor.d/usr.sbin.libvirtd"); err == nil {
			_ = os.Symlink("/etc/apparmor.d/usr.sbin.libvirtd", "/etc/apparmor.d/disable/usr.sbin.libvirtd")
			_ = exec.Command("apparmor_parser", "-R", "/etc/apparmor.d/usr.sbin.libvirtd").Run()
		}
		_ = exec.Command("apparmor_parser", "-r", "/etc/apparmor.d/usr.lib.libvirt.virt-aa-helper").Run()
	}

	// 3. Ensure libvirtd service and sockets are active
	if _, err := exec.LookPath("systemctl"); err == nil {
		if _, err := exec.LookPath("libvirtd"); err == nil {
			_ = exec.Command("systemctl", "reset-failed", "libvirtd", "libvirtd.socket", "libvirtd-ro.socket", "libvirtd-admin.socket").Run()
			_ = exec.Command("systemctl", "start", "libvirtd.socket", "libvirtd.service").Run()
		}
	}

	// 4. Ensure permissions on /mnt and /nas directory structure
	for _, base := range []string{"/mnt", "/nas"} {
		_ = os.Chmod(base, 0755)
		if entries, err := os.ReadDir(base); err == nil {
			for _, e := range entries {
				poolPath := filepath.Join(base, e.Name())
				_ = os.Chmod(poolPath, 0755)
				_ = os.Chmod(filepath.Join(poolPath, "isos"), 0755)
				_ = os.Chmod(filepath.Join(poolPath, "vms"), 0755)
			}
		}
	}

	return nil
}

func (a *Adapter) SetLogger(l *slog.Logger) {
	if l != nil {
		a.logger = l
	}
}

// SetMainPool sets the main ZFS storage pool used for rooting VM storage (/vms/<vm_id>).
func (a *Adapter) SetMainPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mainPool = strings.TrimPrefix(strings.TrimSpace(pool), "zfsPools/")
}

func (a *Adapter) getMainPool() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.mainPool != "" {
		return a.mainPool
	}
	p := config.GetGlobal().System.MainPool
	if p != "" {
		return strings.TrimPrefix(strings.TrimSpace(p), "zfsPools/")
	}
	return "tank"
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
				Dev     string `xml:"dev,attr"`
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
	if strings.HasPrefix(diskPath, "/dev/zvol/") {
		// Clean up any stale regular file mistakenly created in /dev/zvol
		if st, err := os.Lstat(diskPath); err == nil && (st.Mode()&os.ModeDevice == 0) && (st.Mode()&os.ModeSymlink == 0) {
			_ = os.Remove(diskPath)
		}

		// Check if the ZFS volume block device already exists
		if st, err := os.Stat(diskPath); err == nil && (st.Mode()&os.ModeDevice != 0) {
			return nil
		}

		// Create real ZFS volume if zfs command is available
		if _, err := exec.LookPath("zfs"); err == nil {
			vol := strings.TrimPrefix(diskPath, "/dev/zvol/")
			sizeG := sizeBytes / (1024 * 1024 * 1024)
			if sizeG <= 0 {
				sizeG = 32
			}
			cmd := exec.Command("zfs", "create", "-p", "-s", "-V", fmt.Sprintf("%dG", sizeG), vol)
			if out, err := cmd.CombinedOutput(); err != nil {
				if !strings.Contains(string(out), "dataset already exists") {
					return fmt.Errorf("zfs create -V failed for %s: %s: %w", vol, strings.TrimSpace(string(out)), err)
				}
			}
			_ = exec.Command("udevadm", "settle").Run()
			for attempt := 0; attempt < 30; attempt++ {
				if st, err := os.Stat(diskPath); err == nil && (st.Mode()&os.ModeDevice != 0) {
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			if st, err := os.Stat(diskPath); err == nil && (st.Mode()&os.ModeDevice != 0) {
				return nil
			}
			return fmt.Errorf("zfs volume %s was created but block device node %s did not appear", vol, diskPath)
		}

		if st, err := os.Stat(diskPath); err == nil && (st.Mode()&os.ModeDevice != 0) {
			return nil
		}
		return fmt.Errorf("block device %s does not exist on host", diskPath)
	}

	if strings.HasPrefix(diskPath, "/dev/") {
		st, err := os.Stat(diskPath)
		if err == nil && (st.Mode()&os.ModeDevice != 0) {
			return nil
		}
		return fmt.Errorf("block device %s does not exist on host", diskPath)
	}

	if _, err := os.Stat(diskPath); err == nil {
		return nil
	}
	dir := filepath.Dir(diskPath)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return fmt.Errorf("failed to create directory for disk %s: %w", diskPath, err)
	}
	_ = os.Chmod(dir, 0777)
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
			_ = os.Chmod(diskPath, 0666)
			return nil
		}
	}
	f, err := os.OpenFile(diskPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("failed to create disk file %s: %w", diskPath, err)
	}
	defer f.Close()
	_ = os.Chmod(diskPath, 0666)
	if err := f.Truncate(sizeBytes); err != nil {
		return fmt.Errorf("failed to allocate sparse disk %s (%d bytes): %w", diskPath, sizeBytes, err)
	}
	return nil
}

func (a *Adapter) sanitizeAndEnsureDisks(vm *domain.VirtualMachine) bool {
	changed := false
	mainPool := a.getMainPool()

	if len(vm.Disks) == 0 {
		diskPath := domain.GenerateZvolDiskPath(mainPool, vm.ID, 0)
		sizeB := int64(vm.DiskSizeGiB) * 1024 * 1024 * 1024
		if sizeB <= 0 {
			sizeB = 32 * 1024 * 1024 * 1024
		}
		_ = ensureDiskAllocated(diskPath, sizeB)
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
		return true
	}

	for i := range vm.Disks {
		d := &vm.Disks[i]
		if d.ID == "" {
			d.ID = domain.GenerateVMDiskName(vm.ID, i)
			changed = true
		}
		if d.DiskPath == "" {
			d.DiskPath = domain.GenerateZvolDiskPath(mainPool, vm.ID, i)
			changed = true
		}
		if err := ensureDiskAllocated(d.DiskPath, d.SizeBytes); err == nil {
			if fi, sErr := os.Stat(d.DiskPath); sErr == nil && d.SizeBytes <= 0 {
				d.SizeBytes = fi.Size()
			}
		}
	}
	return changed
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

	cpuType := doc.CPU.Model
	if doc.CPU.Mode == "host-passthrough" || cpuType == "qemu64" || cpuType == "" {
		cpuType = "host"
	}

	machType := doc.OS.Type.Machine
	if machType == "pc" || strings.HasPrefix(machType, "pc-i440fx") || machType == "i440fx" {
		machType = "i440fx"
	} else if machType == "q35" || strings.HasPrefix(machType, "pc-q35") {
		machType = "q35"
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
		CPUType:          cpuType,
		MachineType:      machType,
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
		brName := iface.Source.Bridge
		if brName == "" {
			brName = iface.Source.Dev
		}
		nic := domain.VirtualMachineNetworkInterface{
			ID:         fmt.Sprintf("net%d", idx),
			Model:      iface.Model.Type,
			Bridge:     brName,
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

	if hasAccel, desc := domain.HasKVMHardwareAcceleration(); !hasAccel {
		return nil, fmt.Errorf("KVM hardware acceleration is required: %s", desc)
	}

	if a.hasVirsh() {
		if _, err := a.getDomainFromVirsh(ctx, vm.ID); err == nil {
			return nil, fmt.Errorf("%w: virtual machine %q already exists in libvirt", domain.ErrAlreadyExists, vm.ID)
		}

		a.sanitizeAndEnsureDisks(&vm)

		if len(vm.NetworkIfaces) == 0 {
			_, defDev := resolveHostNetworkTarget("")
			if defDev == "" {
				defDev = "br0"
			}
			vm.NetworkIfaces = []domain.VirtualMachineNetworkInterface{
				{
					ID:         "net0",
					Model:      "virtio",
					Bridge:     defDev,
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
			// Auto-heal if start failed (CPU mode unsupported, non-existent disk zvol bind error, macvtap unprivileged error, etc.)
			if createdVM, getErr := a.getDomainFromVirsh(ctx, vm.ID); getErr == nil {
				createdVM.CPUType = "host"
				if strings.Contains(string(startOut), "macvtap") || strings.Contains(string(startOut), "Operation not permitted") || strings.Contains(string(startOut), "bridge helper") {
					for i := range createdVM.NetworkIfaces {
						createdVM.NetworkIfaces[i].Bridge = "user"
					}
				}
				a.sanitizeAndEnsureDisks(createdVM)
				redefStr := buildDomainXML(*createdVM)
				redefCmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
				redefCmd.Stdin = strings.NewReader(redefStr)
				if _, redefErr := redefCmd.CombinedOutput(); redefErr == nil {
					if retryOut, retryErr := exec.CommandContext(ctx, "virsh", "start", vm.ID).CombinedOutput(); retryErr == nil {
						return a.getDomainFromVirsh(ctx, vm.ID)
					} else {
						startOut = retryOut
						err = retryErr
					}
				}
			}
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

	a.sanitizeAndEnsureDisks(&vm)

	if len(vm.NetworkIfaces) == 0 {
		bridge := vm.NetworkBridge
		if bridge == "" || bridge == "virbr0" {
			_, defDev := resolveHostNetworkTarget("")
			if defDev != "" {
				bridge = defDev
			} else {
				bridge = "br0"
			}
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
	var existing domain.VirtualMachine
	var ok bool

	if a.hasVirsh() {
		if dom, err := a.getDomainFromVirsh(ctx, vm.ID); err == nil && dom != nil {
			existing = *dom
			ok = true
		}
	}

	if !ok {
		a.mu.Lock()
		existing, ok = a.vms[vm.ID]
		a.mu.Unlock()
		if !ok {
			return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vm.ID)
		}
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
		existing.BalloonMinMiB = vm.MemoryMiB
	}
	if vm.Hugepages != "" {
		existing.Hugepages = vm.Hugepages
	}
	existing.NUMA = vm.NUMA
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
	if vm.DisplayType != "" {
		existing.DisplayType = vm.DisplayType
	}
	hasDisksPath := false
	hasNicsPath := false
	for _, p := range fields {
		if p == "disks" {
			hasDisksPath = true
		}
		if p == "network_interfaces" || p == "networkInterfaces" {
			hasNicsPath = true
		}
	}

	if hasDisksPath {
		existing.Disks = vm.Disks
	} else if len(vm.Disks) > 0 {
		existing.Disks = vm.Disks
	}
	if vm.CdromIsoImage != "" {
		existing.CdromIsoImage = vm.CdromIsoImage
	}
	if hasNicsPath {
		existing.NetworkIfaces = vm.NetworkIfaces
	} else if len(vm.NetworkIfaces) > 0 {
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
		a.sanitizeAndEnsureDisks(&existing)
		xmlStr := buildDomainXML(existing)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xmlStr)
		if out, err := cmd.CombinedOutput(); err != nil {
			a.logError("define", vm.ID, err, out)
			return nil, fmt.Errorf("failed to update VM definition in libvirt: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	a.mu.Lock()
	a.vms[vm.ID] = existing
	a.mu.Unlock()
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
			// Auto-heal if start failed (CPU mode unsupported, non-existent disk zvol bind error, macvtap unprivileged error, etc.)
			if vm, getErr := a.getDomainFromVirsh(ctx, id); getErr == nil {
				vm.CPUType = "host"
				if strings.Contains(string(out), "macvtap") || strings.Contains(string(out), "Operation not permitted") || strings.Contains(string(out), "bridge helper") {
					for i := range vm.NetworkIfaces {
						vm.NetworkIfaces[i].Bridge = "user"
					}
				}
				a.sanitizeAndEnsureDisks(vm)
				xmlStr := buildDomainXML(*vm)
				redefCmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
				redefCmd.Stdin = strings.NewReader(xmlStr)
				if _, redefErr := redefCmd.CombinedOutput(); redefErr == nil {
					if retryOut, retryErr := exec.CommandContext(ctx, "virsh", "start", id).CombinedOutput(); retryErr == nil {
						return a.getDomainFromVirsh(ctx, id)
					} else {
						out = retryOut
						err = retryErr
					}
				}
			}
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

func (a *Adapter) RebootVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		verb := "reboot"
		if force {
			verb = "reset"
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

	if targetPool == "" {
		targetPool = a.getMainPool()
	}
	clonedDisks := make([]domain.VirtualMachineDisk, 0, len(source.Disks))
	for i, d := range source.Disks {
		cDisk := d
		cDisk.ID = domain.GenerateVMDiskName(newID, i)
		cDisk.DiskPath = domain.GenerateVMDiskFilePath(targetPool, newID, i)
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

func (a *Adapter) findCdromTarget(ctx context.Context, id string) string {
	out, err := exec.CommandContext(ctx, "virsh", "domblklist", id, "--details").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 3 && fields[1] == "cdrom" {
			return fields[2]
		}
	}
	return ""
}

func (a *Adapter) updateDomainCdromBoot(ctx context.Context, id, isoPath string) {
	if vm, err := a.getDomainFromVirsh(ctx, id); err == nil && vm != nil {
		vm.CdromIsoImage = isoPath
		xmlStr := buildDomainXML(*vm)
		cmd := exec.CommandContext(ctx, "virsh", "define", "/dev/stdin")
		cmd.Stdin = strings.NewReader(xmlStr)
		_ = cmd.Run()
	}
}

func (a *Adapter) AttachMedia(ctx context.Context, id, deviceTarget, isoPath string) (*domain.VirtualMachine, error) {
	if a.isRealDomain(ctx, id) {
		dev := deviceTarget
		actual := a.findCdromTarget(ctx, id)
		if actual != "" && (dev == "" || dev == "hdc") {
			dev = actual
		}
		if dev == "" {
			dev = "sdz"
		}
		out, err := exec.CommandContext(ctx, "virsh", "change-media", id, dev, isoPath, "--insert").CombinedOutput()
		if err != nil {
			// If tray is already occupied, try --update
			if _, errUp := exec.CommandContext(ctx, "virsh", "change-media", id, dev, isoPath, "--update").CombinedOutput(); errUp == nil {
				a.updateDomainCdromBoot(ctx, id, isoPath)
				return a.getDomainFromVirsh(ctx, id)
			}
			// If device was hdc/wrong and actual exists, try actual
			if actual != "" && actual != dev {
				if _, errAct := exec.CommandContext(ctx, "virsh", "change-media", id, actual, isoPath, "--insert").CombinedOutput(); errAct == nil {
					a.updateDomainCdromBoot(ctx, id, isoPath)
					return a.getDomainFromVirsh(ctx, id)
				}
				if _, errActUp := exec.CommandContext(ctx, "virsh", "change-media", id, actual, isoPath, "--update").CombinedOutput(); errActUp == nil {
					a.updateDomainCdromBoot(ctx, id, isoPath)
					return a.getDomainFromVirsh(ctx, id)
				}
			}
			// If device doesn't exist, attach as new cdrom device
			if _, errAtt := exec.CommandContext(ctx, "virsh", "attach-disk", id, isoPath, dev, "--type", "cdrom", "--mode", "readonly", "--config").CombinedOutput(); errAtt == nil {
				a.updateDomainCdromBoot(ctx, id, isoPath)
				return a.getDomainFromVirsh(ctx, id)
			}
			a.logError("change_media_insert", id, err, out)
			return nil, fmt.Errorf("virsh change-media failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		a.updateDomainCdromBoot(ctx, id, isoPath)
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
		actual := a.findCdromTarget(ctx, id)
		if actual != "" && (dev == "" || dev == "hdc") {
			dev = actual
		}
		if dev == "" {
			dev = "sdz"
		}
		out, err := exec.CommandContext(ctx, "virsh", "change-media", id, dev, "--eject").CombinedOutput()
		if err != nil && actual != "" && actual != dev {
			if _, errAct := exec.CommandContext(ctx, "virsh", "change-media", id, actual, "--eject").CombinedOutput(); errAct == nil {
				a.updateDomainCdromBoot(ctx, id, "")
				return a.getDomainFromVirsh(ctx, id)
			}
		}
		if err != nil {
			a.logError("change_media_eject", id, err, out)
			return nil, fmt.Errorf("virsh change-media eject failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		a.updateDomainCdromBoot(ctx, id, "")
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

func findQemuEmulator() string {
	candidates := []string{
		"qemu-system-x86_64",
		"qemu-kvm",
		"/usr/bin/qemu-system-x86_64",
		"/bin/qemu-system-x86_64",
		"/usr/libexec/qemu-kvm",
		"/usr/bin/qemu-kvm",
		"/usr/sbin/qemu-system-x86_64",
	}
	for _, c := range candidates {
		if strings.HasPrefix(c, "/") {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		} else {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}
	return ""
}

// resolveHostNetworkTarget determines the libvirt interface type ('bridge', 'direct', or 'user')
// and target device name for a given network bridge/device specification.
// If target is an existing Linux bridge (e.g. br0, vmbr0), it uses type 'bridge'.
// If target is enslaved to a Linux bridge (e.g. ens3 in br0), it uses type 'bridge' with the master bridge.
// If target is an ethernet/physical device (e.g. ens3, eth0), it uses type 'direct' with mode='bridge' (macvtap).
// If target is empty or not found (e.g. legacy default 'virbr0'), it automatically discovers
// the host's bridge (br0) or default route LAN interface.
func resolveHostNetworkTarget(target string) (netType string, devName string) {
	target = strings.TrimSpace(target)

	// Unprivileged non-root users (such as local test runs without CAP_NET_ADMIN) cannot create macvtap or bridge devices
	if os.Geteuid() != 0 {
		return "user", ""
	}

	// 1. If target is explicitly specified
	if target != "" {
		if isLinuxBridge(target) {
			return "bridge", target
		}
		// If target is enslaved to a Linux bridge (e.g. ens3 is enslaved to br0)
		if masterLink, err := os.Readlink(filepath.Join("/sys/class/net", target, "master")); err == nil {
			masterName := filepath.Base(masterLink)
			if isLinuxBridge(masterName) {
				return "bridge", masterName
			}
		}
		// If target is a real non-loopback network interface (e.g. ens3, eth0)
		if ifi, err := net.InterfaceByName(target); err == nil && (ifi.Flags&net.FlagLoopback == 0) {
			return "direct", target
		}
	}

	// 2. Look for dedicated standard VM bridges (e.g. br0, vmbr0, virbr0)
	for _, brCandidate := range []string{"br0", "vmbr0", "virbr0"} {
		if isLinuxBridge(brCandidate) {
			return "bridge", brCandidate
		}
	}

	// 3. Fall back to host's primary default route interface (e.g. br0 or ens3 on physical LAN)
	if defIface := getDefaultRouteInterface(); defIface != "" {
		if isLinuxBridge(defIface) {
			return "bridge", defIface
		}
		if masterLink, err := os.Readlink(filepath.Join("/sys/class/net", defIface, "master")); err == nil {
			masterName := filepath.Base(masterLink)
			if isLinuxBridge(masterName) {
				return "bridge", masterName
			}
		}
		if ifi, err := net.InterfaceByName(defIface); err == nil && (ifi.Flags&net.FlagLoopback == 0) {
			return "direct", defIface
		}
	}

	// 4. Any other non-docker, non-veth bridge
	if entries, err := os.ReadDir("/sys/class/net"); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "lxc") {
				continue
			}
			if isLinuxBridge(name) {
				return "bridge", name
			}
		}
	}

	// 5. Any active physical/ethernet non-loopback, non-virtual interface
	if ifaces, err := net.Interfaces(); err == nil {
		for _, ifi := range ifaces {
			if ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagUp == 0 {
				continue
			}
			if strings.HasPrefix(ifi.Name, "veth") || strings.HasPrefix(ifi.Name, "docker") || strings.HasPrefix(ifi.Name, "br-") || strings.HasPrefix(ifi.Name, "lxc") {
				continue
			}
			return "direct", ifi.Name
		}
	}

	return "user", ""
}

// EnsureHostBridging ensures that if a Linux bridge (such as br0) is used,
// packets between virtual machines and the physical LAN are forwarded cleanly.
// When container runtimes like Docker are installed, Docker sets iptables FORWARD default policy
// to DROP, and the kernel bridge netfilter (net.bridge.bridge-nf-call-iptables) intercepts bridged
// packets and drops them. We configure sysctl to disable bridge-nf packet interception, add explicit
// bridge ACCEPT rules in iptables FORWARD and DOCKER-USER chains, and clean up any obsolete ebtables rules.
func EnsureHostBridging() error {
	if os.Geteuid() != 0 {
		return nil
	}

	// 1. Enable IPv4 packet forwarding
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()

	// 2. Disable netfilter interception for bridged packets so Docker's FORWARD DROP policy does not block VM bridge traffic
	_ = exec.Command("sysctl", "-w", "net.bridge.bridge-nf-call-iptables=0").Run()
	_ = exec.Command("sysctl", "-w", "net.bridge.bridge-nf-call-ip6tables=0").Run()
	_ = exec.Command("sysctl", "-w", "net.bridge.bridge-nf-call-arptables=0").Run()

	// Persist sysctl bridge settings across reboots
	sysctlConfig := `# Managed by nasctl for KVM VM bridging
net.ipv4.ip_forward = 1
net.bridge.bridge-nf-call-iptables = 0
net.bridge.bridge-nf-call-ip6tables = 0
net.bridge.bridge-nf-call-arptables = 0
`
	_ = os.WriteFile("/etc/sysctl.d/99-nas-bridge.conf", []byte(sysctlConfig), 0644)

	// Clean up legacy mynas-ebtables service / rules if present
	_ = exec.Command("systemctl", "disable", "--now", "mynas-ebtables.service").Run()
	_ = os.Remove("/etc/systemd/system/mynas-ebtables.service")
	_ = os.Remove("/etc/mynas-ebtables.rules")

	// 3. For standard bridges (br0, vmbr0, virbr0), ensure iptables FORWARD and DOCKER-USER allow bridged traffic
	bridges := []string{"br0", "vmbr0", "virbr0"}
	if iptables, err := exec.LookPath("iptables"); err == nil {
		// If physdev module is loaded, also allow all bridged packets explicitly
		if exec.Command(iptables, "-C", "FORWARD", "-m", "physdev", "--physdev-is-bridged", "-j", "ACCEPT").Run() != nil {
			_ = exec.Command(iptables, "-I", "FORWARD", "-m", "physdev", "--physdev-is-bridged", "-j", "ACCEPT").Run()
		}

		for _, b := range bridges {
			if !isLinuxBridge(b) {
				continue
			}
			// Allow intra-bridge forwarding in FORWARD
			if exec.Command(iptables, "-C", "FORWARD", "-i", b, "-o", b, "-j", "ACCEPT").Run() != nil {
				_ = exec.Command(iptables, "-I", "FORWARD", "-i", b, "-o", b, "-j", "ACCEPT").Run()
			}
			// Allow intra-bridge forwarding in DOCKER-USER (if chain exists)
			if exec.Command(iptables, "-C", "DOCKER-USER", "-i", b, "-o", b, "-j", "ACCEPT").Run() != nil {
				_ = exec.Command(iptables, "-I", "DOCKER-USER", "-i", b, "-o", b, "-j", "ACCEPT").Run()
			}
		}
	}

	// 4. If host has no functional IPv6 transit, reject forwarded IPv6 to avoid 120s connection timeouts
	if _, err := exec.LookPath("ip6tables"); err == nil {
		if exec.Command("ping6", "-c", "1", "-W", "1", "2606:4700:4700::1111").Run() != nil {
			if err := exec.Command("ip6tables", "-C", "FORWARD", "-j", "REJECT", "--reject-with", "icmp6-no-route").Run(); err != nil {
				_ = exec.Command("ip6tables", "-I", "FORWARD", "-j", "REJECT", "--reject-with", "icmp6-no-route").Run()
			}
		}
	}

	return nil
}

func isLinuxBridge(name string) bool {
	info, err := os.Stat(filepath.Join("/sys/class/net", name, "bridge"))
	return err == nil && info.IsDir()
}

func getDefaultRouteInterface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

// ForcedDomainType specifies that KVM hardware acceleration is strictly enforced;
// software emulation (qemu TCG) is disallowed.
const ForcedDomainType = "kvm"

func buildDomainXML(vm domain.VirtualMachine) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`<domain type='%s' xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>
  <name>%s</name>
`, ForcedDomainType, vm.ID))
	if vm.UUID != "" {
		sb.WriteString(fmt.Sprintf(`  <uuid>%s</uuid>
`, vm.UUID))
	}
	if vm.Description != "" {
		sb.WriteString(fmt.Sprintf(`  <description>%s</description>
`, vm.Description))
	}
	sb.WriteString(fmt.Sprintf(`  <memory unit='MiB'>%d</memory>
  <currentMemory unit='MiB'>%d</currentMemory>
`, vm.MemoryMiB, vm.MemoryMiB))

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

	sb.WriteString(fmt.Sprintf(`  <vcpu placement='static'>%d</vcpu>
`, vm.VCPUs))

	cpuMode := "host-passthrough"
	if vm.CPUType != "" && vm.CPUType != "host" && vm.CPUType != "qemu64" {
		cpuMode = "custom"
	}
	sb.WriteString(fmt.Sprintf(`  <cpu mode='%s' check='none'>
    <topology sockets='%d' cores='%d' threads='1'/>
`, cpuMode, sockets, cores))
	if cpuMode == "custom" {
		sb.WriteString(fmt.Sprintf(`    <model fallback='allow'>%s</model>
`, vm.CPUType))
	}
	sb.WriteString(`  </cpu>
`)

	mach := normalizeMachineTypeForLibvirt(vm.MachineType)
	sb.WriteString("  <os")
	if vm.BiosType == "ovmf" {
		sb.WriteString(" firmware='efi'")
	}
	sb.WriteString(">\n")
	sb.WriteString(fmt.Sprintf(`    <type arch='x86_64' machine='%s'>hvm</type>
`, mach))
	if vm.CdromIsoImage != "" {
		sb.WriteString(`    <boot dev='cdrom'/>
    <boot dev='hd'/>
`)
	} else {
		sb.WriteString(`    <boot dev='hd'/>
`)
	}
	sb.WriteString(`  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <devices>
`)
	if emu := findQemuEmulator(); emu != "" {
		sb.WriteString(fmt.Sprintf(`    <emulator>%s</emulator>
`, emu))
	}
	sb.WriteString(`    <controller type='scsi' model='virtio-scsi'/>
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
			if st, err := os.Stat(d.DiskPath); err == nil && (st.Mode()&os.ModeDevice != 0) {
				diskKind = "block"
				sourceAttr = "dev"
			}
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
      <target dev='sdz' bus='sata'/>
      <readonly/>
    </disk>
`, vm.CdromIsoImage))
	} else {
		sb.WriteString(`    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <target dev='sdz' bus='sata'/>
      <readonly/>
    </disk>
`)
	}

	for _, iface := range vm.NetworkIfaces {
		target := iface.Bridge
		model := iface.Model
		if model == "" {
			model = "virtio"
		}
		netType, dev := resolveHostNetworkTarget(target)
		switch netType {
		case "bridge":
			sb.WriteString(fmt.Sprintf(`    <interface type='bridge'>
      <source bridge='%s'/>
      <model type='%s'/>
`, dev, model))
		case "direct":
			sb.WriteString(fmt.Sprintf(`    <interface type='direct'>
      <source dev='%s' mode='bridge'/>
      <model type='%s'/>
`, dev, model))
		default:
			sb.WriteString(fmt.Sprintf(`    <interface type='user'>
      <model type='%s'/>
`, model))
		}

		if iface.MacAddress != "" {
			sb.WriteString(fmt.Sprintf(`      <mac address='%s'/>
`, iface.MacAddress))
		}
		if iface.VlanTag > 0 && netType == "bridge" {
			sb.WriteString(fmt.Sprintf(`      <vlan><tag id='%d'/></vlan>
`, iface.VlanTag))
		}
		sb.WriteString(`    </interface>
`)
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
    <memballoon model='none'/>
    <graphics type='vnc' port='-1' autoport='yes' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>
` + buildVideoXML(vm.DisplayType) + `    <serial type='pty'>
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

func normalizeMachineTypeForLibvirt(mach string) string {
	mach = strings.TrimSpace(strings.ToLower(mach))
	if mach == "" || mach == "i440fx" || mach == "pc-i440fx" || mach == "pc" {
		return "pc"
	}
	if mach == "q35" || strings.HasPrefix(mach, "pc-q35") {
		return "q35"
	}
	return mach
}

func buildVideoXML(displayType string) string {
	videoModel := "virtio"
	if displayType != "" {
		switch displayType {
		case "virtio-gpu", "virtio":
			videoModel = "virtio"
		case "vga":
			videoModel = "vga"
		case "qxl":
			videoModel = "qxl"
		case "bochs":
			videoModel = "bochs"
		case "none":
			return ""
		default:
			videoModel = displayType
		}
	}
	return fmt.Sprintf(`    <video>
      <model type='%s'/>
    </video>
`, videoModel)
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
