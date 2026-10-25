package grpcadapter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	patternDisk            = "disks/{disk}"
	patternPrerequisite    = "prerequisites/{prerequisite}"
	patternZfsPool         = "zfsPools/{zfs_pool}"
	patternZfsDataset      = "zfsPools/{zfs_pool}/datasets/{dataset}"
	patternZfsVolume       = "zfsPools/{zfs_pool}/volumes/{volume}"
	patternVirtualMachine         = "virtualMachines/{virtual_machine}"
	patternVirtualMachineSnapshot = "virtualMachines/{virtual_machine}/snapshots/{snapshot}"
	patternDockerContainer = "dockerContainers/{docker_container}"
	patternDockerApp       = "dockerApps/{docker_app}"
	patternLxcContainer    = "lxcContainers/{lxc_container}"
	patternCifsShare       = "cifsShares/{cifs_share}"
	patternWebdavShare     = "webdavShares/{webdav_share}"
	patternUser            = "users/{user}"
	patternGroup           = "groups/{group}"
)

type Server struct {
	nasv1.UnimplementedNasServiceServer
	system    *application.SystemService
	prereq    *application.PrerequisiteService
	disk      *application.DiskService
	zfs       *application.ZfsService
	kvm       *application.KvmService
	docker    *application.DockerService
	dockerApp *application.DockerAppService
	lxc       *application.LxcService
	share     *application.ShareService
	identity  *application.IdentityService
	catalog   *application.CatalogService
}

func NewServer(
	system *application.SystemService,
	prereq *application.PrerequisiteService,
	disk *application.DiskService,
	zfs *application.ZfsService,
	kvm *application.KvmService,
	docker *application.DockerService,
	dockerApp *application.DockerAppService,
	lxc *application.LxcService,
	share *application.ShareService,
	identity *application.IdentityService,
	catalog *application.CatalogService,
) *Server {
	return &Server{
		system:    system,
		prereq:    prereq,
		disk:      disk,
		zfs:       zfs,
		kvm:       kvm,
		docker:    docker,
		dockerApp: dockerApp,
		lxc:       lxc,
		share:     share,
		identity:  identity,
		catalog:   catalog,
	}
}

// =============================================================================
// Auth
// =============================================================================

func (s *Server) Login(ctx context.Context, req *nasv1.LoginRequest) (*nasv1.LoginResponse, error) {
	token, expiresIn, user, err := s.identity.Login(ctx, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return &nasv1.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User:        toProtoUser(*user),
	}, nil
}

func (s *Server) GetCurrentUser(ctx context.Context, req *nasv1.GetCurrentUserRequest) (*nasv1.User, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		authVals := md.Get("authorization")
		if len(authVals) > 0 {
			u, err := s.identity.VerifyToken(ctx, authVals[0])
			if err != nil {
				return nil, ToGRPCError(err)
			}
			return toProtoUser(*u), nil
		}
	}
	return nil, ToGRPCError(domain.ErrUnauthorized)
}

// =============================================================================
// System Configuration & Main Pool Management
// =============================================================================

func (s *Server) GetSystemConfig(ctx context.Context, req *nasv1.GetSystemConfigRequest) (*nasv1.SystemConfig, error) {
	if s.system == nil {
		return &nasv1.SystemConfig{}, nil
	}
	cfg, err := s.system.GetConfig(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoSystemConfig(cfg), nil
}

func (s *Server) UpdateSystemConfig(ctx context.Context, req *nasv1.UpdateSystemConfigRequest) (*nasv1.SystemConfig, error) {
	if s.system == nil {
		return &nasv1.SystemConfig{}, nil
	}
	pool := req.GetSystemConfig().GetMainPool()
	if pool != "" {
		cfg, err := s.system.SelectMainPool(ctx, pool, true)
		if err != nil {
			return nil, ToGRPCError(err)
		}
		return toProtoSystemConfig(cfg), nil
	}
	cfg, err := s.system.GetConfig(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoSystemConfig(cfg), nil
}

func (s *Server) SelectMainPool(ctx context.Context, req *nasv1.SelectMainPoolRequest) (*nasv1.SystemConfig, error) {
	if s.system == nil {
		return nil, status.Errorf(codes.Unimplemented, "system service not configured")
	}
	cfg, err := s.system.SelectMainPool(ctx, req.GetPoolName(), req.GetAutoImportExisting())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoSystemConfig(cfg), nil
}

func (s *Server) DiscoverPools(ctx context.Context, req *nasv1.DiscoverPoolsRequest) (*nasv1.DiscoverPoolsResponse, error) {
	if s.system == nil {
		return &nasv1.DiscoverPoolsResponse{}, nil
	}
	pools, err := s.system.DiscoverPools(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	var out []*nasv1.DiscoveredPool
	for _, p := range pools {
		out = append(out, toProtoDiscoveredPool(p))
	}
	return &nasv1.DiscoverPoolsResponse{Pools: out}, nil
}

// =============================================================================
// Prerequisites
// =============================================================================

func (s *Server) ListPrerequisites(ctx context.Context, req *nasv1.ListPrerequisitesRequest) (*nasv1.ListPrerequisitesResponse, error) {
	report, err := s.prereq.GetReport(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}

	items := report.All
	if req.GetFilter() != "" {
		filtered, err := s.prereq.ListPrerequisites(ctx, req.GetFilter())
		if err != nil {
			return nil, ToGRPCError(err)
		}
		items = filtered
	}

	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.Prerequisite, 0, len(paged))
	for _, p := range paged {
		out = append(out, toProtoPrerequisite(p))
	}

	missing := make([]*nasv1.Prerequisite, 0, len(report.Missing))
	for _, p := range report.Missing {
		missing = append(missing, toProtoPrerequisite(p))
	}
	installed := make([]*nasv1.Prerequisite, 0, len(report.Installed))
	for _, p := range report.Installed {
		installed = append(installed, toProtoPrerequisite(p))
	}

	return &nasv1.ListPrerequisitesResponse{
		Prerequisites:          out,
		NextPageToken:          nextToken,
		TotalSize:              total,
		LinuxFlavor:            toProtoLinuxFlavor(report.LinuxFlavor),
		MissingPrerequisites:   missing,
		InstalledPrerequisites: installed,
		RemediationScript:      report.RemediationScript,
		IsFullyCompliant:       report.IsFullyCompliant,
	}, nil
}

func (s *Server) GetPrerequisite(ctx context.Context, req *nasv1.GetPrerequisiteRequest) (*nasv1.Prerequisite, error) {
	id, err := ParseSingleResourceID(patternPrerequisite, req.GetName())
	if err != nil {
		return nil, err
	}
	item, err := s.prereq.GetPrerequisite(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoPrerequisite(*item), nil
}

func (s *Server) CheckPrerequisites(ctx context.Context, req *nasv1.CheckPrerequisitesRequest) (*nasv1.CheckPrerequisitesResponse, error) {
	report, err := s.prereq.GetReport(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	out := make([]*nasv1.Prerequisite, 0, len(report.All))
	for _, p := range report.All {
		out = append(out, toProtoPrerequisite(p))
	}
	missing := make([]*nasv1.Prerequisite, 0, len(report.Missing))
	for _, p := range report.Missing {
		missing = append(missing, toProtoPrerequisite(p))
	}
	installed := make([]*nasv1.Prerequisite, 0, len(report.Installed))
	for _, p := range report.Installed {
		installed = append(installed, toProtoPrerequisite(p))
	}
	return &nasv1.CheckPrerequisitesResponse{
		Prerequisites:          out,
		InstalledCount:        int32(len(report.Installed)),
		MissingCount:          int32(len(report.Missing)),
		HybridFallbackEnabled: report.HybridFallbackActive,
		LinuxFlavor:            toProtoLinuxFlavor(report.LinuxFlavor),
		MissingPrerequisites:   missing,
		InstalledPrerequisites: installed,
		RemediationScript:      report.RemediationScript,
		IsFullyCompliant:       report.IsFullyCompliant,
	}, nil
}

// =============================================================================
// Disks & Storage Block Devices
// =============================================================================

func (s *Server) ListDisks(ctx context.Context, req *nasv1.ListDisksRequest) (*nasv1.ListDisksResponse, error) {
	disks, err := s.disk.ListDisks(ctx, req.GetFilter())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].DiskID < disks[j].DiskID })
	paged, nextToken, total, err := PaginateSlice(disks, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.Disk, 0, len(paged))
	for _, d := range paged {
		out = append(out, toProtoDisk(d))
	}
	return &nasv1.ListDisksResponse{
		Disks:         out,
		NextPageToken: nextToken,
		TotalSize:     int32(total),
	}, nil
}

func (s *Server) GetDisk(ctx context.Context, req *nasv1.GetDiskRequest) (*nasv1.Disk, error) {
	var diskID string
	if err := resourcename.Sscan(req.GetName(), patternDisk, &diskID); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid disk name %q: %v", req.GetName(), err)
	}
	d, err := s.disk.GetDisk(ctx, diskID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDisk(*d), nil
}

func toProtoDisk(d domain.Disk) *nasv1.Disk {
	name := d.Name
	if name == "" {
		name = "disks/" + d.DiskID
	}
	var parts []*nasv1.DiskPartition
	for _, p := range d.Partitions {
		parts = append(parts, &nasv1.DiskPartition{
			Name:          p.Name,
			DevicePath:    p.DevicePath,
			SizeBytes:     p.SizeBytes,
			Fstype:        p.FSType,
			Mountpoint:    p.Mountpoint,
			Label:         p.Label,
			OwningZfsPool: p.OwningZfsPool,
			Aliases:       p.Aliases,
		})
	}
	var smartParams []*nasv1.SmartParameter
	for _, sp := range d.SmartParameters {
		smartParams = append(smartParams, &nasv1.SmartParameter{
			Id:          sp.ID,
			Name:        sp.Name,
			Value:       sp.Value,
			Worst:       sp.Worst,
			Threshold:   sp.Threshold,
			RawValue:    sp.RawValue,
			Status:      sp.Status,
			Type:        sp.Type,
			Description: sp.Description,
		})
	}
	return &nasv1.Disk{
		Name:               name,
		DiskId:             d.DiskID,
		DevicePath:         d.DevicePath,
		SizeBytes:          d.SizeBytes,
		Model:              d.Model,
		Serial:             d.Serial,
		Vendor:             d.Vendor,
		Transport:          d.Transport,
		Rotational:         d.Rotational,
		Type:               d.Type,
		FirmwareRev:        d.FirmwareRev,
		State:              d.State,
		Removable:          d.Removable,
		LogicalBlockSize:   d.LogicalBlockSize,
		PhysicalBlockSize:  d.PhysicalBlockSize,
		TemperatureCelsius: d.TemperatureCelsius,
		SmartStatus:        d.SmartStatus,
		HealthPercentage:   d.HealthPercentage,
		OwningZfsPool:      d.OwningZfsPool,
		Partitions:         parts,
		SmartParameters:    smartParams,
		PowerOnHours:       d.PowerOnHours,
		PowerCycleCount:    d.PowerCycleCount,
		UnsafeShutdowns:    d.UnsafeShutdowns,
		MediaErrors:        d.MediaErrors,
		TotalBytesRead:     d.TotalBytesRead,
		TotalBytesWritten:  d.TotalBytesWritten,
		ReallocatedSectors: d.ReallocatedSectors,
		PendingSectors:     d.PendingSectors,
		Aliases:            d.Aliases,
	}
}

// =============================================================================
// ZFS Pools, Datasets, Volumes
// =============================================================================

func (s *Server) ListZfsPools(ctx context.Context, req *nasv1.ListZfsPoolsRequest) (*nasv1.ListZfsPoolsResponse, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	pools, err := s.zfs.ListPools(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].ID < pools[j].ID })
	paged, nextToken, total, err := PaginateSlice(pools, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.ZfsPool, 0, len(paged))
	for _, p := range paged {
		out = append(out, toProtoZfsPool(p))
	}
	return &nasv1.ListZfsPoolsResponse{
		ZfsPools:      out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetZfsPool(ctx context.Context, req *nasv1.GetZfsPoolRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetName())
	if err != nil {
		return nil, err
	}
	pool, err := s.zfs.GetPool(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*pool), nil
}

