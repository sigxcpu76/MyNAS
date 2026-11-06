package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverDockerMountPaths(t *testing.T) {
	tempDir := t.TempDir()
	mntRoot := filepath.Join(tempDir, "mnt")
	etcDocker := filepath.Join(tempDir, "etc_docker")
	_ = os.MkdirAll(mntRoot, 0755)
	_ = os.MkdirAll(etcDocker, 0755)

	// Case 1: Empty environment
	paths := DiscoverDockerMountPaths(mntRoot, etcDocker, "")
	if len(paths) != 0 {
		t.Fatalf("expected 0 paths in empty environment, got %v", paths)
	}

	// Case 2: daemon.json specifies data-root in /mnt
	daemonJSON := `{"data-root": "` + filepath.Join(mntRoot, "storage", "docker") + `"}`
	_ = os.WriteFile(filepath.Join(etcDocker, "daemon.json"), []byte(daemonJSON), 0644)

	paths = DiscoverDockerMountPaths(mntRoot, etcDocker, "")
	if len(paths) != 1 || paths[0] != filepath.Join(mntRoot, "storage", "docker") {
		t.Fatalf("expected data-root from daemon.json, got %v", paths)
	}

	// Case 3: poolHint with existing directory on disk
	poolDockerDir := filepath.Join(mntRoot, "tank", "docker")
	_ = os.MkdirAll(poolDockerDir, 0755)

	paths = DiscoverDockerMountPaths(mntRoot, etcDocker, "tank")
	// Should contain both storage/docker (from daemon.json) and tank/docker (from disk)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %v", paths)
	}
}

func TestBuildDockerDropinContent(t *testing.T) {
	// Case 1: No mount paths
	contentEmpty := BuildDockerDropinContent(nil)
	if !strings.Contains(contentEmpty, "After=zfs-mount.service") {
		t.Errorf("missing After=zfs-mount.service")
	}
	if strings.Contains(contentEmpty, "RequiresMountsFor=") {
		t.Errorf("unexpected RequiresMountsFor when no paths provided")
	}

	// Case 2: With mount paths
	contentWithPaths := BuildDockerDropinContent([]string{"/mnt/storage/docker", "/mnt/tank/docker"})
	if !strings.Contains(contentWithPaths, "RequiresMountsFor=/mnt/storage/docker /mnt/tank/docker") {
		t.Errorf("expected formatted RequiresMountsFor, got:\n%s", contentWithPaths)
	}
}
