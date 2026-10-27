package share

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
	"golang.org/x/net/webdav"
)

type WebdavAdapter struct {
	mu       sync.RWMutex
	shares   map[string]domain.WebdavShare
	handlers map[string]*webdav.Handler
}

func NewWebdavAdapter() *WebdavAdapter {
	return &WebdavAdapter{
		shares:   make(map[string]domain.WebdavShare),
		handlers: make(map[string]*webdav.Handler),
	}
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
	return nil
}

// ServeHTTP serves WebDAV requests under /dav/{share_id}/...
func (a *WebdavAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/dav/")
	parts := strings.SplitN(trimmed, "/", 2)
	shareID := parts[0]
	a.mu.RLock()
	share, ok := a.shares[shareID]
	handler := a.handlers[shareID]
	a.mu.RUnlock()

	if !ok || !share.Enabled || handler == nil {
		http.Error(w, "WebDAV share not found or disabled", http.StatusNotFound)
		return
	}
	if share.ReadOnly {
		switch r.Method {
		case http.MethodPut, http.MethodPost, http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH":
			http.Error(w, "WebDAV share is read-only", http.StatusForbidden)
			return
		}
	}
	handler.ServeHTTP(w, r)
}
