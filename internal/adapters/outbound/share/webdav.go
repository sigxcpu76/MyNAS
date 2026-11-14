package share

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"golang.org/x/net/webdav"
)

const (
	webdavMaxAuthFailures = 5
	webdavAuthWindow      = 5 * time.Minute
	webdavAuthBlockTime   = 1 * time.Minute
)

type webdavAuthAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

// confinedWebdavDir implements webdav.FileSystem backed by os.Root so symlinks
// inside a share directory can never resolve outside baseDir.
type confinedWebdavDir struct {
	baseDir string
}

func cleanWebdavRelPath(name string) (string, error) {
	if strings.ContainsRune(name, '\x00') || (filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator)) {
		return "", os.ErrNotExist
	}
	clean := path.Clean("/" + name)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" {
		rel = "."
	}
	return filepath.FromSlash(rel), nil
}

func (d confinedWebdavDir) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	rel, err := cleanWebdavRelPath(name)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(d.baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Mkdir(rel, perm)
}

func (d confinedWebdavDir) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	rel, err := cleanWebdavRelPath(name)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.baseDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.OpenFile(rel, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (d confinedWebdavDir) RemoveAll(ctx context.Context, name string) error {
	rel, err := cleanWebdavRelPath(name)
	if err != nil {
		return err
	}
	if rel == "." {
		return os.ErrInvalid
	}
	root, err := os.OpenRoot(d.baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(rel)
}

func (d confinedWebdavDir) Rename(ctx context.Context, oldName, newName string) error {
	oldRel, err := cleanWebdavRelPath(oldName)
	if err != nil {
		return err
	}
	newRel, err := cleanWebdavRelPath(newName)
	if err != nil {
		return err
	}
	if oldRel == "." || newRel == "." {
		return os.ErrInvalid
	}
	root, err := os.OpenRoot(d.baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Rename(oldRel, newRel)
}

func (d confinedWebdavDir) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	rel, err := cleanWebdavRelPath(name)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.baseDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Stat(rel)
}

type WebdavAdapter struct {
	mu            sync.RWMutex
	stateDir      string
	shares        map[string]domain.WebdavShare
	handlers      map[string]*webdav.Handler
	authenticator func(ctx context.Context, username, password string) (*domain.User, error)

	authMu       sync.Mutex
	authAttempts map[string]*webdavAuthAttempt
}

func NewWebdavAdapter(stateDir ...string) *WebdavAdapter {
	a := &WebdavAdapter{
		shares:       make(map[string]domain.WebdavShare),
		handlers:     make(map[string]*webdav.Handler),
		authAttempts: make(map[string]*webdavAuthAttempt),
	}
	if len(stateDir) > 0 && stateDir[0] != "" {
		a.stateDir = stateDir[0]
		a.loadShares()
	}
	return a
}

func (a *WebdavAdapter) SetAuthenticator(fn func(ctx context.Context, username, password string) (*domain.User, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.authenticator = fn
}

func (a *WebdavAdapter) loadShares() {
	if a.stateDir == "" {
		return
	}
	filePath := filepath.Join(a.stateDir, "webdav_shares.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	var loaded []domain.WebdavShare
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	for _, s := range loaded {
		_ = a.registerShare(s)
	}
}

func (a *WebdavAdapter) saveShares() {
	if a.stateDir == "" {
		return
	}
	_ = os.MkdirAll(a.stateDir, 0o700)
	filePath := filepath.Join(a.stateDir, "webdav_shares.json")
	list := make([]domain.WebdavShare, 0, len(a.shares))
	for _, s := range a.shares {
		list = append(list, s)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filePath, data, 0o600)
}

func (a *WebdavAdapter) registerShare(s domain.WebdavShare) error {
	s.URLPrefix = "/dav/" + s.ID
	_ = os.MkdirAll(s.Path, 0o755)
	a.shares[s.ID] = s
	a.handlers[s.ID] = &webdav.Handler{
		Prefix:     s.URLPrefix,
		FileSystem: confinedWebdavDir{baseDir: s.Path},
		LockSystem: webdav.NewMemLS(),
	}
	return nil
}

func (a *WebdavAdapter) checkBasicAuthRateLimit(username string) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.authAttempts == nil {
		return nil
	}
	st, ok := a.authAttempts[username]
	if !ok {
		return nil
	}
	now := time.Now()
	if !st.blockedUntil.IsZero() && now.Before(st.blockedUntil) {
		return domain.ErrResourceExhausted
	}
	if now.Sub(st.windowStart) > webdavAuthWindow {
		delete(a.authAttempts, username)
	}
	return nil
}

func (a *WebdavAdapter) recordBasicAuthFailure(username string) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.authAttempts == nil {
		a.authAttempts = make(map[string]*webdavAuthAttempt)
	}
	now := time.Now()
	st, ok := a.authAttempts[username]
	if !ok || now.Sub(st.windowStart) > webdavAuthWindow {
		st = &webdavAuthAttempt{windowStart: now}
		a.authAttempts[username] = st
	}
	st.failures++
	if st.failures >= webdavMaxAuthFailures {
		st.blockedUntil = now.Add(webdavAuthBlockTime)
	}
}

func (a *WebdavAdapter) clearBasicAuthFailures(username string) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	delete(a.authAttempts, username)
}

func (a *WebdavAdapter) authenticateBasic(
	ctx context.Context,
	w http.ResponseWriter,
	realm string,
	authenticator func(ctx context.Context, username, password string) (*domain.User, error),
	username, password string,
) (*domain.User, bool) {
	cleanUser := strings.TrimSpace(username)
	if err := a.checkBasicAuthRateLimit(cleanUser); err != nil {
		http.Error(w, "Too many failed authentication attempts; please try again later", http.StatusTooManyRequests)
		return nil, false
	}
	user, err := authenticator(ctx, username, password)
	if err != nil {
		if errors.Is(err, domain.ErrResourceExhausted) {
			http.Error(w, "Too many failed authentication attempts; please try again later", http.StatusTooManyRequests)
			return nil, false
		}
		a.recordBasicAuthFailure(cleanUser)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
		http.Error(w, "Unauthorized: invalid username or password", http.StatusUnauthorized)
		return nil, false
	}
	if user != nil && user.Locked {
		a.recordBasicAuthFailure(cleanUser)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
		http.Error(w, "Unauthorized: account is locked", http.StatusUnauthorized)
		return nil, false
	}
	a.clearBasicAuthFailures(cleanUser)
	return user, true
}

func (a *WebdavAdapter) ListShares(ctx context.Context) ([]domain.WebdavShare, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.WebdavShare, 0, len(a.shares))
	for _, s := range a.shares {
		out = append(out, s)
	}
	return out, nil
}

