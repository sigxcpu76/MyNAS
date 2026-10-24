package domain

import "fmt"

type DockerContainer struct {
	ID             string
	ContainerID    string
	ContainerName  string
	Image          string
	State          string // RUNNING, EXITED, CREATED, PAUSED
	StatusText     string
	Ports          []string
	Volumes        []string
	Environment    []string
	RestartPolicy  string
	NetworkMode    string
	CPUPercent     float64
	MemoryUsageMiB int64
	CreatedAt      string
	MemoryLimitMiB int64
	Command        []string
	Privileged     bool
	ComposeYAML    string
}

func (c *DockerContainer) Validate() error {
	if err := ValidateResourceID(c.ID, "docker_container_id"); err != nil {
		return err
	}
	if c.Image == "" && c.ComposeYAML == "" {
		return fmt.Errorf("%w: image or compose_yaml is required for docker container %q", ErrInvalidArgument, c.ID)
	}
	if c.RestartPolicy == "" {
		c.RestartPolicy = "unless-stopped"
	}
	if c.NetworkMode == "" {
		c.NetworkMode = "bridge"
	}
	return nil
}

type LxcContainer struct {
	ID             string
	ContainerName  string
	State          string // RUNNING, STOPPED, FROZEN
	Distribution   string // debian, ubuntu, alpine, archlinux
	Release        string // bookworm, noble, 3.20, current
	Architecture   string // amd64, arm64
	IPv4Address    string
	Autostart      bool
	Unprivileged   bool
	RootfsPath     string
	MemoryLimitMiB int64
	CPUCores       int32
}

func (c *LxcContainer) Validate() error {
	if err := ValidateResourceID(c.ID, "lxc_container_id"); err != nil {
		return err
	}
	if c.Distribution == "" {
		return fmt.Errorf("%w: distribution is required for LXC container %q", ErrInvalidArgument, c.ID)
	}
	if c.Release == "" {
		c.Release = "stable"
	}
	if c.Architecture == "" {
		c.Architecture = "amd64"
	}
	if c.MemoryLimitMiB <= 0 {
		c.MemoryLimitMiB = 512
	}
	if c.CPUCores <= 0 {
		c.CPUCores = 1
	}
	return nil
}
