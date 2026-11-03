package preferences

import (
	"context"
	"fmt"
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

func (a *FileAdapter) getPreferencesFilePath(user *domain.User) (string, error) {
	homeDir, err := a.resolveHomeDir(user)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", PreferencesFileName), nil
}

func (a *FileAdapter) GetPreferences(ctx context.Context, user *domain.User) (*domain.UserPreferences, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	filePath, err := a.getPreferencesFilePath(user)
	if err != nil {
		return nil, fmt.Errorf("resolving preferences path: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
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

	filePath, err := a.getPreferencesFilePath(user)
	if err != nil {
		return nil, fmt.Errorf("resolving preferences path: %w", err)
	}

	configDir := filepath.Dir(filePath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("creating config directory: %w", err)
	}

	// Chown configDir if created as root for non-root user
	if os.Geteuid() == 0 && user != nil && user.UID > 0 {
		_ = os.Chown(configDir, int(user.UID), int(user.PrimaryGID))
	}

	data, err := yaml.Marshal(prefs)
	if err != nil {
		return nil, fmt.Errorf("marshaling preferences: %w", err)
	}

	// Atomic write via temporary file
	tmpFile, err := os.CreateTemp(configDir, "nas_preferences.*.tmp")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("writing preferences: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("syncing preferences: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0644); err != nil {
		return nil, fmt.Errorf("chmod preferences file: %w", err)
	}

	if os.Geteuid() == 0 && user != nil && user.UID > 0 {
		_ = os.Chown(tmpName, int(user.UID), int(user.PrimaryGID))
	}

	if err := os.Rename(tmpName, filePath); err != nil {
		return nil, fmt.Errorf("renaming temp file to target: %w", err)
	}

	return prefs, nil
}
