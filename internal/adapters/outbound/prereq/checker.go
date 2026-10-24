package prereq

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type Spec struct {
	ID          string
	Module      string // zfs, kvm, docker, lxc, cifs, webdav, ssh
	DisplayName string
	Category    string
	Kind        string // BINARY, LIBRARY, KERNEL_MODULE, DAEMON
	Target      string
	VersionArgs []string
	InstallHint string
	RequiredBy  []string
}

var defaultSpecs = []Spec{
	{
		ID:          "zpool",
		Module:      "zfs",
		DisplayName: "ZFS Pool Utility (zpool)",
		Category:    "STORAGE",
		Kind:        "BINARY",
		Target:      "zpool",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install zfsutils-linux  # or: pacman -S zfs-utils",
		RequiredBy:  []string{"ZFS Pools", "Pool Scrubbing"},
	},
	{
		ID:          "zfs",
		Module:      "zfs",
		DisplayName: "ZFS Dataset & Volume CLI (zfs)",
		Category:    "STORAGE",
		Kind:        "BINARY",
		Target:      "zfs",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install zfsutils-linux  # or: pacman -S zfs-utils",
		RequiredBy:  []string{"ZFS Datasets", "ZFS Volumes (ZVols)", "ZFS Snapshots"},
	},
	{
		ID:          "libzfs",
		Module:      "zfs",
		DisplayName: "ZFS Shared Library (libzfs.so)",
		Category:    "STORAGE",
		Kind:        "LIBRARY",
		Target:      "libzfs.so",
		InstallHint: "apt install libzfs4linux  # or: pacman -S zfs-utils",
		RequiredBy:  []string{"ZFS Pools", "ZFS Datasets", "ZFS Volumes"},
	},
	{
		ID:          "smartctl",
		Module:      "storage",
		DisplayName: "S.M.A.R.T. Drive Health CLI (smartctl)",
		Category:    "STORAGE",
		Kind:        "BINARY",
		Target:      "smartctl",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install smartmontools  # or: pacman -S smartmontools",
		RequiredBy:  []string{"Physical Disk Telemetry", "S.M.A.R.T. Drive Health & Temperature"},
	},
	{
		ID:          "nvme",
		Module:      "storage",
		DisplayName: "NVM-Express Management CLI (nvme)",
		Category:    "STORAGE",
		Kind:        "BINARY",
		Target:      "nvme",
		VersionArgs: []string{"version"},
		InstallHint: "apt install nvme-cli  # or: pacman -S nvme-cli",
		RequiredBy:  []string{"NVMe Health Log", "NVMe Temperature & Endurance Telemetry"},
	},
	{
		ID:          "kvm-dev",
		Module:      "kvm",
		DisplayName: "Linux KVM Hardware Virtualization (/dev/kvm)",
		Category:    "VIRTUALIZATION",
		Kind:        "KERNEL_MODULE",
		Target:      "/dev/kvm",
		InstallHint: "modprobe kvm_intel || modprobe kvm_amd",
		RequiredBy:  []string{"KVM Virtual Machines"},
	},
	{
		ID:          "qemu-system-x86_64",
		Module:      "kvm",
		DisplayName: "QEMU x86_64 System Emulator",
		Category:    "VIRTUALIZATION",
		Kind:        "BINARY",
		Target:      "qemu-system-x86_64",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install qemu-system-x86  # or: pacman -S qemu-full",
		RequiredBy:  []string{"KVM Virtual Machines", "VNC Display Server"},
	},
	{
		ID:          "libvirt",
		Module:      "kvm",
		DisplayName: "Libvirt Virtualization Library (libvirt.so)",
		Category:    "VIRTUALIZATION",
		Kind:        "LIBRARY",
		Target:      "libvirt.so",
		InstallHint: "apt install libvirt0 libvirt-daemon-system  # or: pacman -S libvirt",
		RequiredBy:  []string{"KVM Virtual Machines", "noVNC Consoles"},
	},
	{
		ID:          "virsh",
		Module:      "kvm",
		DisplayName: "Libvirt Management CLI (virsh)",
		Category:    "VIRTUALIZATION",
		Kind:        "BINARY",
		Target:      "virsh",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install libvirt-clients  # or: pacman -S libvirt",
		RequiredBy:  []string{"KVM Virtual Machines", "noVNC Consoles"},
	},
	{
		ID:          "docker",
		Module:      "docker",
		DisplayName: "Docker Engine CLI & Daemon",
		Category:    "CONTAINERS",
		Kind:        "BINARY",
		Target:      "docker",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install docker.io  # or: pacman -S docker",
		RequiredBy:  []string{"Docker Containers"},
	},
	{
		ID:          "lxc",
		Module:      "lxc",
		DisplayName: "Linux Containers CLI (lxc-ls)",
		Category:    "CONTAINERS",
		Kind:        "BINARY",
		Target:      "lxc-ls",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install lxc lxc-templates  # or: pacman -S lxc",
		RequiredBy:  []string{"LXC Containers"},
	},
	{
		ID:          "liblxc",
		Module:      "lxc",
		DisplayName: "LXC Shared Runtime Library (liblxc.so)",
		Category:    "CONTAINERS",
		Kind:        "LIBRARY",
		Target:      "liblxc.so",
		InstallHint: "apt install liblxc1  # or: pacman -S lxc",
		RequiredBy:  []string{"LXC Containers"},
	},
	{
		ID:          "smbd",
		Module:      "cifs",
		DisplayName: "Samba SMB/CIFS Server (smbd)",
		Category:    "SHARING",
		Kind:        "DAEMON",
		Target:      "smbd",
		VersionArgs: []string{"--version"},
		InstallHint: "apt install samba  # or: pacman -S samba",
		RequiredBy:  []string{"CIFS / SMB Folder Sharing"},
	},
	{
		ID:          "webdav-builtin",
		Module:      "webdav",
		DisplayName: "Embedded Go WebDAV Server & VFS Engine",
		Category:    "SHARING",
		Kind:        "LIBRARY",
		Target:      "builtin:golang.org/x/net/webdav",
		InstallHint: "Built directly into the single NAS binary",
		RequiredBy:  []string{"WebDAV Folder Sharing"},
	},
	{
		ID:          "novnc-bridge",
		Module:      "novnc",
		DisplayName: "Embedded RFB/noVNC WebSocket Proxy",
		Category:    "VIRTUALIZATION",
		Kind:        "LIBRARY",
		Target:      "builtin:rfb-websocket-bridge",
		InstallHint: "Built directly into the single NAS binary",
		RequiredBy:  []string{"KVM noVNC Browser Consoles"},
	},
	{
		ID:          "sshd",
		Module:      "ssh",
		DisplayName: "OpenSSH Server Daemon (sshd)",
		Category:    "SECURITY",
		Kind:        "DAEMON",
		Target:      "sshd",
		VersionArgs: []string{"-V"},
		InstallHint: "apt install openssh-server  # or: pacman -S openssh",
		RequiredBy:  []string{"User SSH Password & Public Key Access"},
	},
}