func (a *WebdavAdapter) GetShare(ctx context.Context, id string) (*domain.WebdavShare, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s, ok := a.shares[id]
	if !ok {
		return nil, fmt.Errorf("%w: webdav share %q", domain.ErrNotFound, id)
	}
	cp := s
	return &cp, nil
}

func (a *WebdavAdapter) CreateShare(ctx context.Context, share domain.WebdavShare) (*domain.WebdavShare, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.shares[share.ID]; exists {
		return nil, fmt.Errorf("%w: webdav share %q", domain.ErrAlreadyExists, share.ID)
	}
	_ = a.registerShare(share)
	a.saveShares()
	cp := a.shares[share.ID]
	return &cp, nil
}

func (a *WebdavAdapter) UpdateShare(ctx context.Context, share domain.WebdavShare, fields []string) (*domain.WebdavShare, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.shares[share.ID]
	if !ok {
		return nil, fmt.Errorf("%w: webdav share %q", domain.ErrNotFound, share.ID)
	}

	applyAll := len(fields) == 0
	hasField := func(names ...string) bool {
		if applyAll {
			return true
		}
		for _, f := range fields {
			for _, name := range names {
				if f == name {
					return true
				}
			}
		}
		return false
	}

	if hasField("path") && (!applyAll || share.Path != "") {
		existing.Path = share.Path
	}
	if hasField("description") && (!applyAll || share.Description != "") {
		existing.Description = share.Description
	}
	if hasField("read_only", "readOnly") {
		existing.ReadOnly = share.ReadOnly
	}
	if hasField("enabled") {
		existing.Enabled = share.Enabled
	}
	if hasField("allowed_users", "allowedUsers") && (!applyAll || share.AllowedUsers != nil) {
		existing.AllowedUsers = share.AllowedUsers
	}
	if hasField("allowed_groups", "allowedGroups") && (!applyAll || share.AllowedGroups != nil) {
		existing.AllowedGroups = share.AllowedGroups
	}
	_ = a.registerShare(existing)
	a.saveShares()
	cp := a.shares[share.ID]
	return &cp, nil
}

