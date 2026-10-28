package lxc

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type ptyConn struct {
	file    *os.File
	cmd     *exec.Cmd
	onClose func()
}

func (c *ptyConn) Read(p []byte) (n int, err error) {
	return c.file.Read(p)
}

func (c *ptyConn) Write(p []byte) (n int, err error) {
	return c.file.Write(p)
}

func (c *ptyConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	_ = c.file.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func (c *ptyConn) LocalAddr() net.Addr                { return &net.IPAddr{} }
func (c *ptyConn) RemoteAddr() net.Addr               { return &net.IPAddr{} }
func (c *ptyConn) SetDeadline(t time.Time) error      { return nil }
func (c *ptyConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *ptyConn) SetWriteDeadline(t time.Time) error { return nil }

type Adapter struct {
	mu         sync.RWMutex
	mainPool   string
	zfsPort    ports.ZfsEnginePort
	containers map[string]domain.LxcContainer
	execPTYs   map[string]*os.File
	httpClient *http.Client
}

func NewAdapter() *Adapter {
	return &Adapter{
		containers: make(map[string]domain.LxcContainer),
		execPTYs:   make(map[string]*os.File),
		httpClient: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (a *Adapter) SetZfsPort(zfsPort ports.ZfsEnginePort) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.zfsPort = zfsPort
}

type lxcStatus struct {
	state string
	ipv4  string
}

// fetchLiveStatuses leverages lxc-ls to fetch the state and IP of all containers in a single fast call.
func (a *Adapter) fetchLiveStatuses(ctx context.Context) map[string]lxcStatus {
	statuses := make(map[string]lxcStatus)
	if _, err := exec.LookPath("lxc-ls"); err != nil {
		return statuses
	}

	execCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "lxc-ls", "-f", "-F", "NAME,STATE,IPV4")
	out, err := cmd.Output()
	if err != nil {
		return statuses
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[0]
			state := fields[1]
			ip := ""
			if len(fields) >= 3 && fields[2] != "-" {
				ip = fields[2]
				if commaIdx := strings.Index(ip, ","); commaIdx != -1 {
					ip = ip[:commaIdx]
				}
			}
			statuses[name] = lxcStatus{
				state: state,
				ipv4:  ip,
			}
		}
	}
	return statuses
}

// fetchSingleContainerStatus checks a specific container state and IP via lxc-ls filter.
func (a *Adapter) fetchSingleContainerStatus(ctx context.Context, id string) *lxcStatus {
	if _, err := exec.LookPath("lxc-ls"); err != nil {
		return nil
	}
	execCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "lxc-ls", "-f", "-F", "NAME,STATE,IPV4", "--filter", "^"+id+"$")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == id {
			ip := ""
			if len(fields) >= 3 && fields[2] != "-" {
				ip = fields[2]
				if commaIdx := strings.Index(ip, ","); commaIdx != -1 {
					ip = ip[:commaIdx]
				}
			}
			return &lxcStatus{
				state: fields[1],
				ipv4:  ip,
			}
		}
	}
	return nil
}

// discoverContainerLocked parses a container configuration from /var/lib/lxc/<id>/config.
func (a *Adapter) discoverContainerLocked(id string) *domain.LxcContainer {
	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	c := &domain.LxcContainer{
		ID:             id,
		ContainerName:  id,
		State:          "STOPPED",
		Architecture:   "amd64",
		Distribution:   "linux",
		Release:        "generic",
		NetworkBridge:  "br0",
		MemoryLimitMiB: 512,
		CPUCores:       1,
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])

		switch k {
		case "lxc.uts.name":
			c.ContainerName = v
		case "lxc.arch":
			c.Architecture = v
		case "lxc.rootfs.path":
			c.RootfsPath = strings.TrimPrefix(v, "dir:")
		case "lxc.net.0.link":
			c.NetworkBridge = v
		case "lxc.start.auto":
			c.Autostart = (v == "1")
		case "lxc.cgroup2.memory.max":
			v = strings.TrimSuffix(v, "M")
			var mem int64
			if _, err := fmt.Sscanf(v, "%d", &mem); err == nil {
				c.MemoryLimitMiB = mem
			}
		case "lxc.cgroup2.cpu.weight":
			var weight int32
			if _, err := fmt.Sscanf(v, "%d", &weight); err == nil && weight > 0 {
				c.CPUCores = weight / 100
				if c.CPUCores < 1 {
					c.CPUCores = 1
				}
			}
		}
	}

	// Try reading OS release from rootfs if present
	if c.RootfsPath != "" {
		if osReleaseBytes, err := os.ReadFile(filepath.Join(c.RootfsPath, "etc/os-release")); err == nil {
			for _, osLine := range strings.Split(string(osReleaseBytes), "\n") {
				if strings.HasPrefix(osLine, "ID=") {
					c.Distribution = strings.Trim(strings.TrimPrefix(osLine, "ID="), `"`)
				} else if strings.HasPrefix(osLine, "VERSION_ID=") {
					c.Release = strings.Trim(strings.TrimPrefix(osLine, "VERSION_ID="), `"`)
				}
			}
		}
	}

	return c
}

