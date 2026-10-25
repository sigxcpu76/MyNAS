package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/kvm"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestZfsServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := application.NewZfsService(zfs.NewAdapter(true))

	pools, err := svc.ListPools(ctx)
	if err != nil || len(pools) < 2 {
		t.Fatalf("expected seeded pools, got len=%d err=%v", len(pools), err)
	}

	createdPool, err := svc.CreatePool(ctx, domain.ZfsPool{
		ID:         "archive",
		VdevLayout: "mirror",
		Devices:    []string{"/dev/sde", "/dev/sdf"},
	})
	if err != nil {
		t.Fatalf("CreatePool failed: %v", err)
	}
	if createdPool.Mountpoint != "/nas/archive" {
		t.Fatalf("expected mountpoint /nas/archive, got %s", createdPool.Mountpoint)
	}

	scrubbed, err := svc.ScrubPool(ctx, "archive")
	if err != nil || scrubbed.LastScrubStatus == "" {
		t.Fatalf("ScrubPool failed: %v", err)
	}

	ds, err := svc.CreateDataset(ctx, domain.ZfsDataset{
		PoolID:      "archive",
		ID:          "photos",
		Compression: "zstd",
		QuotaBytes:  500 * 1024 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("CreateDataset failed: %v", err)
	}
	if ds.DatasetPath != "archive/photos" {
		t.Fatalf("expected dataset path archive/photos, got %s", ds.DatasetPath)
	}

	snapped, err := svc.SnapshotDataset(ctx, "archive", "photos", "snap1")
	if err != nil || snapped.SnapshotsCount != 1 {
		t.Fatalf("SnapshotDataset failed: %v", err)
	}

	vol, err := svc.CreateVolume(ctx, domain.ZfsVolume{
		PoolID:          "archive",
		ID:              "vm-disk0",
		SizeBytes:       10 * 1024 * 1024 * 1024,
		ThinProvisioned: true,
	})
	if err != nil || vol.DevicePath != "/dev/zvol/archive/vm-disk0" {
		t.Fatalf("CreateVolume failed: %v", err)
	}
}

func TestKvmAndContainersLifecycle(t *testing.T) {
	ctx := context.Background()
	kvmSvc := application.NewKvmService(kvm.NewAdapter(true))

	testVmID := "unit-test-arch-vm"
	defer func() {
		_ = kvmSvc.DeleteVM(ctx, testVmID)
	}()

	vm, err := kvmSvc.CreateVM(ctx, domain.VirtualMachine{
		ID:        testVmID,
		VCPUs:     4,
		MemoryMiB: 4096,
	})
	if err != nil {
		t.Fatalf("CreateVM failed: %v", err)
	}
	sess, err := kvmSvc.OpenConsole(ctx, vm.ID)
	if err != nil || sess.Token == "" {
		t.Fatalf("OpenConsole failed: %v", err)
	}
	if _, err := kvmSvc.ValidateConsoleToken(ctx, vm.ID, sess.Token); err != nil {
		t.Fatalf("ValidateConsoleToken failed: %v", err)
	}

	dockerAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", t.TempDir(), true)
	dockerSvc := application.NewDockerService(dockerAdapter)
	dc, err := dockerSvc.CreateContainer(ctx, domain.DockerContainer{
		ID:    "redis-cache",
		Image: "redis:7-alpine",
		Ports: []string{"6379:6379/tcp"},
	})
	if err != nil || dc.State != "RUNNING" {
		t.Fatalf("CreateContainer (Docker) failed: %v", err)
	}
	if _, err := dockerSvc.StopContainer(ctx, "redis-cache"); err != nil {
		t.Fatalf("StopContainer (Docker) failed: %v", err)
	}
	updatedDc, err := dockerSvc.UpdateContainer(ctx, domain.DockerContainer{
		ID:            "redis-cache",
		Image:         "redis:7.2-alpine",
		RestartPolicy: "always",
	}, []string{"image", "restart_policy"})
	if err != nil || updatedDc.Image != "redis:7.2-alpine" {
		t.Fatalf("UpdateContainer (Docker) failed: %v", err)
	}
	logs, err := dockerSvc.GetContainerLogs(ctx, "redis-cache", 10)
	if err != nil || logs == "" {
		t.Fatalf("GetContainerLogs (Docker) failed: %v", err)
	}

	// Docker App Service (TrueNAS SCALE style)
	appSvc := application.NewDockerAppService(dockerAdapter)
	apps, err := appSvc.ListApps(ctx)
	if err != nil || len(apps) == 0 {
		t.Fatalf("ListApps failed: %v", err)
	}
	ncApp, err := appSvc.GetApp(ctx, "nextcloud")
	if err != nil || ncApp.ID != "nextcloud" || len(ncApp.Containers) != 3 {
		t.Fatalf("GetApp failed or unexpected containers: %v", err)
	}
	if _, err := appSvc.StopApp(ctx, "nextcloud"); err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}
	if _, err := appSvc.StartApp(ctx, "nextcloud"); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	lxcSvc := application.NewLxcService(lxc.NewAdapter(true))
	lc, err := lxcSvc.CreateContainer(ctx, domain.LxcContainer{
		ID:           "deb-worker",
		Distribution: "debian",
		Release:      "bookworm",
	})
	if err != nil || lc.State != "RUNNING" {
		t.Fatalf("CreateContainer (LXC) failed: %v", err)
	}
	frozen, err := lxcSvc.FreezeContainer(ctx, "deb-worker")
	if err != nil || frozen.State != "FROZEN" {
		t.Fatalf("FreezeContainer (LXC) failed: %v", err)
	}
}

