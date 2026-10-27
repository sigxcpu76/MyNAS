package cli_test

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/cli"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/ports"
	"google.golang.org/grpc"
)

type cliZfsPort struct {
	ports.ZfsEnginePort
	pools    map[string]domain.ZfsPool
	datasets map[string]domain.ZfsDataset
	volumes  map[string]domain.ZfsVolume
}

func (s *cliZfsPort) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	p, ok := s.pools[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &p, nil
}

func (s *cliZfsPort) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	var res []domain.ZfsPool
	for _, p := range s.pools {
		res = append(res, p)
	}
	return res, nil
}

func (s *cliZfsPort) ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error) {
	var res []domain.ZfsDataset
	for k, d := range s.datasets {
		if strings.HasPrefix(k, poolID+"/") {
			res = append(res, d)
		}
	}
	return res, nil
}

func (s *cliZfsPort) ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error) {
	var res []domain.ZfsVolume
	for k, v := range s.volumes {
		if strings.HasPrefix(k, poolID+"/") {
			res = append(res, v)
		}
	}
	return res, nil
}

func (s *cliZfsPort) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	key := poolID + "/" + datasetID
	ds, ok := s.datasets[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &ds, nil
}

func (s *cliZfsPort) CreateDataset(ctx context.Context, ds domain.ZfsDataset) (*domain.ZfsDataset, error) {
	key := ds.PoolID + "/" + ds.ID
	s.datasets[key] = ds
	return &ds, nil
}

type cliKvmPort struct {
	ports.KvmHypervisorPort
	vms map[string]domain.VirtualMachine
}

func (k *cliKvmPort) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	var res []domain.VirtualMachine
	for _, vm := range k.vms {
		res = append(res, vm)
	}
	return res, nil
}

type cliDockerPort struct {
	ports.DockerRuntimePort
	ports.DockerAppPort
	containers map[string]domain.DockerContainer
	apps       map[string]domain.DockerApp
}

func (d *cliDockerPort) ListContainers(ctx context.Context) ([]domain.DockerContainer, error) {
	var res []domain.DockerContainer
	for _, c := range d.containers {
		res = append(res, c)
	}
	return res, nil
}

func (d *cliDockerPort) GetContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	c, ok := d.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &c, nil
}

func (d *cliDockerPort) StopContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	c, ok := d.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c.State = "EXITED"
	d.containers[id] = c
	return &c, nil
}

func (d *cliDockerPort) StartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	c, ok := d.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c.State = "RUNNING"
	d.containers[id] = c
	return &c, nil
}

func (d *cliDockerPort) RestartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	c, ok := d.containers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c.State = "RUNNING"
	d.containers[id] = c
	return &c, nil
}

func (d *cliDockerPort) ListApps(ctx context.Context) ([]domain.DockerApp, error) {
	var res []domain.DockerApp
	for _, a := range d.apps {
		res = append(res, a)
	}
	return res, nil
}

func (d *cliDockerPort) GetApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &a, nil
}

func (d *cliDockerPort) StopApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	a.State = "STOPPED"
	d.apps[id] = a
	return &a, nil
}

func (d *cliDockerPort) StartApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a, ok := d.apps[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	a.State = "RUNNING"
	d.apps[id] = a
	return &a, nil
}

type cliLxcPort struct {
	ports.LxcRuntimePort
	containers map[string]domain.LxcContainer
}

func (l *cliLxcPort) ListContainers(ctx context.Context) ([]domain.LxcContainer, error) {
	var res []domain.LxcContainer
	for _, c := range l.containers {
		res = append(res, c)
	}
	return res, nil
}