func (a *WebdavAdapter) DeleteShare(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.shares[id]; !exists {
		return fmt.Errorf("%w: webdav share %q", domain.ErrNotFound, id)
	}
	delete(a.shares, id)
	delete(a.handlers, id)
	a.saveShares()
	return nil
}

func (a *WebdavAdapter) isUserAuthorized(user *domain.User, share domain.WebdavShare) bool {
	// If no user or group restrictions are defined, any valid authenticated user is authorized
	if len(share.AllowedUsers) == 0 && len(share.AllowedGroups) == 0 {
		return true
	}
	for _, u := range share.AllowedUsers {
		if u == user.Username {
			return true
		}
	}
	for _, g := range share.AllowedGroups {
		cleanG := strings.TrimPrefix(g, "@")
		if cleanG == user.PrimaryGroup {
			return true
		}
		for _, sg := range user.SupplementaryGroups {
			if cleanG == sg {
				return true
			}
		}
	}
	return false
}

// ServeHTTP serves WebDAV requests with HTTP Basic Auth, Read-Only enforcement,
// and rich HTML directory auto-indexing for web browsers.
func (a *WebdavAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("DAV", "1, 2")
		w.Header().Set("MS-Author-Via", "DAV")
		w.Header().Set("Allow", "OPTIONS, GET, HEAD, POST, PUT, DELETE, TRACE, PROPFIND, PROPPATCH, COPY, MOVE, LOCK, UNLOCK")
		w.WriteHeader(http.StatusOK)
		return
	}

	trimmed := r.URL.Path
	if strings.HasPrefix(trimmed, "/dav/") {
		trimmed = strings.TrimPrefix(trimmed, "/dav/")
	} else if strings.HasPrefix(trimmed, "/dav") {
		trimmed = strings.TrimPrefix(trimmed, "/dav")
	} else {
		trimmed = strings.TrimPrefix(trimmed, "/")
	}

	parts := strings.SplitN(trimmed, "/", 2)
	shareID := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	// Root access on WebDAV port -> show available shares index (only for authenticated users with access to at least one share)
	if shareID == "" {
		a.mu.RLock()
		sharesList := make([]domain.WebdavShare, 0, len(a.shares))
		for _, s := range a.shares {
			if s.Enabled {
				sharesList = append(sharesList, s)
			}
		}
		authenticator := a.authenticator
		a.mu.RUnlock()

		username, password, hasAuth := r.BasicAuth()
		if !hasAuth {
			w.Header().Set("WWW-Authenticate", `Basic realm="NAS WebDAV"`)
			http.Error(w, "Unauthorized: authentication required", http.StatusUnauthorized)
			return
		}

		var user *domain.User
		if authenticator != nil {
			var okAuth bool
			user, okAuth = a.authenticateBasic(r.Context(), w, "NAS WebDAV", authenticator, username, password)
			if !okAuth {
				return
			}
		} else {
			user = &domain.User{Username: username}
		}

		authorizedShares := make([]domain.WebdavShare, 0, len(sharesList))
		for _, s := range sharesList {
			if a.isUserAuthorized(user, s) {
				authorizedShares = append(authorizedShares, s)
			}
		}
		if len(authorizedShares) == 0 {
			http.Error(w, fmt.Sprintf("Forbidden: user %q not permitted to access any WebDAV share", username), http.StatusForbidden)
			return
		}

		serveSharesIndex(w, r, authorizedShares)
		return
	}

	a.mu.RLock()
	share, ok := a.shares[shareID]
	handler := a.handlers[shareID]
	authenticator := a.authenticator
	a.mu.RUnlock()

	if !ok || !share.Enabled || handler == nil {
		http.Error(w, fmt.Sprintf("WebDAV share %q not found or disabled", shareID), http.StatusNotFound)
		return
	}

	// Enforce HTTP Basic Auth if authenticator is configured or if allowed identity is specified
	if authenticator != nil || len(share.AllowedUsers) > 0 || len(share.AllowedGroups) > 0 {
		realm := fmt.Sprintf("NAS WebDAV (%s)", share.ID)
		username, password, hasAuth := r.BasicAuth()
		if !hasAuth {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
			http.Error(w, "Unauthorized: authentication required", http.StatusUnauthorized)
			return
		}

		if authenticator != nil {
			user, okAuth := a.authenticateBasic(r.Context(), w, realm, authenticator, username, password)
			if !okAuth {
				return
			}
			if !a.isUserAuthorized(user, share) {
				http.Error(w, fmt.Sprintf("Forbidden: user %q not permitted to access share %q", username, share.ID), http.StatusForbidden)
				return
			}
		} else if len(share.AllowedUsers) > 0 {
			// Fallback check when authenticator is not configured
			authorized := false
			for _, u := range share.AllowedUsers {
				if u == username {
					authorized = true
					break
				}
			}
			if !authorized {
				http.Error(w, fmt.Sprintf("Forbidden: user %q not permitted", username), http.StatusForbidden)
				return
			}
		}
	}

	// Enforce Read-Only restrictions on mutating methods
	if share.ReadOnly {
		switch r.Method {
		case http.MethodPut, http.MethodPost, http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH", "LOCK", "UNLOCK":
			http.Error(w, "WebDAV share is read-only", http.StatusForbidden)
			return
		}
	}

	// Browser Auto-Index check on GET or HEAD (confined via os.Root so symlinks cannot escape share.Path)
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if rel, relErr := cleanWebdavRelPath(subPath); relErr == nil {
			if root, rootErr := os.OpenRoot(share.Path); rootErr == nil {
				fi, statErr := root.Stat(rel)
				_ = root.Close()
				if statErr == nil && fi.IsDir() {
					// Ensure trailing slash for browser relative URL resolution
					if !strings.HasSuffix(r.URL.Path, "/") {
						http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
						return
					}

					cleanRel := "/"
					if rel != "." {
						cleanRel = "/" + filepath.ToSlash(rel)
					}
					diskPath := filepath.Join(share.Path, rel)
					// Render rich auto-index HTML (never auto-execute raw index.html from user shares)
					serveAutoIndex(w, r, share, cleanRel, diskPath)
					return
				}
			}
		}
	}

	// Isolate downloaded/inline shared files from executing scripts in the NAS Web UI origin
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'")

	// Forward to webdav.Handler (normalize path prefix to /dav/{shareID} so Prefix stripping succeeds)
	if !strings.HasPrefix(r.URL.Path, "/dav/") {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/dav/" + trimmed
		handler.ServeHTTP(w, r2)
		return
	}

	handler.ServeHTTP(w, r)
}
