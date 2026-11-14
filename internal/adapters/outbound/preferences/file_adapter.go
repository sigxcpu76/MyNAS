package preferences

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	osuser "os/user"
	"path/filepath"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
	"gopkg.in/yaml.v3"
)

var _ ports.PreferencesPort = (*FileAdapter)(nil)

const PreferencesFileName = "nas_preferences.yaml"

type FileAdapter struct {
	mu               sync.RWMutex
	baseHomeOverride string
}

func NewFileAdapter() *FileAdapter {
	return &FileAdapter{}
}

// NewFileAdapterWithBase is useful for unit testing in temporary directories.
func NewFileAdapterWithBase(baseHome string) *FileAdapter {
	return &FileAdapter{baseHomeOverride: baseHome}
}

func DefaultUserPreferences() *domain.UserPreferences {
	return &domain.UserPreferences{
		Dashboard: domain.DashboardPreferences{
			ColumnCount: 3,
			Layout: []domain.DashboardWidgetPreference{
				{ID: "widget-kpis", Type: "kpis", Lane: -1},
				{ID: "widget-pools", Type: "pools", Lane: 0},
				{ID: "widget-network", Type: "network", Lane: 1},
				{ID: "widget-sensors", Type: "sensors", Lane: 2},
			},
		},
	}
}

func (a *FileAdapter) resolveHomeDir(user *domain.User) (string, error) {
	if a.baseHomeOverride != "" {
		if user != nil && user.Username != "" {
			return filepath.Join(a.baseHomeOverride, user.Username), nil
		}
		return a.baseHomeOverride, nil
	}

	if user != nil {
		if user.HomeDirectory != "" {
			return user.HomeDirectory, nil
		}
		if user.Username != "" {
			if u, err := osuser.Lookup(user.Username); err == nil && u.HomeDir != "" {
				return u.HomeDir, nil
			}
			if user.Username == "root" || user.UID == 0 {
				return "/root", nil
			}
			return filepath.Join("/home", user.Username), nil
		}
	}

	if cur, err := osuser.Current(); err == nil && cur.HomeDir != "" {
		return cur.HomeDir, nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return "/root", nil
}

func (a *FileAdapter) openHomeRoot(user *domain.User, createIfMissing bool) (*os.Root, error) {
	homeDir, err := a.resolveHomeDir(user)
	if err != nil {
		return nil, err
	}
	fi, lstatErr := os.Lstat(homeDir)
	if lstatErr != nil {
		if errors.Is(lstatErr, fs.ErrNotExist) && createIfMissing {
			if mkErr := os.MkdirAll(homeDir, 0o750); mkErr != nil {
				return nil, fmt.Errorf("creating home directory: %w", mkErr)
			}
			if os.Geteuid() == 0 && user != nil && user.UID > 0 {
				_ = os.Lchown(homeDir, int(user.UID), int(user.PrimaryGID))
			}
		} else {
			return nil, lstatErr
		}
	} else if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return nil, fmt.Errorf("refusing unsafe home directory %q", homeDir)
	}
	return os.OpenRoot(homeDir)
}

func (a *FileAdapter) GetPreferences(ctx context.Context, user *domain.User) (*domain.UserPreferences, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	root, err := a.openHomeRoot(user, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultUserPreferences(), nil
		}
		return nil, fmt.Errorf("resolving preferences root: %w", err)
	}
	defer root.Close()

	if cfgInfo, err := root.Lstat(".config"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultUserPreferences(), nil
		}
		return nil, fmt.Errorf("checking preferences directory: %w", err)
	} else if cfgInfo.Mode()&os.ModeSymlink != 0 || !cfgInfo.IsDir() {
		return nil, fmt.Errorf("refusing symlinked or non-directory .config")
	}

	relPath := filepath.Join(".config", PreferencesFileName)
	if fileInfo, err := root.Lstat(relPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultUserPreferences(), nil
		}
		return nil, fmt.Errorf("checking preferences file: %w", err)
	} else if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing symlinked or non-regular preferences file")
	}

	data, err := root.ReadFile(relPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultUserPreferences(), nil
		}
		return nil, fmt.Errorf("reading preferences file: %w", err)
	}

	if len(data) == 0 {
		return DefaultUserPreferences(), nil
	}

	var prefs domain.UserPreferences
	if err := yaml.Unmarshal(data, &prefs); err != nil {
		return nil, fmt.Errorf("unmarshaling preferences: %w", err)
	}

	if prefs.Dashboard.ColumnCount <= 0 {
		prefs.Dashboard.ColumnCount = 3
	}
	if prefs.Dashboard.Layout == nil {
		prefs.Dashboard.Layout = DefaultUserPreferences().Dashboard.Layout
	}

	return &prefs, nil
}

func (a *FileAdapter) UpdatePreferences(ctx context.Context, user *domain.User, prefs *domain.UserPreferences) (*domain.UserPreferences, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if prefs == nil {
		prefs = DefaultUserPreferences()
	}

	if prefs.Dashboard.ColumnCount <= 0 {
		prefs.Dashboard.ColumnCount = 3
	}

	root, err := a.openHomeRoot(user, a.baseHomeOverride != "")
	if err != nil {
		return nil, fmt.Errorf("opening user home directory: %w", err)
	}
	defer root.Close()

	if err := root.Mkdir(".config", 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("creating config directory: %w", err)
	}
	cfgInfo, err := root.Lstat(".config")
	if err != nil {
		return nil, fmt.Errorf("stat config directory: %w", err)
	}
	if cfgInfo.Mode()&os.ModeSymlink != 0 || !cfgInfo.IsDir() {
		return nil, fmt.Errorf("refusing symlinked or non-directory .config")
	}

	// Chown .config via an open file descriptor inside os.Root so symlinks can never be followed
	if os.Geteuid() == 0 && user != nil && user.UID > 0 {
		if dirFile, openErr := root.Open(".config"); openErr == nil {
			_ = dirFile.Chown(int(user.UID), int(user.PrimaryGID))
			_ = dirFile.Close()
		}
	}

	relTarget := filepath.Join(".config", PreferencesFileName)
	if targetInfo, err := root.Lstat(relTarget); err == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 || targetInfo.IsDir() {
			return nil, fmt.Errorf("refusing symlinked or directory preferences target")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("checking preferences target: %w", err)
	}

	data, err := yaml.Marshal(prefs)
	if err != nil {
		return nil, fmt.Errorf("marshaling preferences: %w", err)
	}

	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	tmpRel := filepath.Join(".config", fmt.Sprintf(".nas_preferences.%s.tmp", hex.EncodeToString(randBytes[:])))

	tmpFile, err := root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	defer func() {
		_ = root.Remove(tmpRel)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("writing preferences: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("syncing preferences: %w", err)
	}
	if os.Geteuid() == 0 && user != nil && user.UID > 0 {
		_ = tmpFile.Chown(int(user.UID), int(user.PrimaryGID))
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("closing temp file: %w", err)
	}

	if err := root.Rename(tmpRel, relTarget); err != nil {
		return nil, fmt.Errorf("renaming temp file to target: %w", err)
	}

	return prefs, nil
}
