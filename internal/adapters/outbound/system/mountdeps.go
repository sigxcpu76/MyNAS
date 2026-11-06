package system

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverDockerMountPaths inspects daemon.json, known pool hints, and the /mnt filesystem
// to discover any ZFS-backed docker data directories that must be mounted before Docker starts.
func DiscoverDockerMountPaths(mntRoot, etcDockerDir, poolHint string) []string {
	found := make(map[string]bool)

	// 1. Check /etc/docker/daemon.json
	daemonJSONPath := filepath.Join(etcDockerDir, "daemon.json")
	if data, err := os.ReadFile(daemonJSONPath); err == nil {
		var daemonCfg struct {
			DataRoot string `json:"data-root"`
			Graph    string `json:"graph"`
		}
		if json.Unmarshal(data, &daemonCfg) == nil {
			target := daemonCfg.DataRoot
			if target == "" {
				target = daemonCfg.Graph
			}
			clean := filepath.Clean(strings.TrimSpace(target))
			if clean != "" && strings.HasPrefix(clean, mntRoot) {
				found[clean] = true
			}
		}
	}

	// 2. Check poolHint (e.g. "storage" -> "/mnt/storage/docker")
	cleanHint := strings.TrimPrefix(strings.TrimSpace(poolHint), "zfsPools/")
	if cleanHint != "" {
		candidates := []string{
			filepath.Join(mntRoot, cleanHint, "docker"),
			filepath.Join(mntRoot, cleanHint, "apps", "docker"),
			filepath.Join(mntRoot, cleanHint, "ix-apps", "docker"),
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				found[filepath.Clean(c)] = true
			}
		}
	}

	// 3. Scan mntRoot for any existing pool/docker directories
	if entries, err := os.ReadDir(mntRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			poolDir := filepath.Join(mntRoot, e.Name())
			candidates := []string{
				filepath.Join(poolDir, "docker"),
				filepath.Join(poolDir, "apps", "docker"),
				filepath.Join(poolDir, "ix-apps", "docker"),
			}
			for _, c := range candidates {
				if fi, err := os.Stat(c); err == nil && fi.IsDir() {
					found[filepath.Clean(c)] = true
				}
			}
		}
	}

	var results []string
	for p := range found {
		results = append(results, p)
	}
	sort.Strings(results)
	return results
}

// BuildDockerDropinContent generates the systemd drop-in configuration for docker.service.
func BuildDockerDropinContent(mountPaths []string) string {
	var sb strings.Builder
	sb.WriteString("# Managed by nasctl: Ensures ZFS pool and datasets are mounted before starting Docker daemon\n")
	sb.WriteString("[Unit]\n")
	sb.WriteString("After=zfs-mount.service zfs-import.target zfs-import-cache.service\n")
	sb.WriteString("Wants=zfs-mount.service\n")
	if len(mountPaths) > 0 {
		sb.WriteString(fmt.Sprintf("RequiresMountsFor=%s\n", strings.Join(mountPaths, " ")))
	}
	return sb.String()
}

// EnsureDockerMountDependencies patches docker.service systemd drop-in to depend on ZFS mounts.
// It also enables zfs-import-cache.service and zfs-mount.service for boot safety.
func EnsureDockerMountDependencies(logger *slog.Logger, poolHint string) (bool, error) {
	if os.Geteuid() != 0 {
		return false, nil
	}
	systemdDir := "/etc/systemd/system"
	if fi, err := os.Stat(systemdDir); err != nil || !fi.IsDir() {
		return false, nil
	}

	mountPaths := DiscoverDockerMountPaths("/mnt", "/etc/docker", poolHint)

	dropinDir := filepath.Join(systemdDir, "docker.service.d")
	if err := os.MkdirAll(dropinDir, 0755); err != nil {
		return false, err
	}
	dropinFile := filepath.Join(dropinDir, "nas-mounts.conf")

	expectedContent := BuildDockerDropinContent(mountPaths)
	current, err := os.ReadFile(dropinFile)
	if err == nil && string(current) == expectedContent {
		return false, nil
	}

	if err := os.WriteFile(dropinFile, []byte(expectedContent), 0644); err != nil {
		return false, err
	}

	// Enable ZFS mount & import services so datasets are imported and mounted on boot
	_ = exec.Command("systemctl", "enable", "zfs-import-cache.service", "zfs-mount.service").Run()
	_ = exec.Command("systemctl", "daemon-reload").Run()

	if logger != nil {
		logger.Info("configured docker.service systemd mount dependencies for ZFS persistence",
			"dropin", dropinFile,
			"requires_mounts_for", mountPaths,
		)
	}

	return true, nil
}
