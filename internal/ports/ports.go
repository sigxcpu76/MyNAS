package ports

import (
	"context"
	"net"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

// =============================================================================
// Outbound (Driven) Ports — Infrastructure Adapters implement these interfaces
// =============================================================================

type PrerequisiteCheckerPort interface {
	ListPrerequisites(ctx context.Context) ([]domain.Prerequisite, error)
	GetPrerequisite(ctx context.Context, id string) (*domain.Prerequisite, error)
	CheckAll(ctx context.Context) ([]domain.Prerequisite, error)
	GetReport(ctx context.Context) (domain.PrerequisitesReport, error)
	GetLinuxFlavor() domain.LinuxFlavor
	IsModuleEnabled(module string) (bool, string)
	HasHostPrerequisites(module string) bool
	SetMainPool(pool string)
	GetMainPool() string
}

type SystemPort interface {
	GetConfig(ctx context.Context) (*domain.SystemConfig, error)
	SelectMainPool(ctx context.Context, poolName string, autoImportExisting bool) (*domain.SystemConfig, error)
	DiscoverPools(ctx context.Context) ([]domain.DiscoveredPool, error)
	GetTelemetry(ctx context.Context) (*domain.SystemTelemetry, error)
}

type DiskPort interface {
	ListDisks(ctx context.Context) ([]domain.Disk, error)
	GetDisk(ctx context.Context, id string) (*domain.Disk, error)
}

type ZfsEnginePort interface {
	ListPools(ctx context.Context) ([]domain.ZfsPool, error)
	GetPool(ctx context.Context, id string) (*domain.ZfsPool, error)
	CreatePool(ctx context.Context, pool domain.ZfsPool) (*domain.ZfsPool, error)
	UpdatePool(ctx context.Context, id string, update domain.ZfsPoolUpdate) (*domain.ZfsPool, error)
	DeletePool(ctx context.Context, id string, force bool) error
	ExportPool(ctx context.Context, id string, force bool) error
	ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error)
	AddVdev(ctx context.Context, poolID string, req domain.ZfsAddVdevRequest) (*domain.ZfsPool, error)
	RemoveVdev(ctx context.Context, poolID string, device string) (*domain.ZfsPool, error)

	ListDatasets(ctx context.Context, poolID string) ([]domain.ZfsDataset, error)
	GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error)
	CreateDataset(ctx context.Context, dataset domain.ZfsDataset) (*domain.ZfsDataset, error)
	UpdateDataset(ctx context.Context, dataset domain.ZfsDataset, fields []string) (*domain.ZfsDataset, error)
	DeleteDataset(ctx context.Context, poolID, datasetID string, recursive bool) error
	SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error)

	ListVolumes(ctx context.Context, poolID string) ([]domain.ZfsVolume, error)
	GetVolume(ctx context.Context, poolID, volumeID string) (*domain.ZfsVolume, error)
	CreateVolume(ctx context.Context, volume domain.ZfsVolume) (*domain.ZfsVolume, error)
	UpdateVolume(ctx context.Context, volume domain.ZfsVolume, fields []string) (*domain.ZfsVolume, error)
	DeleteVolume(ctx context.Context, poolID, volumeID string) error
}

type KvmHypervisorPort interface {
	ListVMs(ctx context.Context) ([]domain.VirtualMachine, error)
	GetVM(ctx context.Context, id string) (*domain.VirtualMachine, error)
	CreateVM(ctx context.Context, vm domain.VirtualMachine) (*domain.VirtualMachine, error)
	UpdateVM(ctx context.Context, vm domain.VirtualMachine, fields []string) (*domain.VirtualMachine, error)
	DeleteVM(ctx context.Context, id string) error
	StartVM(ctx context.Context, id string) (*domain.VirtualMachine, error)
	StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error)
	RebootVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error)
	ResetVM(ctx context.Context, id string) (*domain.VirtualMachine, error)
	PauseVM(ctx context.Context, id string) (*domain.VirtualMachine, error)
	ResumeVM(ctx context.Context, id string) (*domain.VirtualMachine, error)
	CloneVM(ctx context.Context, id, newID, newName, targetPool string, linkedClone bool) (*domain.VirtualMachine, error)
	AttachMedia(ctx context.Context, id, deviceID, isoPath string) (*domain.VirtualMachine, error)
	EjectMedia(ctx context.Context, id, deviceID string) (*domain.VirtualMachine, error)
	ListSnapshots(ctx context.Context, vmID string) ([]domain.VirtualMachineSnapshot, error)
	CreateSnapshot(ctx context.Context, vmID string, snap domain.VirtualMachineSnapshot) (*domain.VirtualMachineSnapshot, error)
	RollbackSnapshot(ctx context.Context, vmID, snapID string) (*domain.VirtualMachine, error)
	DeleteSnapshot(ctx context.Context, vmID, snapID string) error
	OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error)
	ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error)
	DialVNC(ctx context.Context, vmID string) (net.Conn, error)
}

