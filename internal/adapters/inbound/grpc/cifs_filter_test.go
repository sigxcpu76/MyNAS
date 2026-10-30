package grpcadapter_test

import (
	"context"
	"errors"
	"path/filepath"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestCifsShares_HiddenShareUserFiltering(t *testing.T) {
	tempDir := t.TempDir()
	checker := prereq.NewSystemChecker()
	checker.SetAllModulesInstalledForTest()
	checker.SetMainPool("tank")
	dockerAdapter := docker.NewAdapter()
	zfsAdapter := newStubZfsServerPort(tempDir)
	sysAdapter := system.NewAdapter(zfsAdapter, checker)
	cifsAdapter := share.NewCifsAdapter(tempDir)
	webdavAdapter := share.NewWebdavAdapter()
	identAdapter := identity.NewAdapter()
	identAdapter.SetPasswordVerifier(func(username, password string) error {
		if password != "password123" {
			return errors.New("bad password")
		}
		return nil
	})

	shareSvc := application.NewShareService(cifsAdapter, webdavAdapter)
	identSvc := application.NewIdentityService(identAdapter, "super-secret-jwt-key-for-test-32b")

	srv := grpcadapter.NewServer(
		application.NewSystemService(sysAdapter),
		application.NewPrerequisiteService(checker),
		application.NewDiskService(disk.NewAdapter(zfsAdapter)),
		application.NewZfsService(zfsAdapter),
		application.NewKvmService(kvm.NewAdapter()),
		application.NewDockerService(dockerAdapter),
		application.NewDockerAppService(dockerAdapter),
		application.NewLxcService(lxc.NewAdapter()),
		shareSvc,
		identSvc,
		application.NewCatalogService(catalog.NewAdapter(tempDir)),
	)

	ctx := context.Background()
	checker.SetMainPool("tank")

	// 1. Create users: alice (regular), bob (regular), nasadmin (admin)
	_, _ = identAdapter.CreateUser(ctx, domain.User{ID: "alice", Username: "alice", IsAdmin: false, Password: "password123"})
	_, _ = identAdapter.CreateUser(ctx, domain.User{ID: "bob", Username: "bob", IsAdmin: false, Password: "password123"})
	_, _ = identAdapter.CreateUser(ctx, domain.User{ID: "nasadmin", Username: "nasadmin", IsAdmin: true, Password: "password123"})

	aliceToken, _, _, err := identSvc.Login(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("alice login failed: %v", err)
	}
	bobToken, _, _, err := identSvc.Login(ctx, "bob", "password123")
	if err != nil {
		t.Fatalf("bob login failed: %v", err)
	}
	adminToken, _, _, err := identSvc.Login(ctx, "nasadmin", "password123")
	if err != nil {
		t.Fatalf("admin login failed: %v", err)
	}

	// 2. Create shares:
	// - public-share: Browseable = true
	// - alice-hidden: Browseable = false, ValidUsers = ["alice"]
	_, err = shareSvc.CreateCifsShare(ctx, domain.CifsShare{
		ID:         "public-share",
		ShareName:  "public-share",
		Path:       filepath.Join(tempDir, "public"),
		Browseable: true,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create public share failed: %v", err)
	}

	_, err = shareSvc.CreateCifsShare(ctx, domain.CifsShare{
		ID:         "alice-hidden",
		ShareName:  "alice-hidden",
		Path:       filepath.Join(tempDir, "alice"),
		Browseable: false,
		ValidUsers: []string{"alice"},
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create alice hidden share failed: %v", err)
	}

	withToken := func(c context.Context, token string) context.Context {
		if token == "" {
			return c
		}
		return metadata.NewIncomingContext(c, metadata.Pairs("authorization", "Bearer "+token))
	}

	// 3. Test: Unauthenticated caller should ONLY see public-share
	unauthResp, err := srv.ListCifsShares(ctx, &nasv1.ListCifsSharesRequest{})
	if err != nil {
		t.Fatalf("ListCifsShares unauthenticated failed: %v", err)
	}
	if len(unauthResp.GetCifsShares()) != 1 || unauthResp.GetCifsShares()[0].GetShareName() != "public-share" {
		t.Fatalf("expected unauthenticated caller to only see public-share, got: %+v", unauthResp.GetCifsShares())
	}

	// 4. Test: Bob (not alice) should ONLY see public-share
	bobCtx := withToken(ctx, bobToken)
	bobResp, err := srv.ListCifsShares(bobCtx, &nasv1.ListCifsSharesRequest{})
	if err != nil {
		t.Fatalf("ListCifsShares bob failed: %v", err)
	}
	if len(bobResp.GetCifsShares()) != 1 || bobResp.GetCifsShares()[0].GetShareName() != "public-share" {
		t.Fatalf("expected bob to not see alice-hidden, got: %+v", bobResp.GetCifsShares())
	}

	// Bob directly querying alice-hidden should return NotFound
	_, err = srv.GetCifsShare(bobCtx, &nasv1.GetCifsShareRequest{Name: "cifsShares/alice-hidden"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for bob querying alice-hidden, got code %v, err: %v", status.Code(err), err)
	}

	// 5. Test: Alice SHOULD see both public-share AND alice-hidden
	aliceCtx := withToken(ctx, aliceToken)
	aliceResp, err := srv.ListCifsShares(aliceCtx, &nasv1.ListCifsSharesRequest{})
	if err != nil {
		t.Fatalf("ListCifsShares alice failed: %v", err)
	}
	if len(aliceResp.GetCifsShares()) != 2 {
		t.Fatalf("expected alice to see both shares, got %d", len(aliceResp.GetCifsShares()))
	}
	aliceShare, err := srv.GetCifsShare(aliceCtx, &nasv1.GetCifsShareRequest{Name: "cifsShares/alice-hidden"})
	if err != nil {
		t.Fatalf("expected alice to access alice-hidden, got: %v", err)
	}
	if aliceShare.GetShareName() != "alice-hidden" {
		t.Fatalf("expected alice-hidden, got %s", aliceShare.GetShareName())
	}

	// 6. Test: Admin SHOULD see both shares
	adminCtx := withToken(ctx, adminToken)
	adminResp, err := srv.ListCifsShares(adminCtx, &nasv1.ListCifsSharesRequest{})
	if err != nil {
		t.Fatalf("ListCifsShares admin failed: %v", err)
	}
	if len(adminResp.GetCifsShares()) != 2 {
		t.Fatalf("expected admin to see all shares, got %d", len(adminResp.GetCifsShares()))
	}
}
