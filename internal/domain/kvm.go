package domain

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type VirtualMachineDisk struct {
	ID             string `json:"id"`             // "scsi0", "virtio0", "sata1"
	Bus            string `json:"bus"`            // "scsi", "virtio", "sata", "ide", "nvme"
	DiskPath       string `json:"disk_path"`      // ZFS zvol or qcow2 file path
	SizeBytes      int64  `json:"size_bytes"`
	CacheMode      string `json:"cache_mode"`     // "none", "writeback", "writethrough", "unsafe"
	Discard        bool   `json:"discard"`        // TRIM passthrough
	IOThread       bool   `json:"iothread"`       // Dedicated IOThread
	ReadOnly       bool   `json:"read_only"`
	SSDEmulation   bool   `json:"ssd_emulation"`
	Backup         bool   `json:"backup"`
	IOPSLimitRead  int64  `json:"iops_limit_read"`
	IOPSLimitWrite int64  `json:"iops_limit_write"`
	MbpsLimitRead  int64  `json:"mbps_limit_read"`
	MbpsLimitWrite int64  `json:"mbps_limit_write"`
}

type VirtualMachineNetworkInterface struct {
	ID            string `json:"id"`              // "net0", "net1"
	Model         string `json:"model"`           // "virtio", "e1000", "rtl8139", "vmxnet3"
	Bridge        string `json:"bridge"`          // "virbr0", "vmbr0", "br0"
	VlanTag       int32  `json:"vlan_tag"`        // 0 = untagged, 1..4094
	MacAddress    string `json:"mac_address"`
	RateLimitMbps int64  `json:"rate_limit_mbps"`
	Disconnected  bool   `json:"disconnected"`
	Firewall      bool   `json:"firewall"`
}

type VirtualMachineCloudInit struct {
	User           string   `json:"user"`
	Password       string   `json:"password"`
	SSHPublicKeys  []string `json:"ssh_public_keys"`
	IPConfig       string   `json:"ip_config"` // "dhcp" or "ip=192.168.1.50/24,gw=192.168.1.1"
	DNSNameservers string   `json:"dns_nameservers"`
	DNSSearch      string   `json:"dns_search"`
	CustomUserData string   `json:"custom_user_data"`
}

type VirtualMachinePciDevice struct {
	HostAddress string `json:"host_address"` // "0000:01:00.0"
	PCIE        bool   `json:"pcie"`
	PrimaryGPU  bool   `json:"primary_gpu"`
}

type VirtualMachineUsbDevice struct {
	HostBusPort     string `json:"host_bus_port"`      // "1-1.2"
	VendorProductID string `json:"vendor_product_id"` // "046d:c52b"
}

type VirtualMachineSnapshot struct {
	Name           string    `json:"name"`
	SnapshotID     string    `json:"snapshot_id"`
	Description    string    `json:"description"`
	CreateTime     time.Time `json:"create_time"`
	IncludeRAM     bool      `json:"include_ram"`
	ParentSnapshot string    `json:"parent_snapshot"`
	VMState        string    `json:"vm_state"`
}

type VirtualMachine struct {
	ID            string `json:"id"`
	VMName        string `json:"vm_name"`
	UUID          string `json:"uuid"`
	State         string `json:"state"` // RUNNING, STOPPED, PAUSED
	VCPUs         int32  `json:"vcpus"`
	MemoryMiB     int64  `json:"memory_mib"`
	DiskPath      string `json:"disk_path"`
	DiskSizeGiB   int64  `json:"disk_size_gib"`
	ISOImage      string `json:"iso_image"`
	NetworkBridge string `json:"network_bridge"`
	VNCPort       int32  `json:"vnc_port"`
	VNCDisplay    string `json:"vnc_display"`
	Autostart     bool   `json:"autostart"`
	OSVariant     string `json:"os_variant"`
	Description   string `json:"description"`

	// Proxmox VE Hardware Topology
	Sockets       int32  `json:"sockets"`
	Cores         int32  `json:"cores"`
	CPUType       string `json:"cpu_type"` // "host", "kvm64", "x86-64-v2-AES", "EPYC"
	CPUUnits      int32  `json:"cpu_units"`
	CPULimit      int32  `json:"cpu_limit"`
	NUMA          bool   `json:"numa"`
	BalloonMinMiB int64  `json:"balloon_min_mib"`
	Hugepages     string `json:"hugepages"`    // "off", "2MB", "1GB"
	MachineType   string `json:"machine_type"` // "q35", "i440fx"
	BiosType      string `json:"bios_type"`    // "seabios", "ovmf" (UEFI)
	TPMVersion    string `json:"tpm_version"`  // "none", "v2.0"
	SecureBoot    bool   `json:"secure_boot"`

	// Multi-Disk & Media
	Disks          []VirtualMachineDisk             `json:"disks"`
	CdromIsoImage  string                           `json:"cdrom_iso_image"`
	NetworkIfaces  []VirtualMachineNetworkInterface `json:"network_interfaces"`
	CloudInit      VirtualMachineCloudInit          `json:"cloud_init"`
	PciDevices     []VirtualMachinePciDevice        `json:"pci_devices"`
	UsbDevices     []VirtualMachineUsbDevice        `json:"usb_devices"`

	// QEMU Guest Agent & Runtime Telemetry
	AgentEnabled       bool     `json:"agent_enabled"`
	AgentRunning       bool     `json:"agent_running"`
	GuestIPAddresses   []string `json:"guest_ip_addresses"`
	GuestOSName        string   `json:"guest_os_name"`
	GuestCPUPercent    float64  `json:"guest_cpu_percent"`
	GuestMemoryUsedMiB int64    `json:"guest_memory_used_mib"`

	// Lifecycle, Watchdog & Consoles
	StartupDelaySeconds int32    `json:"startup_delay_seconds"`
	BootOrder           string   `json:"boot_order"`
	WatchdogModel       string   `json:"watchdog_model"`
	WatchdogAction      string   `json:"watchdog_action"`
	DisplayType         string   `json:"display_type"`
	IsTemplate          bool     `json:"is_template"`
	QemuCommandlineArgs []string `json:"qemu_commandline_args"`
	SerialConsoleURL    string   `json:"serial_console_url"`
}