func (s *Server) CreateZfsPool(ctx context.Context, req *nasv1.CreateZfsPoolRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetZfsPoolId()
	if id == "" && req.GetZfsPool() != nil {
		id = strings.TrimPrefix(req.GetZfsPool().GetName(), "zfsPools/")
		if id == "" {
			id = req.GetZfsPool().GetPoolName()
		}
	}
	p := req.GetZfsPool()
	created, err := s.zfs.CreatePool(ctx, domain.ZfsPool{
		ID:          id,
		VdevLayout:  p.GetVdevLayout(),
		Devices:     p.GetDevices(),
		Mountpoint:  p.GetMountpoint(),
		Compression: p.GetCompression(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*created), nil
}

func (s *Server) DeleteZfsPool(ctx context.Context, req *nasv1.DeleteZfsPoolRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.zfs.DeletePool(ctx, id, req.GetForce()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ScrubZfsPool(ctx context.Context, req *nasv1.ScrubZfsPoolRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetName())
	if err != nil {
		return nil, err
	}
	pool, err := s.zfs.ScrubPool(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*pool), nil
}

func (s *Server) UpdateZfsPool(ctx context.Context, req *nasv1.UpdateZfsPoolRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	if req.GetZfsPool() == nil {
		return nil, status.Error(codes.InvalidArgument, "zfs_pool is required")
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetZfsPool().GetName())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	update := domain.ZfsPoolUpdate{
		Sync:        req.GetZfsPool().GetSync(),
		Autotrim:    req.GetZfsPool().GetAutotrim(),
		Compression: req.GetZfsPool().GetCompression(),
	}
	updated, err := s.zfs.UpdatePool(ctx, id, update)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*updated), nil
}

func (s *Server) AddZfsPoolVdev(ctx context.Context, req *nasv1.AddZfsPoolVdevRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetName())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	vdevReq := domain.ZfsAddVdevRequest{
		Type:    req.GetType(),
		Layout:  req.GetLayout(),
		Devices: req.GetDevices(),
	}
	updated, err := s.zfs.AddVdev(ctx, id, vdevReq)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*updated), nil
}

func (s *Server) RemoveZfsPoolVdev(ctx context.Context, req *nasv1.RemoveZfsPoolVdevRequest) (*nasv1.ZfsPool, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternZfsPool, req.GetName())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	updated, err := s.zfs.RemoveVdev(ctx, id, req.GetDevice())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsPool(*updated), nil
}