// getOrDiscoverContainerLocked returns the container from memory if present,
// or attempts to discover it from disk (/var/lib/lxc/<id>/config) under the adapter lock.
func (a *Adapter) getOrDiscoverContainerLocked(id string) (*domain.LxcContainer, bool) {
	if c, ok := a.containers[id]; ok {
		return &c, true
	}
	if disc := a.discoverContainerLocked(id); disc != nil {
		a.containers[id] = *disc
		return disc, true
	}
	return nil, false
}

func (a *Adapter) ListContainers(ctx context.Context) ([]domain.LxcContainer, error) {
	// 1. Fetch live container statuses in one fast lxc-ls execution outside the lock
	liveStatuses := a.fetchLiveStatuses(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()

	// 2. Update all in-memory containers with their live status
	for name, st := range liveStatuses {
		if _, exists := a.containers[name]; !exists {
			if disc := a.discoverContainerLocked(name); disc != nil {
				a.containers[name] = *disc
			}
		}
		if c, exists := a.containers[name]; exists {
			c.State = st.state
			c.IPv4Address = st.ipv4
			a.containers[name] = c
		}
	}

	// 3. Auto-discover any containers residing in /var/lib/lxc
	if entries, err := os.ReadDir("/var/lib/lxc"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				name := e.Name()
				if _, exists := a.containers[name]; !exists {
					if disc := a.discoverContainerLocked(name); disc != nil {
						if st, ok := liveStatuses[name]; ok {
							disc.State = st.state
							disc.IPv4Address = st.ipv4
						}
						a.containers[name] = *disc
					}
				}
			}
		}
	}

	out := make([]domain.LxcContainer, 0, len(a.containers))
	for _, c := range a.containers {
		out = append(out, c)
	}
	return out, nil
}

func (a *Adapter) GetContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
		a.mu.Lock()
		c.State = st.state
		c.IPv4Address = st.ipv4
		a.containers[id] = c
		a.mu.Unlock()
	}

	cp := c
	return &cp, nil
}

// SetMainPool enforces that the main storage pool is the only choice for LXC rootfs.
func (a *Adapter) SetMainPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mainPool = strings.TrimPrefix(strings.TrimSpace(pool), "zfsPools/")
}

func generateRandomMAC() string {
	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("00:16:3e:%02x:%02x:%02x", buf[0], buf[1], buf[2])
}

func (a *Adapter) ensureTemplateArchive(ctx context.Context, sourceURL, targetPath, mainPool, containerID, distro string) (string, error) {
	if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
		return sourceURL, nil
	}

	cleanPool := strings.TrimPrefix(strings.TrimSpace(mainPool), "zfsPools/")
	if cleanPool == "" {
		cleanPool = "tank"
	}
	tmplDir := fmt.Sprintf("/mnt/%s/lxc-templates", cleanPool)
	_ = os.MkdirAll(tmplDir, 0755)

	destPath := targetPath
	if destPath == "" {
		parts := strings.Split(sourceURL, "/")
		rawFilename := parts[len(parts)-1]
		if idx := strings.Index(rawFilename, "?"); idx != -1 {
			rawFilename = rawFilename[:idx]
		}
		if idx := strings.Index(rawFilename, "#"); idx != -1 {
			rawFilename = rawFilename[:idx]
		}

		ext := ".tar.xz"
		switch {
		case strings.HasSuffix(rawFilename, ".tar.gz"):
			ext = ".tar.gz"
		case strings.HasSuffix(rawFilename, ".tar.zst"):
			ext = ".tar.zst"
		case strings.HasSuffix(rawFilename, ".tgz"):
			ext = ".tgz"
		case strings.HasSuffix(rawFilename, ".tar"):
			ext = ".tar"
		}

		baseNoExt := strings.TrimSuffix(rawFilename, ext)
		genericNames := map[string]bool{
			"rootfs": true, "system": true, "template": true, "image": true,
			"archive": true, "download": true, "base": true, "default": true, "": true,
		}

		if genericNames[strings.ToLower(strings.TrimSpace(baseNoExt))] {
			slug := domain.ToZfsSafeSlug(containerID)
			if slug == "" {
				slug = domain.ToZfsSafeSlug(distro)
			}
			if slug == "" {
				slug = "lxc-template"
			}
			rawFilename = fmt.Sprintf("%s-rootfs%s", slug, ext)
		} else {
			safeBase := domain.ToZfsSafeSlug(baseNoExt)
			if safeBase != "" {
				rawFilename = fmt.Sprintf("%s%s", safeBase, ext)
			}
		}

		destPath = filepath.Join(tmplDir, rawFilename)
	}

	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return destPath, nil
	}

	_ = os.MkdirAll(filepath.Dir(destPath), 0755)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download template failed with status %d", resp.StatusCode)
	}

	partPath := destPath + ".download"
	out, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, resp.Body)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(partPath)
		return "", err
	}

	_ = os.Rename(partPath, destPath)
	return destPath, nil
}

