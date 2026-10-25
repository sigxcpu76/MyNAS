package grpcadapter_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/kvm"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type stubZfsServerPort struct {
	ports.ZfsEnginePort
	tempDir  string
	pools    map[string]domain.ZfsPool
	datasets map[string]domain.ZfsDataset
}

func newStubZfsServerPort(tempDir string) *stubZfsServerPort {
	return &stubZfsServerPort{
		tempDir: tempDir,
		pools: map[string]domain.ZfsPool{
			"tank": {
				ID:         "tank",
				PoolName:   "tank",
				Mountpoint: tempDir,
				State:      "ONLINE",
			},
		},
		datasets: make(map[string]domain.ZfsDataset),
	}
}

func (s *stubZfsServerPort) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	var res []domain.ZfsPool
	for _, p := range s.pools {
		res = append(res, p)
	}
	return res, nil
}

func (s *stubZfsServerPort) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	p, ok := s.pools[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &p, nil
}

func (s *stubZfsServerPort) CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error) {
	if pool.Mountpoint == "" {
		pool.Mountpoint = filepath.Join(s.tempDir, pool.ID)
	}
	s.pools[pool.ID] = pool
	return &pool, nil
}

func (s *stubZfsServerPort) ExportPool(ctx context.Context, id string, force bool) error {
	if _, ok := s.pools[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.pools, id)
	return nil
}

func (s *stubZfsServerPort) DeletePool(ctx context.Context, id string, force bool) error {
	if _, ok := s.pools[id]; !ok {
		return domain.ErrNotFound
	}
	delete(s.pools, id)
	return nil
}

func (s *stubZfsServerPort) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	key := poolID + "/" + datasetID
	ds, ok := s.datasets[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &ds, nil
}

func (s *stubZfsServerPort) CreateDataset(ctx context.Context, ds domain.ZfsDataset) (*domain.ZfsDataset, error) {
	key := ds.PoolID + "/" + ds.ID
	s.datasets[key] = ds
	return &ds, nil
}

func (s *stubZfsServerPort) ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	var res []domain.ZfsDataset
	for k, d := range s.datasets {
		if strings.HasPrefix(k, poolID+"/") {
			res = append(res, d)
		}
	}
	return res, nil
}

func (s *stubZfsServerPort) ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	return nil, nil
}

