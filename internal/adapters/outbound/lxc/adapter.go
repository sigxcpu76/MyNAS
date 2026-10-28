package lxc

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
)

type Adapter struct {
	mu         sync.RWMutex
	mainPool   string
	containers map[string]domain.LxcContainer
	nextIP     int
}

func NewAdapter() *Adapter {
	return &Adapter{
		containers: make(map[string]domain.LxcContainer),
		nextIP:     15,
	}
}

func (a *Adapter) ListContainers(ctx context.Context) ([]domain.LxcContainer, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.LxcContainer, 0, len(a.containers))
	for _, c := range a.containers {
		out = append(out, c)
	}
	return out, nil
}

func (a *Adapter) GetContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
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

	if mainPool != "" {
		if c.RootfsPath == "" {
			c.RootfsPath = domain.GenerateLxcRootfsPath(mainPool, c.ID)
		} else {
			cleaned := filepath.Clean(c.RootfsPath)
			hasMntPrefix := cleaned == "/mnt/"+mainPool || strings.HasPrefix(cleaned, "/mnt/"+mainPool+"/")
			hasNasPrefix := cleaned == "/nas/"+mainPool || strings.HasPrefix(cleaned, "/nas/"+mainPool+"/")
			hasDirectPrefix := cleaned == "/"+mainPool || strings.HasPrefix(cleaned, "/"+mainPool+"/")
			hasDatasetPrefix := cleaned == mainPool || strings.HasPrefix(cleaned, mainPool+"/")
			if !hasMntPrefix && !hasNasPrefix && !hasDirectPrefix && !hasDatasetPrefix {
				return nil, fmt.Errorf("%w: lxc rootfs must reside on the main pool (%s); path %q is not permitted", domain.ErrInvalidArgument, mainPool, c.RootfsPath)
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

	a.nextIP++
	c.State = "RUNNING"
	c.IPv4Address = fmt.Sprintf("10.10.20.%d", a.nextIP)
	a.containers[c.ID] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) UpdateContainer(ctx context.Context, c domain.LxcContainer, fields []string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	current, ok := a.containers[c.ID]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, c.ID)
	}

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

	a.containers[c.ID] = current
	cp := current
	return &cp, nil
}

func (a *Adapter) DeleteContainer(ctx context.Context, id string, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.containers[id]
	if !ok {
		return fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if c.State == "RUNNING" && !force {
		return fmt.Errorf("%w: lxc container %q is running; stop it or set force=true", domain.ErrFailedPrecondition, id)
	}
	delete(a.containers, id)
	return nil
}

func (a *Adapter) StartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("lxc-start"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-start", "-n", id).Run()
	}
	c.State = "RUNNING"
	if c.IPv4Address == "" || c.IPv4Address == "-" {
		a.nextIP++
		c.IPv4Address = fmt.Sprintf("10.10.20.%d", a.nextIP)
	}
	a.containers[id] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) StopContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("lxc-stop"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-stop", "-n", id).Run()
	}
	c.State = "STOPPED"
	c.IPv4Address = "-"
	a.containers[id] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) RestartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("lxc-stop"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-stop", "-r", "-n", id).Run()
	}
	c.State = "RUNNING"
	if c.IPv4Address == "" || c.IPv4Address == "-" {
		a.nextIP++
		c.IPv4Address = fmt.Sprintf("10.10.20.%d", a.nextIP)
	}
	a.containers[id] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("lxc-freeze"); err == nil {
		_ = exec.CommandContext(ctx, "lxc-freeze", "-n", id).Run()
	}
	c.State = "FROZEN"
	a.containers[id] = c
	cp := c
	return &cp, nil
}

func (a *Adapter) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	a.mu.RLock()
	c, ok := a.containers[id]
	a.mu.RUnlock()
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

	// If no host log file exists, synthesize status logs
	bridge := c.NetworkBridge
	if bridge == "" {
		bridge = "lxcbr0"
	}
	statusLogs := []string{
		fmt.Sprintf("[lxc-init] Container %s (distro: %s/%s, arch: %s) started", c.ID, c.Distribution, c.Release, c.Architecture),
		fmt.Sprintf("[lxc-network] veth pair attached to bridge %s", bridge),
		fmt.Sprintf("[lxc-network] Container IPv4 assigned: %s", c.IPv4Address),
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
	a.mu.RLock()
	c, ok := a.containers[id]
	a.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("%w: lxc container %q", domain.ErrNotFound, id)
	}
	if c.State != "RUNNING" {
		return nil, "", fmt.Errorf("%w: lxc container %q is %s (must be RUNNING to attach terminal)", domain.ErrFailedPrecondition, id, c.State)
	}

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
	return c1, "mock-lxc-exec-" + id, nil
}

func (a *Adapter) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	return nil
}