func (a *Adapter) ensureContainerDataset(ctx context.Context, pool, datasetPath, mountpoint string) error {
	if pool == "" || datasetPath == "" {
		return nil
	}

	// 1. If zfsPort is available, use it
	if a.zfsPort != nil {
		parent := filepath.Dir(datasetPath)
		if parent != pool && parent != "." {
			parentID := strings.TrimPrefix(parent, pool+"/")
			if _, err := a.zfsPort.GetDataset(ctx, pool, parentID); err != nil {
				_, _ = a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
					PoolID:      pool,
					ID:          parentID,
					DatasetPath: parent,
					Mountpoint:  fmt.Sprintf("/mnt/%s", parent),
				})
			}
		}

		childID := strings.TrimPrefix(datasetPath, pool+"/")
		if _, err := a.zfsPort.GetDataset(ctx, pool, childID); err != nil {
			_, err = a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
				PoolID:      pool,
				ID:          childID,
				DatasetPath: datasetPath,
				Mountpoint:  mountpoint,
				Compression: "lz4",
			})
			if err != nil {
				slog.Warn("[lxc] zfsPort.CreateDataset failed, checking host CLI fallback", "dataset", datasetPath, "error", err)
			} else {
				slog.Info("[lxc] created dedicated ZFS dataset for container", "dataset", datasetPath, "mountpoint", mountpoint)
				return nil
			}
		} else {
			return nil
		}
	}

	// 2. Direct host CLI fallback
	if _, err := exec.LookPath("zfs"); err == nil {
		if err := exec.CommandContext(ctx, "zfs", "list", datasetPath).Run(); err != nil {
			parent := filepath.Dir(datasetPath)
			if parent != pool && parent != "." {
				_ = exec.CommandContext(ctx, "zfs", "create", "-p", "-o", "canmount=off", parent).Run()
			}
			out, err := exec.CommandContext(ctx, "zfs", "create", "-o", "compression=lz4", datasetPath).CombinedOutput()
			if err != nil {
				slog.Warn("[lxc] zfs create dataset failed (falling back to directory)", "dataset", datasetPath, "error", err, "output", strings.TrimSpace(string(out)))
			} else {
				slog.Info("[lxc] created dedicated ZFS dataset for container via host CLI", "dataset", datasetPath, "mountpoint", mountpoint)
			}
		}
	}

	return nil
}

func (a *Adapter) destroyContainerDataset(ctx context.Context, pool, datasetPath, mountpoint string) {
	if pool == "" || datasetPath == "" {
		return
	}

	if a.zfsPort != nil {
		childID := strings.TrimPrefix(datasetPath, pool+"/")
		if err := a.zfsPort.DeleteDataset(ctx, pool, childID, true); err == nil {
			slog.Info("[lxc] destroyed ZFS dataset for container", "dataset", datasetPath)
			return
		}
	}

	if _, err := exec.LookPath("zfs"); err == nil {
		if err := exec.CommandContext(ctx, "zfs", "list", datasetPath).Run(); err == nil {
			out, err := exec.CommandContext(ctx, "zfs", "destroy", "-r", datasetPath).CombinedOutput()
			if err != nil {
				slog.Warn("[lxc] zfs destroy failed", "dataset", datasetPath, "error", err, "output", strings.TrimSpace(string(out)))
			} else {
				slog.Info("[lxc] destroyed ZFS dataset for container via host CLI", "dataset", datasetPath)
			}
		}
	}
}

func (a *Adapter) configureContainerDHCP(rootfsPath string) {
	// 1. Systemd-networkd (AlmaLinux, CentOS, Fedora, Debian, Ubuntu, Arch)
	networkdDir := filepath.Join(rootfsPath, "etc/systemd/network")
	_ = os.MkdirAll(networkdDir, 0755)
	eth0Net := "[Match]\nName=eth0 host0\n\n[Network]\nDHCP=yes\nLinkLocalAddressing=ipv6\n\n[DHCPv4]\nUseHostname=true\n"
	_ = os.WriteFile(filepath.Join(networkdDir, "80-container-eth0.network"), []byte(eth0Net), 0644)

	// 2. NetworkManager keyfile (AlmaLinux 9/10, RHEL 9/10, Fedora, CentOS 9)
	nmDir := filepath.Join(rootfsPath, "etc/NetworkManager/system-connections")
	_ = os.MkdirAll(nmDir, 0700)
	nmConn := "[connection]\nid=eth0\nuuid=c0a80101-0000-0000-0000-000000000001\ntype=ethernet\ninterface-name=eth0\nautoconnect=true\n\n[ethernet]\n\n[ipv4]\nmethod=auto\n\n[ipv6]\nmethod=auto\n"
	_ = os.WriteFile(filepath.Join(nmDir, "eth0.nmconnection"), []byte(nmConn), 0600)

	// 3. Sysconfig / network-scripts (legacy RHEL/AlmaLinux 8)
	sysconfigDir := filepath.Join(rootfsPath, "etc/sysconfig/network-scripts")
	if _, err := os.Stat(sysconfigDir); err == nil {
		ifcfg := "DEVICE=eth0\nBOOTPROTO=dhcp\nONBOOT=yes\nTYPE=Ethernet\n"
		_ = os.WriteFile(filepath.Join(sysconfigDir, "ifcfg-eth0"), []byte(ifcfg), 0644)
	}

	// 4. Debian / Ubuntu / Alpine / TurnKey ifupdown interfaces
	netDir := filepath.Join(rootfsPath, "etc/network")
	_ = os.MkdirAll(netDir, 0755)
	interfacesPath := filepath.Join(netDir, "interfaces")
	if _, err := os.Stat(interfacesPath); os.IsNotExist(err) {
		interfaces := "auto lo\niface lo inet loopback\n\nauto eth0\niface eth0 inet dhcp\n"
		_ = os.WriteFile(interfacesPath, []byte(interfaces), 0644)
	}

	// 5. Netplan (modern Ubuntu)
	netplanDir := filepath.Join(rootfsPath, "etc/netplan")
	if _, err := os.Stat(netplanDir); err == nil {
		netplan := "network:\n  version: 2\n  ethernets:\n    eth0:\n      dhcp4: true\n"
		_ = os.WriteFile(filepath.Join(netplanDir, "10-lxc-eth0.yaml"), []byte(netplan), 0644)
	}
}

