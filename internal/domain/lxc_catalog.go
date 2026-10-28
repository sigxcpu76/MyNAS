package domain

import "time"

// LxcCatalogItem represents an application, appliance, or base OS from one of the supported LXC catalogs:
// 1. "community-scripts" (Community-Scripts / Proxmox Helper Scripts)
// 2. "turnkey" (TurnKey Linux Appliances)
// 3. "linuxcontainers" (Official LinuxContainers / Canonical minimal base images)
type LxcCatalogItem struct {
	ID                    string    `json:"id"`
	Title                 string    `json:"title"`
	Description           string    `json:"description"`
	Source                string    `json:"source"` // "community-scripts", "turnkey", "linuxcontainers"
	Category              string    `json:"category"`
	Categories            []string  `json:"categories"`
	IconURL               string    `json:"iconUrl"`
	Website               string    `json:"website"`
	DocumentationURL      string    `json:"documentationUrl"`
	BaseDistribution      string    `json:"baseDistribution"`
	BaseRelease           string    `json:"baseRelease"`
	Architecture          string    `json:"architecture"`
	DefaultCPUCores       int32     `json:"defaultCpuCores"`
	DefaultMemoryLimitMiB int64     `json:"defaultMemoryLimitMib"`
	DefaultPort           int       `json:"defaultPort"`
	DownloadURL           string    `json:"downloadUrl"`
	InstallScriptURL      string    `json:"installScriptUrl"`
	InstallRecipe         string    `json:"installRecipe"`
	PrivilegedRequired    bool      `json:"privilegedRequired"`
	RecommendedMounts     []string  `json:"recommendedMounts"`
	GpuRecommended        bool      `json:"gpuRecommended"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// LxcCatalogSyncSummary provides metrics on LXC catalog synchronization.
type LxcCatalogSyncSummary struct {
	Success      bool      `json:"success"`
	Message      string    `json:"message"`
	TotalItems   int       `json:"totalItems"`
	LastSyncTime time.Time `json:"lastSyncTime"`
}
