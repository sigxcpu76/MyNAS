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
	Module      string // zfs, kvm, docker, lxc, cifs, webdav, ssh, storage
	DisplayName string
	Category    string
	Kind        string // BINARY, LIBRARY, KERNEL_MODULE, DAEMON, FIRMWARE
	Target      string
	VersionArgs []string
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
		RequiredBy:  []string{"ZFS Datasets", "ZFS Volumes (ZVols)", "ZFS Snapshots"},
	},
	{
		ID:          "libzfs",
		Module:      "zfs",
		DisplayName: "ZFS Shared Library (libzfs.so)",
		Category:    "STORAGE",
		Kind:        "LIBRARY",
		Target:      "libzfs.so",
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
		RequiredBy:  []string{"NVMe Health Log", "NVMe Temperature & Endurance Telemetry"},
	},
	{
		ID:          "kvm-dev",
		Module:      "kvm",
		DisplayName: "Linux KVM Hardware Virtualization (/dev/kvm)",
		Category:    "VIRTUALIZATION",
		Kind:        "KERNEL_MODULE",
		Target:      "/dev/kvm",
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
		RequiredBy:  []string{"KVM Virtual Machines", "VNC Display Server"},
	},
	{
		ID:          "qemu-img",
		Module:      "kvm",
		DisplayName: "QEMU Disk Image Utility (qemu-img)",
		Category:    "VIRTUALIZATION",
		Kind:        "BINARY",
		Target:      "qemu-img",
		VersionArgs: []string{"--version"},
		RequiredBy:  []string{"KVM Virtual Disks", "QCOW2 Image Management"},
	},
	{
		ID:          "swtpm",
		Module:      "kvm",
		DisplayName: "Software TPM 2.0 Emulator (swtpm)",
		Category:    "VIRTUALIZATION",
		Kind:        "BINARY",
		Target:      "swtpm",
		VersionArgs: []string{"--version"},
		RequiredBy:  []string{"KVM Virtual Machines", "Virtual TPM 2.0 (vTPM)"},
	},
	{
		ID:          "edk2-ovmf",
		Module:      "kvm",
		DisplayName: "OVMF UEFI BIOS Firmware (edk2-ovmf)",
		Category:    "VIRTUALIZATION",
		Kind:        "FIRMWARE",
		Target:      "/usr/share/edk2-ovmf",
		RequiredBy:  []string{"KVM Virtual Machines", "UEFI Secure Boot Firmware"},
	},
	{
		ID:          "libvirt",
		Module:      "kvm",
		DisplayName: "Libvirt Virtualization Library (libvirt.so)",
		Category:    "VIRTUALIZATION",
		Kind:        "LIBRARY",
		Target:      "libvirt.so",
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
		RequiredBy:  []string{"LXC Containers"},
	},
	{
		ID:          "liblxc",
		Module:      "lxc",
		DisplayName: "LXC Shared Runtime Library (liblxc.so)",
		Category:    "CONTAINERS",
		Kind:        "LIBRARY",
		Target:      "liblxc.so",
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
		RequiredBy:  []string{"CIFS / SMB Folder Sharing"},
	},
	{
		ID:          "webdav-builtin",
		Module:      "webdav",
		DisplayName: "Embedded Go WebDAV Server & VFS Engine",
		Category:    "SHARING",
		Kind:        "LIBRARY",
		Target:      "builtin:golang.org/x/net/webdav",
		RequiredBy:  []string{"WebDAV Folder Sharing"},
	},
	{
		ID:          "novnc-bridge",
		Module:      "novnc",
		DisplayName: "Embedded RFB/noVNC WebSocket Proxy",
		Category:    "VIRTUALIZATION",
		Kind:        "LIBRARY",
		Target:      "builtin:rfb-websocket-bridge",
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
		RequiredBy:  []string{"User SSH Password & Public Key Access"},
	},
}

type SystemChecker struct {
	mu            sync.RWMutex
	flavor        domain.LinuxFlavor
	mainPool      string
	cached        []domain.Prerequisite
	moduleSpecMap map[string][]string
}