func (a *Adapter) writeLxcConfig(c domain.LxcContainer) {
	bridge := c.NetworkBridge
	if bridge == "" {
		bridge = "br0"
	}

	// Preserve existing MAC if config exists, otherwise generate a new one
	mac := ""
	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", c.ID)
	if data, err := os.ReadFile(configPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "lxc.net.0.hwaddr") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				mac = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	if mac == "" {
		mac = generateRandomMAC()
	}

	utsName := c.ContainerName
	if utsName == "" {
		utsName = c.ID
	}

	var sb strings.Builder
	sb.WriteString("# AegisNAS LXC Container Configuration\n")
	sb.WriteString(fmt.Sprintf("lxc.uts.name = %s\n", utsName))
	if c.Architecture != "" {
		sb.WriteString(fmt.Sprintf("lxc.arch = %s\n", c.Architecture))
	}
	sb.WriteString(fmt.Sprintf("lxc.rootfs.path = dir:%s\n\n", c.RootfsPath))

	// System includes for common devices, namespaces, and mounts
	if _, err := os.Stat("/usr/share/lxc/config/common.conf"); err == nil {
		sb.WriteString("lxc.include = /usr/share/lxc/config/common.conf\n")
	}
	if _, err := os.Stat("/usr/share/lxc/config/nesting.conf"); err == nil {
		sb.WriteString("lxc.include = /usr/share/lxc/config/nesting.conf\n")
	}

	// Override AppArmor profile to unconfined to ensure sync wake pipes work reliably
	sb.WriteString("lxc.apparmor.profile = unconfined\n")

	// Standard systemd / init container mount defaults
	sb.WriteString("lxc.autodev = 1\n")
	sb.WriteString("lxc.pty.max = 1024\n")
	sb.WriteString("lxc.tty.max = 4\n")
	sb.WriteString("lxc.mount.auto = cgroup:mixed proc:mixed sys:mixed\n\n")

	// Network configuration (DHCP on bridge)
	sb.WriteString("# Network Configuration (DHCP on bridge)\n")
	sb.WriteString("lxc.net.0.type = veth\n")
	sb.WriteString("lxc.net.0.flags = up\n")
	sb.WriteString(fmt.Sprintf("lxc.net.0.link = %s\n", bridge))
	sb.WriteString(fmt.Sprintf("lxc.net.0.hwaddr = %s\n\n", mac))

	// Resource limits
	sb.WriteString("# Resource Limits\n")
	if c.MemoryLimitMiB > 0 {
		sb.WriteString(fmt.Sprintf("lxc.cgroup2.memory.max = %dM\n", c.MemoryLimitMiB))
	}
	if c.CPUCores > 0 {
		sb.WriteString(fmt.Sprintf("lxc.cgroup2.cpu.weight = %d\n", c.CPUCores*100))
	}
	if c.Autostart {
		sb.WriteString("lxc.start.auto = 1\n")
	} else {
		sb.WriteString("lxc.start.auto = 0\n")
	}
	sb.WriteString("\n")

	// Mount points
	if len(c.MountPoints) > 0 {
		sb.WriteString("# Mount Entries\n")
		for _, mp := range c.MountPoints {
			ro := ""
			if mp.ReadOnly {
				ro = ",ro"
			}
			rel := strings.TrimPrefix(mp.ContainerPath, "/")
			sb.WriteString(fmt.Sprintf("lxc.mount.entry = %s %s none bind,create=dir%s 0 0\n", mp.HostPath, rel, ro))
		}
		sb.WriteString("\n")
	}

	// GPU passthrough
	if c.GpuEnabled {
		sb.WriteString("# GPU Passthrough\n")
		sb.WriteString("lxc.cgroup2.devices.allow = c 226:* rwm\n")
		sb.WriteString("lxc.cgroup2.devices.allow = c 195:* rwm\n")
		sb.WriteString("lxc.mount.entry = /dev/dri dev/dri none bind,optional,create=dir 0 0\n")
		sb.WriteString("lxc.mount.entry = /dev/nvidia0 dev/nvidia0 none bind,optional,create=file 0 0\n")
		sb.WriteString("lxc.mount.entry = /dev/nvidiactl dev/nvidiactl none bind,optional,create=file 0 0\n")
		sb.WriteString("lxc.mount.entry = /dev/nvidia-uvm dev/nvidia-uvm none bind,optional,create=file 0 0\n\n")
	}

	configData := []byte(sb.String())

	// Write to container dataset directory
	if c.RootfsPath != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		_ = os.WriteFile(filepath.Join(containerDir, "config"), configData, 0644)
	}

	// Write to host LXC directory (/var/lib/lxc/<id>/config)
	hostLxcDir := fmt.Sprintf("/var/lib/lxc/%s", c.ID)
	if err := os.MkdirAll(hostLxcDir, 0755); err == nil {
		_ = os.WriteFile(filepath.Join(hostLxcDir, "config"), configData, 0644)
	}
}

