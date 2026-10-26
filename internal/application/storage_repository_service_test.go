package application_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/application"
)

func TestStorageRepoService(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	svc := application.NewStorageRepoService(nil, tempDir)
	svc.SetMainPool("tank")

	// 1. Initial listing is empty
	isos, err := svc.ListISOs(ctx)
	if err != nil {
		t.Fatalf("ListISOs failed: %v", err)
	}
	if len(isos) != 0 {
		t.Fatalf("expected 0 ISOs, got %d", len(isos))
	}

	// 2. Save ISO
	isoData := []byte("iso dummy content data")
	savedIso, err := svc.SaveISO(ctx, "ubuntu-24.04-live-server.iso", bytes.NewReader(isoData))
	if err != nil {
		t.Fatalf("SaveISO failed: %v", err)
	}
	if savedIso.Name != "ubuntu-24.04-live-server.iso" || savedIso.SizeBytes != int64(len(isoData)) {
		t.Fatalf("unexpected saved ISO: %+v", savedIso)
	}

	// 3. List ISOs contains the new file
	isos, err = svc.ListISOs(ctx)
	if err != nil || len(isos) != 1 || isos[0].Name != "ubuntu-24.04-live-server.iso" {
		t.Fatalf("expected 1 ISO, got %+v", isos)
	}

	// 4. Delete ISO
	if err := svc.DeleteISO(ctx, "ubuntu-24.04-live-server.iso"); err != nil {
		t.Fatalf("DeleteISO failed: %v", err)
	}
	isos, _ = svc.ListISOs(ctx)
	if len(isos) != 0 {
		t.Fatalf("expected 0 ISOs after deletion, got %d", len(isos))
	}

	// 5. Save and List LXC Template
	tmplData := []byte("template dummy archive data")
	savedTmpl, err := svc.SaveLxcTemplate(ctx, "debian-12-standard.tar.xz", bytes.NewReader(tmplData))
	if err != nil {
		t.Fatalf("SaveLxcTemplate failed: %v", err)
	}
	if savedTmpl.Name != "debian-12-standard.tar.xz" {
		t.Fatalf("unexpected saved template: %+v", savedTmpl)
	}

	tmpls, err := svc.ListLxcTemplates(ctx)
	if err != nil || len(tmpls) != 1 || tmpls[0].Name != "debian-12-standard.tar.xz" {
		t.Fatalf("expected 1 template, got %+v", tmpls)
	}

	// 6. Delete LXC Template
	if err := svc.DeleteLxcTemplate(ctx, "debian-12-standard.tar.xz"); err != nil {
		t.Fatalf("DeleteLxcTemplate failed: %v", err)
	}
	tmpls, _ = svc.ListLxcTemplates(ctx)
	if len(tmpls) != 0 {
		t.Fatalf("expected 0 templates after deletion, got %d", len(tmpls))
	}

	// 7. Security: reject path traversal
	if _, err := svc.SaveISO(ctx, "../escape.iso", strings.NewReader("bad")); err == nil {
		t.Fatalf("expected error on path traversal filename")
	}
	if _, err := svc.SaveISO(ctx, ".hidden.iso", strings.NewReader("bad")); err == nil {
		t.Fatalf("expected error on hidden filename")
	}
}
