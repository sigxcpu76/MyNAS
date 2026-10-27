package application

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

// StorageRepoService manages ISO images and LXC templates on the main storage pool.
type StorageRepoService struct {
	mu              sync.RWMutex
	mainPool        string
	baseDirOverride string
	zfsPort         ports.ZfsEnginePort
}

func NewStorageRepoService(zfsPort ports.ZfsEnginePort, baseDirOverride string) *StorageRepoService {
	return &StorageRepoService{
		zfsPort:         zfsPort,
		baseDirOverride: baseDirOverride,
	}
}

func (s *StorageRepoService) SetMainPool(pool string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mainPool = strings.TrimPrefix(strings.TrimSpace(pool), "zfsPools/")
}

func (s *StorageRepoService) GetMainPool() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.mainPool != "" {
		return s.mainPool
	}
	p := config.GetGlobal().System.MainPool
	if p != "" {
		return strings.TrimPrefix(strings.TrimSpace(p), "zfsPools/")
	}
	return "tank"
}

func (s *StorageRepoService) GetIsosDirectory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.baseDirOverride != "" {
		return filepath.Join(s.baseDirOverride, "isos")
	}
	pool := s.mainPool
	if pool == "" {
		pool = config.GetGlobal().System.MainPool
	}
	if pool == "" {
		pool = "tank"
	}
	return fmt.Sprintf("/mnt/%s/isos", pool)
}

func (s *StorageRepoService) GetLxcTemplatesDirectory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.baseDirOverride != "" {
		return filepath.Join(s.baseDirOverride, "lxc-templates")
	}
	pool := s.mainPool
	if pool == "" {
		pool = config.GetGlobal().System.MainPool
	}
	if pool == "" {
		pool = "tank"
	}
	return fmt.Sprintf("/mnt/%s/lxc-templates", pool)
}

// EnsureRepositories verifies or provisions the isos and lxc-templates datasets and filesystem directories.
func (s *StorageRepoService) EnsureRepositories(ctx context.Context) error {
	pool := s.GetMainPool()
	if s.zfsPort != nil && pool != "" {
		// Ensure isos dataset
		if _, err := s.zfsPort.GetDataset(ctx, pool, "isos"); err != nil {
			_, _ = s.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
				PoolID:      pool,
				ID:          "isos",
				DatasetPath: fmt.Sprintf("%s/isos", pool),
				Mountpoint:  s.GetIsosDirectory(),
			})
		}
		// Ensure lxc-templates dataset
		if _, err := s.zfsPort.GetDataset(ctx, pool, "lxc-templates"); err != nil {
			_, _ = s.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
				PoolID:      pool,
				ID:          "lxc-templates",
				DatasetPath: fmt.Sprintf("%s/lxc-templates", pool),
				Mountpoint:  s.GetLxcTemplatesDirectory(),
			})
		}
	}

	_ = os.MkdirAll(s.GetIsosDirectory(), 0755)
	_ = os.MkdirAll(s.GetLxcTemplatesDirectory(), 0755)
	return nil
}

// ListISOs returns all ISO and OS disk images located in the main pool ISOs repository.
func (s *StorageRepoService) ListISOs(ctx context.Context) ([]domain.StorageImage, error) {
	dir := s.GetIsosDirectory()
	_ = os.MkdirAll(dir, 0755)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read ISOs directory %s: %w", dir, err)
	}

	var results []domain.StorageImage
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		results = append(results, domain.StorageImage{
			Name:          entry.Name(),
			Path:          filepath.Join(dir, entry.Name()),
			SizeBytes:     info.Size(),
			SizeFormatted: domain.FormatBytes(info.Size()),
			ModTime:       info.ModTime(),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// SaveISO writes a new ISO file to the ISOs repository directory.
func (s *StorageRepoService) SaveISO(ctx context.Context, filename string, r io.Reader) (*domain.StorageImage, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("%w: invalid filename %q: path traversal not allowed", domain.ErrInvalidArgument, filename)
	}
	cleanName := filepath.Base(strings.TrimSpace(filename))
	if cleanName == "" || cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".") {
		return nil, fmt.Errorf("%w: invalid ISO filename %q", domain.ErrInvalidArgument, filename)
	}

	dir := s.GetIsosDirectory()
	_ = os.MkdirAll(dir, 0755)

	destPath := filepath.Join(dir, cleanName)
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("failed to write ISO data: %w", err)
	}

	fi, _ := f.Stat()
	modTime := fi.ModTime()

	return &domain.StorageImage{
		Name:          cleanName,
		Path:          destPath,
		SizeBytes:     n,
		SizeFormatted: domain.FormatBytes(n),
		ModTime:       modTime,
	}, nil
}