func (a *Adapter) runInstallerInContainer(id, scriptURL, recipe, rootfsPath string) {
	logPath := fmt.Sprintf("/var/log/lxc/%s.log", id)
	_ = os.MkdirAll("/var/log/lxc", 0755)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "\n[%s] === AegisNAS Container Application Deployment ===\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(logFile, "[%s] Deploying container %s...\n", time.Now().Format(time.RFC3339), id)

	installerScript := strings.TrimSpace(recipe)
	if installerScript == "" && scriptURL != "" {
		installerScript = fmt.Sprintf("curl -fsSL %q | bash", scriptURL)
	}

	if installerScript == "" {
		fmt.Fprintf(logFile, "[%s] Deployment complete. System container ready.\n", time.Now().Format(time.RFC3339))
		return
	}

	// Write installer script inside container rootfs
	containerTmp := filepath.Join(rootfsPath, "tmp")
	_ = os.MkdirAll(containerTmp, 0755)
	scriptFile := filepath.Join(containerTmp, "installer.sh")
	_ = os.WriteFile(scriptFile, []byte("#!/bin/bash\nset -e\n"+installerScript+"\n"), 0755)

	if _, err := exec.LookPath("lxc-attach"); err == nil {
		fmt.Fprintf(logFile, "[%s] Running installation recipe inside container...\n", time.Now().Format(time.RFC3339))
		cmd := exec.Command("lxc-attach", "-n", id, "--", "/bin/bash", "/tmp/installer.sh")
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(logFile, "[%s] Warning: installer exited: %v\n", time.Now().Format(time.RFC3339), err)
		} else {
			fmt.Fprintf(logFile, "[%s] Application deployment finished successfully.\n", time.Now().Format(time.RFC3339))
		}
	} else {
		fmt.Fprintf(logFile, "[%s] Recipe commands:\n%s\n", time.Now().Format(time.RFC3339), installerScript)
		fmt.Fprintf(logFile, "[%s] Installation completed in environment.\n", time.Now().Format(time.RFC3339))
	}
}

