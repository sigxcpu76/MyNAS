package domain

import (
	"time"
)

// StorageImage represents an ISO disk image or LXC template archive stored on the main pool repository.
type StorageImage struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	SizeBytes     int64     `json:"sizeBytes"`
	SizeFormatted string    `json:"sizeFormatted"`
	ModTime       time.Time `json:"modTime"`
}
