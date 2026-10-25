package prereq

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
)

// DistroFamily represents the major distribution packaging family.
type DistroFamily string

const (
	DistroArch    DistroFamily = "arch"
	DistroDebian  DistroFamily = "debian"
	DistroFedora  DistroFamily = "fedora"
	DistroOpenSUSE DistroFamily = "suse"
	DistroAlpine  DistroFamily = "alpine"
	DistroGeneric DistroFamily = "generic"
)

// DetectLinuxFlavor parses /etc/os-release (or /usr/lib/os-release) and detects
// available package managers to return the host's Linux flavor and command templates.
func DetectLinuxFlavor() domain.LinuxFlavor {
	info := parseOSRelease()

	id := strings.ToLower(info["ID"])
	idLike := strings.ToLower(info["ID_LIKE"])
	name := info["NAME"]
	prettyName := info["PRETTY_NAME"]
	versionID := info["VERSION_ID"]

	if name == "" {
		name = "Linux"
	}
	if prettyName == "" {
		if versionID != "" {
			prettyName = fmt.Sprintf("%s %s", name, versionID)
		} else {
			prettyName = name
		}
	}

	pkgMgr, tmpl := detectPackageManager(id, idLike)

	return domain.LinuxFlavor{
		ID:                     id,
		Name:                   name,
		PrettyName:             prettyName,
		VersionID:              versionID,
		PackageManager:         pkgMgr,
		InstallCommandTemplate: tmpl,
	}
}

func parseOSRelease() map[string]string {
	data := make(map[string]string)
	paths := []string{"/etc/os-release", "/usr/lib/os-release"}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				data[key] = val
			}
		}
		if len(data) > 0 {
			break
		}
	}
	return data
}

func detectPackageManager(id, idLike string) (string, string) {
	// First check known distribution families
	matches := func(target string) bool {
		return id == target || strings.Contains(id, target) || strings.Contains(idLike, target)
	}

	// 1. Arch family (Arch, Manjaro, EndeavourOS, etc.)
	if matches("arch") || matches("manjaro") || matches("endeavouros") || hasBinary("pacman") {
		return "pacman", "sudo pacman -S --needed %s"
	}

	// 2. Debian/Ubuntu family (Ubuntu, Debian, Pop!_OS, Linux Mint, Proxmox, etc.)
	if matches("debian") || matches("ubuntu") || matches("pop") || matches("mint") || matches("proxmox") || hasBinary("apt-get") || hasBinary("apt") {
		return "apt", "sudo apt-get update && sudo apt-get install -y %s"
	}

	// 3. Fedora/RHEL family (Fedora, RHEL, CentOS, Rocky Linux, AlmaLinux)
	if matches("fedora") || matches("rhel") || matches("centos") || matches("rocky") || matches("almalinux") || hasBinary("dnf") {
		return "dnf", "sudo dnf install -y %s"
	}

	// 4. openSUSE/SLES family
	if matches("suse") || matches("opensuse") || matches("sles") || hasBinary("zypper") {
		return "zypper", "sudo zypper install -y %s"
	}

	// 5. Alpine
	if matches("alpine") || hasBinary("apk") {
		return "apk", "sudo apk add %s"
	}

	return "package-manager", "sudo package-manager install %s"
}

