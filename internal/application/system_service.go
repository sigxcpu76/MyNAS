package application

import (
	"context"
	"fmt"

	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type SystemService struct {
	port ports.SystemPort
}

func NewSystemService(port ports.SystemPort) *SystemService {
	return &SystemService{port: port}
}

func (s *SystemService) GetConfig(ctx context.Context) (*domain.SystemConfig, error) {
	return s.port.GetConfig(ctx)
}

func (s *SystemService) SelectMainPool(ctx context.Context, poolName string, autoImportExisting bool) (*domain.SystemConfig, error) {
	if poolName == "" {
		return nil, fmt.Errorf("%w: pool name cannot be empty", domain.ErrInvalidArgument)
	}
	return s.port.SelectMainPool(ctx, poolName, autoImportExisting)
}

func (s *SystemService) DiscoverPools(ctx context.Context) ([]domain.DiscoveredPool, error) {
	return s.port.DiscoverPools(ctx)
}

func (s *SystemService) GetTelemetry(ctx context.Context) (*domain.SystemTelemetry, error) {
	return s.port.GetTelemetry(ctx)
}

func (s *SystemService) RequireMainPool() error {
	cfg := config.GetGlobal()
	if cfg.System.MainPool == "" {
		return fmt.Errorf("%w: main ZFS storage pool is not configured. Please select or import a main storage pool to enable this feature", domain.ErrFailedPrecondition)
	}
	return nil
}
