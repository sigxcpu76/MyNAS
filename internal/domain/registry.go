package domain

import (
	"fmt"
	"strings"
	"time"
)

// DockerRegistry holds credentials for a container registry (Docker Hub, GHCR, private registries).
// Password is write-only: adapters never return it to callers.
type DockerRegistry struct {
	ID              string    `json:"id"`
	Server          string    `json:"server"` // canonical host, e.g. "docker.io", "ghcr.io"
	Username        string    `json:"username"`
	Password        string    `json:"-"`
	HasPassword     bool      `json:"hasPassword"`
	Description     string    `json:"description"`
	LastLoginStatus string    `json:"lastLoginStatus"`
	LastLoginTime   time.Time `json:"lastLoginTime"`
}

// DockerHubAuthKey is the key Docker CLI uses for Docker Hub credentials in config.json.
const DockerHubAuthKey = "https://index.docker.io/v1/"

// NormalizeRegistryServer returns the canonical registry host for user input such as
// "https://ghcr.io/", "index.docker.io" or "" (Docker Hub).
func NormalizeRegistryServer(server string) string {
	s := strings.TrimSpace(strings.ToLower(server))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "", "docker.io", "index.docker.io", "registry-1.docker.io", "hub.docker.com", "registry.hub.docker.com":
		return "docker.io"
	}
	return s
}

// RegistryAuthKey returns the config.json "auths" key Docker expects for a canonical server.
func RegistryAuthKey(server string) string {
	if NormalizeRegistryServer(server) == "docker.io" {
		return DockerHubAuthKey
	}
	return NormalizeRegistryServer(server)
}

// RegistryForImage returns the canonical registry host an image reference is pulled from.
func RegistryForImage(image string) string {
	ref := strings.TrimSpace(image)
	first, _, found := strings.Cut(ref, "/")
	if !found {
		return "docker.io"
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return NormalizeRegistryServer(first)
	}
	return "docker.io"
}

// Validate checks required registry fields.
func (r *DockerRegistry) Validate() error {
	if strings.TrimSpace(r.Server) == "" {
		return fmt.Errorf("%w: registry server is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(r.Username) == "" {
		return fmt.Errorf("%w: registry username is required", ErrInvalidArgument)
	}
	return nil
}
