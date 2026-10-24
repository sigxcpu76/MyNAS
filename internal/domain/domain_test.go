package domain_test

import (
	"errors"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestZfsDomainValidation(t *testing.T) {
	validPool := domain.ZfsPool{
		ID:         "tank",
		VdevLayout: "raidz1",
		Devices:    []string{"/dev/sda", "/dev/sdb", "/dev/sdc"},
	}
	if err := validPool.Validate(); err != nil {
		t.Fatalf("expected valid pool, got error: %v", err)
	}

	noDevPool := domain.ZfsPool{ID: "empty", VdevLayout: "mirror"}
	if err := noDevPool.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for pool without devices, got: %v", err)
	}

	badLayout := domain.ZfsPool{ID: "tank2", VdevLayout: "raid99", Devices: []string{"/dev/sda"}}
	if err := badLayout.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for bad vdev layout, got: %v", err)
	}

	ds := domain.ZfsDataset{PoolID: "tank", ID: "media", QuotaBytes: -100}
	if err := ds.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for negative quota, got: %v", err)
	}

	vol := domain.ZfsVolume{PoolID: "tank", ID: "zvol0", SizeBytes: 1024 * 1024 * 1024}
	if err := vol.Validate(); err != nil {
		t.Fatalf("expected valid volume, got: %v", err)
	}
	if vol.DevicePath != "/dev/zvol/tank/zvol0" {
		t.Fatalf("expected auto-populated device path, got %q", vol.DevicePath)
	}
}

func TestKvmAndContainerValidation(t *testing.T) {
	vm := domain.VirtualMachine{ID: "ubuntu-vm", VCPUs: 2, MemoryMiB: 2048}
	if err := vm.Validate(); err != nil {
		t.Fatalf("expected valid VM, got: %v", err)
	}
	badVM := domain.VirtualMachine{ID: "tiny-vm", VCPUs: 0, MemoryMiB: 64}
	if err := badVM.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for invalid VM specs, got: %v", err)
	}

	dc := domain.DockerContainer{ID: "nginx", Image: "nginx:alpine"}
	if err := dc.Validate(); err != nil {
		t.Fatalf("expected valid docker container, got: %v", err)
	}
	badDC := domain.DockerContainer{ID: "no-image"}
	if err := badDC.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for missing docker image, got: %v", err)
	}

	lxc := domain.LxcContainer{ID: "deb-ct", Distribution: "debian"}
	if err := lxc.Validate(); err != nil {
		t.Fatalf("expected valid LXC container, got: %v", err)
	}
}

func TestShareAndIdentityValidation(t *testing.T) {
	cifs := domain.CifsShare{ID: "public", Path: "relative/path"}
	if err := cifs.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for relative CIFS path, got: %v", err)
	}

	dav := domain.WebdavShare{ID: "docs", Path: "/tank/docs"}
	if err := dav.Validate(); err != nil {
		t.Fatalf("expected valid WebDAV share, got: %v", err)
	}
	if dav.URLPrefix != "/dav/docs" {
		t.Fatalf("expected /dav/docs prefix, got %q", dav.URLPrefix)
	}

	user := domain.User{
		ID:                "alice",
		SSHEnabled:        true,
		SSHAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAliceKey alice@host"},
	}
	if err := user.Validate(); err != nil {
		t.Fatalf("expected valid user, got: %v", err)
	}

	badKeyUser := domain.User{
		ID:                "bob",
		SSHAuthorizedKeys: []string{"not-an-ssh-key-xyz"},
	}
	if err := badKeyUser.Validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for malformed SSH public key, got: %v", err)
	}
}
