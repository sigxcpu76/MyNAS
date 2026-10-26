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
	ContainerImage string                 `json:"containerImage"`
	ComposeYAML    string                 `json:"composeYaml"`
	DefaultPort    int                    `json:"defaultPort"`
	QuestionGroups []CatalogQuestionGroup `json:"questionGroups,omitempty"`
	Questions      []CatalogQuestion      `json:"questions,omitempty"`
}

// CatalogQuestion represents an individual configuration question from TrueNAS questions.yaml.
type CatalogQuestion struct {
	Variable     string   `json:"variable"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Type         string   `json:"type"` // "string", "int", "boolean", "hostpath", "select"
	DefaultValue string   `json:"defaultValue"`
	Required     bool     `json:"required"`
	Group        string   `json:"group"`
	Options      []string `json:"options,omitempty"`
	MountPath    string   `json:"mountPath,omitempty"`
}

// CatalogQuestionGroup groups related configuration questions.
type CatalogQuestionGroup struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Questions   []CatalogQuestion `json:"questions"`
}

// CatalogSyncSummary provides metrics on catalog synchronization.
type CatalogSyncSummary struct {
	Success      bool      `json:"success"`
	Message      string    `json:"message"`
	TotalApps    int       `json:"totalApps"`
	LastSyncTime time.Time `json:"lastSyncTime"`
}
