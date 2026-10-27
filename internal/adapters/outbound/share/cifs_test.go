package share

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestCifsAdapter_LifecycleAndSync(t *testing.T) {
	tempDir := t.TempDir()
	adapter := NewCifsAdapter(tempDir)

	ctx := context.Background()

	// 1. Check default global config
	cfg, err := adapter.GetGlobalConfig(ctx)
	if err != nil {
		t.Fatalf("failed to get global config: %v", err)
	}
	if cfg.Workgroup != "WORKGROUP" {
		t.Errorf("expected WORKGROUP, got %s", cfg.Workgroup)
	}

	// 2. Update global config
	updatedCfg, err := adapter.UpdateGlobalConfig(ctx, domain.CifsGlobalConfig{
		Workgroup:    "HOMELAB",
		ServerString: "My NAS Server",
		FruitAAPL:    true,
		WSDDEnabled:  true,
	}, nil)
	if err != nil {
		t.Fatalf("failed to update global config: %v", err)
	}
	if updatedCfg.Workgroup != "HOMELAB" {
		t.Errorf("expected HOMELAB, got %s", updatedCfg.Workgroup)
	}

	// 3. Create a share with VFS modules and ACLs
	share := domain.CifsShare{
		ID:                 "media",
		ShareName:          "media",
		Path:               "/tank/media",
		Comment:            "Media Library",
		ReadOnly:           false,
		GuestOk:            true,
		Browseable:         false,
		ValidUsers:         []string{"alice", "bob"},
		ValidGroups:        []string{"family"},
		ReadList:           []string{"guest"},
		WriteList:          []string{"alice"},
		ForceUser:          "alice",
		ForceGroup:         "family",
		CreateMask:         "0664",
		DirectoryMask:      "0775",
		ForceCreateMode:    "0664",
		ForceDirectoryMode: "0775",
		ShadowCopy:         true,
		TimeMachine:        true,
		RecycleBin:         true,
		Enabled:            true,
	}

	created, err := adapter.CreateShare(ctx, share)
	if err != nil {
		t.Fatalf("failed to create share: %v", err)
	}
	if created.ShareName != "media" {
		t.Errorf("expected media, got %s", created.ShareName)
	}

	// 4. Verify generated config content
	confBytes, err := os.ReadFile(adapter.configPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}
	confStr := string(confBytes)

	if !strings.Contains(confStr, "[global]") {
		t.Errorf("expected [global] section in config")
	}
	if !strings.Contains(confStr, "workgroup = HOMELAB") {
		t.Errorf("expected HOMELAB workgroup in config")
	}
	if !strings.Contains(confStr, "[media]") {
		t.Errorf("expected [media] share in config")
	}
	if !strings.Contains(confStr, "browseable = no") {
		t.Errorf("expected browseable = no for hidden share")
	}
	if !strings.Contains(confStr, "valid users = alice bob @family") {
		t.Errorf("expected valid users with alice bob @family")
	}
	if !strings.Contains(confStr, "vfs objects = catia fruit streams_xattr shadow_copy2 recycle") {
		t.Errorf("expected vfs objects in config, got:\n%s", confStr)
	}
	if !strings.Contains(confStr, "fruit:time machine = yes") {
		t.Errorf("expected fruit:time machine = yes")
	}
	if !strings.Contains(confStr, "shadow:snapdir = .zfs/snapshot") {
		t.Errorf("expected shadow:snapdir in config")
	}
	if !strings.Contains(confStr, "recycle:repository = .recycle/%U") {
		t.Errorf("expected recycle repository in config")
	}

	// 5. Test loading/parsing config on startup
	adapter2 := NewCifsAdapter(tempDir)
	cfg2, err := adapter2.GetGlobalConfig(ctx)
	if err != nil {
		t.Fatalf("failed to get global config on reload: %v", err)
	}
	if cfg2.Workgroup != "HOMELAB" {
		t.Errorf("expected reloaded workgroup HOMELAB, got %s", cfg2.Workgroup)
	}
	reloadedShare, err := adapter2.GetShare(ctx, "media")
	if err != nil {
		t.Fatalf("failed to get reloaded share: %v", err)
	}
	if reloadedShare.Path != "/tank/media" {
		t.Errorf("expected /tank/media, got %s", reloadedShare.Path)
	}
	if reloadedShare.Browseable != false {
		t.Errorf("expected Browseable = false, got %v", reloadedShare.Browseable)
	}
	if !reloadedShare.ShadowCopy || !reloadedShare.TimeMachine || !reloadedShare.RecycleBin {
		t.Errorf("expected VFS flags preserved on reload")
	}

	// 6. Test updating share with update mask to clear fields
	updated, err := adapter2.UpdateShare(ctx, domain.CifsShare{
		ID:          "media",
		ValidUsers:  nil,
		ValidGroups: nil,
	}, []string{"valid_users", "valid_groups"})
	if err != nil {
		t.Fatalf("failed to update share with mask: %v", err)
	}
	if len(updated.ValidUsers) != 0 || len(updated.ValidGroups) != 0 {
		t.Errorf("expected valid_users and valid_groups cleared, got %v / %v", updated.ValidUsers, updated.ValidGroups)
	}

	confBytes2, err := os.ReadFile(adapter2.configPath)
	if err != nil {
		t.Fatalf("failed to read generated config after update: %v", err)
	}
	if strings.Contains(string(confBytes2), "valid users =") {
		t.Errorf("expected 'valid users =' omitted after clearing validUsers and validGroups, got:\n%s", string(confBytes2))
	}
}
