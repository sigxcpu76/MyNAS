package domain

import "time"

// CatalogApp represents an application from the TrueNAS SCALE app catalog.
type CatalogApp struct {
	AppID       string   `json:"appId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Categories  []string `json:"categories"`
	IconURL     string   `json:"iconUrl"`
	Home        string   `json:"home"`
	Train       string   `json:"train"` // "stable", "community"
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Tags        []string `json:"tags"`
	Screenshots    []string `json:"screenshots"`
	Sources        []string `json:"sources"`
	ContainerImage string   `json:"containerImage"`
	ComposeYAML    string   `json:"composeYaml"`
	DefaultPort    int      `json:"defaultPort"`
}

// CatalogSyncSummary provides metrics on catalog synchronization.
type CatalogSyncSummary struct {
	Success      bool      `json:"success"`
	Message      string    `json:"message"`
	TotalApps    int       `json:"totalApps"`
	LastSyncTime time.Time `json:"lastSyncTime"`
}
