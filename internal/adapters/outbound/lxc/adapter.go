package lxc

import (
	"context"
	"fmt"
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
			hasNasPrefix := cleaned == "/nas/"+mainPool || strings.HasPrefix(cleaned, "/nas/"+mainPool+"/")
			hasDirectPrefix := cleaned == "/"+mainPool || strings.HasPrefix(cleaned, "/"+mainPool+"/")
			hasDatasetPrefix := cleaned == mainPool || strings.HasPrefix(cleaned, mainPool+"/")
			if !hasNasPrefix && !hasDirectPrefix && !hasDatasetPrefix {
				return nil, fmt.Errorf("%w: lxc rootfs must reside on the main pool (%s); path %q is not permitted", domain.ErrInvalidArgument, mainPool, c.RootfsPath)
			}
		}
	} else if c.RootfsPath == "" {
		c.RootfsPath = fmt.Sprintf("/var/lib/lxc/%s/rootfs", c.ID)
	}

	a.nextIP++
	c.State = "RUNNING"
	c.IPv4Address = fmt.Sprintf("10.10.20.%d", a.nextIP)
	a.containers[c.ID] = c
	cp := c
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
