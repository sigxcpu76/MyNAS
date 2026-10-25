package application

import (
	"context"
	"net"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type DockerService struct {
	runtime ports.DockerRuntimePort
}

func NewDockerService(runtime ports.DockerRuntimePort) *DockerService {
	return &DockerService{runtime: runtime}
}

func (s *DockerService) ListContainers(ctx context.Context) ([]domain.DockerContainer, error) {
	return s.runtime.ListContainers(ctx)
}

func (s *DockerService) GetContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return nil, err
	}
	return s.runtime.GetContainer(ctx, id)
}

func (s *DockerService) CreateContainer(ctx context.Context, c domain.DockerContainer) (*domain.DockerContainer, error) {
	if c.ID == "" {
		c.ID = domain.GenerateNanosecondID()
	}
	if c.ContainerName == "" {
		c.ContainerName = c.ID
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return s.runtime.CreateContainer(ctx, c)
}

func (s *DockerService) UpdateContainer(ctx context.Context, c domain.DockerContainer, fields []string) (*domain.DockerContainer, error) {
	if err := domain.ValidateResourceID(c.ID, "docker_container"); err != nil {
		return nil, err
	}
	if c.Image == "" {
		existing, err := s.runtime.GetContainer(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		c.Image = existing.Image
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return s.runtime.UpdateContainer(ctx, c, fields)
}

func (s *DockerService) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 100
	}
	return s.runtime.GetContainerLogs(ctx, id, tail)
}

func (s *DockerService) DeleteContainer(ctx context.Context, id string, force bool) error {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return err
	}
	return s.runtime.DeleteContainer(ctx, id, force)
}

func (s *DockerService) StartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return nil, err
	}
	return s.runtime.StartContainer(ctx, id)
}

func (s *DockerService) StopContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return nil, err
	}
	return s.runtime.StopContainer(ctx, id)
}

func (s *DockerService) RestartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return nil, err
	}
	return s.runtime.RestartContainer(ctx, id)
}

func (s *DockerService) ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error) {
	if err := domain.ValidateResourceID(id, "docker_container"); err != nil {
		return nil, "", err
	}
	return s.runtime.ExecAttach(ctx, id, cmd)
}

func (s *DockerService) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	return s.runtime.ResizeExec(ctx, execID, height, width)
}

type LxcService struct {
	runtime ports.LxcRuntimePort
}

func NewLxcService(runtime ports.LxcRuntimePort) *LxcService {
	return &LxcService{runtime: runtime}
}

func (s *LxcService) ListContainers(ctx context.Context) ([]domain.LxcContainer, error) {
	return s.runtime.ListContainers(ctx)
}

func (s *LxcService) GetContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	if err := domain.ValidateResourceID(id, "lxc_container"); err != nil {
		return nil, err
	}
	return s.runtime.GetContainer(ctx, id)
}

func (s *LxcService) CreateContainer(ctx context.Context, c domain.LxcContainer) (*domain.LxcContainer, error) {
	if c.ID == "" {
		c.ID = domain.GenerateNanosecondID()
	}
	if c.ContainerName == "" {
		c.ContainerName = c.ID
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return s.runtime.CreateContainer(ctx, c)
}

func (s *LxcService) DeleteContainer(ctx context.Context, id string, force bool) error {
	if err := domain.ValidateResourceID(id, "lxc_container"); err != nil {
		return err
	}
	return s.runtime.DeleteContainer(ctx, id, force)
}

func (s *LxcService) StartContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	if err := domain.ValidateResourceID(id, "lxc_container"); err != nil {
		return nil, err
	}
	return s.runtime.StartContainer(ctx, id)
}

func (s *LxcService) StopContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	if err := domain.ValidateResourceID(id, "lxc_container"); err != nil {
		return nil, err
	}
	return s.runtime.StopContainer(ctx, id)
}

func (s *LxcService) FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	if err := domain.ValidateResourceID(id, "lxc_container"); err != nil {
		return nil, err
	}
	return s.runtime.FreezeContainer(ctx, id)
}
