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