func NewSystemChecker() *SystemChecker {
	modMap := make(map[string][]string)
	for _, s := range defaultSpecs {
		modMap[s.Module] = append(modMap[s.Module], s.ID)
	}
	flavor := DetectLinuxFlavor()
	c := &SystemChecker{
		flavor:        flavor,
		moduleSpecMap: modMap,
	}
	_, _ = c.CheckAll(context.Background())
	return c
}

func (c *SystemChecker) SetMainPool(pool string) {
	c.mu.Lock()
	c.mainPool = strings.TrimSpace(pool)
	c.mu.Unlock()
}

func (c *SystemChecker) GetMainPool() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mainPool
}

func (c *SystemChecker) GetLinuxFlavor() domain.LinuxFlavor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flavor
}

// SetAllModulesInstalledForTest forces all prerequisite items to be marked as installed.
// This is used in unit tests to decouple domain, state-machine, and API routing tests
// from host machine environmental dependencies.
func (c *SystemChecker) SetAllModulesInstalledForTest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.cached {
		c.cached[i].Status = domain.StatusInstalled
		if c.cached[i].Version == "" || c.cached[i].Version == "not-found" {
			c.cached[i].Version = "test-mock-v1.0"
		}
	}
}

// IsModuleEnabled returns true if all required underlying binaries and libraries for the module
// ("zfs", "kvm", "docker", "lxc", "cifs") are installed on the host,
// and verifies that a main ZFS storage pool has been selected for modules other than zfs.
func (c *SystemChecker) IsModuleEnabled(module string) (bool, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Gating: Only ZFS and basic storage telemetry are allowed until a main pool is selected
	if module != "zfs" && module != "storage" {
		if c.mainPool == "" {
			return false, fmt.Sprintf("module %q is paused: no main ZFS pool selected. Please select a main storage pool to initialize system datasets and enable this module", strings.ToUpper(module))
		}
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

func (c *SystemChecker) CheckAll(ctx context.Context) ([]domain.Prerequisite, error) {
	now := time.Now().UTC()
	flavor := DetectLinuxFlavor()

	results := make([]domain.Prerequisite, 0, len(defaultSpecs))

	for _, spec := range defaultSpecs {
		p := domain.Prerequisite{
			ID:           spec.ID,
			DisplayName:  spec.DisplayName,
			Category:     spec.Category,
			Kind:         spec.Kind,
			BinaryOrPath: spec.Target,
			InstallHint:  FormatInstallHint(spec.ID, flavor),
			RequiredBy:   spec.RequiredBy,
			CheckedTime:  now,
		}

		if strings.HasPrefix(spec.Target, "builtin:") {
			p.Status = domain.StatusInstalled
			p.Version = "embedded-v1.0"
			p.Details = "Compiled directly into the nasctl binary"
			p.InstallHint = "Built directly into the single NAS binary"
			results = append(results, p)
			continue
		}

		if spec.Kind == "FIRMWARE" {
			paths := []string{
				spec.Target,
				"/usr/share/OVMF",
				"/usr/share/edk2",
				"/usr/share/edk2-ovmf",
				"/usr/share/qemu/edk2-x86_64-code.fd",
				"/usr/share/edk2/x64/OVMF_CODE.fd",
				"/usr/share/OVMF/OVMF_CODE.fd",
			}
			foundPath := ""
			for _, pth := range paths {
				if info, err := os.Stat(pth); err == nil {
					foundPath = pth
					_ = info
					break
				}
			}
			if foundPath != "" {
				p.BinaryOrPath = foundPath
				p.Status = domain.StatusInstalled
				p.Version = "uefi-ovmf"
				p.Details = fmt.Sprintf("UEFI firmware found at %s", foundPath)
			} else {
				p.Status = domain.StatusMissing
				p.Version = "missing"
				p.Details = fmt.Sprintf("OVMF UEFI firmware not found in standard paths (%s)", spec.Target)
			}
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
			} else if _, err := os.Stat(spec.Target); err == nil {
				p.Status = domain.StatusInstalled
				p.Version = "kernel-loaded"
				p.Details = fmt.Sprintf("Kernel path %s is active on host", spec.Target)
			} else if isModuleInProcModules(modName) {
				p.Status = domain.StatusInstalled
				p.Version = "kernel-loaded"
				p.Details = fmt.Sprintf("Kernel module %s is loaded in /proc/modules", modName)
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
			} else if spec.ID == "libzfs" && (isModuleInProcModules("zfs") || isDirExists("/sys/module/zfs")) {
				// OpenZFS kernel module loaded with in-kernel kstat/sysfs API
				version := readFileTrim("/sys/module/zfs/version")
				if version == "" {
					version = "in-kernel"
				}
				p.Status = domain.StatusInstalled
				p.Version = version
				p.Details = fmt.Sprintf("ZFS kernel module active via /sys/module/zfs (%s)", version)
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
			p.Version = detectVersion(ctx, resolvedPath, spec.VersionArgs)
			p.Details = fmt.Sprintf("Found executable at %s", resolvedPath)
		} else {
			p.Status = domain.StatusMissing
			p.Version = "missing"
			p.Details = fmt.Sprintf("Executable %q not found in PATH or /usr/sbin", spec.Target)
		}
		results = append(results, p)
	}

	c.mu.Lock()
	c.cached = results
	c.flavor = flavor
	c.mu.Unlock()
	return results, nil
}

// GetReport compiles a comprehensive status report separating missing and installed prerequisites,
// along with the host Linux flavor and a copy-pasteable remediation command.
func (c *SystemChecker) GetReport(ctx context.Context) (domain.PrerequisitesReport, error) {
	items, err := c.CheckAll(ctx)
	if err != nil {
		return domain.PrerequisitesReport{}, err
	}

	c.mu.RLock()
	flavor := c.flavor
	c.mu.RUnlock()

	var missing []domain.Prerequisite
	var installed []domain.Prerequisite
	pkgSet := make(map[string]struct{})
	var uniquePkgs []string

	needKvmMod := false
	needServices := make(map[string]struct{})

	for _, item := range items {
		if item.Status == domain.StatusInstalled {
			installed = append(installed, item)
		} else {
			missing = append(missing, item)

			// Collect package requirements
			pkgs := PackagesForPrereq(item.ID, flavor.PackageManager)
			for _, pkg := range pkgs {
				if _, seen := pkgSet[pkg]; !seen {
					pkgSet[pkg] = struct{}{}
					uniquePkgs = append(uniquePkgs, pkg)
				}
			}

			if item.ID == "kvm-dev" {
				needKvmMod = true
			}
			if item.ID == "libvirt" || item.ID == "virsh" {
				needServices["libvirtd"] = struct{}{}
			}
			if item.ID == "docker" {
				needServices["docker"] = struct{}{}
			}
			if item.ID == "smbd" {
				if flavor.PackageManager == "pacman" {
					needServices["smb"] = struct{}{}
				} else {
					needServices["smbd"] = struct{}{}
				}
			}
			if item.ID == "sshd" {
				if flavor.PackageManager == "apt" {
					needServices["ssh"] = struct{}{}
				} else {
					needServices["sshd"] = struct{}{}
				}
			}
		}
	}

	// Build remediation script
	var cmdParts []string
	if len(uniquePkgs) > 0 {
		cmdParts = append(cmdParts, fmt.Sprintf(flavor.InstallCommandTemplate, strings.Join(uniquePkgs, " ")))
	}
	if needKvmMod {
		cmdParts = append(cmdParts, "(sudo modprobe kvm_intel 2>/dev/null || sudo modprobe kvm_amd 2>/dev/null || sudo modprobe kvm 2>/dev/null || true)")
	}
	if len(needServices) > 0 {
		var srvList []string
		for srv := range needServices {
			srvList = append(srvList, srv)
		}
		cmdParts = append(cmdParts, fmt.Sprintf("sudo systemctl enable --now %s", strings.Join(srvList, " ")))
	}

	remediationScript := strings.Join(cmdParts, " && \\\n  ")

	return domain.PrerequisitesReport{
		LinuxFlavor:       flavor,
		Missing:           missing,
		Installed:         installed,
		All:               items,
		RemediationScript: remediationScript,
		IsFullyCompliant:  len(missing) == 0,
	}, nil
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
