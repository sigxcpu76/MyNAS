package application

import (
	"context"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

// DockerRegistryService exposes container registry credential management use-cases.
type DockerRegistryService struct {
	port ports.DockerRegistryPort
}

// NewDockerRegistryService creates a DockerRegistryService.
func NewDockerRegistryService(port ports.DockerRegistryPort) *DockerRegistryService {
	return &DockerRegistryService{port: port}
}

func registryID(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "dockerRegistries/")
}

func (s *DockerRegistryService) List(ctx context.Context) ([]domain.DockerRegistry, error) {
	return s.port.ListRegistries(ctx)
}

func (s *DockerRegistryService) Get(ctx context.Context, name string) (*domain.DockerRegistry, error) {
	return s.port.GetRegistry(ctx, registryID(name))
}

func (s *DockerRegistryService) Create(ctx context.Context, id string, reg domain.DockerRegistry) (*domain.DockerRegistry, error) {
	reg.ID = registryID(id)
	return s.port.CreateRegistry(ctx, reg)
}

func (s *DockerRegistryService) Update(ctx context.Context, reg domain.DockerRegistry, fields []string) (*domain.DockerRegistry, error) {
	reg.ID = registryID(reg.ID)
	return s.port.UpdateRegistry(ctx, reg, fields)
}

func (s *DockerRegistryService) Delete(ctx context.Context, name string) error {
	return s.port.DeleteRegistry(ctx, registryID(name))
}
