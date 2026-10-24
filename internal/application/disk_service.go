package application

import (
	"context"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type DiskService struct {
	diskPort ports.DiskPort
}

func NewDiskService(diskPort ports.DiskPort) *DiskService {
	return &DiskService{diskPort: diskPort}
}

func (s *DiskService) ListDisks(ctx context.Context, filter string) ([]domain.Disk, error) {
	disks, err := s.diskPort.ListDisks(ctx)
	if err != nil {
		return nil, err
	}

	if filter == "" {
		return disks, nil
	}

	filterLower := strings.ToLower(filter)
	var filtered []domain.Disk
	for _, d := range disks {
		if strings.Contains(strings.ToLower(d.DiskID), filterLower) ||
			strings.Contains(strings.ToLower(d.Model), filterLower) ||
			strings.Contains(strings.ToLower(d.Vendor), filterLower) ||
			strings.Contains(strings.ToLower(d.Serial), filterLower) ||
			strings.Contains(strings.ToLower(d.Type), filterLower) ||
			strings.Contains(strings.ToLower(d.OwningZfsPool), filterLower) {
			filtered = append(filtered, d)
		}
	}
	return filtered, nil
}

func (s *DiskService) GetDisk(ctx context.Context, id string) (*domain.Disk, error) {
	cleanID := strings.TrimPrefix(id, "disks/")
	return s.diskPort.GetDisk(ctx, cleanID)
}