func TestNasctlCobraSubcommands(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer lis.Close()

	tempDir := t.TempDir()
	zfsPort := &cliZfsPort{
		pools: map[string]domain.ZfsPool{
			"tank": {ID: "tank", PoolName: "tank", State: "ONLINE", Mountpoint: tempDir},
		},
		datasets: map[string]domain.ZfsDataset{
			"tank/media": {PoolID: "tank", ID: "media", DatasetPath: "tank/media", Mountpoint: "/mnt/tank/media"},
		},
		volumes: map[string]domain.ZfsVolume{
			"tank/kvm-storage-lun0": {PoolID: "tank", ID: "kvm-storage-lun0", DevicePath: "/dev/zvol/tank/kvm-storage-lun0"},
		},
	}
	prereqChecker := prereq.NewSystemChecker()
	prereqChecker.SetAllModulesInstalledForTest()
	prereqChecker.SetMainPool("tank")
	sysAdapter := system.NewAdapter(zfsPort, prereqChecker)

	kvmPort := &cliKvmPort{
		vms: map[string]domain.VirtualMachine{
			"ubuntu-24-lts": {ID: "ubuntu-24-lts", VMName: "Ubuntu 24.04 LTS", State: "RUNNING"},
		},
	}

	dockerPort := &cliDockerPort{
		containers: map[string]domain.DockerContainer{
			"jellyfin-media": {ID: "jellyfin-media", ContainerName: "jellyfin-media", Image: "jellyfin/jellyfin:latest", State: "RUNNING"},
		},
		apps: map[string]domain.DockerApp{
			"nextcloud": {ID: "nextcloud", Title: "Nextcloud Hub", State: "RUNNING"},
		},
	}

	lxcPort := &cliLxcPort{
		containers: map[string]domain.LxcContainer{
			"pihole-dns": {ID: "pihole-dns", State: "RUNNING"},
		},
	}

	cifsAdapter := share.NewCifsAdapter(t.TempDir())
	_, _ = cifsAdapter.CreateShare(context.Background(), domain.CifsShare{
		ID:       "media",
		Path:     "/mnt/tank/media",
		Enabled:  true,
		ReadOnly: false,
	})

	webdavAdapter := share.NewWebdavAdapter()
	_, _ = webdavAdapter.CreateShare(context.Background(), domain.WebdavShare{
		ID:      "documents",
		Path:    t.TempDir(),
		Enabled: true,
	})

	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(prereqChecker),
		application.NewDiskService(disk.NewAdapter(zfsPort)),
		application.NewZfsService(zfsPort),
		application.NewKvmService(kvmPort),
		application.NewDockerService(dockerPort),
		application.NewDockerAppService(dockerPort),
		application.NewLxcService(lxcPort),
		application.NewShareService(cifsAdapter, webdavAdapter),
		application.NewIdentityService(identity.NewAdapter(), "cli-test-secret"),
		application.NewCatalogService(catalog.NewAdapter(t.TempDir())),
	)
	grpcServer := grpc.NewServer()
	nasv1.RegisterNasServiceServer(grpcServer, srv)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	addr := lis.Addr().String()

	cases := []struct {
		args     []string
		contains string
	}{
		{[]string{"-s", addr, "system", "status"}, "SYSTEM CONFIGURATION & POOL STATUS"},
		{[]string{"-s", addr, "system", "select-pool", "tank"}, "Successfully configured \"tank\" as Main Storage Pool"},
		{[]string{"-s", addr, "system", "discover"}, "POOL NAME"},
		{[]string{"-s", addr, "prereq", "list"}, "prerequisites/zpool"},
		{[]string{"-s", addr, "prereq", "check"}, "PREREQUISITES STATUS REPORT"},
		{[]string{"-s", addr, "prereq", "report"}, "PAGE 1: MISSING PREREQUISITES"},
		{[]string{"-s", addr, "zfs", "pools"}, "zfsPools/tank"},
		{[]string{"-s", addr, "zfs", "datasets", "tank"}, "zfsPools/tank/datasets/media"},
		{[]string{"-s", addr, "zfs", "volumes", "tank"}, "zfsPools/tank/volumes/kvm-storage-lun0"},
		{[]string{"-s", addr, "vm", "list"}, "virtualMachines/ubuntu-24-lts"},
		{[]string{"-s", addr, "docker", "list"}, "dockerContainers/jellyfin-media"},
		{[]string{"-s", addr, "docker", "get", "jellyfin-media"}, "jellyfin/jellyfin:latest"},
		{[]string{"-s", addr, "docker", "stop", "jellyfin-media"}, "is now EXITED"},
		{[]string{"-s", addr, "docker", "start", "jellyfin-media"}, "is now RUNNING"},
		{[]string{"-s", addr, "docker", "restart", "jellyfin-media"}, "restarted successfully"},
		{[]string{"-s", addr, "app", "list"}, "nextcloud"},
		{[]string{"-s", addr, "app", "get", "nextcloud"}, "Nextcloud Hub"},
		{[]string{"-s", addr, "app", "stop", "nextcloud"}, "is now STOPPED"},
		{[]string{"-s", addr, "app", "start", "nextcloud"}, "is now RUNNING"},
		{[]string{"-s", addr, "lxc", "list"}, "lxcContainers/pihole-dns"},
		{[]string{"-s", addr, "share", "cifs"}, "cifsShares/media"},
		{[]string{"-s", addr, "share", "webdav"}, "/dav/documents"},
		{[]string{"-s", addr, "user", "list"}, "users/"},
		{[]string{"-s", addr, "group", "list"}, "groups/"},
	}

	for _, tc := range cases {
		cmd := cli.NewRootCmd()
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("nasctl %v failed: %v (stderr: %s)", tc.args, err, errBuf.String())
		}
		if !strings.Contains(outBuf.String(), tc.contains) {
			t.Fatalf("nasctl %v output %q did not contain %q", tc.args, outBuf.String(), tc.contains)
		}
	}
}

func TestStartCmdFlags(t *testing.T) {
	cmd := cli.NewRootCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"start", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute start --help: %v", err)
	}

	helpOutput := outBuf.String()
	if !strings.Contains(helpOutput, "--http-port") {
		t.Errorf("expected help to contain '--http-port', got:\n%s", helpOutput)
	}
	if !strings.Contains(helpOutput, "--grpc-port") {
		t.Errorf("expected help to contain '--grpc-port', got:\n%s", helpOutput)
	}
	if strings.Contains(helpOutput, "--http-addr") {
		t.Errorf("expected help NOT to contain deprecated '--http-addr', got:\n%s", helpOutput)
	}
	if strings.Contains(helpOutput, "--grpc-addr") {
		t.Errorf("expected help NOT to contain deprecated '--grpc-addr', got:\n%s", helpOutput)
	}
}