type SystemChecker struct {
	mu             sync.RWMutex
	hybridFallback bool
	cached         []domain.Prerequisite
	moduleSpecMap  map[string][]string
}

func NewSystemChecker(hybridFallback bool) *SystemChecker {
	modMap := make(map[string][]string)
	for _, s := range defaultSpecs {
		modMap[s.Module] = append(modMap[s.Module], s.ID)
	}
	c := &SystemChecker{
		hybridFallback: hybridFallback,
		moduleSpecMap:  modMap,
	}
	_, _, _ = c.CheckAll(context.Background())
	return c
}

// IsModuleEnabled returns true if all required underlying binaries and libraries for the module
// ("zfs", "kvm", "docker", "lxc", "cifs") are installed (or if hybrid fallback mode is active).
func (c *SystemChecker) IsModuleEnabled(module string) (bool, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.hybridFallback {
		return true, "hybrid fallback mode active"
	}

	reqIDs := c.moduleSpecMap[module]
	if len(reqIDs) == 0 {
		return true, "enabled"
	}

	var missing []string
	for _, id := range reqIDs {
		for _, p := range c.cached {
			if p.ID == id && p.Status != domain.StatusInstalled {
				missing = append(missing, fmt.Sprintf("%s (%s)", p.DisplayName, p.BinaryOrPath))
			}
		}
	}
	if len(missing) > 0 {
		return false, fmt.Sprintf("module %q is disabled: missing required host binaries/libraries: %s", strings.ToUpper(module), strings.Join(missing, ", "))
	}
	return true, "all underlying binaries and libraries are installed"
}

