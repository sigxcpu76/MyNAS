package application

import (
	"context"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type CatalogService struct {
	port ports.CatalogPort
}

func NewCatalogService(port ports.CatalogPort) *CatalogService {
	return &CatalogService{port: port}
}

func (s *CatalogService) ListApps(ctx context.Context, train, category, query string) ([]domain.CatalogApp, error) {
	if s.port == nil {
		return nil, nil
	}
	return s.port.ListApps(ctx, train, category, query)
}

func (s *CatalogService) GetApp(ctx context.Context, train, appID string) (*domain.CatalogApp, error) {
	if s.port == nil {
		return nil, domain.ErrNotFound
	}
	return s.port.GetApp(ctx, train, appID)
}

func (s *CatalogService) SyncCatalog(ctx context.Context, force bool) (int, time.Time, error) {
	if s.port == nil {
		return 0, time.Time{}, nil
	}
	return s.port.SyncCatalog(ctx, force)
}

func (s *CatalogService) LastSyncTime() time.Time {
	if s.port == nil {
		return time.Time{}
	}
	return s.port.LastSyncTime()
}

func (s *CatalogService) SetAppsPool(pool string) {
	if s.port != nil {
		s.port.SetAppsPool(pool)
	}
}

func (s *CatalogService) SetAppdataPath(path string) {
	if s.port != nil {
		s.port.SetAppdataPath(path)
	}
}
