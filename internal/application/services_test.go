package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type stubZfsEnginePort struct {
	ports.ZfsEnginePort
	pools     map[string]domain.ZfsPool
	datasets  map[string]domain.ZfsDataset
	volumes   map[string]domain.ZfsVolume
	snapshots map[string]int
}

func newStubZfsEnginePort() *stubZfsEnginePort {
	return &stubZfsEnginePort{
		pools: map[string]domain.ZfsPool{
			"tank": {ID: "tank", PoolName: "tank", State: "ONLINE", Mountpoint: "/nas/tank"},
			"fast": {ID: "fast", PoolName: "fast", State: "ONLINE", Mountpoint: "/nas/fast"},
		},
		datasets:  make(map[string]domain.ZfsDataset),
		volumes:   make(map[string]domain.ZfsVolume),
		snapshots: make(map[string]int),
	}
}

func (s *stubZfsEnginePort) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	var res []domain.ZfsPool
	for _, p := range s.pools {
		res = append(res, p)
	}
	return res, nil
}

func (s *stubZfsEnginePort) CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error) {
	if pool.Mountpoint == "" {
		pool.Mountpoint = "/nas/" + pool.ID
	}
	s.pools[pool.ID] = pool
	return &pool, nil
}

func (s *stubZfsEnginePort) ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	p, ok := s.pools[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	p.LastScrubStatus = "Scrub completed without errors"
	s.pools[id] = p
	return &p, nil
}

func (s *stubZfsEnginePort) CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error) {
	dataset.DatasetPath = dataset.PoolID + "/" + dataset.ID
	s.datasets[dataset.DatasetPath] = dataset
	return &dataset, nil
}

func (s *stubZfsEnginePort) SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	key := poolID + "/" + datasetID
	ds, ok := s.datasets[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	s.snapshots[key]++
	ds.SnapshotsCount = int32(s.snapshots[key])
	s.datasets[key] = ds
	return &ds, nil
}

func (s *stubZfsEnginePort) CreateVolume(ctx context.Context, volume domain.ZfsVolume) (*domain.ZfsVolume, error) {
	volume.DevicePath = fmt.Sprintf("/dev/zvol/%s/%s", volume.PoolID, volume.ID)
	key := volume.PoolID + "/" + volume.ID
	s.volumes[key] = volume
	return &volume, nil
}

type stubKvmPort struct {
	ports.KvmHypervisorPort
	vms      map[string]domain.VirtualMachine
	sessions map[string]domain.ConsoleSession
}

func newStubKvmPort() *stubKvmPort {
	return &stubKvmPort{
		vms:      make(map[string]domain.VirtualMachine),
		sessions: make(map[string]domain.ConsoleSession),
	}
}

func (k *stubKvmPort) CreateVM(ctx context.Context, vm domain.VirtualMachine) (*domain.VirtualMachine, error) {
	vm.State = "RUNNING"
	if len(vm.Disks) == 0 {
		vm.Disks = []domain.VirtualMachineDisk{
			{
				ID:       domain.GenerateVMDiskName(vm.ID, 0),
				DiskPath: fmt.Sprintf("/nas/mypool/vms/%s.raw", domain.GenerateVMDiskName(vm.ID, 0)),
			},
		}
	}
	k.vms[vm.ID] = vm
	return &vm, nil
}

func (k *stubKvmPort) DeleteVM(ctx context.Context, id string) error {
	delete(k.vms, id)
	return nil
}

func (k *stubKvmPort) OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error) {
	sess := domain.ConsoleSession{
		VMName:    id,
		Token:     "test-token-" + id,
		ExpiresAt: time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	}
	k.sessions[id] = sess
	return &sess, nil
}

func (k *stubKvmPort) ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error) {
	sess, ok := k.sessions[vmID]
	if !ok || sess.Token != token {
		return nil, domain.ErrPermissionDenied
	}
	vm, ok := k.vms[vmID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &vm, nil
}

func (k *stubKvmPort) CreateSnapshot(ctx context.Context, vmID string, snap domain.VirtualMachineSnapshot) (*domain.VirtualMachineSnapshot, error) {
	return &snap, nil
}

type stubDockerPort struct {
	ports.DockerRuntimePort
	ports.DockerAppPort
	containers map[string]domain.DockerContainer
	apps       map[string]domain.DockerApp
}

func newStubDockerPort() *stubDockerPort {
	return &stubDockerPort{
		containers: make(map[string]domain.DockerContainer),
		apps:       make(map[string]domain.DockerApp),
	}
}

func (d *stubDockerPort) CreateContainer(ctx context.Context, c domain.DockerContainer) (*domain.DockerContainer, error) {
	c.State = "RUNNING"
	d.containers[c.ID] = c
	return &c, nil
}

func (d *stubDockerPort) StopContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	c, ok := d.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c.State = "EXITED"
	d.containers[id] = c
	return &c, nil
}