func TestAIPResourceNamesPaginationAndErrors(t *testing.T) {
	ctx := context.Background()
	dockerAdapter := docker.NewAdapter()
	zfsAdapter := newStubZfsServerPort(t.TempDir())
	prereqChecker := prereq.NewSystemChecker()
	prereqChecker.SetAllModulesInstalledForTest()
	sysAdapter := system.NewAdapter(zfsAdapter, prereqChecker)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(prereqChecker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter()),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter()),
		application.NewShareService(share.NewCifsAdapter(t.TempDir()), share.NewWebdavAdapter()),
		application.NewIdentityService(identity.NewAdapter(), "grpc-test-secret"),
		application.NewCatalogService(catalog.NewAdapter(t.TempDir())),
	)

	// 1. Test AIP-158 Pagination on ListPrerequisites
	page1, err := srv.ListPrerequisites(ctx, &nasv1.ListPrerequisitesRequest{PageSize: 4})
	if err != nil {
		t.Fatalf("ListPrerequisites page 1 failed: %v", err)
	}
	if len(page1.GetPrerequisites()) != 4 || page1.GetNextPageToken() == "" {
		t.Fatalf("expected 4 items and non-empty next_page_token, got len=%d token=%q", len(page1.GetPrerequisites()), page1.GetNextPageToken())
	}

	page2, err := srv.ListPrerequisites(ctx, &nasv1.ListPrerequisitesRequest{
		PageSize:  4,
		PageToken: page1.GetNextPageToken(),
	})
	if err != nil || len(page2.GetPrerequisites()) == 0 {
		t.Fatalf("ListPrerequisites page 2 failed: %v", err)
	}

	// 2. Test AIP-122 Invalid Resource Name rejection -> InvalidArgument
	_, err = srv.GetZfsPool(ctx, &nasv1.GetZfsPoolRequest{Name: "wrongCollection/tank"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for mismatched resource name pattern, got %v", status.Code(err))
	}

	// 3. Test AIP-193 NotFound mapping
	_, err = srv.GetZfsPool(ctx, &nasv1.GetZfsPoolRequest{Name: "zfsPools/nonexistent"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for nonexistent ZFS pool, got %v", status.Code(err))
	}

	// 4. Test AIP-134 Update with FieldMask
	updatedUser, err := srv.UpdateUser(ctx, &nasv1.UpdateUserRequest{
		User: &nasv1.User{
			Name:              "users/sigxcpu",
			FullName:          "Updated Linux User",
			SshEnabled:        true,
			SshPasswordAuth:   false,
			SshAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIUpdatedKey sigxcpu@nas"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"full_name", "ssh_enabled", "ssh_password_auth", "ssh_authorized_keys"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateUser with FieldMask failed: %v", err)
	}
	if updatedUser.GetFullName() != "Updated Linux User" || len(updatedUser.GetSshAuthorizedKeys()) != 1 {
		t.Fatalf("unexpected updated user fields: %+v", updatedUser)
	}
}

func TestModuleAvailabilityEnforcement(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker()
	dockerAdapter := docker.NewAdapter()
	zfsAdapter := newStubZfsServerPort(t.TempDir())
	sysAdapter := system.NewAdapter(zfsAdapter, checker)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter()),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter()),
		application.NewShareService(share.NewCifsAdapter(t.TempDir()), share.NewWebdavAdapter()),
		application.NewIdentityService(identity.NewAdapter(), "strict-test-secret"),
		application.NewCatalogService(catalog.NewAdapter(t.TempDir())),
	)

	zfsEnabled, _ := checker.IsModuleEnabled("zfs")
	_, zfsErr := srv.ListZfsPools(ctx, &nasv1.ListZfsPoolsRequest{})
	if !zfsEnabled && status.Code(zfsErr) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition when ZFS binaries/libraries are missing, got %v", status.Code(zfsErr))
	}

	kvmEnabled, _ := checker.IsModuleEnabled("kvm")
	_, kvmErr := srv.ListVirtualMachines(ctx, &nasv1.ListVirtualMachinesRequest{})
	if !kvmEnabled && status.Code(kvmErr) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition when KVM/libvirt binaries/libraries are missing, got %v", status.Code(kvmErr))
	}

	dockerEnabled, _ := checker.IsModuleEnabled("docker")
	_, dockerErr := srv.ListDockerContainers(ctx, &nasv1.ListDockerContainersRequest{})
	if dockerEnabled && dockerErr != nil {
		t.Fatalf("expected Docker module to be enabled when docker binary is present, got %v", dockerErr)
	}

	lxcEnabled, _ := checker.IsModuleEnabled("lxc")
	_, lxcErr := srv.ListLxcContainers(ctx, &nasv1.ListLxcContainersRequest{})
	if lxcEnabled && lxcErr != nil {
		t.Fatalf("expected LXC module to be enabled when lxc-ls and liblxc.so are present, got %v", lxcErr)
	}
}

func TestDockerConfigAndFilesystemGRPC(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker()
	unitsDir := t.TempDir()
	dockerAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir)
	zfsAdapter := newStubZfsServerPort(t.TempDir())
	dockerAdapter.SetZfsPort(zfsAdapter)

	sysAdapter := system.NewAdapter(zfsAdapter, checker)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter()),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter()),
		application.NewShareService(share.NewCifsAdapter(t.TempDir()), share.NewWebdavAdapter()),
		application.NewIdentityService(identity.NewAdapter(), "test-secret"),
		application.NewCatalogService(catalog.NewAdapter(t.TempDir())),
	)

	// 1. Get initial DockerConfig
	cfg, err := srv.GetDockerConfig(ctx, &nasv1.GetDockerConfigRequest{})
	if err != nil {
		t.Fatalf("GetDockerConfig failed: %v", err)
	}
	if cfg.GetPool() == "" {
		t.Fatalf("expected pool to be non-empty")
	}

	// 2. Update DockerConfig
	updated, err := srv.UpdateDockerConfig(ctx, &nasv1.UpdateDockerConfigRequest{
		DockerConfig: &nasv1.DockerConfig{
			Dataset: "tank/apps",
		},
	})
	if err != nil {
		t.Fatalf("UpdateDockerConfig failed: %v", err)
	}
	if updated.GetPool() != "tank" || updated.GetDataset() != "tank/apps" || !strings.HasSuffix(updated.GetAppdataPath(), "/apps/appdata") {
		t.Fatalf("unexpected updated config: %v", updated)
	}

	// 3. List filesystem directories
	fsRes, err := srv.ListFileSystemDirectories(ctx, &nasv1.ListFileSystemDirectoriesRequest{
		Path: unitsDir,
	})
	if err != nil {
		t.Fatalf("ListFileSystemDirectories failed: %v", err)
	}
	_ = fsRes
}

