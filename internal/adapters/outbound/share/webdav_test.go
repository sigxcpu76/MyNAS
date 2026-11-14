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

func TestWebdavConfinesSymlinkEscapeAndHostHeaderXSS(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	aliceDir := filepath.Join(tmp, "alice")
	outsideDir := filepath.Join(tmp, "outside")
	_ = os.MkdirAll(aliceDir, 0o755)
	_ = os.MkdirAll(outsideDir, 0o755)

	secretPath := filepath.Join(outsideDir, "shadow.txt")
	if err := os.WriteFile(secretPath, []byte("TOP-SECRET-SHADOW"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(secretPath, filepath.Join(aliceDir, "escape.txt")); err != nil {
		t.Fatalf("create escaping file symlink: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(aliceDir, "escape-dir")); err != nil {
		t.Fatalf("create escaping dir symlink: %v", err)
	}

	locked := false
	adapter := share.NewWebdavAdapter(filepath.Join(tmp, "state"))
	adapter.SetAuthenticator(func(ctx context.Context, username, password string) (*domain.User, error) {
		if password != "secret" {
			return nil, fmt.Errorf("bad credentials")
		}
		return &domain.User{Username: username, PrimaryGroup: "users", Locked: locked}, nil
	})

	_, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:           "alice-vault",
		Path:         aliceDir,
		Enabled:      true,
		AllowedUsers: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	// 1. Direct GET on symlink escaping share root must fail and never leak secret content
	escReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/escape.txt", nil)
	escReq.SetBasicAuth("alice", "secret")
	escRec := httptest.NewRecorder()
	adapter.ServeHTTP(escRec, escReq)
	if escRec.Code == http.StatusOK || strings.Contains(escRec.Body.String(), "TOP-SECRET-SHADOW") {
		t.Fatalf("symlink escape leaked outside file: status=%d body=%q", escRec.Code, escRec.Body.String())
	}

	// 2. Directory GET on escaping symlink dir must not render outside directory listing
	escDirReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/escape-dir/", nil)
	escDirReq.SetBasicAuth("alice", "secret")
	escDirRec := httptest.NewRecorder()
	adapter.ServeHTTP(escDirRec, escDirReq)
	if escDirRec.Code == http.StatusOK || strings.Contains(escDirRec.Body.String(), "shadow.txt") {
		t.Fatalf("symlink dir escape leaked outside listing: status=%d body=%q", escDirRec.Code, escDirRec.Body.String())
	}

	// 3. Crafted Host header with single quotes / script payload must not inject JS into auto-index
	xssReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/", nil)
	xssReq.Host = "evil.example');alert(1);//"
	xssReq.SetBasicAuth("alice", "secret")
	xssRec := httptest.NewRecorder()
	adapter.ServeHTTP(xssRec, xssReq)
	if xssRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on auto-index, got %d", xssRec.Code)
	}
	htmlBody := xssRec.Body.String()
	if strings.Contains(htmlBody, "alert(1)") {
		t.Fatalf("reflected Host header injected payload into auto-index HTML: %s", htmlBody)
	}
	if xssRec.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("expected Content-Security-Policy header on auto-index response")
	}

	// 4. Locked user must be rejected even with valid password
	locked = true
	lockReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/", nil)
	lockReq.SetBasicAuth("alice", "secret")
	lockRec := httptest.NewRecorder()
	adapter.ServeHTTP(lockRec, lockReq)
	if lockRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for locked user on WebDAV, got %d", lockRec.Code)
	}
	locked = false

	// 5. Brute-force Basic Auth must trigger HTTP 429 rate limiting
	for i := 0; i < 5; i++ {
		badReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/", nil)
		badReq.SetBasicAuth("bruteforce-user", "wrong")
		badRec := httptest.NewRecorder()
		adapter.ServeHTTP(badRec, badReq)
	}
	rateReq := httptest.NewRequest(http.MethodGet, "/dav/alice-vault/", nil)
	rateReq.SetBasicAuth("bruteforce-user", "wrong")
	rateRec := httptest.NewRecorder()
	adapter.ServeHTTP(rateRec, rateReq)
	if rateRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests after repeated Basic Auth failures, got %d", rateRec.Code)
	}
}

func TestCifsSanitizesSmbConfNewlines(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	cifs := share.NewCifsAdapter(tmp)

	_, err := cifs.CreateShare(ctx, domain.CifsShare{
		ID:         "docs",
		ShareName:  "docs]\n[injected",
		Path:       filepath.Join(tmp, "docs"),
		Comment:    "Team docs\n   root preexec = /bin/evil",
		Enabled:    true,
		Browseable: true,
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	confBytes, err := os.ReadFile(filepath.Join(tmp, "smb.nas.conf"))
	if err != nil {
		t.Fatalf("read smb.conf: %v", err)
	}
	for _, line := range strings.Split(string(confBytes), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "root preexec") || trimmed == "[injected]" {
			t.Fatalf("smb.conf directive/section injection succeeded: line=%q\nfull:\n%s", line, string(confBytes))
		}
	}
}