func (d *stubDockerPort) UpdateContainer(ctx context.Context, c domain.DockerContainer, fields []string) (*domain.DockerContainer, error) {
	d.containers[c.ID] = c
	return &c, nil
}

func (d *stubDockerPort) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	return "mock container logs", nil
}

func (d *stubDockerPort) ListApps(ctx context.Context) ([]domain.DockerApp, error) {
	var list []domain.DockerApp
	for _, a := range d.apps {
		list = append(list, a)
	}
	return list, nil
}

func (d *stubDockerPort) GetApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &a, nil
}

func (d *stubDockerPort) CreateApp(ctx context.Context, app domain.DockerApp) (*domain.DockerApp, error) {
	app.State = "RUNNING"
	d.apps[app.ID] = app
	return &app, nil
}

func (d *stubDockerPort) StopApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	a.State = "STOPPED"
	d.apps[id] = a
	return &a, nil
}

func (d *stubDockerPort) StartApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	a.State = "RUNNING"
	d.apps[id] = a
	return &a, nil
}

type stubLxcPort struct {
	ports.LxcRuntimePort
	containers map[string]domain.LxcContainer
	mainPool   string
}

func newStubLxcPort() *stubLxcPort {
	return &stubLxcPort{
		containers: make(map[string]domain.LxcContainer),
	}
}

func (l *stubLxcPort) SetMainPool(pool string) {
	l.mainPool = pool
}

func (l *stubLxcPort) CreateContainer(ctx context.Context, c domain.LxcContainer) (*domain.LxcContainer, error) {
	c.State = "RUNNING"
	if c.RootfsPath == "" && l.mainPool != "" {
		c.RootfsPath = domain.GenerateLxcRootfsPath(l.mainPool, c.ID)
	}
	l.containers[c.ID] = c
	return &c, nil
}

func (l *stubLxcPort) FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error) {
	c, ok := l.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c.State = "FROZEN"
	l.containers[id] = c
	return &c, nil
}

func TestZfsServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	stub := newStubZfsEnginePort()
	svc := application.NewZfsService(stub)

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
	kvmPort := newStubKvmPort()
	kvmSvc := application.NewKvmService(kvmPort)

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

	dockerPort := newStubDockerPort()
	dockerSvc := application.NewDockerService(dockerPort)
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

	// Docker App Service
	appSvc := application.NewDockerAppService(dockerPort)
	createdApp, err := appSvc.CreateApp(ctx, "nextcloud", domain.DockerApp{
		ID:    "nextcloud",
		Title: "Nextcloud Stack",
		ComposeYAML: `services:
  web:
    image: nextcloud
`,
	})
	if err != nil {
		t.Fatalf("CreateApp failed: %v", err)
	}
	apps, err := appSvc.ListApps(ctx)
	if err != nil || len(apps) == 0 {
		t.Fatalf("ListApps failed: %v", err)
	}
	ncApp, err := appSvc.GetApp(ctx, "nextcloud")
	if err != nil || ncApp.ID != "nextcloud" {
		t.Fatalf("GetApp failed or unexpected app: %v", err)
	}
	if _, err := appSvc.StopApp(ctx, createdApp.ID); err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}
	if _, err := appSvc.StartApp(ctx, createdApp.ID); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	lxcPort := newStubLxcPort()
	lxcSvc := application.NewLxcService(lxcPort)
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
	prereqSvc := application.NewPrerequisiteService(prereq.NewSystemChecker())
	items, installed, missing, err := prereqSvc.CheckPrerequisites(ctx)
	if err != nil || len(items) == 0 {
		t.Fatalf("CheckPrerequisites failed: len=%d installed=%d missing=%d err=%v", len(items), installed, missing, err)
	}

	idSvc := application.NewIdentityService(identity.NewAdapter(), "unit-test-secret")
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

	shareSvc := application.NewShareService(share.NewCifsAdapter(t.TempDir()), share.NewWebdavAdapter())
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
	kvmPort := newStubKvmPort()
	kvmSvc := application.NewKvmService(kvmPort)
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
	dockerPort := newStubDockerPort()
	dockerSvc := application.NewDockerService(dockerPort)
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
	appSvc := application.NewDockerAppService(dockerPort)
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
	lxcPort := newStubLxcPort()
	lxcPort.SetMainPool("mypool")
	lxcSvc := application.NewLxcService(lxcPort)
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
