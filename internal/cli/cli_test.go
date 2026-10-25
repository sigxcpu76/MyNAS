package cli_test

import (
	"bytes"
	"net"
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
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/cli"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"google.golang.org/grpc"
)

func TestNasctlCobraSubcommands(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer lis.Close()

	dockerAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", t.TempDir(), true)
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
		application.NewIdentityService(identity.NewAdapter(true), "cli-test-secret"),
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
		{[]string{"-s", addr, "user", "list"}, "users/sigxcpu"},
		{[]string{"-s", addr, "group", "list"}, "groups/sigxcpu"},
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