func (vm *VirtualMachine) Validate() error {
	if err := ValidateResourceID(vm.ID, "virtual_machine_id"); err != nil {
		return err
	}
	if vm.VCPUs <= 0 {
		if vm.Sockets > 0 && vm.Cores > 0 {
			vm.VCPUs = vm.Sockets * vm.Cores
		} else {
			return fmt.Errorf("%w: vcpus must be >= 1", ErrInvalidArgument)
		}
	}
	if vm.Sockets <= 0 {
		vm.Sockets = 1
	}
	if vm.Cores <= 0 {
		vm.Cores = vm.VCPUs / vm.Sockets
		if vm.Cores <= 0 {
			vm.Cores = 1
		}
	}
	if vm.MemoryMiB < 128 {
		return fmt.Errorf("%w: memory_mib must be >= 128", ErrInvalidArgument)
	}
	if vm.MachineType == "" {
		vm.MachineType = "q35"
	}
	if vm.BiosType == "" {
		vm.BiosType = "seabios"
	}
	if vm.CPUType == "" {
		vm.CPUType = "host"
	}
	if vm.DisplayType == "" {
		vm.DisplayType = "virtio-gpu"
	}
	if vm.NetworkBridge == "" {
		vm.NetworkBridge = "virbr0"
	}
	if vm.OSVariant == "" {
		vm.OSVariant = "linux2024"
	}

	// Migrate legacy single-disk to Disks slice if Disks is empty
	if len(vm.Disks) == 0 && (vm.DiskPath != "" || vm.DiskSizeGiB > 0) {
		vm.Disks = []VirtualMachineDisk{
			{
				ID:        "scsi0",
				Bus:       "scsi",
				DiskPath:  vm.DiskPath,
				SizeBytes: vm.DiskSizeGiB * 1024 * 1024 * 1024,
				CacheMode: "none",
				Discard:   true,
				IOThread:  true,
			},
		}
	}

	// Migrate legacy single network bridge to NetworkInterfaces if empty
	if len(vm.NetworkIfaces) == 0 && vm.NetworkBridge != "" {
		vm.NetworkIfaces = []VirtualMachineNetworkInterface{
			{
				ID:       "net0",
				Model:    "virtio",
				Bridge:   vm.NetworkBridge,
				Firewall: true,
			},
		}
	}

	return nil
}

type ConsoleSession struct {
	VMName       string
	Token        string
	WebsocketURL string
	VNCPort      int32
	Resolution   string
	ExpiresAt    string
}

// HasCPUVirtualization checks /proc/cpuinfo for hardware virtualization extensions:
// "vmx" for Intel VT-x, or "svm" for AMD-V.
func HasCPUVirtualization() (bool, string) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false, "failed to read /proc/cpuinfo: " + err.Error()
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "flags") || strings.HasPrefix(line, "Features") {
			fields := strings.Fields(line)
			for _, f := range fields {
				switch f {
				case "vmx":
					return true, "Intel VT-x (VMX)"
				case "svm":
					return true, "AMD-V (SVM)"
				}
			}
		}
	}
	return false, "hardware virtualization extension (VMX/SVM) not present in /proc/cpuinfo"
}

// HasKVMDevice checks if the /dev/kvm character device node is present on the host.
func HasKVMDevice() bool {
	st, err := os.Stat("/dev/kvm")
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeDevice != 0
}

// HasKVMHardwareAcceleration returns true only if both hardware virtualization (VMX/SVM)
// and the /dev/kvm device node are present and functional.
func HasKVMHardwareAcceleration() (bool, string) {
	hasVirt, virtDesc := HasCPUVirtualization()
	if !hasVirt {
		return false, virtDesc
	}
	if !HasKVMDevice() {
		return false, fmt.Sprintf("CPU supports %s, but /dev/kvm device node is missing (kvm_intel/kvm_amd kernel module not loaded)", virtDesc)
	}
	return true, fmt.Sprintf("KVM acceleration active with %s via /dev/kvm", virtDesc)
}