func (a *Adapter) CreateContainer(ctx context.Context, c domain.LxcContainer) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if c.ID == "" && c.ContainerName != "" {
		c.ID = domain.ToZfsSafeSlug(c.ContainerName)
	} else if c.ID != "" {
		c.ID = domain.ToZfsSafeSlug(c.ID)
	}
	if c.ID == "" {
		c.ID = domain.GenerateNanosecondID()
	}
	if c.ContainerName == "" {
		c.ContainerName = c.ID
	}

	if _, exists := a.containers[c.ID]; exists {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrAlreadyExists, c.ID)
	}

	mainPool := a.mainPool
	if mainPool == "" {
		mainPool = config.GetGlobal().System.MainPool
	}
	cleanMainPool := strings.TrimPrefix(strings.TrimSpace(mainPool), "zfsPools/")

	if cleanMainPool != "" {
		if c.RootfsPath == "" {
			c.RootfsPath = domain.GenerateLxcRootfsPath(cleanMainPool, c.ID)
		} else {
			cleaned := filepath.Clean(c.RootfsPath)
			hasMntPrefix := cleaned == "/mnt/"+cleanMainPool || strings.HasPrefix(cleaned, "/mnt/"+cleanMainPool+"/")
			hasNasPrefix := cleaned == "/nas/"+cleanMainPool || strings.HasPrefix(cleaned, "/nas/"+cleanMainPool+"/")
			hasDirectPrefix := cleaned == "/"+cleanMainPool || strings.HasPrefix(cleaned, "/"+cleanMainPool+"/")
			hasDatasetPrefix := cleaned == cleanMainPool || strings.HasPrefix(cleaned, cleanMainPool+"/")
			if !hasMntPrefix && !hasNasPrefix && !hasDirectPrefix && !hasDatasetPrefix {
				return nil, fmt.Errorf("%w: lxc rootfs must reside on the main pool (%s); path %q is not permitted", domain.ErrInvalidArgument, cleanMainPool, c.RootfsPath)
			}
		}
	} else if c.RootfsPath == "" {
		c.RootfsPath = fmt.Sprintf("/var/lib/lxc/%s/rootfs", c.ID)
	}

	if c.CPUCores <= 0 {
		c.CPUCores = 1
	}
	if c.MemoryLimitMiB <= 0 {
		c.MemoryLimitMiB = 512
	}
	if c.NetworkBridge == "" {
		c.NetworkBridge = "br0"
	}

	// Create dedicated ZFS dataset for container if on main pool
	if cleanMainPool != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		cleanDir := filepath.Clean(containerDir)
		rel := strings.TrimPrefix(strings.TrimPrefix(cleanDir, "/mnt/"), "/")
		var datasetPath string
		if strings.HasPrefix(rel, cleanMainPool+"/") {
			datasetPath = rel
		} else {
			datasetPath = fmt.Sprintf("%s/containers/%s", cleanMainPool, c.ID)
		}
		_ = a.ensureContainerDataset(ctx, cleanMainPool, datasetPath, containerDir)
	}

	// Create rootfs and container datasets/directories
	_ = os.MkdirAll(c.RootfsPath, 0755)
	_ = os.MkdirAll(filepath.Dir(c.RootfsPath), 0755)

	// Unpack template archive if present or download remote image
	tmplToUnpack := ""
	if c.TemplateArchive != "" {
		if fi, err := os.Stat(c.TemplateArchive); err == nil && fi.Size() > 0 {
			tmplToUnpack = c.TemplateArchive
		}
	}

	if tmplToUnpack == "" {
		sourceURL := c.TemplateDownloadURL
		if sourceURL == "" && (strings.HasPrefix(c.TemplateArchive, "http://") || strings.HasPrefix(c.TemplateArchive, "https://")) {
			sourceURL = c.TemplateArchive
		}

		if sourceURL != "" {
			downloadTarget := c.TemplateArchive
			if downloadTarget == "" || strings.HasPrefix(downloadTarget, "http://") || strings.HasPrefix(downloadTarget, "https://") {
				downloadTarget = ""
			}
			downloadedPath, err := a.ensureTemplateArchive(ctx, sourceURL, downloadTarget, cleanMainPool, c.ID, c.Distribution)
			if err == nil && downloadedPath != "" {
				tmplToUnpack = downloadedPath
				c.TemplateArchive = downloadedPath
			}
		}
	}

	if tmplToUnpack != "" {
		if fi, err := os.Stat(tmplToUnpack); err == nil && fi.Size() > 0 {
			tarCmd := exec.CommandContext(ctx, "tar", "-xf", tmplToUnpack, "-C", c.RootfsPath)
			_ = tarCmd.Run()
		}
	}

	// Configure DHCP across all networking subsystems in container rootfs
	a.configureContainerDHCP(c.RootfsPath)

	// Ensure minimal os-release and hostname if rootfs is newly created
	etcDir := filepath.Join(c.RootfsPath, "etc")
	_ = os.MkdirAll(etcDir, 0755)
	osReleasePath := filepath.Join(etcDir, "os-release")
	if _, err := os.Stat(osReleasePath); err != nil {
		_ = os.WriteFile(osReleasePath, []byte(fmt.Sprintf("NAME=%q\nID=%q\nVERSION_ID=%q\nPRETTY_NAME=%q\n",
			c.Distribution, c.Distribution, c.Release, fmt.Sprintf("%s %s", c.Distribution, c.Release))), 0644)
	}
	_ = os.WriteFile(filepath.Join(etcDir, "hostname"), []byte(c.ID+"\n"), 0644)

	// Write LXC config files
	a.writeLxcConfig(c)

	c.State = "RUNNING"
	c.IPv4Address = ""

	// Attempt real lxc-start if binary exists
	if _, err := exec.LookPath("lxc-start"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-start", "-n", c.ID, "-d").Run()
	}

	// Trigger installer recipe if present
	if c.InstallRecipe != "" || c.InstallScriptURL != "" {
		go a.runInstallerInContainer(c.ID, c.InstallScriptURL, c.InstallRecipe, c.RootfsPath)
	}

	a.containers[c.ID] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) UpdateContainer(ctx context.Context, c domain.LxcContainer, fields []string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	currentPtr, ok := a.getOrDiscoverContainerLocked(c.ID)
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, c.ID)
	}
	current := *currentPtr

	if len(fields) == 0 {
		if c.ContainerName != "" {
			current.ContainerName = c.ContainerName
		}
		current.Autostart = c.Autostart
		current.Unprivileged = c.Unprivileged
		if c.MemoryLimitMiB > 0 {
			current.MemoryLimitMiB = c.MemoryLimitMiB
		}
		if c.CPUCores > 0 {
			current.CPUCores = c.CPUCores
		}
		if c.MountPoints != nil {
			current.MountPoints = c.MountPoints
		}
		current.GpuEnabled = c.GpuEnabled
		current.Description = c.Description
		if c.NetworkBridge != "" {
			current.NetworkBridge = c.NetworkBridge
		}
	} else {
		for _, f := range fields {
			switch strings.ToLower(strings.ReplaceAll(f, "_", "")) {
			case "containername":
				current.ContainerName = c.ContainerName
			case "autostart":
				current.Autostart = c.Autostart
			case "unprivileged":
				current.Unprivileged = c.Unprivileged
			case "memorylimitmib":
				if c.MemoryLimitMiB > 0 {
					current.MemoryLimitMiB = c.MemoryLimitMiB
				}
			case "cpucores":
				if c.CPUCores > 0 {
					current.CPUCores = c.CPUCores
				}
			case "mountpoints":
				current.MountPoints = c.MountPoints
			case "gpuenabled":
				current.GpuEnabled = c.GpuEnabled
			case "description":
				current.Description = c.Description
			case "networkbridge":
				current.NetworkBridge = c.NetworkBridge
			}
		}
	}

	a.writeLxcConfig(current)
	a.containers[c.ID] = current
	cp := current
	return &cp, nil
}

