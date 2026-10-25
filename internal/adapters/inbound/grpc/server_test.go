package grpcadapter_test

import (
	"context"
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
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestAIPResourceNamesPaginationAndErrors(t *testing.T) {
	ctx := context.Background()
	dockerAdapter := docker.NewAdapter(true)
	zfsAdapter := zfs.NewAdapter(true)
	prereqChecker := prereq.NewSystemChecker(true)
	sysAdapter := system.NewAdapter(zfsAdapter, prereqChecker, true)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(prereqChecker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter, true)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter(true)),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter(true)),
		application.NewShareService(share.NewCifsAdapter(true, t.TempDir()), share.NewWebdavAdapter(true)),
		application.NewIdentityService(identity.NewAdapter(true), "grpc-test-secret"),
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
	checker := prereq.NewSystemChecker(false)
	dockerAdapter := docker.NewAdapter(false)
	zfsAdapter := zfs.NewAdapter(false)
	sysAdapter := system.NewAdapter(zfsAdapter, checker, false)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter, false)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter(false)),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter(false)),
		application.NewShareService(share.NewCifsAdapter(false, t.TempDir()), share.NewWebdavAdapter(true)),
		application.NewIdentityService(identity.NewAdapter(true), "strict-test-secret"),
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
	checker := prereq.NewSystemChecker(true)
	unitsDir := t.TempDir()
	dockerAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir, true)
	zfsAdapter := zfs.NewAdapter(true)
	dockerAdapter.SetZfsPort(zfsAdapter)

	sysAdapter := system.NewAdapter(zfsAdapter, checker, true)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter, true)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter(true)),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter(true)),
		application.NewShareService(share.NewCifsAdapter(true, t.TempDir()), share.NewWebdavAdapter(true)),
		application.NewIdentityService(identity.NewAdapter(true), "test-secret"),
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
	if updated.GetPool() != "tank" || updated.GetDataset() != "tank/apps" || updated.GetAppdataPath() != "/tank/apps/appdata" {
		t.Fatalf("unexpected updated config: %v", updated)
	}

	// 3. List filesystem directories
	fsRes, err := srv.ListFileSystemDirectories(ctx, &nasv1.ListFileSystemDirectoriesRequest{
		Path: "/tank",
	})
	if err != nil {
		t.Fatalf("ListFileSystemDirectories failed: %v", err)
	}
	if len(fsRes.GetEntries()) == 0 {
		t.Fatalf("expected entries in /tank, got 0")
	}
}

func TestSystemConfigAndPoolSelection(t *testing.T) {
	ctx := context.Background()
	checker := prereq.NewSystemChecker(true)
	checker.SetMainPool("") // unselect initially

	zfsAdapter := zfs.NewAdapter(true)
	sysAdapter := system.NewAdapter(zfsAdapter, checker, true)
	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter, true)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter(true)),
		application.NewDockerService(docker.NewAdapter(true)),
		application.NewDockerAppService(docker.NewAdapter(true)),
		application.NewLxcService(lxc.NewAdapter(true)),
		application.NewShareService(share.NewCifsAdapter(true, t.TempDir()), share.NewWebdavAdapter(true)),
		application.NewIdentityService(identity.NewAdapter(true), "test-secret"),
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
}