func TestSystemConfigAndPoolSelection(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker()
	checker.SetAllModulesInstalledForTest()
	checker.SetMainPool("") // unselect initially

	tempDir := t.TempDir()
	zfsAdapter := newStubZfsServerPort(tempDir)
	sysAdapter := system.NewAdapter(zfsAdapter, checker)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter()),
		application.NewDockerService(docker.NewAdapter()),
		application.NewDockerAppService(docker.NewAdapter()),
		application.NewLxcService(lxc.NewAdapter()),
		application.NewShareService(share.NewCifsAdapter(t.TempDir()), share.NewWebdavAdapter()),
		application.NewIdentityService(identity.NewAdapter(), "test-secret"),
		application.NewCatalogService(catalog.NewAdapter(t.TempDir())),
	)

	// 1. Initial State: No main pool -> Only ZFS allowed
	initialCfg, err := srv.GetSystemConfig(ctx, &nasv1.GetSystemConfigRequest{})
	if err != nil {
		t.Fatalf("GetSystemConfig failed: %v", err)
	}
	if initialCfg.GetMainPool() != "" {
		t.Fatalf("expected empty initial main pool, got %q", initialCfg.GetMainPool())
	}

	zfsOk, _ := checker.IsModuleEnabled("zfs")
	if !zfsOk {
		t.Fatalf("expected ZFS to be enabled even without main pool")
	}
	kvmOk, kvmReason := checker.IsModuleEnabled("kvm")
	if kvmOk {
		t.Fatalf("expected KVM to be disabled when main pool is unconfigured")
	}
	t.Logf("KVM paused as expected: %s", kvmReason)

	// 2. Select Main Pool "tank"
	selResp, err := srv.SelectMainPool(ctx, &nasv1.SelectMainPoolRequest{
		PoolName:           "tank",
		AutoImportExisting: true,
	})
	if err != nil {
		t.Fatalf("SelectMainPool failed: %v", err)
	}
	if selResp.GetMainPool() != "tank" {
		t.Fatalf("expected main pool to be tank, got %q", selResp.GetMainPool())
	}
	if selResp.GetSystemDataset() != "tank/.nas-system" {
		t.Fatalf("expected system dataset tank/.nas-system, got %q", selResp.GetSystemDataset())
	}

	// 3. After selection, KVM and Docker should now be enabled
	kvmNowOk, _ := checker.IsModuleEnabled("kvm")
	if !kvmNowOk {
		t.Fatalf("expected KVM to become enabled after selecting main pool")
	}

	// 4. Discover Pools
	discResp, err := srv.DiscoverPools(ctx, &nasv1.DiscoverPoolsRequest{})
	if err != nil {
		t.Fatalf("DiscoverPools failed: %v", err)
	}
	if len(discResp.GetPools()) == 0 {
		t.Fatalf("expected discovered pools")
	}
	t.Logf("Discovered %d pools; main pool is %s", len(discResp.GetPools()), selResp.GetMainPool())

	// 5. Create pool with multi-VDEV topology and Export pool
	createResp, err := srv.CreateZfsPool(ctx, &nasv1.CreateZfsPoolRequest{
		ZfsPoolId: "archive",
		ZfsPool: &nasv1.ZfsPool{
			Vdevs: []*nasv1.ZfsVdev{
				{
					Type:   "data",
					Layout: "mirror",
					Children: []*nasv1.ZfsVdev{
						{Path: "/dev/sdb"},
						{Path: "/dev/sdc"},
					},
				},
				{
					Type:   "cache",
					Layout: "stripe",
					Children: []*nasv1.ZfsVdev{
						{Path: "/dev/nvme1n1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateZfsPool with VDEVs failed: %v", err)
	}
	if createResp.GetPoolName() != "archive" {
		t.Fatalf("expected archive pool, got %s", createResp.GetPoolName())
	}

	// Export the newly created pool
	_, err = srv.ExportZfsPool(ctx, &nasv1.ExportZfsPoolRequest{
		Name:  "zfsPools/archive",
		Force: true,
	})
	if err != nil {
		t.Fatalf("ExportZfsPool failed: %v", err)
	}
}
