package share

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestWebdavAdapter_LifecycleAndPersistence(t *testing.T) {
	stateDir := t.TempDir()
	shareDir := t.TempDir()

	adapter := NewWebdavAdapter(stateDir)
	ctx := context.Background()

	// 1. Create Share
	created, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:          "docs",
		ShareName:   "Documents",
		Path:        shareDir,
		Description: "General documents",
		ReadOnly:    false,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("CreateShare failed: %v", err)
	}
	if created.ID != "docs" || created.URLPrefix != "/dav/docs" {
		t.Fatalf("unexpected created share: %+v", created)
	}

	// Verify persistence file exists
	persistFile := filepath.Join(stateDir, "webdav_shares.json")
	if _, err := os.Stat(persistFile); err != nil {
		t.Fatalf("expected webdav_shares.json to exist: %v", err)
	}

	// 2. Load into fresh adapter instance
	reloaded := NewWebdavAdapter(stateDir)
	sh, err := reloaded.GetShare(ctx, "docs")
	if err != nil {
		t.Fatalf("failed to get reloaded share: %v", err)
	}
	if sh.Description != "General documents" {
		t.Fatalf("expected description 'General documents', got %q", sh.Description)
	}

	// 3. Update Share
	updated, err := reloaded.UpdateShare(ctx, domain.WebdavShare{
		ID:          "docs",
		Description: "Updated Documents",
		ReadOnly:    true,
	}, []string{"description", "read_only"})
	if err != nil {
		t.Fatalf("UpdateShare failed: %v", err)
	}
	if !updated.ReadOnly || updated.Description != "Updated Documents" {
		t.Fatalf("unexpected updated share: %+v", updated)
	}

	// 4. Delete Share
	if err := reloaded.DeleteShare(ctx, "docs"); err != nil {
		t.Fatalf("DeleteShare failed: %v", err)
	}
	_, err = reloaded.GetShare(ctx, "docs")
	if err == nil {
		t.Fatalf("expected error getting deleted share, got nil")
	}
}

func TestWebdavAdapter_AuthenticationAndAuthorization(t *testing.T) {
	shareDir := t.TempDir()
	adapter := NewWebdavAdapter()

	// Configure mock authenticator
	adapter.SetAuthenticator(func(ctx context.Context, username, password string) (*domain.User, error) {
		if username == "alice" && password == "secret123" {
			return &domain.User{
				Username:     "alice",
				PrimaryGroup: "staff",
			}, nil
		}
		if username == "bob" && password == "secret456" {
			return &domain.User{
				Username:     "bob",
				PrimaryGroup: "guests",
			}, nil
		}
		return nil, domain.ErrUnauthorized
	})

	ctx := context.Background()
	_, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:           "finance",
		Path:         shareDir,
		Enabled:      true,
		AllowedUsers: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateShare failed: %v", err)
	}

	// 1. Request without auth -> 401 with WWW-Authenticate header
	req := httptest.NewRequest(http.MethodGet, "/dav/finance/", nil)
	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without credentials, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("missing WWW-Authenticate header: %v", w.Header())
	}

	// 2. Request with invalid credentials -> 401
	req = httptest.NewRequest(http.MethodGet, "/dav/finance/", nil)
	req.SetBasicAuth("alice", "wrongpass")
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad credentials, got %d", w.Code)
	}

	// 3. Request with valid user but not authorized for share (bob is not in AllowedUsers) -> 403
	req = httptest.NewRequest(http.MethodGet, "/dav/finance/", nil)
	req.SetBasicAuth("bob", "secret456")
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized user bob, got %d", w.Code)
	}

	// 4. Request with authorized user alice -> 200 OK (renders auto-index)
	req = httptest.NewRequest(http.MethodGet, "/dav/finance/", nil)
	req.SetBasicAuth("alice", "secret123")
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for alice, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebdavAdapter_ReadOnlyEnforcement(t *testing.T) {
	shareDir := t.TempDir()
	adapter := NewWebdavAdapter()
	ctx := context.Background()

	_, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:       "public",
		Path:     shareDir,
		Enabled:  true,
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("CreateShare failed: %v", err)
	}

	// PUT request on read-only share must fail with 403
	putReq := httptest.NewRequest(http.MethodPut, "/dav/public/test.txt", strings.NewReader("content"))
	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, putReq)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on PUT to read-only share, got %d", w.Code)
	}

	// DELETE request must also fail with 403
	delReq := httptest.NewRequest(http.MethodDelete, "/dav/public/test.txt", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, delReq)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on DELETE to read-only share, got %d", w.Code)
	}

	// GET request succeeds
	getReq := httptest.NewRequest(http.MethodGet, "/dav/public/", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET to read-only share, got %d", w.Code)
	}
}

