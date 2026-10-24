package domain

// DockerConfig represents the TrueNAS SCALE Docker and Apps ZFS dataset storage configuration.
type DockerConfig struct {
	Dataset           string   `json:"dataset"`           // e.g. "tank/apps" or "tank/docker"
	Pool              string   `json:"pool"`              // e.g. "tank"
	Mountpoint        string   `json:"mountpoint"`        // e.g. "/tank/apps"
	AppdataPath       string   `json:"appdataPath"`       // e.g. "/tank/apps/appdata"
	DockerDataRoot    string   `json:"dockerDataRoot"`    // e.g. "/tank/apps/docker"
	AvailableDatasets []string `json:"availableDatasets"` // e.g. ["tank/apps", "tank/media", ...]
	AvailablePools    []string `json:"availablePools"`    // e.g. ["tank"]
	DatasetSelected   bool     `json:"datasetSelected"`   // whether a dataset has been explicitly selected
	AppdataDataset    string   `json:"appdataDataset"`    // alias for AppdataPath
	PoolSelected      bool     `json:"poolSelected"`      // alias for DatasetSelected
}

// DirectoryEntry represents a directory found when browsing the host filesystem.
type DirectoryEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// DirectoryList represents directory contents for auto-completion and browsing.
type DirectoryList struct {
	CurrentPath string           `json:"currentPath"`
	ParentPath  string           `json:"parentPath"`
	Entries     []DirectoryEntry `json:"entries"`
}
