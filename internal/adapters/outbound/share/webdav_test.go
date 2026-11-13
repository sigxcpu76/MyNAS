package share_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestWebdavHardeningAndIndexFiltering(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	aliceDir := filepath.Join(tmp, "alice")
	bobDir := filepath.Join(tmp, "bob")
	_ = os.MkdirAll(aliceDir, 0o755)
	_ = os.MkdirAll(bobDir, 0o755)

	evilHTML := `<script>alert("xss")</script>`
	if err := os.WriteFile(filepath.Join(aliceDir, "index.html"), []byte(evilHTML), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	adapter := share.NewWebdavAdapter(filepath.Join(tmp, "state"))
	adapter.SetAuthenticator(func(ctx context.Context, username, password string) (*domain.User, error) {
		if password != "secret" {
			return nil, fmt.Errorf("bad credentials")
		}
		return &domain.User{Username: username, PrimaryGroup: "users"}, nil
	})

	_, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:           "alice-vault",
		Path:         aliceDir,
		Enabled:      true,
		AllowedUsers: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateShare alice-vault: %v", err)
	}

	_, err = adapter.CreateShare(ctx, domain.WebdavShare{
		ID:           "bob-secret",
		Path:         bobDir,
		Enabled:      true,
		AllowedUsers: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateShare bob-secret: %v", err)
	}

	// 1. Root /dav/ index for alice must NOT leak bob-secret or bob's path
	req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /dav/, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "alice-vault") {
		t.Fatalf("expected alice-vault in /dav/ index")
	}
	if strings.Contains(body, "bob-secret") || strings.Contains(body, bobDir) {
		t.Fatalf("leaked unauthorized share metadata in /dav/ index")
	}

	// 2. Directory GET /dav/alice-vault/ must render auto-index, NOT raw index.html script
	dirReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/", nil)
	dirReq.SetBasicAuth("alice", "secret")
	dirRec := httptest.NewRecorder()
	adapter.ServeHTTP(dirRec, dirReq)
	if dirRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /dav/alice-vault/, got %d", dirRec.Code)
	}
	if strings.Contains(dirRec.Body.String(), evilHTML) {
		t.Fatalf("directory GET executed raw index.html instead of safe auto-index")
	}

	// 3. Direct file GET /dav/alice-vault/index.html must carry strict sandbox CSP + nosniff
	fileReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/index.html", nil)
	fileReq.SetBasicAuth("alice", "secret")
	fileRec := httptest.NewRecorder()
	adapter.ServeHTTP(fileRec, fileReq)
	if fileRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /dav/alice-vault/index.html, got %d", fileRec.Code)
	}
	csp := fileRec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox") {
		t.Fatalf("expected sandbox CSP header on file response, got %q", csp)
	}
	if got := fileRec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
}
