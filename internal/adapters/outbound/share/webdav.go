package share

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
	"golang.org/x/net/webdav"
)

type WebdavAdapter struct {
	mu            sync.RWMutex
	stateDir      string
	shares        map[string]domain.WebdavShare
	handlers      map[string]*webdav.Handler
	authenticator func(ctx context.Context, username, password string) (*domain.User, error)
}

func NewWebdavAdapter(stateDir ...string) *WebdavAdapter {
	a := &WebdavAdapter{
		shares:   make(map[string]domain.WebdavShare),
		handlers: make(map[string]*webdav.Handler),
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
	_ = os.MkdirAll(a.stateDir, 0o755)
	filePath := filepath.Join(a.stateDir, "webdav_shares.json")
	list := make([]domain.WebdavShare, 0, len(a.shares))
	for _, s := range a.shares {
		list = append(list, s)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filePath, data, 0o644)
}

func (a *WebdavAdapter) registerShare(s domain.WebdavShare) error {
	s.URLPrefix = "/dav/" + s.ID
	_ = os.MkdirAll(s.Path, 0o755)
	a.shares[s.ID] = s
	a.handlers[s.ID] = &webdav.Handler{
		Prefix:     s.URLPrefix,
		FileSystem: webdav.Dir(s.Path),
		LockSystem: webdav.NewMemLS(),
	}
	return nil
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
		if g == user.PrimaryGroup {
			return true
		}
		for _, sg := range user.SupplementaryGroups {
			if g == sg {
				return true
			}
		}
	}
	return false
}

// ServeHTTP serves WebDAV requests with HTTP Basic Auth, Read-Only enforcement,
// and rich HTML directory auto-indexing for web browsers.
func (a *WebdavAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for web-based DAV clients and cross-origin tools
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, HEAD, POST, PUT, DELETE, TRACE, PROPFIND, PROPPATCH, COPY, MOVE, LOCK, UNLOCK")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Depth, Destination, If, Lock-Token, Timeout, X-Requested-With")

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

	// Root access on WebDAV port -> show available shares index
	if shareID == "" {
		a.mu.RLock()
		sharesList := make([]domain.WebdavShare, 0, len(a.shares))
		for _, s := range a.shares {
			if s.Enabled {
				sharesList = append(sharesList, s)
			}
		}
		a.mu.RUnlock()
		serveSharesIndex(w, r, sharesList)
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
		username, password, hasAuth := r.BasicAuth()
		if !hasAuth {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="NAS WebDAV (%s)"`, share.ID))
			http.Error(w, "Unauthorized: authentication required", http.StatusUnauthorized)
			return
		}

		if authenticator != nil {
			user, err := authenticator(r.Context(), username, password)
			if err != nil {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="NAS WebDAV (%s)"`, share.ID))
				http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
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

	// Browser Auto-Index check on GET or HEAD
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		cleanRel := filepath.Clean("/" + subPath)
		diskPath := filepath.Join(share.Path, cleanRel)
		if fi, err := os.Stat(diskPath); err == nil && fi.IsDir() {
			// Ensure trailing slash for browser relative URL resolution
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
				return
			}

			// If directory contains an index.html, serve it directly
			indexPath := filepath.Join(diskPath, "index.html")
			if indexFi, err := os.Stat(indexPath); err == nil && !indexFi.IsDir() {
				http.ServeFile(w, r, indexPath)
				return
			}

			// Render rich auto-index HTML
			serveAutoIndex(w, r, share, cleanRel, diskPath)
			return
		}
	}

	// Forward to webdav.Handler (normalize path prefix to /dav/{shareID} so Prefix stripping succeeds)
	if !strings.HasPrefix(r.URL.Path, "/dav/") {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/dav/" + trimmed
		handler.ServeHTTP(w, r2)
		return
	}

	handler.ServeHTTP(w, r)
}