func (s *Server) ListZfsDatasets(ctx context.Context, req *nasv1.ListZfsDatasetsRequest) (*nasv1.ListZfsDatasetsResponse, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, err := ParseSingleResourceID(patternZfsPool, req.GetParent())
	if err != nil {
		return nil, err
	}
	items, err := s.zfs.ListDatasets(ctx, poolID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.ZfsDataset, 0, len(paged))
	for _, ds := range paged {
		out = append(out, toProtoZfsDataset(ds))
	}
	return &nasv1.ListZfsDatasetsResponse{
		ZfsDatasets:   out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetZfsDataset(ctx context.Context, req *nasv1.GetZfsDatasetRequest) (*nasv1.ZfsDataset, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, dsID, err := ParseChildResourceIDs(patternZfsDataset, req.GetName())
	if err != nil {
		return nil, err
	}
	ds, err := s.zfs.GetDataset(ctx, poolID, dsID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsDataset(*ds), nil
}

func (s *Server) CreateZfsDataset(ctx context.Context, req *nasv1.CreateZfsDatasetRequest) (*nasv1.ZfsDataset, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, err := ParseSingleResourceID(patternZfsPool, req.GetParent())
	if err != nil {
		return nil, err
	}
	dsID := req.GetZfsDatasetId()
	in := req.GetZfsDataset()
	if dsID == "" && in != nil {
		parts := strings.Split(in.GetName(), "/")
		dsID = parts[len(parts)-1]
	}
	created, err := s.zfs.CreateDataset(ctx, domain.ZfsDataset{
		PoolID:           poolID,
		ID:               dsID,
		Mountpoint:       in.GetMountpoint(),
		QuotaBytes:       in.GetQuotaBytes(),
		Compression:      in.GetCompression(),
		Readonly:         in.GetReadonly(),
		Atime:            in.GetAtime(),
		Recordsize:       in.GetRecordsize(),
		Sync:             in.GetSync(),
		ReservationBytes: in.GetReservationBytes(),
		Dedup:            in.GetDedup(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsDataset(*created), nil
}

func (s *Server) UpdateZfsDataset(ctx context.Context, req *nasv1.UpdateZfsDatasetRequest) (*nasv1.ZfsDataset, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetZfsDataset()
	poolID, dsID, err := ParseChildResourceIDs(patternZfsDataset, in.GetName())
	if err != nil {
		return nil, err
	}
	if err := fieldmask.Validate(req.GetUpdateMask(), in); err != nil {
		return nil, ToGRPCError(domain.ErrInvalidArgument)
	}
	updated, err := s.zfs.UpdateDataset(ctx, domain.ZfsDataset{
		PoolID:           poolID,
		ID:               dsID,
		Mountpoint:       in.GetMountpoint(),
		QuotaBytes:       in.GetQuotaBytes(),
		Compression:      in.GetCompression(),
		Readonly:         in.GetReadonly(),
		Atime:            in.GetAtime(),
		Recordsize:       in.GetRecordsize(),
		Sync:             in.GetSync(),
		ReservationBytes: in.GetReservationBytes(),
		Dedup:            in.GetDedup(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsDataset(*updated), nil
}

func (s *Server) DeleteZfsDataset(ctx context.Context, req *nasv1.DeleteZfsDatasetRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, dsID, err := ParseChildResourceIDs(patternZfsDataset, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.zfs.DeleteDataset(ctx, poolID, dsID, req.GetRecursive()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) SnapshotZfsDataset(ctx context.Context, req *nasv1.SnapshotZfsDatasetRequest) (*nasv1.ZfsDataset, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, dsID, err := ParseChildResourceIDs(patternZfsDataset, req.GetName())
	if err != nil {
		return nil, err
	}
	ds, err := s.zfs.SnapshotDataset(ctx, poolID, dsID, req.GetSnapshotName())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsDataset(*ds), nil
}

func (s *Server) ListZfsVolumes(ctx context.Context, req *nasv1.ListZfsVolumesRequest) (*nasv1.ListZfsVolumesResponse, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, err := ParseSingleResourceID(patternZfsPool, req.GetParent())
	if err != nil {
		return nil, err
	}
	items, err := s.zfs.ListVolumes(ctx, poolID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.ZfsVolume, 0, len(paged))
	for _, v := range paged {
		out = append(out, toProtoZfsVolume(v))
	}
	return &nasv1.ListZfsVolumesResponse{
		ZfsVolumes:    out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetZfsVolume(ctx context.Context, req *nasv1.GetZfsVolumeRequest) (*nasv1.ZfsVolume, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, volID, err := ParseChildResourceIDs(patternZfsVolume, req.GetName())
	if err != nil {
		return nil, err
	}
	vol, err := s.zfs.GetVolume(ctx, poolID, volID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsVolume(*vol), nil
}

func (s *Server) CreateZfsVolume(ctx context.Context, req *nasv1.CreateZfsVolumeRequest) (*nasv1.ZfsVolume, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, err := ParseSingleResourceID(patternZfsPool, req.GetParent())
	if err != nil {
		return nil, err
	}
	volID := req.GetZfsVolumeId()
	in := req.GetZfsVolume()
	created, err := s.zfs.CreateVolume(ctx, domain.ZfsVolume{
		PoolID:          poolID,
		ID:              volID,
		SizeBytes:       in.GetSizeBytes(),
		Volblocksize:    in.GetVolblocksize(),
		Compression:     in.GetCompression(),
		ThinProvisioned: in.GetThinProvisioned(),
		Sync:            in.GetSync(),
		Readonly:        in.GetReadonly(),
		Dedup:           in.GetDedup(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsVolume(*created), nil
}

func (s *Server) UpdateZfsVolume(ctx context.Context, req *nasv1.UpdateZfsVolumeRequest) (*nasv1.ZfsVolume, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetZfsVolume()
	poolID, volID, err := ParseChildResourceIDs(patternZfsVolume, in.GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.zfs.UpdateVolume(ctx, domain.ZfsVolume{
		PoolID:          poolID,
		ID:              volID,
		SizeBytes:       in.GetSizeBytes(),
		Volblocksize:    in.GetVolblocksize(),
		Compression:     in.GetCompression(),
		ThinProvisioned: in.GetThinProvisioned(),
		Sync:            in.GetSync(),
		Readonly:        in.GetReadonly(),
		Dedup:           in.GetDedup(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoZfsVolume(*updated), nil
}

func (s *Server) DeleteZfsVolume(ctx context.Context, req *nasv1.DeleteZfsVolumeRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("zfs"); err != nil {
		return nil, ToGRPCError(err)
	}
	poolID, volID, err := ParseChildResourceIDs(patternZfsVolume, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.zfs.DeleteVolume(ctx, poolID, volID); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

// =============================================================================
// KVM Virtual Machines + noVNC
// =============================================================================

func (s *Server) ListVirtualMachines(ctx context.Context, req *nasv1.ListVirtualMachinesRequest) (*nasv1.ListVirtualMachinesResponse, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	vms, err := s.kvm.ListVMs(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(vms, func(i, j int) bool { return vms[i].ID < vms[j].ID })
	paged, nextToken, total, err := PaginateSlice(vms, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.VirtualMachine, 0, len(paged))
	for _, vm := range paged {
		out = append(out, toProtoVM(vm))
	}
	return &nasv1.ListVirtualMachinesResponse{
		VirtualMachines: out,
		NextPageToken:   nextToken,
		TotalSize:       total,
	}, nil
}

func (s *Server) GetVirtualMachine(ctx context.Context, req *nasv1.GetVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.GetVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) CreateVirtualMachine(ctx context.Context, req *nasv1.CreateVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetVirtualMachineId()
	in := req.GetVirtualMachine()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "virtualMachines/")
	}

	var disks []domain.VirtualMachineDisk
	for _, d := range in.GetDisks() {
		disks = append(disks, fromProtoVMDisk(d))
	}
	var ifaces []domain.VirtualMachineNetworkInterface
	for _, iface := range in.GetNetworkInterfaces() {
		ifaces = append(ifaces, fromProtoVMNetworkInterface(iface))
	}
	var pcis []domain.VirtualMachinePciDevice
	for _, p := range in.GetPciDevices() {
		pcis = append(pcis, domain.VirtualMachinePciDevice{
			HostAddress: p.GetHostAddress(),
			PCIE:        p.GetPcie(),
			PrimaryGPU:  p.GetPrimaryGpu(),
		})
	}
	var usbs []domain.VirtualMachineUsbDevice
	for _, u := range in.GetUsbDevices() {
		usbs = append(usbs, domain.VirtualMachineUsbDevice{
			HostBusPort:     u.GetHostBusPort(),
			VendorProductID: u.GetVendorProductId(),
		})
	}

	created, err := s.kvm.CreateVM(ctx, domain.VirtualMachine{
		ID:                  id,
		VCPUs:               in.GetVcpus(),
		MemoryMiB:           in.GetMemoryMib(),
		DiskPath:            in.GetDiskPath(),
		DiskSizeGiB:         in.GetDiskSizeGib(),
		ISOImage:            in.GetIsoImage(),
		CdromIsoImage:       in.GetCdromIsoImage(),
		NetworkBridge:       in.GetNetworkBridge(),
		Autostart:           in.GetAutostart(),
		OSVariant:           in.GetOsVariant(),
		Description:         in.GetDescription(),
		Sockets:             in.GetSockets(),
		Cores:               in.GetCores(),
		CPUType:             in.GetCpuType(),
		CPUUnits:            in.GetCpuUnits(),
		CPULimit:            in.GetCpuLimit(),
		NUMA:                in.GetNuma(),
		BalloonMinMiB:       in.GetBalloonMinMib(),
		Hugepages:           in.GetHugepages(),
		MachineType:         in.GetMachineType(),
		BiosType:            in.GetBiosType(),
		TPMVersion:          in.GetTpmVersion(),
		SecureBoot:          in.GetSecureBoot(),
		Disks:               disks,
		NetworkIfaces:       ifaces,
		CloudInit:           fromProtoVMCloudInit(in.GetCloudInit()),
		PciDevices:          pcis,
		UsbDevices:          usbs,
		StartupDelaySeconds: in.GetStartupDelaySeconds(),
		BootOrder:           in.GetBootOrder(),
		WatchdogModel:       in.GetWatchdogModel(),
		WatchdogAction:      in.GetWatchdogAction(),
		DisplayType:         in.GetDisplayType(),
		IsTemplate:          in.GetIsTemplate(),
		QemuCommandlineArgs: in.GetQemuCommandlineArgs(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*created), nil
}

func (s *Server) UpdateVirtualMachine(ctx context.Context, req *nasv1.UpdateVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetVirtualMachine()
	id, err := ParseSingleResourceID(patternVirtualMachine, in.GetName())
	if err != nil {
		return nil, err
	}

	var disks []domain.VirtualMachineDisk
	for _, d := range in.GetDisks() {
		disks = append(disks, fromProtoVMDisk(d))
	}
	var ifaces []domain.VirtualMachineNetworkInterface
	for _, iface := range in.GetNetworkInterfaces() {
		ifaces = append(ifaces, fromProtoVMNetworkInterface(iface))
	}

	updated, err := s.kvm.UpdateVM(ctx, domain.VirtualMachine{
		ID:                  id,
		VCPUs:               in.GetVcpus(),
		MemoryMiB:           in.GetMemoryMib(),
		NetworkBridge:       in.GetNetworkBridge(),
		Autostart:           in.GetAutostart(),
		Description:         in.GetDescription(),
		Sockets:             in.GetSockets(),
		Cores:               in.GetCores(),
		CPUType:             in.GetCpuType(),
		BalloonMinMiB:       in.GetBalloonMinMib(),
		MachineType:         in.GetMachineType(),
		BiosType:            in.GetBiosType(),
		TPMVersion:          in.GetTpmVersion(),
		SecureBoot:          in.GetSecureBoot(),
		Disks:               disks,
		CdromIsoImage:       in.GetCdromIsoImage(),
		NetworkIfaces:       ifaces,
		CloudInit:           fromProtoVMCloudInit(in.GetCloudInit()),
		BootOrder:           in.GetBootOrder(),
		WatchdogModel:       in.GetWatchdogModel(),
		WatchdogAction:      in.GetWatchdogAction(),
		DisplayType:         in.GetDisplayType(),
		QemuCommandlineArgs: in.GetQemuCommandlineArgs(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*updated), nil
}

func (s *Server) DeleteVirtualMachine(ctx context.Context, req *nasv1.DeleteVirtualMachineRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.kvm.DeleteVM(ctx, id); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) StartVirtualMachine(ctx context.Context, req *nasv1.StartVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.StartVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) StopVirtualMachine(ctx context.Context, req *nasv1.StopVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.StopVM(ctx, id, req.GetForce())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) RebootVirtualMachine(ctx context.Context, req *nasv1.RebootVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.RebootVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) OpenVirtualMachineConsole(ctx context.Context, req *nasv1.OpenVirtualMachineConsoleRequest) (*nasv1.VirtualMachineConsoleSession, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	sess, err := s.kvm.OpenConsole(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return &nasv1.VirtualMachineConsoleSession{
		VmName:       sess.VMName,
		Token:        sess.Token,
		WebsocketUrl: sess.WebsocketURL,
		VncPort:      sess.VNCPort,
		Resolution:   sess.Resolution,
		ExpiresAt:    sess.ExpiresAt,
	}, nil
}

func (s *Server) ResetVirtualMachine(ctx context.Context, req *nasv1.ResetVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.ResetVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) PauseVirtualMachine(ctx context.Context, req *nasv1.PauseVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.PauseVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) ResumeVirtualMachine(ctx context.Context, req *nasv1.ResumeVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.ResumeVM(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) CloneVirtualMachine(ctx context.Context, req *nasv1.CloneVirtualMachineRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.CloneVM(ctx, id, req.GetNewVirtualMachineId(), req.GetNewName(), req.GetTargetPool(), req.GetLinkedClone())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) AttachVirtualMachineMedia(ctx context.Context, req *nasv1.AttachVirtualMachineMediaRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.AttachMedia(ctx, id, req.GetDeviceId(), req.GetIsoPath())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) EjectVirtualMachineMedia(ctx context.Context, req *nasv1.EjectVirtualMachineMediaRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternVirtualMachine, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.EjectMedia(ctx, id, req.GetDeviceId())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) ListVirtualMachineSnapshots(ctx context.Context, req *nasv1.ListVirtualMachineSnapshotsRequest) (*nasv1.ListVirtualMachineSnapshotsResponse, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	vmID, err := ParseSingleResourceID(patternVirtualMachine, req.GetParent())
	if err != nil {
		return nil, err
	}
	snaps, err := s.kvm.ListSnapshots(ctx, vmID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	paged, nextToken, total, err := PaginateSlice(snaps, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.VirtualMachineSnapshot, 0, len(paged))
	for _, snap := range paged {
		out = append(out, toProtoVMSnapshot(vmID, snap))
	}
	return &nasv1.ListVirtualMachineSnapshotsResponse{
		Snapshots:     out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) CreateVirtualMachineSnapshot(ctx context.Context, req *nasv1.CreateVirtualMachineSnapshotRequest) (*nasv1.VirtualMachineSnapshot, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	vmID, err := ParseSingleResourceID(patternVirtualMachine, req.GetParent())
	if err != nil {
		return nil, err
	}
	in := req.GetSnapshot()
	created, err := s.kvm.CreateSnapshot(ctx, vmID, domain.VirtualMachineSnapshot{
		SnapshotID:  req.GetSnapshotId(),
		Description: in.GetDescription(),
		IncludeRAM:  in.GetIncludeRam(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVMSnapshot(vmID, *created), nil
}

func (s *Server) RollbackVirtualMachineSnapshot(ctx context.Context, req *nasv1.RollbackVirtualMachineSnapshotRequest) (*nasv1.VirtualMachine, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	vmID, snapID, err := ParseChildResourceIDs(patternVirtualMachineSnapshot, req.GetName())
	if err != nil {
		return nil, err
	}
	vm, err := s.kvm.RollbackSnapshot(ctx, vmID, snapID)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoVM(*vm), nil
}

func (s *Server) DeleteVirtualMachineSnapshot(ctx context.Context, req *nasv1.DeleteVirtualMachineSnapshotRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("kvm"); err != nil {
		return nil, ToGRPCError(err)
	}
	vmID, snapID, err := ParseChildResourceIDs(patternVirtualMachineSnapshot, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.kvm.DeleteSnapshot(ctx, vmID, snapID); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

// =============================================================================
// Docker Containers
// =============================================================================

func (s *Server) ListDockerContainers(ctx context.Context, req *nasv1.ListDockerContainersRequest) (*nasv1.ListDockerContainersResponse, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	items, err := s.docker.ListContainers(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.DockerContainer, 0, len(paged))
	for _, c := range paged {
		out = append(out, toProtoDocker(c))
	}
	return &nasv1.ListDockerContainersResponse{
		DockerContainers: out,
		NextPageToken:    nextToken,
		TotalSize:        total,
	}, nil
}

func (s *Server) GetDockerContainer(ctx context.Context, req *nasv1.GetDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.docker.GetContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*c), nil
}

func (s *Server) CreateDockerContainer(ctx context.Context, req *nasv1.CreateDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetDockerContainerId()
	in := req.GetDockerContainer()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "dockerContainers/")
	}
	created, err := s.docker.CreateContainer(ctx, domain.DockerContainer{
		ID:             id,
		Image:          in.GetImage(),
		Ports:          in.GetPorts(),
		Volumes:        in.GetVolumes(),
		Environment:    in.GetEnvironment(),
		RestartPolicy:  in.GetRestartPolicy(),
		NetworkMode:    in.GetNetworkMode(),
		MemoryLimitMiB: in.GetMemoryLimitMib(),
		Command:        in.GetCommand(),
		Privileged:     in.GetPrivileged(),
		ComposeYAML:    in.GetComposeYaml(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*created), nil
}

func (s *Server) UpdateDockerContainer(ctx context.Context, req *nasv1.UpdateDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetDockerContainer()
	if in == nil {
		return nil, ToGRPCError(fmt.Errorf("%w: docker_container is required", domain.ErrInvalidArgument))
	}
	id, err := ParseSingleResourceID(patternDockerContainer, in.GetName())
	if err != nil {
		return nil, err
	}
	fields := req.GetUpdateMask().GetPaths()
	updated, err := s.docker.UpdateContainer(ctx, domain.DockerContainer{
		ID:             id,
		Image:          in.GetImage(),
		Ports:          in.GetPorts(),
		Volumes:        in.GetVolumes(),
		Environment:    in.GetEnvironment(),
		RestartPolicy:  in.GetRestartPolicy(),
		NetworkMode:    in.GetNetworkMode(),
		MemoryLimitMiB: in.GetMemoryLimitMib(),
		Command:        in.GetCommand(),
		Privileged:     in.GetPrivileged(),
		ComposeYAML:    in.GetComposeYaml(),
	}, fields)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*updated), nil
}

func (s *Server) GetDockerContainerLogs(ctx context.Context, req *nasv1.GetDockerContainerLogsRequest) (*nasv1.GetDockerContainerLogsResponse, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	tail := int(req.GetTail())
	if tail <= 0 {
		tail = 100
	}
	logs, err := s.docker.GetContainerLogs(ctx, id, tail)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return &nasv1.GetDockerContainerLogsResponse{Logs: logs}, nil
}

func (s *Server) DeleteDockerContainer(ctx context.Context, req *nasv1.DeleteDockerContainerRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.docker.DeleteContainer(ctx, id, req.GetForce()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) StartDockerContainer(ctx context.Context, req *nasv1.StartDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.docker.StartContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*c), nil
}

func (s *Server) StopDockerContainer(ctx context.Context, req *nasv1.StopDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.docker.StopContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*c), nil
}

func (s *Server) RestartDockerContainer(ctx context.Context, req *nasv1.RestartDockerContainerRequest) (*nasv1.DockerContainer, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.docker.RestartContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoDocker(*c), nil
}

// =============================================================================
// Docker Apps (TrueNAS SCALE Compose Descriptors)
// =============================================================================

func dockerAppToProto(a *domain.DockerApp) *nasv1.DockerApp {
	if a == nil {
		return nil
	}
	var containers []*nasv1.DockerAppContainer
	for _, c := range a.Containers {
		containers = append(containers, &nasv1.DockerAppContainer{
			ContainerId: c.ID,
			Name:        c.Name,
			ServiceName: c.ServiceName,
			Image:       c.Image,
			State:       c.State,
			Status:      c.Status,
			CpuPercent:  c.CPUPercent,
			MemoryMib:   c.MemoryMiB,
			Ports:       c.Ports,
			CreatedAt:   c.CreatedAt.Format(time.RFC3339),
			NetRxBytes:  c.NetRxBytes,
			NetTxBytes:  c.NetTxBytes,
			NetRxRate:   c.NetRxRate,
			NetTxRate:   c.NetTxRate,
		})
	}
	return &nasv1.DockerApp{
		Name:            domain.DockerAppName(a.ID),
		AppId:           a.ID,
		Title:           a.Title,
		State:           a.State,
		StatusText:      a.StatusText,
		ComposeYaml:     a.ComposeYAML,
		ComposeFilePath: a.ComposeFilePath,
		Services:        a.Services,
		Containers:      containers,
		PortMappings:    a.PortMappings,
		Volumes:         a.Volumes,
		WebUiUrl:        a.WebUIURL,
		TotalCpuPercent: a.TotalCPUPercent,
		TotalMemoryMib:  a.TotalMemoryMiB,
		CreatedAt:       a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       a.UpdatedAt.Format(time.RFC3339),
		TotalNetRxBytes: a.TotalNetRxBytes,
		TotalNetTxBytes: a.TotalNetTxBytes,
		TotalNetRxRate:  a.TotalNetRxRate,
		TotalNetTxRate:  a.TotalNetTxRate,
	}
}

func (s *Server) ListDockerApps(ctx context.Context, req *nasv1.ListDockerAppsRequest) (*nasv1.ListDockerAppsResponse, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	items, err := s.dockerApp.ListApps(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.DockerApp, 0, len(paged))
	for _, a := range paged {
		out = append(out, dockerAppToProto(&a))
	}
	return &nasv1.ListDockerAppsResponse{
		DockerApps:    out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetDockerApp(ctx context.Context, req *nasv1.GetDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	app, err := s.dockerApp.GetApp(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(app), nil
}

func (s *Server) CreateDockerApp(ctx context.Context, req *nasv1.CreateDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetDockerAppId()
	in := req.GetDockerApp()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "dockerApps/")
	}
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "docker_app is required")
	}
	created, err := s.dockerApp.CreateApp(ctx, id, domain.DockerApp{
		ID:          id,
		Title:       in.GetTitle(),
		ComposeYAML: in.GetComposeYaml(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(created), nil
}

func (s *Server) UpdateDockerApp(ctx context.Context, req *nasv1.UpdateDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetDockerApp()
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "docker_app is required")
	}
	id, err := ParseSingleResourceID(patternDockerApp, in.GetName())
	if err != nil {
		return nil, err
	}
	var paths []string
	if mask := req.GetUpdateMask(); mask != nil {
		paths = mask.GetPaths()
	}
	updated, err := s.dockerApp.UpdateApp(ctx, domain.DockerApp{
		ID:          id,
		Title:       in.GetTitle(),
		ComposeYAML: in.GetComposeYaml(),
	}, paths)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(updated), nil
}

func (s *Server) DeleteDockerApp(ctx context.Context, req *nasv1.DeleteDockerAppRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.dockerApp.DeleteApp(ctx, id, req.GetDeleteVolumes()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) StartDockerApp(ctx context.Context, req *nasv1.StartDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	app, err := s.dockerApp.StartApp(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(app), nil
}

func (s *Server) StopDockerApp(ctx context.Context, req *nasv1.StopDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	app, err := s.dockerApp.StopApp(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(app), nil
}

func (s *Server) RestartDockerApp(ctx context.Context, req *nasv1.RestartDockerAppRequest) (*nasv1.DockerApp, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	app, err := s.dockerApp.RestartApp(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return dockerAppToProto(app), nil
}

func (s *Server) GetDockerAppLogs(ctx context.Context, req *nasv1.GetDockerAppLogsRequest) (*nasv1.GetDockerAppLogsResponse, error) {
	if err := s.prereq.RequireModule("docker"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternDockerApp, req.GetName())
	if err != nil {
		return nil, err
	}
	tail := int(req.GetTail())
	if tail <= 0 {
		tail = 100
	}
	logs, err := s.dockerApp.GetAppLogs(ctx, id, tail, req.GetServiceName())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	lineCount := int32(len(strings.Split(strings.TrimSpace(logs), "\n")))
	if logs == "" {
		lineCount = 0
	}
	return &nasv1.GetDockerAppLogsResponse{
		Logs:      logs,
		LineCount: lineCount,
	}, nil
}

// =============================================================================
// Docker Storage Pool Configuration & Host Filesystem Browser
// =============================================================================

func (s *Server) GetDockerConfig(ctx context.Context, _ *nasv1.GetDockerConfigRequest) (*nasv1.DockerConfig, error) {
	if s.dockerApp == nil {
		return nil, status.Error(codes.Unavailable, "docker app service unavailable")
	}
	cfg, err := s.dockerApp.GetDockerConfig(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return &nasv1.DockerConfig{
		Dataset:           cfg.Dataset,
		Pool:              cfg.Pool,
		Mountpoint:        cfg.Mountpoint,
		AppdataPath:       cfg.AppdataPath,
		DockerDataRoot:    cfg.DockerDataRoot,
		AvailableDatasets: cfg.AvailableDatasets,
		AvailablePools:    cfg.AvailablePools,
		DatasetSelected:   cfg.DatasetSelected,
		AppdataDataset:    cfg.AppdataPath,
		PoolSelected:      cfg.DatasetSelected,
	}, nil
}

func (s *Server) UpdateDockerConfig(ctx context.Context, req *nasv1.UpdateDockerConfigRequest) (*nasv1.DockerConfig, error) {
	if s.dockerApp == nil {
		return nil, status.Error(codes.Unavailable, "docker app service unavailable")
	}
	if req == nil || req.GetDockerConfig() == nil {
		return nil, status.Error(codes.InvalidArgument, "docker_config is required")
	}

	input := domain.DockerConfig{
		Dataset: req.GetDockerConfig().GetDataset(),
		Pool:    req.GetDockerConfig().GetPool(),
	}

	updated, err := s.dockerApp.UpdateDockerConfig(ctx, input)
	if err != nil {
		return nil, ToGRPCError(err)
	}

	if s.catalog != nil {
		s.catalog.SetAppsPool(updated.Pool)
		s.catalog.SetAppdataPath(updated.AppdataPath)
	}

	return &nasv1.DockerConfig{
		Dataset:           updated.Dataset,
		Pool:              updated.Pool,
		Mountpoint:        updated.Mountpoint,
		AppdataPath:       updated.AppdataPath,
		DockerDataRoot:    updated.DockerDataRoot,
		AvailableDatasets: updated.AvailableDatasets,
		AvailablePools:    updated.AvailablePools,
		DatasetSelected:   updated.DatasetSelected,
		AppdataDataset:    updated.AppdataPath,
		PoolSelected:      updated.DatasetSelected,
	}, nil
}

func (s *Server) ListFileSystemDirectories(ctx context.Context, req *nasv1.ListFileSystemDirectoriesRequest) (*nasv1.ListFileSystemDirectoriesResponse, error) {
	if s.dockerApp == nil {
		return nil, status.Error(codes.Unavailable, "docker app service unavailable")
	}
	path := ""
	if req != nil {
		path = req.GetPath()
	}
	res, err := s.dockerApp.ListFileSystemDirectories(ctx, path)
	if err != nil {
		return nil, ToGRPCError(err)
	}

	pbEntries := make([]*nasv1.DirectoryEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		pbEntries = append(pbEntries, &nasv1.DirectoryEntry{
			Name:  e.Name,
			Path:  e.Path,
			IsDir: e.IsDir,
		})
	}

	return &nasv1.ListFileSystemDirectoriesResponse{
		CurrentPath: res.CurrentPath,
		ParentPath:  res.ParentPath,
		Entries:     pbEntries,
	}, nil
}

// =============================================================================
// LXC Containers
// =============================================================================

func (s *Server) ListLxcContainers(ctx context.Context, req *nasv1.ListLxcContainersRequest) (*nasv1.ListLxcContainersResponse, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	items, err := s.lxc.ListContainers(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.LxcContainer, 0, len(paged))
	for _, c := range paged {
		out = append(out, toProtoLxc(c))
	}
	return &nasv1.ListLxcContainersResponse{
		LxcContainers: out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetLxcContainer(ctx context.Context, req *nasv1.GetLxcContainerRequest) (*nasv1.LxcContainer, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternLxcContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.lxc.GetContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoLxc(*c), nil
}

func (s *Server) CreateLxcContainer(ctx context.Context, req *nasv1.CreateLxcContainerRequest) (*nasv1.LxcContainer, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetLxcContainerId()
	in := req.GetLxcContainer()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "lxcContainers/")
	}
	created, err := s.lxc.CreateContainer(ctx, domain.LxcContainer{
		ID:             id,
		Distribution:   in.GetDistribution(),
		Release:        in.GetRelease(),
		Architecture:   in.GetArchitecture(),
		Autostart:      in.GetAutostart(),
		Unprivileged:   in.GetUnprivileged(),
		RootfsPath:     in.GetRootfsPath(),
		MemoryLimitMiB: in.GetMemoryLimitMib(),
		CPUCores:       in.GetCpuCores(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoLxc(*created), nil
}

func (s *Server) DeleteLxcContainer(ctx context.Context, req *nasv1.DeleteLxcContainerRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternLxcContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.lxc.DeleteContainer(ctx, id, req.GetForce()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) StartLxcContainer(ctx context.Context, req *nasv1.StartLxcContainerRequest) (*nasv1.LxcContainer, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternLxcContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.lxc.StartContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoLxc(*c), nil
}

func (s *Server) StopLxcContainer(ctx context.Context, req *nasv1.StopLxcContainerRequest) (*nasv1.LxcContainer, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternLxcContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.lxc.StopContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoLxc(*c), nil
}

func (s *Server) FreezeLxcContainer(ctx context.Context, req *nasv1.FreezeLxcContainerRequest) (*nasv1.LxcContainer, error) {
	if err := s.prereq.RequireModule("lxc"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternLxcContainer, req.GetName())
	if err != nil {
		return nil, err
	}
	c, err := s.lxc.FreezeContainer(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoLxc(*c), nil
}

// =============================================================================
// CIFS & WebDAV Shares
// =============================================================================

func (s *Server) ListCifsShares(ctx context.Context, req *nasv1.ListCifsSharesRequest) (*nasv1.ListCifsSharesResponse, error) {
	if err := s.prereq.RequireModule("cifs"); err != nil {
		return nil, ToGRPCError(err)
	}
	items, err := s.share.ListCifsShares(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.CifsShare, 0, len(paged))
	for _, sh := range paged {
		out = append(out, toProtoCifs(sh))
	}
	return &nasv1.ListCifsSharesResponse{
		CifsShares:    out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetCifsShare(ctx context.Context, req *nasv1.GetCifsShareRequest) (*nasv1.CifsShare, error) {
	if err := s.prereq.RequireModule("cifs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternCifsShare, req.GetName())
	if err != nil {
		return nil, err
	}
	sh, err := s.share.GetCifsShare(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoCifs(*sh), nil
}

func (s *Server) CreateCifsShare(ctx context.Context, req *nasv1.CreateCifsShareRequest) (*nasv1.CifsShare, error) {
	if err := s.prereq.RequireModule("cifs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id := req.GetCifsShareId()
	in := req.GetCifsShare()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "cifsShares/")
	}
	created, err := s.share.CreateCifsShare(ctx, domain.CifsShare{
		ID:            id,
		Path:          in.GetPath(),
		Comment:       in.GetComment(),
		ReadOnly:      in.GetReadOnly(),
		GuestOk:       in.GetGuestOk(),
		Browseable:    in.GetBrowseable(),
		ValidUsers:    in.GetValidUsers(),
		ValidGroups:   in.GetValidGroups(),
		CreateMask:    in.GetCreateMask(),
		DirectoryMask: in.GetDirectoryMask(),
		Enabled:       in.GetEnabled(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoCifs(*created), nil
}

func (s *Server) UpdateCifsShare(ctx context.Context, req *nasv1.UpdateCifsShareRequest) (*nasv1.CifsShare, error) {
	if err := s.prereq.RequireModule("cifs"); err != nil {
		return nil, ToGRPCError(err)
	}
	in := req.GetCifsShare()
	id, err := ParseSingleResourceID(patternCifsShare, in.GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.share.UpdateCifsShare(ctx, domain.CifsShare{
		ID:            id,
		Path:          in.GetPath(),
		Comment:       in.GetComment(),
		ReadOnly:      in.GetReadOnly(),
		GuestOk:       in.GetGuestOk(),
		Browseable:    in.GetBrowseable(),
		ValidUsers:    in.GetValidUsers(),
		ValidGroups:   in.GetValidGroups(),
		CreateMask:    in.GetCreateMask(),
		DirectoryMask: in.GetDirectoryMask(),
		Enabled:       in.GetEnabled(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoCifs(*updated), nil
}

func (s *Server) DeleteCifsShare(ctx context.Context, req *nasv1.DeleteCifsShareRequest) (*emptypb.Empty, error) {
	if err := s.prereq.RequireModule("cifs"); err != nil {
		return nil, ToGRPCError(err)
	}
	id, err := ParseSingleResourceID(patternCifsShare, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.share.DeleteCifsShare(ctx, id); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ListWebdavShares(ctx context.Context, req *nasv1.ListWebdavSharesRequest) (*nasv1.ListWebdavSharesResponse, error) {
	items, err := s.share.ListWebdavShares(ctx)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	paged, nextToken, total, err := PaginateSlice(items, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.WebdavShare, 0, len(paged))
	for _, sh := range paged {
		out = append(out, toProtoWebdav(sh))
	}
	return &nasv1.ListWebdavSharesResponse{
		WebdavShares:  out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetWebdavShare(ctx context.Context, req *nasv1.GetWebdavShareRequest) (*nasv1.WebdavShare, error) {
	id, err := ParseSingleResourceID(patternWebdavShare, req.GetName())
	if err != nil {
		return nil, err
	}
	sh, err := s.share.GetWebdavShare(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoWebdav(*sh), nil
}

func (s *Server) CreateWebdavShare(ctx context.Context, req *nasv1.CreateWebdavShareRequest) (*nasv1.WebdavShare, error) {
	id := req.GetWebdavShareId()
	in := req.GetWebdavShare()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "webdavShares/")
	}
	created, err := s.share.CreateWebdavShare(ctx, domain.WebdavShare{
		ID:            id,
		Path:          in.GetPath(),
		Description:   in.GetDescription(),
		ReadOnly:      in.GetReadOnly(),
		AllowedUsers:  in.GetAllowedUsers(),
		AllowedGroups: in.GetAllowedGroups(),
		Enabled:       in.GetEnabled(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoWebdav(*created), nil
}

func (s *Server) UpdateWebdavShare(ctx context.Context, req *nasv1.UpdateWebdavShareRequest) (*nasv1.WebdavShare, error) {
	in := req.GetWebdavShare()
	id, err := ParseSingleResourceID(patternWebdavShare, in.GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.share.UpdateWebdavShare(ctx, domain.WebdavShare{
		ID:            id,
		Path:          in.GetPath(),
		Description:   in.GetDescription(),
		ReadOnly:      in.GetReadOnly(),
		AllowedUsers:  in.GetAllowedUsers(),
		AllowedGroups: in.GetAllowedGroups(),
		Enabled:       in.GetEnabled(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoWebdav(*updated), nil
}

func (s *Server) DeleteWebdavShare(ctx context.Context, req *nasv1.DeleteWebdavShareRequest) (*emptypb.Empty, error) {
	id, err := ParseSingleResourceID(patternWebdavShare, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.share.DeleteWebdavShare(ctx, id); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

// =============================================================================
// Linux Users & Groups
// =============================================================================

func (s *Server) ListUsers(ctx context.Context, req *nasv1.ListUsersRequest) (*nasv1.ListUsersResponse, error) {
	users, err := s.identity.ListUsers(ctx, req.GetIncludeSystemAccounts())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	paged, nextToken, total, err := PaginateSlice(users, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.User, 0, len(paged))
	for _, u := range paged {
		out = append(out, toProtoUser(u))
	}
	return &nasv1.ListUsersResponse{
		Users:         out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetUser(ctx context.Context, req *nasv1.GetUserRequest) (*nasv1.User, error) {
	id, err := ParseSingleResourceID(patternUser, req.GetName())
	if err != nil {
		return nil, err
	}
	u, err := s.identity.GetUser(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoUser(*u), nil
}

func (s *Server) CreateUser(ctx context.Context, req *nasv1.CreateUserRequest) (*nasv1.User, error) {
	id := req.GetUserId()
	in := req.GetUser()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "users/")
	}
	created, err := s.identity.CreateUser(ctx, domain.User{
		ID:                  id,
		UID:                 in.GetUid(),
		PrimaryGroup:        in.GetPrimaryGroup(),
		SupplementaryGroups: in.GetSupplementaryGroups(),
		FullName:            in.GetFullName(),
		HomeDirectory:       in.GetHomeDirectory(),
		Shell:               in.GetShell(),
		SSHEnabled:          in.GetSshEnabled(),
		SSHPasswordAuth:     in.GetSshPasswordAuth(),
		SSHAuthorizedKeys:   in.GetSshAuthorizedKeys(),
		Locked:              in.GetLocked(),
		Password:            in.GetPassword(),
		IsAdmin:             in.GetIsAdmin(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoUser(*created), nil
}

func (s *Server) UpdateUser(ctx context.Context, req *nasv1.UpdateUserRequest) (*nasv1.User, error) {
	in := req.GetUser()
	id, err := ParseSingleResourceID(patternUser, in.GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.identity.UpdateUser(ctx, domain.User{
		ID:                  id,
		PrimaryGroup:        in.GetPrimaryGroup(),
		SupplementaryGroups: in.GetSupplementaryGroups(),
		FullName:            in.GetFullName(),
		HomeDirectory:       in.GetHomeDirectory(),
		Shell:               in.GetShell(),
		SSHEnabled:          in.GetSshEnabled(),
		SSHPasswordAuth:     in.GetSshPasswordAuth(),
		SSHAuthorizedKeys:   in.GetSshAuthorizedKeys(),
		Locked:              in.GetLocked(),
		Password:            in.GetPassword(),
		IsAdmin:             in.GetIsAdmin(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoUser(*updated), nil
}

func (s *Server) DeleteUser(ctx context.Context, req *nasv1.DeleteUserRequest) (*emptypb.Empty, error) {
	id, err := ParseSingleResourceID(patternUser, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.identity.DeleteUser(ctx, id, req.GetRemoveHome()); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) ListGroups(ctx context.Context, req *nasv1.ListGroupsRequest) (*nasv1.ListGroupsResponse, error) {
	groups, err := s.identity.ListGroups(ctx, req.GetIncludeSystemGroups())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })
	paged, nextToken, total, err := PaginateSlice(groups, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := make([]*nasv1.Group, 0, len(paged))
	for _, g := range paged {
		out = append(out, toProtoGroup(g))
	}
	return &nasv1.ListGroupsResponse{
		Groups:        out,
		NextPageToken: nextToken,
		TotalSize:     total,
	}, nil
}

func (s *Server) GetGroup(ctx context.Context, req *nasv1.GetGroupRequest) (*nasv1.Group, error) {
	id, err := ParseSingleResourceID(patternGroup, req.GetName())
	if err != nil {
		return nil, err
	}
	g, err := s.identity.GetGroup(ctx, id)
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoGroup(*g), nil
}

func (s *Server) CreateGroup(ctx context.Context, req *nasv1.CreateGroupRequest) (*nasv1.Group, error) {
	id := req.GetGroupId()
	in := req.GetGroup()
	if id == "" && in != nil {
		id = strings.TrimPrefix(in.GetName(), "groups/")
	}
	created, err := s.identity.CreateGroup(ctx, domain.Group{
		ID:      id,
		GID:     in.GetGid(),
		Members: in.GetMembers(),
	})
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoGroup(*created), nil
}

func (s *Server) UpdateGroup(ctx context.Context, req *nasv1.UpdateGroupRequest) (*nasv1.Group, error) {
	in := req.GetGroup()
	id, err := ParseSingleResourceID(patternGroup, in.GetName())
	if err != nil {
		return nil, err
	}
	updated, err := s.identity.UpdateGroup(ctx, domain.Group{
		ID:      id,
		GID:     in.GetGid(),
		Members: in.GetMembers(),
	}, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, ToGRPCError(err)
	}
	return toProtoGroup(*updated), nil
}

func (s *Server) DeleteGroup(ctx context.Context, req *nasv1.DeleteGroupRequest) (*emptypb.Empty, error) {
	id, err := ParseSingleResourceID(patternGroup, req.GetName())
	if err != nil {
		return nil, err
	}
	if err := s.identity.DeleteGroup(ctx, id); err != nil {
		return nil, ToGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

// =============================================================================
// Domain -> Protobuf Mappers (using go.einride.tech/aip/resourcename.Sprint)
// =============================================================================

func toProtoSystemConfig(c *domain.SystemConfig) *nasv1.SystemConfig {
	if c == nil {
		return &nasv1.SystemConfig{}
	}
	var pools []*nasv1.DiscoveredPool
	for _, p := range c.DiscoveredPools {
		pools = append(pools, toProtoDiscoveredPool(p))
	}
	return &nasv1.SystemConfig{
		MainPool:         c.MainPool,
		SystemDataset:    c.SystemDataset,
		SystemMountpoint: c.SystemMountpoint,
		IsInitialized:    c.IsInitialized,
		EnabledModules:   c.EnabledModules,
		ConfigFilePath:   c.ConfigFilePath,
		AutoDiscover:     c.AutoDiscover,
		DiscoveredPools:  pools,
	}
}

func toProtoDiscoveredPool(p domain.DiscoveredPool) *nasv1.DiscoveredPool {
	return &nasv1.DiscoveredPool{
		PoolName:      p.PoolName,
		HasNasSystem:  p.HasNasSystem,
		ConfigVersion: p.ConfigVersion,
		LastModified:  p.LastModified,
		IsImported:    p.IsImported,
		DatasetPath:   p.DatasetPath,
		Mountpoint:    p.Mountpoint,
	}
}

func toProtoLinuxFlavor(f domain.LinuxFlavor) *nasv1.LinuxFlavor {
	return &nasv1.LinuxFlavor{
		Id:                     f.ID,
		Name:                   f.Name,
		PrettyName:             f.PrettyName,
		VersionId:              f.VersionID,
		PackageManager:         f.PackageManager,
		InstallCommandTemplate: f.InstallCommandTemplate,
	}
}

func toProtoPrerequisite(p domain.Prerequisite) *nasv1.Prerequisite {
	return &nasv1.Prerequisite{
		Name:         resourcename.Sprint(patternPrerequisite, p.ID),
		DisplayName:  p.DisplayName,
		Category:     p.Category,
		Kind:         p.Kind,
		BinaryOrPath: p.BinaryOrPath,
		Status:       string(p.Status),
		Version:      p.Version,
		Details:      p.Details,
		InstallHint:  p.InstallHint,
		RequiredBy:   p.RequiredBy,
		FallbackMode: p.FallbackMode,
		CheckedTime:  timestamppb.New(p.CheckedTime),
	}
}

func toProtoZfsVdev(v domain.ZfsVdev) *nasv1.ZfsVdev {
	var children []*nasv1.ZfsVdev
	for _, c := range v.Children {
		children = append(children, toProtoZfsVdev(c))
	}
	return &nasv1.ZfsVdev{
		Name:           v.Name,
		Type:           v.Type,
		Layout:         v.Layout,
		State:          v.State,
		Path:           v.Path,
		ReadErrors:     v.ReadErrors,
		WriteErrors:    v.WriteErrors,
		ChecksumErrors: v.ChecksumErrors,
		Children:       children,
	}
}

func toProtoZfsPool(p domain.ZfsPool) *nasv1.ZfsPool {
	var protoVdevs []*nasv1.ZfsVdev
	for _, v := range p.Vdevs {
		protoVdevs = append(protoVdevs, toProtoZfsVdev(v))
	}
	return &nasv1.ZfsPool{
		Name:                 resourcename.Sprint(patternZfsPool, p.ID),
		PoolName:             p.PoolName,
		State:                p.State,
		SizeBytes:            p.SizeBytes,
		AllocatedBytes:       p.AllocatedBytes,
		FreeBytes:            p.FreeBytes,
		FragmentationPercent: p.FragmentationPercent,
		CapacityPercent:      p.CapacityPercent,
		DedupRatio:           p.DedupRatio,
		VdevLayout:           p.VdevLayout,
		Devices:              p.Devices,
		Mountpoint:           p.Mountpoint,
		Compression:          p.Compression,
		LastScrubStatus:      p.LastScrubStatus,
		HealthDetails:        p.HealthDetails,
		Sync:                 p.Sync,
		Autotrim:             p.Autotrim,
		Vdevs:                protoVdevs,
	}
}

func toProtoZfsDataset(d domain.ZfsDataset) *nasv1.ZfsDataset {
	return &nasv1.ZfsDataset{
		Name:             resourcename.Sprint(patternZfsDataset, d.PoolID, d.ID),
		DatasetPath:      d.DatasetPath,
		Mountpoint:       d.Mountpoint,
		UsedBytes:        d.UsedBytes,
		AvailableBytes:   d.AvailableBytes,
		ReferencedBytes:  d.ReferencedBytes,
		QuotaBytes:       d.QuotaBytes,
		Compression:      d.Compression,
		Readonly:         d.Readonly,
		Atime:            d.Atime,
		Recordsize:       d.Recordsize,
		SnapshotsCount:   d.SnapshotsCount,
		LastSnapshot:     d.LastSnapshot,
		Sync:             d.Sync,
		ReservationBytes: d.ReservationBytes,
		Dedup:            d.Dedup,
	}
}

func toProtoZfsVolume(v domain.ZfsVolume) *nasv1.ZfsVolume {
	return &nasv1.ZfsVolume{
		Name:            resourcename.Sprint(patternZfsVolume, v.PoolID, v.ID),
		VolumePath:      v.VolumePath,
		DevicePath:      v.DevicePath,
		SizeBytes:       v.SizeBytes,
		UsedBytes:       v.UsedBytes,
		Volblocksize:    v.Volblocksize,
		Compression:     v.Compression,
		ThinProvisioned: v.ThinProvisioned,
		Sync:            v.Sync,
		Readonly:        v.Readonly,
		Dedup:           v.Dedup,
	}
}

func toProtoVM(vm domain.VirtualMachine) *nasv1.VirtualMachine {
	var protoDisks []*nasv1.VirtualMachineDisk
	for _, d := range vm.Disks {
		protoDisks = append(protoDisks, toProtoVMDisk(d))
	}
	var protoIfaces []*nasv1.VirtualMachineNetworkInterface
	for _, iface := range vm.NetworkIfaces {
		protoIfaces = append(protoIfaces, toProtoVMNetworkInterface(iface))
	}
	var protoPcis []*nasv1.VirtualMachinePciDevice
	for _, p := range vm.PciDevices {
		protoPcis = append(protoPcis, &nasv1.VirtualMachinePciDevice{
			HostAddress: p.HostAddress,
			Pcie:        p.PCIE,
			PrimaryGpu:  p.PrimaryGPU,
		})
	}
	var protoUsbs []*nasv1.VirtualMachineUsbDevice
	for _, u := range vm.UsbDevices {
		protoUsbs = append(protoUsbs, &nasv1.VirtualMachineUsbDevice{
			HostBusPort:     u.HostBusPort,
			VendorProductId: u.VendorProductID,
		})
	}

	return &nasv1.VirtualMachine{
		Name:                resourcename.Sprint(patternVirtualMachine, vm.ID),
		VmName:              vm.VMName,
		Uuid:                vm.UUID,
		State:               vm.State,
		Vcpus:               vm.VCPUs,
		MemoryMib:           vm.MemoryMiB,
		DiskPath:            vm.DiskPath,
		DiskSizeGib:         vm.DiskSizeGiB,
		IsoImage:            vm.ISOImage,
		CdromIsoImage:       vm.CdromIsoImage,
		NetworkBridge:       vm.NetworkBridge,
		VncPort:             vm.VNCPort,
		VncDisplay:          vm.VNCDisplay,
		Autostart:           vm.Autostart,
		OsVariant:           vm.OSVariant,
		Description:         vm.Description,
		Sockets:             vm.Sockets,
		Cores:               vm.Cores,
		CpuType:             vm.CPUType,
		CpuUnits:            vm.CPUUnits,
		CpuLimit:            vm.CPULimit,
		Numa:                vm.NUMA,
		BalloonMinMib:       vm.BalloonMinMiB,
		Hugepages:           vm.Hugepages,
		MachineType:         vm.MachineType,
		BiosType:            vm.BiosType,
		TpmVersion:          vm.TPMVersion,
		SecureBoot:          vm.SecureBoot,
		Disks:               protoDisks,
		NetworkInterfaces:   protoIfaces,
		CloudInit:           toProtoVMCloudInit(vm.CloudInit),
		PciDevices:          protoPcis,
		UsbDevices:          protoUsbs,
		AgentEnabled:        vm.AgentEnabled,
		AgentRunning:        vm.AgentRunning,
		GuestIpAddresses:    vm.GuestIPAddresses,
		GuestOsName:         vm.GuestOSName,
		GuestCpuPercent:     vm.GuestCPUPercent,
		GuestMemoryUsedMib:  vm.GuestMemoryUsedMiB,
		StartupDelaySeconds: vm.StartupDelaySeconds,
		BootOrder:           vm.BootOrder,
		WatchdogModel:       vm.WatchdogModel,
		WatchdogAction:      vm.WatchdogAction,
		DisplayType:         vm.DisplayType,
		IsTemplate:          vm.IsTemplate,
		QemuCommandlineArgs: vm.QemuCommandlineArgs,
		SerialConsoleUrl:    vm.SerialConsoleURL,
	}
}

func toProtoVMDisk(d domain.VirtualMachineDisk) *nasv1.VirtualMachineDisk {
	return &nasv1.VirtualMachineDisk{
		Id:             d.ID,
		Bus:            d.Bus,
		DiskPath:       d.DiskPath,
		SizeBytes:      d.SizeBytes,
		CacheMode:      d.CacheMode,
		Discard:        d.Discard,
		Iothread:       d.IOThread,
		ReadOnly:       d.ReadOnly,
		SsdEmulation:   d.SSDEmulation,
		Backup:         d.Backup,
		IopsLimitRead:  d.IOPSLimitRead,
		IopsLimitWrite: d.IOPSLimitWrite,
		MbpsLimitRead:  d.MbpsLimitRead,
		MbpsLimitWrite: d.MbpsLimitWrite,
	}
}

func fromProtoVMDisk(d *nasv1.VirtualMachineDisk) domain.VirtualMachineDisk {
	if d == nil {
		return domain.VirtualMachineDisk{}
	}
	return domain.VirtualMachineDisk{
		ID:             d.GetId(),
		Bus:            d.GetBus(),
		DiskPath:       d.GetDiskPath(),
		SizeBytes:      d.GetSizeBytes(),
		CacheMode:      d.GetCacheMode(),
		Discard:        d.GetDiscard(),
		IOThread:       d.GetIothread(),
		ReadOnly:       d.GetReadOnly(),
		SSDEmulation:   d.GetSsdEmulation(),
		Backup:         d.GetBackup(),
		IOPSLimitRead:  d.GetIopsLimitRead(),
		IOPSLimitWrite: d.GetIopsLimitWrite(),
		MbpsLimitRead:  d.GetMbpsLimitRead(),
		MbpsLimitWrite: d.GetMbpsLimitWrite(),
	}
}

func toProtoVMNetworkInterface(iface domain.VirtualMachineNetworkInterface) *nasv1.VirtualMachineNetworkInterface {
	return &nasv1.VirtualMachineNetworkInterface{
		Id:            iface.ID,
		Model:         iface.Model,
		Bridge:        iface.Bridge,
		VlanTag:       iface.VlanTag,
		MacAddress:    iface.MacAddress,
		RateLimitMbps: iface.RateLimitMbps,
		Disconnected:  iface.Disconnected,
		Firewall:      iface.Firewall,
	}
}

func fromProtoVMNetworkInterface(iface *nasv1.VirtualMachineNetworkInterface) domain.VirtualMachineNetworkInterface {
	if iface == nil {
		return domain.VirtualMachineNetworkInterface{}
	}
	return domain.VirtualMachineNetworkInterface{
		ID:            iface.GetId(),
		Model:         iface.GetModel(),
		Bridge:        iface.GetBridge(),
		VlanTag:       iface.GetVlanTag(),
		MacAddress:    iface.GetMacAddress(),
		RateLimitMbps: iface.GetRateLimitMbps(),
		Disconnected:  iface.GetDisconnected(),
		Firewall:      iface.GetFirewall(),
	}
}

func toProtoVMCloudInit(c domain.VirtualMachineCloudInit) *nasv1.VirtualMachineCloudInit {
	return &nasv1.VirtualMachineCloudInit{
		User:           c.User,
		Password:       c.Password,
		SshPublicKeys:  c.SSHPublicKeys,
		IpConfig:       c.IPConfig,
		DnsNameservers: c.DNSNameservers,
		DnsSearch:      c.DNSSearch,
		CustomUserData: c.CustomUserData,
	}
}

func fromProtoVMCloudInit(c *nasv1.VirtualMachineCloudInit) domain.VirtualMachineCloudInit {
	if c == nil {
		return domain.VirtualMachineCloudInit{}
	}
	return domain.VirtualMachineCloudInit{
		User:           c.GetUser(),
		Password:       c.GetPassword(),
		SSHPublicKeys:  c.GetSshPublicKeys(),
		IPConfig:       c.GetIpConfig(),
		DNSNameservers: c.GetDnsNameservers(),
		DNSSearch:      c.GetDnsSearch(),
		CustomUserData: c.GetCustomUserData(),
	}
}

func toProtoVMSnapshot(vmID string, s domain.VirtualMachineSnapshot) *nasv1.VirtualMachineSnapshot {
	return &nasv1.VirtualMachineSnapshot{
		Name:           resourcename.Sprint(patternVirtualMachineSnapshot, vmID, s.SnapshotID),
		SnapshotId:     s.SnapshotID,
		Description:    s.Description,
		CreateTime:     timestamppb.New(s.CreateTime),
		IncludeRam:     s.IncludeRAM,
		ParentSnapshot: s.ParentSnapshot,
		VmState:        s.VMState,
	}
}

func toProtoDocker(c domain.DockerContainer) *nasv1.DockerContainer {
	return &nasv1.DockerContainer{
		Name:           resourcename.Sprint(patternDockerContainer, c.ID),
		ContainerId:    c.ContainerID,
		ContainerName:  c.ContainerName,
		Image:          c.Image,
		State:          c.State,
		StatusText:     c.StatusText,
		Ports:          c.Ports,
		Volumes:        c.Volumes,
		Environment:    c.Environment,
		RestartPolicy:  c.RestartPolicy,
		NetworkMode:    c.NetworkMode,
		CpuPercent:     c.CPUPercent,
		MemoryUsageMib: c.MemoryUsageMiB,
		CreatedAt:      c.CreatedAt,
		MemoryLimitMib: c.MemoryLimitMiB,
		Command:        c.Command,
		Privileged:     c.Privileged,
		ComposeYaml:    c.ComposeYAML,
	}
}

func toProtoLxc(c domain.LxcContainer) *nasv1.LxcContainer {
	return &nasv1.LxcContainer{
		Name:           resourcename.Sprint(patternLxcContainer, c.ID),
		ContainerName:  c.ContainerName,
		State:          c.State,
		Distribution:   c.Distribution,
		Release:        c.Release,
		Architecture:   c.Architecture,
		Ipv4Address:    c.IPv4Address,
		Autostart:      c.Autostart,
		Unprivileged:   c.Unprivileged,
		RootfsPath:     c.RootfsPath,
		MemoryLimitMib: c.MemoryLimitMiB,
		CpuCores:       c.CPUCores,
	}
}

func toProtoCifs(s domain.CifsShare) *nasv1.CifsShare {
	return &nasv1.CifsShare{
		Name:          resourcename.Sprint(patternCifsShare, s.ID),
		ShareName:     s.ShareName,
		Path:          s.Path,
		Comment:       s.Comment,
		ReadOnly:      s.ReadOnly,
		GuestOk:       s.GuestOk,
		Browseable:    s.Browseable,
		ValidUsers:    s.ValidUsers,
		ValidGroups:   s.ValidGroups,
		CreateMask:    s.CreateMask,
		DirectoryMask: s.DirectoryMask,
		Enabled:       s.Enabled,
	}
}

func toProtoWebdav(s domain.WebdavShare) *nasv1.WebdavShare {
	return &nasv1.WebdavShare{
		Name:          resourcename.Sprint(patternWebdavShare, s.ID),
		ShareName:     s.ShareName,
		Path:          s.Path,
		UrlPrefix:     s.URLPrefix,
		Description:   s.Description,
		ReadOnly:      s.ReadOnly,
		AllowedUsers:  s.AllowedUsers,
		AllowedGroups: s.AllowedGroups,
		Enabled:       s.Enabled,
	}
}

func toProtoUser(u domain.User) *nasv1.User {
	return &nasv1.User{
		Name:                resourcename.Sprint(patternUser, u.ID),
		Username:            u.Username,
		Uid:                 u.UID,
		PrimaryGid:          u.PrimaryGID,
		PrimaryGroup:        u.PrimaryGroup,
		SupplementaryGroups: u.SupplementaryGroups,
		FullName:            u.FullName,
		HomeDirectory:       u.HomeDirectory,
		Shell:               u.Shell,
		SshEnabled:          u.SSHEnabled,
		SshPasswordAuth:     u.SSHPasswordAuth,
		SshAuthorizedKeys:   u.SSHAuthorizedKeys,
		Locked:              u.Locked,
		SystemAccount:       u.SystemAccount,
		IsAdmin:             u.IsAdmin,
	}
}

func toProtoGroup(g domain.Group) *nasv1.Group {
	return &nasv1.Group{
		Name:        resourcename.Sprint(patternGroup, g.ID),
		GroupName:   g.GroupName,
		Gid:         g.GID,
		Members:     g.Members,
		SystemGroup: g.SystemGroup,
	}
}

// -----------------------------------------------------------------------------
// Application Catalog RPC Handlers
// -----------------------------------------------------------------------------

func (s *Server) ListCatalogApps(ctx context.Context, req *nasv1.ListCatalogAppsRequest) (*nasv1.ListCatalogAppsResponse, error) {
	if s.catalog == nil {
		return &nasv1.ListCatalogAppsResponse{}, nil
	}
	train := ""
	category := ""
	query := ""
	if req != nil {
		train = req.GetTrain()
		category = req.GetCategory()
		query = req.GetQuery()
	}

	apps, err := s.catalog.ListApps(ctx, train, category, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list catalog apps: %v", err)
	}

	pbApps := make([]*nasv1.CatalogApp, 0, len(apps))
	for _, a := range apps {
		pbApps = append(pbApps, domainCatalogAppToPB(a))
	}

	var lastSync string
	syncT := s.catalog.LastSyncTime()
	if !syncT.IsZero() {
		lastSync = syncT.Format(time.RFC3339)
	}

	return &nasv1.ListCatalogAppsResponse{
		Apps:         pbApps,
		TotalSize:    int32(len(pbApps)),
		LastSyncTime: lastSync,
	}, nil
}

func (s *Server) GetCatalogApp(ctx context.Context, req *nasv1.GetCatalogAppRequest) (*nasv1.CatalogApp, error) {
	if s.catalog == nil {
		return nil, status.Error(codes.NotFound, "catalog service unavailable")
	}
	if req == nil || req.GetAppId() == "" {
		return nil, status.Error(codes.InvalidArgument, "app_id is required")
	}

	app, err := s.catalog.GetApp(ctx, req.GetTrain(), req.GetAppId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "catalog app not found: %v", err)
	}

	return domainCatalogAppToPB(*app), nil
}

func (s *Server) SyncCatalog(ctx context.Context, req *nasv1.SyncCatalogRequest) (*nasv1.SyncCatalogResponse, error) {
	if s.catalog == nil {
		return nil, status.Error(codes.Unavailable, "catalog service unavailable")
	}
	force := false
	if req != nil {
		force = req.GetForce()
	}

	count, syncTime, err := s.catalog.SyncCatalog(ctx, force)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "catalog sync failed: %v", err)
	}

	return &nasv1.SyncCatalogResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully synced %d apps from TrueNAS SCALE catalog", count),
		TotalApps:    int32(count),
		LastSyncTime: syncTime.Format(time.RFC3339),
	}, nil
}

func domainCatalogAppToPB(a domain.CatalogApp) *nasv1.CatalogApp {
	return &nasv1.CatalogApp{
		AppId:       a.AppID,
		Title:       a.Title,
		Description: a.Description,
		Category:    a.Category,
		Categories:  a.Categories,
		IconUrl:     a.IconURL,
		Home:        a.Home,
		Train:       a.Train,
		Version:     a.Version,
		AppVersion:  a.AppVersion,
		Tags:        a.Tags,
		Screenshots: a.Screenshots,
		ComposeYaml: a.ComposeYAML,
		DefaultPort: int32(a.DefaultPort),
	}
}
