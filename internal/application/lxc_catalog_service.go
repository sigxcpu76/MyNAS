package application

import (
	"context"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type LxcCatalogService struct {
	port ports.LxcCatalogPort
}

func NewLxcCatalogService(port ports.LxcCatalogPort) *LxcCatalogService {
	return &LxcCatalogService{port: port}
}

func (s *LxcCatalogService) ListItems(ctx context.Context, source, category, query string) ([]domain.LxcCatalogItem, error) {
	return s.port.ListItems(ctx, source, category, query)
}

func (s *LxcCatalogService) GetItem(ctx context.Context, id string) (*domain.LxcCatalogItem, error) {
	if err := domain.ValidateResourceID(id, "lxc_catalog_item"); err != nil {
		return nil, err
	}
	return s.port.GetItem(ctx, id)
}

func (s *LxcCatalogService) SyncCatalog(ctx context.Context, force bool) (int, time.Time, error) {
	return s.port.SyncCatalog(ctx, force)
}

func (s *LxcCatalogService) LastSyncTime() time.Time {
	return s.port.LastSyncTime()
}