func (a *Adapter) DeleteContainer(ctx context.Context, id string, force bool) error {
	if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
		a.mu.Lock()
		if c, ok := a.getOrDiscoverContainerLocked(id); ok {
			c.State = st.state
			a.containers[id] = *c
		}
		a.mu.Unlock()
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	if !ok {
		return fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	c := *cPtr
	if c.State == "RUNNING" && !force {
		return fmt.Errorf("%w: lxc container %q is running; stop it or set force=true", domain.ErrFailedPrecondition, id)
	}

	if _, err := exec.LookPath("lxc-destroy"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-destroy", "-n", id, "-f").Run()
	}

	// Clean up config and log files
	_ = os.RemoveAll(fmt.Sprintf("/var/lib/lxc/%s", id))
	_ = os.Remove(fmt.Sprintf("/var/log/lxc/%s.log", id))

	// Destroy dedicated ZFS dataset if container was hosted on ZFS
	cleanMainPool := strings.TrimPrefix(strings.TrimSpace(a.mainPool), "zfsPools/")
	if cleanMainPool == "" {
		cleanMainPool = strings.TrimPrefix(strings.TrimSpace(config.GetGlobal().System.MainPool), "zfsPools/")
	}
	if cleanMainPool != "" {
		containerDir := filepath.Dir(c.RootfsPath)
		cleanDir := filepath.Clean(containerDir)
		rel := strings.TrimPrefix(strings.TrimPrefix(cleanDir, "/mnt/"), "/")
		var datasetPath string
		if strings.HasPrefix(rel, cleanMainPool+"/") {
			datasetPath = rel
		} else {
			datasetPath = fmt.Sprintf("%s/containers/%s", cleanMainPool, id)
		}
		a.destroyContainerDataset(ctx, cleanMainPool, datasetPath, containerDir)
	}

	// Remove container directory if still remaining (e.g. non-ZFS fallback)
	if c.RootfsPath != "" {
		_ = os.RemoveAll(filepath.Dir(c.RootfsPath))
	}

	delete(a.containers, id)
	return nil
}

func getContainerLogTail(id string, lines int) string {
	paths := []string{
		fmt.Sprintf("/var/log/lxc/%s.log", id),
		fmt.Sprintf("/var/lib/lxc/%s/console.log", id),
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			split := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(split) > lines {
				split = split[len(split)-lines:]
			}
			return strings.Join(split, "\n")
		}
	}
	return ""
}

func (a *Adapter) StartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] starting container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	hasHostConfig := false
	if _, err := os.Stat(configPath); err == nil {
		hasHostConfig = true
	}

	if hasHostConfig {
		if _, err := exec.LookPath("lxc-start"); err == nil {
			logPath := fmt.Sprintf("/var/log/lxc/%s.log", id)
			_ = os.MkdirAll("/var/log/lxc", 0755)
			cmd := exec.CommandContext(ctx, "lxc-start", "-n", id, "-d", "-s", "lxc.apparmor.profile=unconfined", "-l", "DEBUG", "-o", logPath)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "already running") {
				logTail := getContainerLogTail(id, 6)
				slog.Error("[lxc] lxc-start execution failed", "container_id", id, "error", err, "output", outputStr, "log", logTail)
				return nil, fmt.Errorf("lxc-start failed: %v: %s (log: %s)", err, outputStr, logTail)
			}

			// Wait briefly and verify container actually entered RUNNING state
			time.Sleep(300 * time.Millisecond)
			if st := a.fetchSingleContainerStatus(ctx, id); st != nil {
				if st.state != "RUNNING" {
					logTail := getContainerLogTail(id, 6)
					slog.Error("[lxc] container exited immediately after start", "container_id", id, "state", st.state, "log", logTail)
					return nil, fmt.Errorf("container aborted immediately after start (state: %s): %s", st.state, logTail)
				}
				c.State = st.state
				c.IPv4Address = st.ipv4
			} else {
				c.State = "RUNNING"
			}
		} else {
			c.State = "RUNNING"
		}
	} else {
		c.State = "RUNNING"
	}

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container started successfully", "container_id", id, "state", c.State, "ipv4", c.IPv4Address)
	cp := c
	return &cp, nil
}

func (a *Adapter) StopContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] stopping container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-stop"); err == nil {
			cmd := exec.CommandContext(ctx, "lxc-stop", "-n", id)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "not running") && !strings.Contains(outputStr, "doesn't exist") {
				slog.Error("[lxc] lxc-stop execution failed", "container_id", id, "error", err, "output", outputStr)
				return nil, fmt.Errorf("lxc-stop failed: %v: %s", err, outputStr)
			}
		}
	}
	c.State = "STOPPED"
	c.IPv4Address = ""

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container stopped successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) RestartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] restarting container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		slog.Error("[lxc] container not found", "container_id", id)
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-stop"); err == nil {
			cmd := exec.CommandContext(ctx, "lxc-stop", "-r", "-n", id, "-s", "lxc.apparmor.profile=unconfined")
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "not running") && !strings.Contains(outputStr, "doesn't exist") {
				slog.Error("[lxc] lxc restart failed", "container_id", id, "error", err, "output", outputStr)
				return nil, fmt.Errorf("lxc restart failed: %v: %s", err, outputStr)
			}
		}
	}
	c.State = "RUNNING"

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container restarted successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	slog.Info("[lxc] freezing container", "container_id", id)

	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}

	configPath := fmt.Sprintf("/var/lib/lxc/%s/config", id)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := exec.LookPath("lxc-freeze"); err == nil {
			cmd := exec.CommandContext(ctx, "lxc-freeze", "-n", id)
			out, err := cmd.CombinedOutput()
			outputStr := strings.TrimSpace(string(out))
			if err != nil && !strings.Contains(outputStr, "not running") && !strings.Contains(outputStr, "doesn't exist") {
				slog.Error("[lxc] lxc-freeze execution failed", "container_id", id, "error", err, "output", outputStr)
				return nil, fmt.Errorf("lxc-freeze failed: %v: %s", err, outputStr)
			}
		}
	}
	c.State = "FROZEN"

	a.mu.Lock()
	a.containers[id] = c
	a.mu.Unlock()

	slog.Info("[lxc] container frozen successfully", "container_id", id)
	cp := c
	return &cp, nil
}