// HasHostPrerequisites returns true if all required host binaries and libraries for the module are installed.
func (c *SystemChecker) HasHostPrerequisites(module string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	reqIDs := c.moduleSpecMap[module]
	if len(reqIDs) == 0 {
		return true
	}

	for _, id := range reqIDs {
		for _, p := range c.cached {
			if p.ID == id && p.Status != domain.StatusInstalled {
				return false
			}
		}
	}
	return true
}

func (c *SystemChecker) ListPrerequisites(ctx context.Context) ([]domain.Prerequisite, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]domain.Prerequisite, len(c.cached))
	copy(out, c.cached)
	return out, nil
}

func (c *SystemChecker) GetPrerequisite(ctx context.Context, id string) (*domain.Prerequisite, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.cached {
		if p.ID == id {
			cp := p
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("%w: prerequisite %q", domain.ErrNotFound, id)
}

func (c *SystemChecker) CheckAll(ctx context.Context) ([]domain.Prerequisite, bool, error) {
	now := time.Now().UTC()
	results := make([]domain.Prerequisite, 0, len(defaultSpecs))

	for _, spec := range defaultSpecs {
		p := domain.Prerequisite{
			ID:           spec.ID,
			DisplayName:  spec.DisplayName,
			Category:     spec.Category,
			Kind:         spec.Kind,
			BinaryOrPath: spec.Target,
			InstallHint:  spec.InstallHint,
			RequiredBy:   spec.RequiredBy,
			FallbackMode: c.hybridFallback,
			CheckedTime:  now,
		}

		if strings.HasPrefix(spec.Target, "builtin:") {
			p.Status = domain.StatusInstalled
			p.Version = "embedded-v1.0"
			p.Details = "Compiled directly into the nasctl binary"
			p.FallbackMode = false
			results = append(results, p)
			continue
		}

		if spec.Kind == "KERNEL_MODULE" {
			modName := strings.TrimPrefix(spec.Target, "/dev/")
			modSysPath := filepath.Join("/sys/module", modName)
			if info, err := os.Stat(modSysPath); err == nil && info.IsDir() {
				p.Status = domain.StatusInstalled
				version := readFileTrim(filepath.Join(modSysPath, "version"))
				if version == "" {
					version = "kernel-loaded"
				}
				p.Version = version
				p.Details = fmt.Sprintf("Kernel module %s is active in /sys/module (%s)", modName, version)
				p.FallbackMode = false
			} else if _, err := os.Stat(spec.Target); err == nil {
				p.Status = domain.StatusInstalled
				p.Version = "kernel-loaded"
				p.Details = fmt.Sprintf("Kernel path %s is active on host", spec.Target)
				p.FallbackMode = false
			} else if isModuleInProcModules(modName) {
				p.Status = domain.StatusInstalled
				p.Version = "kernel-loaded"
				p.Details = fmt.Sprintf("Kernel module %s is loaded in /proc/modules", modName)
				p.FallbackMode = false
			} else if c.hybridFallback {
				p.Status = domain.StatusFallbackActive
				p.Version = "hybrid-sandbox"
				p.Details = fmt.Sprintf("Host path %s not present; hybrid sandbox emulation active", spec.Target)
			} else {
				p.Status = domain.StatusMissing
				p.Version = "not-loaded"
				p.Details = fmt.Sprintf("Required kernel module/device %s is missing", spec.Target)
			}
			results = append(results, p)
			continue
		}

		if spec.Kind == "LIBRARY" {
			if libPath, ok := findSharedLibrary(spec.Target); ok {
				p.BinaryOrPath = libPath
				p.Status = domain.StatusInstalled
				p.Version = filepath.Base(libPath)
				p.Details = fmt.Sprintf("Shared library located at %s", libPath)
				p.FallbackMode = false
			} else if spec.ID == "libzfs" && (isModuleInProcModules("zfs") || isDirExists("/sys/module/zfs")) {
				// OpenZFS kernel module loaded with in-kernel kstat/sysfs API
				version := readFileTrim("/sys/module/zfs/version")
				if version == "" {
					version = "in-kernel"
				}
				p.Status = domain.StatusInstalled
				p.Version = version
				p.Details = fmt.Sprintf("ZFS kernel module active via /sys/module/zfs (%s)", version)
				p.FallbackMode = false
			} else if c.hybridFallback {
				p.Status = domain.StatusFallbackActive
				p.Version = "hybrid-sandbox"
				p.Details = fmt.Sprintf("Shared library %q not found on host; running in hybrid sandbox mode", spec.Target)
			} else {
				p.Status = domain.StatusMissing
				p.Version = "missing"
				p.Details = fmt.Sprintf("Shared library %q not found in system library paths", spec.Target)
			}
			results = append(results, p)
			continue
		}

		// Binary or Daemon lookup
		resolvedPath, err := lookPathWithSbin(spec.Target)
		if err == nil {
			p.BinaryOrPath = resolvedPath
			p.Status = domain.StatusInstalled
			p.FallbackMode = false
			p.Version = detectVersion(ctx, resolvedPath, spec.VersionArgs)
			p.Details = fmt.Sprintf("Found executable at %s", resolvedPath)
		} else if c.hybridFallback {
			p.Status = domain.StatusFallbackActive
			p.Version = "hybrid-sandbox"
			p.Details = fmt.Sprintf("Binary %q not installed on host; running in hybrid sandbox mode", spec.Target)
		} else {
			p.Status = domain.StatusMissing
			p.Version = "missing"
			p.Details = fmt.Sprintf("Executable %q not found in PATH or /usr/sbin", spec.Target)
		}
		results = append(results, p)
	}

	c.mu.Lock()
	c.cached = results
	c.mu.Unlock()
	return results, c.hybridFallback, nil
}

func findSharedLibrary(libPrefix string) (string, bool) {
	searchDirs := []string{"/usr/lib", "/usr/lib64", "/lib", "/lib64", "/usr/lib/x86_64-linux-gnu", "/lib/x86_64-linux-gnu", "/usr/local/lib"}
	for _, dir := range searchDirs {
		matches, err := filepath.Glob(filepath.Join(dir, libPrefix+"*"))
		if err == nil && len(matches) > 0 {
			return matches[0], true
		}
	}
	if out, err := exec.Command("ldconfig", "-p").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, libPrefix) {
				parts := strings.Split(line, "=>")
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), true
				}
			}
		}
	}
	return "", false
}

func lookPathWithSbin(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range []string{"/usr/sbin", "/sbin", "/usr/local/sbin", "/usr/bin"} {
		candidate := dir + "/" + name
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func detectVersion(ctx context.Context, bin string, args []string) string {
	if len(args) == 0 {
		return "installed"
	}
	tctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(tctx, bin, args...)
	out, _ := cmd.CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		first := strings.TrimSpace(lines[0])
		if len(first) > 64 {
			first = first[:64]
		}
		return first
	}
	return "installed"
}

func isModuleInProcModules(modName string) bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == modName {
			return true
		}
	}
	return false
}

func isDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