func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// PackageMapping returns the distro-specific package names for a given prerequisite ID.
var packageMapping = map[string]map[string][]string{
	"zpool": {
		"pacman": {"zfs-utils"},
		"apt":    {"zfsutils-linux"},
		"dnf":    {"zfs"},
		"zypper": {"zfs"},
		"apk":    {"zfs"},
	},
	"zfs": {
		"pacman": {"zfs-utils"},
		"apt":    {"zfsutils-linux"},
		"dnf":    {"zfs"},
		"zypper": {"zfs"},
		"apk":    {"zfs"},
	},
	"libzfs": {
		"pacman": {"zfs-utils"},
		"apt":    {"zfsutils-linux"},
		"dnf":    {"zfs"},
		"zypper": {"zfs"},
		"apk":    {"zfs"},
	},
	"smartctl": {
		"pacman": {"smartmontools"},
		"apt":    {"smartmontools"},
		"dnf":    {"smartmontools"},
		"zypper": {"smartmontools"},
		"apk":    {"smartmontools"},
	},
	"nvme": {
		"pacman": {"nvme-cli"},
		"apt":    {"nvme-cli"},
		"dnf":    {"nvme-cli"},
		"zypper": {"nvme-cli"},
		"apk":    {"nvme-cli"},
	},
	"kvm-dev": {
		"pacman": {"qemu-base"},
		"apt":    {"qemu-system-x86"},
		"dnf":    {"qemu-kvm"},
		"zypper": {"qemu-kvm"},
		"apk":    {"qemu-modules"},
	},
	"qemu-system-x86_64": {
		"pacman": {"qemu-system-x86"},
		"apt":    {"qemu-system-x86"},
		"dnf":    {"qemu-kvm"},
		"zypper": {"qemu-x86"},
		"apk":    {"qemu-system-x86_64"},
	},
	"qemu-img": {
		"pacman": {"qemu-img"},
		"apt":    {"qemu-utils"},
		"dnf":    {"qemu-img"},
		"zypper": {"qemu-tools"},
		"apk":    {"qemu-img"},
	},
	"swtpm": {
		"pacman": {"swtpm"},
		"apt":    {"swtpm", "swtpm-tools"},
		"dnf":    {"swtpm", "swtpm-tools"},
		"zypper": {"swtpm"},
		"apk":    {"swtpm"},
	},
	"edk2-ovmf": {
		"pacman": {"edk2-ovmf"},
		"apt":    {"ovmf"},
		"dnf":    {"edk2-ovmf"},
		"zypper": {"qemu-ovmf-x86_64"},
		"apk":    {"ovmf"},
	},
	"libvirt": {
		"pacman": {"libvirt"},
		"apt":    {"libvirt-daemon-system", "libvirt-clients"},
		"dnf":    {"libvirt-daemon-kvm", "libvirt-client"},
		"zypper": {"libvirt-daemon-qemu", "libvirt-client"},
		"apk":    {"libvirt-daemon", "libvirt-client"},
	},
	"virsh": {
		"pacman": {"libvirt"},
		"apt":    {"libvirt-clients"},
		"dnf":    {"libvirt-client"},
		"zypper": {"libvirt-client"},
		"apk":    {"libvirt-client"},
	},
	"docker": {
		"pacman": {"docker", "docker-compose"},
		"apt":    {"docker.io", "docker-compose-v2"},
		"dnf":    {"docker-ce", "docker-compose-plugin"},
		"zypper": {"docker", "docker-compose"},
		"apk":    {"docker", "docker-compose"},
	},
	"lxc": {
		"pacman": {"lxc"},
		"apt":    {"lxc", "lxc-templates"},
		"dnf":    {"lxc", "lxc-templates"},
		"zypper": {"lxc", "lxc-templates"},
		"apk":    {"lxc"},
	},
	"liblxc": {
		"pacman": {"lxc"},
		"apt":    {"lxc"},
		"dnf":    {"lxc"},
		"zypper": {"lxc"},
		"apk":    {"lxc"},
	},
	"smbd": {
		"pacman": {"samba"},
		"apt":    {"samba"},
		"dnf":    {"samba"},
		"zypper": {"samba"},
		"apk":    {"samba"},
	},
	"sshd": {
		"pacman": {"openssh"},
		"apt":    {"openssh-server"},
		"dnf":    {"openssh-server"},
		"zypper": {"openssh"},
		"apk":    {"openssh"},
	},
}

// PackagesForPrereq returns the package names needed for a prerequisite under the given package manager.
func PackagesForPrereq(prereqID, pkgMgr string) []string {
	if m, ok := packageMapping[prereqID]; ok {
		if pkgs, found := m[pkgMgr]; found {
			return pkgs
		}
		// Fallback to apt or pacman defaults
		if pkgs, found := m["apt"]; found {
			return pkgs
		}
	}
	return nil
}

// FormatInstallHint returns a tailored install hint string for a prerequisite given the detected Linux flavor.
func FormatInstallHint(specID string, flavor domain.LinuxFlavor) string {
	if specID == "kvm-dev" {
		pkgs := PackagesForPrereq(specID, flavor.PackageManager)
		pkgStr := ""
		if len(pkgs) > 0 {
			pkgStr = fmt.Sprintf(flavor.InstallCommandTemplate, strings.Join(pkgs, " ")) + " && "
		}
		return pkgStr + "(sudo modprobe kvm_intel 2>/dev/null || sudo modprobe kvm_amd 2>/dev/null || sudo modprobe kvm 2>/dev/null || true)"
	}

	pkgs := PackagesForPrereq(specID, flavor.PackageManager)
	if len(pkgs) == 0 {
		return "Built directly into NAS binary"
	}
	return fmt.Sprintf(flavor.InstallCommandTemplate, strings.Join(pkgs, " "))
}
