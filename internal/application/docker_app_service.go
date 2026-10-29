package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

// DockerAppService orchestrates TrueNAS SCALE style Docker Compose Applications.
type DockerAppService struct {
	port       ports.DockerAppPort
	configPort ports.DockerConfigPort
	fsPort     ports.FileSystemPort
}

// NewDockerAppService creates a new DockerAppService.
func NewDockerAppService(port ports.DockerAppPort) *DockerAppService {
	s := &DockerAppService{port: port}
	if cp, ok := port.(ports.DockerConfigPort); ok {
		s.configPort = cp
	}
	if fp, ok := port.(ports.FileSystemPort); ok {
		s.fsPort = fp
	}
	return s
}

// SetPorts sets optional config and filesystem ports if injected separately.
func (s *DockerAppService) SetPorts(cp ports.DockerConfigPort, fp ports.FileSystemPort) {
	s.configPort = cp
	s.fsPort = fp
}

// GetDockerConfig returns the Docker storage pool and dataset configuration.
func (s *DockerAppService) GetDockerConfig(ctx context.Context) (*domain.DockerConfig, error) {
	if s.configPort == nil {
		return &domain.DockerConfig{
			Pool:           "tank",
			AppdataDataset: "/tank/appdata",
			DockerDataRoot: "/tank/docker",
			AvailablePools: []string{"tank"},
			PoolSelected:   false,
		}, nil
	}
	return s.configPort.GetConfig(ctx)
}

// UpdateDockerConfig updates the Docker owned ZFS storage pool.
func (s *DockerAppService) UpdateDockerConfig(ctx context.Context, cfg domain.DockerConfig) (*domain.DockerConfig, error) {
	if s.configPort == nil {
		return nil, fmt.Errorf("docker config port unavailable")
	}
	return s.configPort.UpdateConfig(ctx, cfg)
}

// ListFileSystemDirectories lists directories for host path auto-completion and directory browsing.
func (s *DockerAppService) ListFileSystemDirectories(ctx context.Context, path string) (*domain.DirectoryList, error) {
	if s.fsPort == nil {
		return &domain.DirectoryList{
			CurrentPath: path,
			ParentPath:  "/",
			Entries:     []domain.DirectoryEntry{},
		}, nil
	}
	return s.fsPort.ListDirectories(ctx, path)
}

// ListApps returns all deployed Docker Compose Applications.
func (s *DockerAppService) ListApps(ctx context.Context) ([]domain.DockerApp, error) {
	return s.port.ListApps(ctx)
}

// GetApp retrieves a single Docker Compose Application by AIP resource name or ID.
func (s *DockerAppService) GetApp(ctx context.Context, name string) (*domain.DockerApp, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return nil, fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.GetApp(ctx, appID)
}

// CreateApp deploys a new Docker Compose Application.
func (s *DockerAppService) CreateApp(ctx context.Context, appID string, app domain.DockerApp) (*domain.DockerApp, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		appID = app.ID
	}
	if appID == "" {
		appID = domain.GenerateNanosecondID()
	}
	app.ID = appID
	if app.Name == "" {
		app.Name = domain.DockerAppName(app.ID)
	}
	if err := app.Validate(); err != nil {
		return nil, err
	}
	return s.port.CreateApp(ctx, app)
}

// UpdateApp reconfigures and redeploys an existing Docker Compose Application.
func (s *DockerAppService) UpdateApp(ctx context.Context, app domain.DockerApp, fields []string) (*domain.DockerApp, error) {
	if app.ID == "" && app.Name != "" {
		app.ID = strings.TrimPrefix(app.Name, "dockerApps/")
	}
	if app.ID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}
	if app.ComposeYAML == "" {
		return nil, fmt.Errorf("compose YAML cannot be empty for update")
	}
	return s.port.UpdateApp(ctx, app, fields)
}

// DeleteApp tears down and removes a Docker Compose Application.
func (s *DockerAppService) DeleteApp(ctx context.Context, name string, deleteVolumes bool) error {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.DeleteApp(ctx, appID, deleteVolumes)
}

// StartApp starts all containers in a Docker Compose Application.
func (s *DockerAppService) StartApp(ctx context.Context, name string) (*domain.DockerApp, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return nil, fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.StartApp(ctx, appID)
}

// StopApp stops all containers in a Docker Compose Application.
func (s *DockerAppService) StopApp(ctx context.Context, name string) (*domain.DockerApp, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return nil, fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.StopApp(ctx, appID)
}

// RestartApp restarts all containers in a Docker Compose Application.
func (s *DockerAppService) RestartApp(ctx context.Context, name string) (*domain.DockerApp, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return nil, fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.RestartApp(ctx, appID)
}

// UpgradeApp triggers an update and redeploy of the application stack, optionally taking a pre-upgrade ZFS snapshot.
func (s *DockerAppService) UpgradeApp(ctx context.Context, name string, takeSnapshot bool) (*domain.DockerApp, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return nil, fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	return s.port.UpgradeApp(ctx, appID, takeSnapshot)
}

// GetAppLogs retrieves aggregated logs for a Docker Compose Application.
func (s *DockerAppService) GetAppLogs(ctx context.Context, name string, tail int, serviceName string) (string, error) {
	appID := strings.TrimPrefix(name, "dockerApps/")
	if appID == "" {
		return "", fmt.Errorf("invalid app name %q: empty app ID", name)
	}
	if tail <= 0 {
		tail = 100
	}
	return s.port.GetAppLogs(ctx, appID, tail, serviceName)
}

// CheckHostPort checks whether a single host port is available for binding.
func (s *DockerAppService) CheckHostPort(ctx context.Context, port int, protocol string, excludeAppID string) (domain.HostPortStatus, error) {
	return s.port.CheckHostPort(ctx, port, protocol, excludeAppID)
}

// CheckHostPorts checks whether a list of host ports are available for binding.
func (s *DockerAppService) CheckHostPorts(ctx context.Context, ports []int, protocol string, excludeAppID string) ([]domain.HostPortStatus, error) {
	return s.port.CheckHostPorts(ctx, ports, protocol, excludeAppID)
}

// ListNetworks retrieves all Docker networks on the host daemon.
func (s *DockerAppService) ListNetworks(ctx context.Context) ([]domain.DockerNetwork, error) {
	return s.port.ListNetworks(ctx)
}