// DeleteISO removes an ISO from the repository.
func (s *StorageRepoService) DeleteISO(ctx context.Context, filename string) error {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return fmt.Errorf("%w: invalid filename %q: path traversal not allowed", domain.ErrInvalidArgument, filename)
	}
	cleanName := filepath.Base(strings.TrimSpace(filename))
	if cleanName == "" || cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".") {
		return fmt.Errorf("%w: invalid ISO filename %q", domain.ErrInvalidArgument, filename)
	}

	target := filepath.Join(s.GetIsosDirectory(), cleanName)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return fmt.Errorf("%w: ISO %q not found", domain.ErrNotFound, cleanName)
	}

	if err := os.Remove(target); err != nil {
		return fmt.Errorf("failed to remove ISO %s: %w", cleanName, err)
	}
	return nil
}

// ListLxcTemplates returns all template archives located in the main pool lxc-templates repository.
func (s *StorageRepoService) ListLxcTemplates(ctx context.Context) ([]domain.StorageImage, error) {
	dir := s.GetLxcTemplatesDirectory()
	_ = os.MkdirAll(dir, 0755)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory %s: %w", dir, err)
	}

	var results []domain.StorageImage
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		results = append(results, domain.StorageImage{
			Name:          entry.Name(),
			Path:          filepath.Join(dir, entry.Name()),
			SizeBytes:     info.Size(),
			SizeFormatted: domain.FormatBytes(info.Size()),
			ModTime:       info.ModTime(),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// SaveLxcTemplate writes a new template archive to the LXC templates repository.
func (s *StorageRepoService) SaveLxcTemplate(ctx context.Context, filename string, r io.Reader) (*domain.StorageImage, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("%w: invalid filename %q: path traversal not allowed", domain.ErrInvalidArgument, filename)
	}
	cleanName := filepath.Base(strings.TrimSpace(filename))
	if cleanName == "" || cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".") {
		return nil, fmt.Errorf("%w: invalid template filename %q", domain.ErrInvalidArgument, filename)
	}

	dir := s.GetLxcTemplatesDirectory()
	_ = os.MkdirAll(dir, 0755)

	destPath := filepath.Join(dir, cleanName)
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("failed to write template data: %w", err)
	}

	fi, _ := f.Stat()
	modTime := fi.ModTime()

	return &domain.StorageImage{
		Name:          cleanName,
		Path:          destPath,
		SizeBytes:     n,
		SizeFormatted: domain.FormatBytes(n),
		ModTime:       modTime,
	}, nil
}

// DeleteLxcTemplate removes an LXC template archive from the repository.
func (s *StorageRepoService) DeleteLxcTemplate(ctx context.Context, filename string) error {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return fmt.Errorf("%w: invalid filename %q: path traversal not allowed", domain.ErrInvalidArgument, filename)
	}
	cleanName := filepath.Base(strings.TrimSpace(filename))
	if cleanName == "" || cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, ".") {
		return fmt.Errorf("%w: invalid template filename %q", domain.ErrInvalidArgument, filename)
	}

	target := filepath.Join(s.GetLxcTemplatesDirectory(), cleanName)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return fmt.Errorf("%w: template %q not found", domain.ErrNotFound, cleanName)
	}

	if err := os.Remove(target); err != nil {
		return fmt.Errorf("failed to remove template %s: %w", cleanName, err)
	}
	return nil
}
