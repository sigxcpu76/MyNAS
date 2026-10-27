package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type ZfsService struct {
	engine ports.ZfsEnginePort
}

func NewZfsService(engine ports.ZfsEnginePort) *ZfsService {
	return &ZfsService{engine: engine}
}

func (s *ZfsService) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	return s.engine.ListPools(ctx)
}

func (s *ZfsService) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	if err := domain.ValidateResourceID(id, "zfs_pool"); err != nil {
		return nil, err
	}
	return s.engine.GetPool(ctx, id)
}

func (s *ZfsService) CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error) {
	pool.PoolName = pool.ID
	if pool.VdevLayout == "" {
		pool.VdevLayout = "mirror"
	}
	if pool.Compression == "" {
		pool.Compression = "lz4"
	}
	if pool.Mountpoint == "" {
		pool.Mountpoint = "/mnt/" + pool.ID
	}
	if err := pool.Validate(); err != nil {
		return nil, err
	}
	return s.engine.CreatePool(ctx, pool)
}

func (s *ZfsService) DeletePool(ctx context.Context, id string, force bool) error {
	if err := domain.ValidateResourceID(id, "zfs_pool"); err != nil {
		return err
	}
	return s.engine.DeletePool(ctx, id, force)
}

func (s *ZfsService) ExportPool(ctx context.Context, id string, force bool) error {
	if err := domain.ValidateResourceID(id, "zfs_pool"); err != nil {
		return err
	}
	return s.engine.ExportPool(ctx, id, force)
}

func (s *ZfsService) ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	if err := domain.ValidateResourceID(id, "zfs_pool"); err != nil {
		return nil, err
	}
	return s.engine.ScrubPool(ctx, id)
}

func (s *ZfsService) UpdatePool(ctx context.Context, id string, update domain.ZfsPoolUpdate) (*domain.ZfsPool, error) {
	if err := domain.ValidateResourceID(id, "zfs_pool"); err != nil {
		return nil, err
	}
	return s.engine.UpdatePool(ctx, id, update)
}

func (s *ZfsService) AddVdev(ctx context.Context, poolID string, req domain.ZfsAddVdevRequest) (*domain.ZfsPool, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	if len(req.Devices) == 0 {
		return nil, fmt.Errorf("%w: at least one device is required to add vdev", domain.ErrInvalidArgument)
	}
	return s.engine.AddVdev(ctx, poolID, req)
}

func (s *ZfsService) RemoveVdev(ctx context.Context, poolID string, device string) (*domain.ZfsPool, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("%w: device is required to remove vdev", domain.ErrInvalidArgument)
	}
	return s.engine.RemoveVdev(ctx, poolID, strings.TrimSpace(device))
}

func (s *ZfsService) ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	return s.engine.ListDatasets(ctx, poolID)
}

func (s *ZfsService) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	if err := domain.ValidateResourceID(datasetID, "dataset"); err != nil {
		return nil, err
	}
	return s.engine.GetDataset(ctx, poolID, datasetID)
}

func (s *ZfsService) CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error) {
	if dataset.Compression == "" {
		dataset.Compression = "lz4"
	}
	if dataset.Recordsize == "" {
		dataset.Recordsize = "128K"
	}
	if dataset.Mountpoint == "" {
		dataset.Mountpoint = fmt.Sprintf("/%s/%s", dataset.PoolID, dataset.ID)
	}
	dataset.DatasetPath = fmt.Sprintf("%s/%s", dataset.PoolID, dataset.ID)
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	return s.engine.CreateDataset(ctx, dataset)
}

func (s *ZfsService) UpdateDataset(ctx context.Context, dataset domain.ZfsDataset, fields []string) (*domain.ZfsDataset, error) {
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	return s.engine.UpdateDataset(ctx, dataset, fields)
}

func (s *ZfsService) DeleteDataset(ctx context.Context, poolID, datasetID string, recursive bool) error {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return err
	}
	if err := domain.ValidateResourceID(datasetID, "dataset"); err != nil {
		return err
	}
	return s.engine.DeleteDataset(ctx, poolID, datasetID, recursive)
}

func (s *ZfsService) SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	if err := domain.ValidateResourceID(datasetID, "dataset"); err != nil {
		return nil, err
	}
	if err := domain.ValidateResourceID(snapName, "snapshot_name"); err != nil {
		return nil, err
	}
	return s.engine.SnapshotDataset(ctx, poolID, datasetID, snapName)
}

func (s *ZfsService) ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	return s.engine.ListVolumes(ctx, poolID)
}

func (s *ZfsService) GetVolume(ctx context.Context, poolID, volumeID string) (*domain.ZfsVolume, error) {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return nil, err
	}
	if err := domain.ValidateResourceID(volumeID, "volume"); err != nil {
		return nil, err
	}
	return s.engine.GetVolume(ctx, poolID, volumeID)
}

func (s *ZfsService) CreateVolume(ctx context.Context, volume domain.ZfsVolume) (*domain.ZfsVolume, error) {
	volume.VolumePath = fmt.Sprintf("%s/%s", volume.PoolID, volume.ID)
	volume.DevicePath = fmt.Sprintf("/dev/zvol/%s/%s", volume.PoolID, volume.ID)
	if err := volume.Validate(); err != nil {
		return nil, err
	}
	return s.engine.CreateVolume(ctx, volume)
}

func (s *ZfsService) UpdateVolume(ctx context.Context, volume domain.ZfsVolume, fields []string) (*domain.ZfsVolume, error) {
	if err := volume.Validate(); err != nil {
		return nil, err
	}
	return s.engine.UpdateVolume(ctx, volume, fields)
}

func (s *ZfsService) DeleteVolume(ctx context.Context, poolID, volumeID string) error {
	if err := domain.ValidateResourceID(poolID, "zfs_pool"); err != nil {
		return err
	}
	if err := domain.ValidateResourceID(volumeID, "volume"); err != nil {
		return err
	}
	return s.engine.DeleteVolume(ctx, poolID, volumeID)
}