type DockerRuntimePort interface {
	ListContainers(ctx context.Context) ([]domain.DockerContainer, error)
	GetContainer(ctx context.Context, id string) (*domain.DockerContainer, error)
	CreateContainer(ctx context.Context, c domain.DockerContainer) (*domain.DockerContainer, error)
	UpdateContainer(ctx context.Context, c domain.DockerContainer, fields []string) (*domain.DockerContainer, error)
	DeleteContainer(ctx context.Context, id string, force bool) error
	StartContainer(ctx context.Context, id string) (*domain.DockerContainer, error)
	StopContainer(ctx context.Context, id string) (*domain.DockerContainer, error)
	RestartContainer(ctx context.Context, id string) (*domain.DockerContainer, error)
	GetContainerLogs(ctx context.Context, id string, tail int) (string, error)
	ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error)
	ResizeExec(ctx context.Context, execID string, height, width uint) error
}

type DockerAppPort interface {
	ListApps(ctx context.Context) ([]domain.DockerApp, error)
	GetApp(ctx context.Context, id string) (*domain.DockerApp, error)
	CreateApp(ctx context.Context, app domain.DockerApp) (*domain.DockerApp, error)
	UpdateApp(ctx context.Context, app domain.DockerApp, fields []string) (*domain.DockerApp, error)
	DeleteApp(ctx context.Context, id string, deleteVolumes bool) error
	StartApp(ctx context.Context, id string) (*domain.DockerApp, error)
	StopApp(ctx context.Context, id string) (*domain.DockerApp, error)
	RestartApp(ctx context.Context, id string) (*domain.DockerApp, error)
	GetAppLogs(ctx context.Context, id string, tail int, serviceName string) (string, error)
	CheckHostPort(ctx context.Context, port int, protocol string, excludeAppID string) (domain.HostPortStatus, error)
	CheckHostPorts(ctx context.Context, ports []int, protocol string, excludeAppID string) ([]domain.HostPortStatus, error)
}

type LxcRuntimePort interface {
	ListContainers(ctx context.Context) ([]domain.LxcContainer, error)
	GetContainer(ctx context.Context, id string) (*domain.LxcContainer, error)
	CreateContainer(ctx context.Context, c domain.LxcContainer) (*domain.LxcContainer, error)
	DeleteContainer(ctx context.Context, id string, force bool) error
	StartContainer(ctx context.Context, id string) (*domain.LxcContainer, error)
	StopContainer(ctx context.Context, id string) (*domain.LxcContainer, error)
	FreezeContainer(ctx context.Context, id string) (*domain.LxcContainer, error)
}

type CifsSharePort interface {
	ListShares(ctx context.Context) ([]domain.CifsShare, error)
	GetShare(ctx context.Context, id string) (*domain.CifsShare, error)
	CreateShare(ctx context.Context, share domain.CifsShare) (*domain.CifsShare, error)
	UpdateShare(ctx context.Context, share domain.CifsShare, fields []string) (*domain.CifsShare, error)
	DeleteShare(ctx context.Context, id string) error
}

type WebdavSharePort interface {
	ListShares(ctx context.Context) ([]domain.WebdavShare, error)
	GetShare(ctx context.Context, id string) (*domain.WebdavShare, error)
	CreateShare(ctx context.Context, share domain.WebdavShare) (*domain.WebdavShare, error)
	UpdateShare(ctx context.Context, share domain.WebdavShare, fields []string) (*domain.WebdavShare, error)
	DeleteShare(ctx context.Context, id string) error
}

type LinuxIdentityPort interface {
	Authenticate(ctx context.Context, username, password string) (*domain.User, error)
	ListUsers(ctx context.Context, includeSystem bool) ([]domain.User, error)
	GetUser(ctx context.Context, username string) (*domain.User, error)
	CreateUser(ctx context.Context, user domain.User) (*domain.User, error)
	UpdateUser(ctx context.Context, user domain.User, fields []string) (*domain.User, error)
	DeleteUser(ctx context.Context, username string, removeHome bool) error

	ListGroups(ctx context.Context, includeSystem bool) ([]domain.Group, error)
	GetGroup(ctx context.Context, groupName string) (*domain.Group, error)
	CreateGroup(ctx context.Context, group domain.Group) (*domain.Group, error)
	UpdateGroup(ctx context.Context, group domain.Group, fields []string) (*domain.Group, error)
	DeleteGroup(ctx context.Context, groupName string) error
}

type CatalogPort interface {
	ListApps(ctx context.Context, train, category, query string) ([]domain.CatalogApp, error)
	GetApp(ctx context.Context, train, appID string) (*domain.CatalogApp, error)
	SyncCatalog(ctx context.Context, force bool) (int, time.Time, error)
	LastSyncTime() time.Time
	SetAppsPool(pool string)
	SetAppdataPath(path string)
}

type DockerConfigPort interface {
	GetConfig(ctx context.Context) (*domain.DockerConfig, error)
	UpdateConfig(ctx context.Context, cfg domain.DockerConfig) (*domain.DockerConfig, error)
}

type FileSystemPort interface {
	ListDirectories(ctx context.Context, path string) (*domain.DirectoryList, error)
}
