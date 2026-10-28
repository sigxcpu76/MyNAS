package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DockerAppContainer represents a container instance running inside a Docker Compose App.
type DockerAppContainer struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ServiceName string    `json:"serviceName"`
	Image       string    `json:"image"`
	State       string    `json:"state"` // RUNNING, EXITED, CREATED, PAUSED
	Status      string    `json:"status"`
	CPUPercent  float64   `json:"cpuPercent"`
	MemoryMiB   int64     `json:"memoryMib"`
	Ports       []string  `json:"ports"`
	CreatedAt   time.Time `json:"createdAt"`
	NetRxBytes  int64     `json:"netRxBytes"`
	NetTxBytes  int64     `json:"netTxBytes"`
	NetRxRate   int64     `json:"netRxRate"`
	NetTxRate   int64     `json:"netTxRate"`
}

// DockerAppPortal represents an exposed web UI portal link defined by the app manifest.
type DockerAppPortal struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "http" or "https"
	Path     string `json:"path"`
}

// HostPortStatus represents preemptive port availability status on the host system.
type HostPortStatus struct {
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"` // "tcp" or "udp"
	Available bool   `json:"available"`
	InUseBy   string `json:"in_use_by"`
}

// DockerApp represents a Docker Compose Application descriptor and its child containers.
type DockerApp struct {
	ID              string               `json:"id"` // Unique app identifier e.g. "nextcloud"
	Name            string               `json:"name"` // "dockerApps/{id}"
	Title           string               `json:"title"` // Display title e.g. "Nextcloud Stack"
	State           string               `json:"state"` // RUNNING, STOPPED, PARTIALLY_RUNNING, ERROR
	StatusText      string               `json:"statusText"`
	AutoStart       bool                 `json:"autoStart"`
	ComposeYAML     string               `json:"composeYaml"`
	ComposeFilePath string               `json:"composeFilePath"`
	Services        []string             `json:"services"`
	Containers      []DockerAppContainer `json:"containers"`
	PortMappings    []string             `json:"portMappings"`
	Volumes         []string             `json:"volumes"`
	WebUIURL        string               `json:"webUiUrl"`
	Portals         []DockerAppPortal    `json:"portals"`
	TotalCPUPercent float64              `json:"totalCpuPercent"`
	TotalMemoryMiB  int64                `json:"totalMemoryMib"`
	TotalNetRxBytes int64                `json:"totalNetRxBytes"`
	TotalNetTxBytes int64                `json:"totalNetTxBytes"`
	TotalNetRxRate  int64                `json:"totalNetRxRate"`
	TotalNetTxRate  int64                `json:"totalNetTxRate"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

// Validate checks that the DockerApp has a valid ID and non-empty Compose YAML.
func (a *DockerApp) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("app ID cannot be empty")
	}
	if strings.ContainsAny(a.ID, " /\\#%?&:@") {
		return fmt.Errorf("app ID %q contains invalid characters", a.ID)
	}
	if strings.TrimSpace(a.ComposeYAML) == "" {
		return errors.New("compose YAML cannot be empty for a Docker App")
	}
	return nil
}

// ParseDockerAppName extracts the app ID from an AIP resource name "dockerApps/{app}".
func ParseDockerAppName(name string) (string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] != "dockerApps" || parts[1] == "" {
		return "", fmt.Errorf("invalid docker app resource name %q, expected dockerApps/{app_id}", name)
	}
	return parts[1], nil
}

// DockerAppName constructs an AIP resource name "dockerApps/{app_id}".
func DockerAppName(appID string) string {
	return fmt.Sprintf("dockerApps/%s", appID)
}