func TestWebdavAdapter_AutoIndexAndFileServing(t *testing.T) {
	shareDir := t.TempDir()
	subDir := filepath.Join(shareDir, "photos")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	testFile := filepath.Join(subDir, "family.jpg")
	if err := os.WriteFile(testFile, []byte("fake-jpeg-data"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	adapter := NewWebdavAdapter()
	ctx := context.Background()

	_, err := adapter.CreateShare(ctx, domain.WebdavShare{
		ID:          "media",
		Path:        shareDir,
		Description: "Media Store",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("CreateShare failed: %v", err)
	}

	// 1. Missing trailing slash on directory redirects with 301
	req := httptest.NewRequest(http.MethodGet, "/dav/media", nil)
	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect for directory without trailing slash, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/dav/media/" {
		t.Fatalf("expected Location /dav/media/, got %q", loc)
	}

	// 2. GET on directory returns HTML Auto-Index
	req = httptest.NewRequest(http.MethodGet, "/dav/media/", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected Content-Type text/html, got %q", contentType)
	}
	body := w.Body.String()
	if !strings.Contains(body, "photos") {
		t.Fatalf("expected directory auto-index to list 'photos', got: %s", body)
	}
	if !strings.Contains(body, "Index of /media/") {
		t.Fatalf("expected breadcrumb title in body, got: %s", body)
	}

	// 3. GET on subdirectory auto-index
	req = httptest.NewRequest(http.MethodGet, "/dav/media/photos/", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body = w.Body.String()
	if !strings.Contains(body, "family.jpg") {
		t.Fatalf("expected subdir auto-index to list 'family.jpg', got: %s", body)
	}
	if !strings.Contains(body, "[Parent Directory]") {
		t.Fatalf("expected parent directory link in subdir auto-index")
	}
	if !strings.Contains(body, "onclick=\"sortTable('name')\"") ||
		!strings.Contains(body, "onclick=\"sortTable('size')\"") ||
		!strings.Contains(body, "onclick=\"sortTable('date')\"") ||
		!strings.Contains(body, "onclick=\"sortTable('type')\"") {
		t.Fatalf("expected sortable columns in auto-index table, got: %s", body)
	}
	if !strings.Contains(body, "data-size=") || !strings.Contains(body, "data-time=") {
		t.Fatalf("expected data-size and data-time attributes in auto-index rows, got: %s", body)
	}

	// 4. Custom index.html overrides auto-index
	indexPath := filepath.Join(shareDir, "index.html")
	_ = os.WriteFile(indexPath, []byte("<h1>Welcome Custom Index</h1>"), 0o644)
	req = httptest.NewRequest(http.MethodGet, "/dav/media/", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<h1>Welcome Custom Index</h1>") {
		t.Fatalf("expected custom index.html content, got: %s", w.Body.String())
	}
	_ = os.Remove(indexPath)

	// 5. Root access on WebDAV port shows shares index
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on root shares index, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Available WebDAV Shares") || !strings.Contains(w.Body.String(), "media") {
		t.Fatalf("expected root shares index to list 'media', got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "onclick=\"sortShares('id')\"") ||
		!strings.Contains(w.Body.String(), "onclick=\"sortShares('path')\"") {
		t.Fatalf("expected sortable columns in shares index, got: %s", w.Body.String())
	}

	// 6. Direct access without /dav/ prefix (dedicated port simulation: /media/photos/family.jpg)
	req = httptest.NewRequest(http.MethodGet, "/media/photos/family.jpg", nil)
	w = httptest.NewRecorder()
	adapter.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on file GET without /dav/ prefix, got %d", w.Code)
	}
	fileBody, _ := io.ReadAll(w.Body)
	if string(fileBody) != "fake-jpeg-data" {
		t.Fatalf("expected file content 'fake-jpeg-data', got %q", string(fileBody))
	}
}