func TestIdentityAndPrerequisites(t *testing.T) {
	ctx := context.Background()
	prereqSvc := application.NewPrerequisiteService(prereq.NewSystemChecker(true))
	items, installed, missing, hybrid, err := prereqSvc.CheckPrerequisites(ctx)
	if err != nil || len(items) == 0 || !hybrid {
		t.Fatalf("CheckPrerequisites failed: len=%d installed=%d missing=%d err=%v", len(items), installed, missing, err)
	}

	idSvc := application.NewIdentityService(identity.NewAdapter(true), "unit-test-secret")
	token, _, user, err := idSvc.Login(ctx, "sigxcpu", "admin123")
	if err != nil || token == "" || user.Username != "sigxcpu" {
		t.Fatalf("Login failed: %v", err)
	}
	verified, err := idSvc.VerifyToken(ctx, "Bearer "+token)
	if err != nil || verified.Username != "sigxcpu" {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	// Ensure root cannot be deleted
	if err := idSvc.DeleteUser(ctx, "root", false); !errors.Is(err, domain.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition when deleting root, got: %v", err)
	}

	shareSvc := application.NewShareService(share.NewCifsAdapter(true, t.TempDir()), share.NewWebdavAdapter(true))
	cifsShare, err := shareSvc.CreateCifsShare(ctx, domain.CifsShare{
		ID:      "projects",
		Path:    "/tank/projects",
		Enabled: true,
	})
	if err != nil || cifsShare.ShareName != "projects" {
		t.Fatalf("CreateCifsShare failed: %v", err)
	}
}

func TestNanosecondIDLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. VM with auto-generated nanosecond ID and disk naming
	kvmSvc := application.NewKvmService(kvm.NewAdapter(true))
	vm, err := kvmSvc.CreateVM(ctx, domain.VirtualMachine{
		VCPUs:     2,
		MemoryMiB: 2048,
	})
	if err != nil {
		t.Fatalf("CreateVM with auto ID failed: %v", err)
	}
	defer func() {
		_ = kvmSvc.DeleteVM(ctx, vm.ID)
	}()
	if vm.ID == "" || len(vm.ID) < 16 {
		t.Fatalf("expected nanosecond timestamp ID for VM, got: %s", vm.ID)
	}
	if len(vm.Disks) == 0 {
		t.Fatalf("expected auto-allocated disk on VM")
	}
	expectedDiskID := domain.GenerateVMDiskName(vm.ID, 0)
	if !strings.Contains(vm.Disks[0].DiskPath, expectedDiskID) {
		t.Fatalf("expected disk path to contain %q, got: %q", expectedDiskID, vm.Disks[0].DiskPath)
	}

	// Snapshot with auto-generated nanosecond ID
	snap, err := kvmSvc.CreateSnapshot(ctx, vm.ID, domain.VirtualMachineSnapshot{
		Description: "auto-id-snapshot",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot with auto ID failed: %v", err)
	}
	if snap.SnapshotID == "" || len(snap.SnapshotID) < 16 {
		t.Fatalf("expected nanosecond timestamp ID for snapshot, got: %s", snap.SnapshotID)
	}

	// 2. Docker container with auto-generated nanosecond ID
	dockerAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", t.TempDir(), true)
	dockerSvc := application.NewDockerService(dockerAdapter)
	dc, err := dockerSvc.CreateContainer(ctx, domain.DockerContainer{
		Image: "alpine:latest",
	})
	if err != nil {
		t.Fatalf("CreateContainer (Docker) with auto ID failed: %v", err)
	}
	if dc.ID == "" || len(dc.ID) < 16 {
		t.Fatalf("expected nanosecond timestamp ID for docker container, got: %s", dc.ID)
	}

	// 3. Docker App with auto-generated nanosecond ID
	appSvc := application.NewDockerAppService(dockerAdapter)
	app, err := appSvc.CreateApp(ctx, "", domain.DockerApp{
		Title: "Nano App",
		ComposeYAML: `services:
  web:
    image: nginx:alpine
`,
	})
	if err != nil {
		t.Fatalf("CreateApp with auto ID failed: %v", err)
	}
	if app.ID == "" || len(app.ID) < 16 {
		t.Fatalf("expected nanosecond timestamp ID for docker app, got: %s", app.ID)
	}

	// 4. LXC container with auto-generated nanosecond ID
	lxcAdapter := lxc.NewAdapter(true)
	lxcAdapter.SetMainPool("mypool")
	lxcSvc := application.NewLxcService(lxcAdapter)
	lc, err := lxcSvc.CreateContainer(ctx, domain.LxcContainer{
		Distribution: "debian",
		Release:      "bookworm",
	})
	if err != nil {
		t.Fatalf("CreateContainer (LXC) with auto ID failed: %v", err)
	}
	if lc.ID == "" || len(lc.ID) < 16 {
		t.Fatalf("expected nanosecond timestamp ID for lxc container, got: %s", lc.ID)
	}
	expectedRootfs := domain.GenerateLxcRootfsPath("mypool", lc.ID)
	if lc.RootfsPath != expectedRootfs {
		t.Fatalf("expected rootfs path %q, got: %q", expectedRootfs, lc.RootfsPath)
	}
}
