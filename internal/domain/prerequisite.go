package domain

import "time"

type PrerequisiteStatus string

const (
	StatusInstalled      PrerequisiteStatus = "INSTALLED"
	StatusMissing        PrerequisiteStatus = "MISSING"
	StatusDegraded       PrerequisiteStatus = "DEGRADED"
	StatusFallbackActive PrerequisiteStatus = "FALLBACK_ACTIVE"
)

type Prerequisite struct {
	ID           string
	DisplayName  string
	Category     string // STORAGE, VIRTUALIZATION, CONTAINERS, SHARING, SECURITY
	Kind         string // BINARY, LIBRARY, KERNEL_MODULE, DAEMON
	BinaryOrPath string
	Status       PrerequisiteStatus
	Version      string
	Details      string
	InstallHint  string
	RequiredBy   []string
	FallbackMode bool
	CheckedTime  time.Time
}

type LinuxFlavor struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	PrettyName             string `json:"prettyName"`
	VersionID              string `json:"versionId"`
	PackageManager         string `json:"packageManager"`
	InstallCommandTemplate string `json:"installCommandTemplate"`
}

type PrerequisitesReport struct {
	LinuxFlavor       LinuxFlavor    `json:"linuxFlavor"`
	Missing           []Prerequisite `json:"missing"`
	Installed         []Prerequisite `json:"installed"`
	All               []Prerequisite `json:"all"`
	RemediationScript string         `json:"remediationScript"`
	IsFullyCompliant  bool           `json:"isFullyCompliant"`
}
