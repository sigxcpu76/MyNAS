package domain

type DiscoveredPool struct {
	PoolName      string `json:"poolName"`
	HasNasSystem  bool   `json:"hasNasSystem"`
	ConfigVersion string `json:"configVersion"`
	LastModified  string `json:"lastModified"`
	IsImported    bool   `json:"isImported"`
	DatasetPath   string `json:"datasetPath"`
	Mountpoint    string `json:"mountpoint"`
}

type SystemConfig struct {
	MainPool         string           `json:"mainPool"`
	SystemDataset    string           `json:"systemDataset"`
	SystemMountpoint string           `json:"systemMountpoint"`
	IsInitialized    bool             `json:"isInitialized"`
	EnabledModules   []string         `json:"enabledModules"`
	ConfigFilePath   string           `json:"configFilePath"`
	AutoDiscover     bool             `json:"autoDiscover"`
	DiscoveredPools  []DiscoveredPool `json:"discoveredPools"`
}

type CpuTelemetry struct {
	Model              string    `json:"model"`
	PhysicalCores      int32     `json:"physicalCores"`
	LogicalCores       int32     `json:"logicalCores"`
	UsagePercent       float64   `json:"usagePercent"`
	Load1              float64   `json:"load1"`
	Load5              float64   `json:"load5"`
	Load15             float64   `json:"load15"`
	TemperatureCelsius float64   `json:"temperatureCelsius"`
	CoreTemperatures   []float64 `json:"coreTemperatures"`
}

type MemoryTelemetry struct {
	TotalBytes      int64 `json:"totalBytes"`
	FreeBytes       int64 `json:"freeBytes"`
	AvailableBytes  int64 `json:"availableBytes"`
	UsedBytes       int64 `json:"usedBytes"`
	ZfsArcBytes     int64 `json:"zfsArcBytes"`
	ZfsArcMaxBytes  int64 `json:"zfsArcMaxBytes"`
	BuffersBytes    int64 `json:"buffersBytes"`
	CachedBytes     int64 `json:"cachedBytes"`
	ServicesBytes   int64 `json:"servicesBytes"`
}

type HostHardwareInfo struct {
	Hostname           string   `json:"hostname"`
	OsName             string   `json:"osName"`
	OsVersion          string   `json:"osVersion"`
	KernelVersion      string   `json:"kernelVersion"`
	Architecture       string   `json:"architecture"`
	UptimeSeconds      int64    `json:"uptimeSeconds"`
	MotherboardVendor  string   `json:"motherboardVendor"`
	MotherboardName    string   `json:"motherboardName"`
	MotherboardVersion string   `json:"motherboardVersion"`
	BiosVendor         string   `json:"biosVendor"`
	BiosVersion        string   `json:"biosVersion"`
	BiosDate           string   `json:"biosDate"`
	Gpus               []string `json:"gpus"`
}

type NetworkInterfaceTelemetry struct {
	Name               string   `json:"name"`
	IpAddresses        []string `json:"ipAddresses"`
	MacAddress         string   `json:"macAddress"`
	IsUp               bool     `json:"isUp"`
	SpeedMbps          int64    `json:"speedMbps"`
	RxBytes            int64    `json:"rxBytes"`
	TxBytes            int64    `json:"txBytes"`
	RxRateBytesPerSec  int64    `json:"rxRateBytesPerSec"`
	TxRateBytesPerSec  int64    `json:"txRateBytesPerSec"`
	IsDefault          bool     `json:"isDefault"`
}

type SystemTelemetry struct {
	Cpu     CpuTelemetry                `json:"cpu"`
	Memory  MemoryTelemetry             `json:"memory"`
	Host    HostHardwareInfo            `json:"host"`
	Network []NetworkInterfaceTelemetry `json:"network"`
}