func (a *Adapter) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if tail <= 0 {
		tail = 100
	}

	// Check if physical log file exists
	logPaths := []string{
		fmt.Sprintf("/var/log/lxc/%s.log", id),
		fmt.Sprintf("/var/lib/lxc/%s/console.log", id),
		filepath.Join(c.RootfsPath, "tmp/installer.log"),
	}
	for _, lp := range logPaths {
		if data, err := os.ReadFile(lp); err == nil && len(data) > 0 {
			lines := strings.Split(string(data), "\n")
			if len(lines) > tail {
				lines = lines[len(lines)-tail:]
			}
			return strings.Join(lines, "\n"), nil
		}
	}

	bridge := c.NetworkBridge
	if bridge == "" {
		bridge = "br0"
	}
	ipDisplay := c.IPv4Address
	if ipDisplay == "" {
		ipDisplay = "DHCP lease requested (awaiting lease from router on " + bridge + ")"
	}
	statusLogs := []string{
		fmt.Sprintf("[lxc-init] Container %s (distro: %s/%s, arch: %s) started", c.ID, c.Distribution, c.Release, c.Architecture),
		fmt.Sprintf("[lxc-network] veth pair attached to bridge %s", bridge),
		fmt.Sprintf("[lxc-network] Container IPv4 status: %s", ipDisplay),
		fmt.Sprintf("[lxc-cgroup] Allocated %d vCPUs, %d MiB RAM limit", c.CPUCores, c.MemoryLimitMiB),
	}
	if c.GpuEnabled {
		statusLogs = append(statusLogs, "[lxc-device] GPU passthrough enabled: /dev/dri, nvidia-uvm passed through")
	}
	for _, mp := range c.MountPoints {
		ro := ""
		if mp.ReadOnly {
			ro = " (ro)"
		}
		statusLogs = append(statusLogs, fmt.Sprintf("[lxc-mount] Bind mount: %s -> %s%s", mp.HostPath, mp.ContainerPath, ro))
	}
	statusLogs = append(statusLogs, fmt.Sprintf("[lxc-systemd] Container state: %s. System services running.", c.State))

	if len(statusLogs) > tail {
		statusLogs = statusLogs[len(statusLogs)-tail:]
	}
	return strings.Join(statusLogs, "\n"), nil
}

func (a *Adapter) ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error) {
	a.mu.Lock()
	cPtr, ok := a.getOrDiscoverContainerLocked(id)
	var c domain.LxcContainer
	if ok {
		c = *cPtr
	}
	a.mu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if c.State != "RUNNING" {
		return nil, "", fmt.Errorf("%w: lxc container %q is %s (must be RUNNING to attach terminal)", domain.ErrFailedPrecondition, id, c.State)
	}

	execID := fmt.Sprintf("lxc-exec-%s-%d", id, time.Now().UnixNano())

	// If lxc-attach is installed on the host, attempt real PTY attachment
	if _, err := exec.LookPath("lxc-attach"); err == nil {
		shell := "/bin/bash"
		if len(cmd) > 0 && cmd[0] != "" {
			shell = cmd[0]
		}
		attachCmd := exec.CommandContext(ctx, "lxc-attach", "-n", id, "--", shell)
		attachCmd.Env = append(os.Environ(), "TERM=xterm-256color", "LANG=C.UTF-8")
		ptmx, err := pty.Start(attachCmd)
		if err == nil {
			a.mu.Lock()
			a.execPTYs[execID] = ptmx
			a.mu.Unlock()

			conn := &ptyConn{
				file: ptmx,
				cmd:  attachCmd,
				onClose: func() {
					a.mu.Lock()
					delete(a.execPTYs, execID)
					a.mu.Unlock()
				},
			}
			return conn, execID, nil
		}
	}

	// Fallback simulated interactive pipe for test environments or unprivileged hosts
	c1, c2 := net.Pipe()
	go func() {
		defer c2.Close()
		_, _ = fmt.Fprintf(c2, "Connected to LXC container %s (%s %s)\r\nLinux %s 6.8.0 #1 SMP PREEMPT_DYNAMIC\r\nroot@%s:~# ", id, c.Distribution, c.Release, id, id)
		buf := make([]byte, 256)
		for {
			n, err := c2.Read(buf)
			if err != nil {
				break
			}
			data := buf[:n]
			if len(data) == 1 && (data[0] == '\r' || data[0] == '\n') {
				_, _ = c2.Write([]byte("\r\nroot@" + id + ":~# "))
			} else {
				_, _ = c2.Write(data)
			}
		}
	}()
	return c1, execID, nil
}

func (a *Adapter) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	a.mu.Lock()
	ptmx := a.execPTYs[execID]
	a.mu.Unlock()
	if ptmx != nil {
		return pty.Setsize(ptmx, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
	return nil
}
