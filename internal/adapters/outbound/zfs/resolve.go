package zfs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Resource IDs for nested datasets/volumes are derived by flattening the ZFS
// path relative to the pool with '/' -> '-' (e.g. "tank/vms/debian-server"
// becomes "vms-debian-server"). That mapping is lossy, so it cannot be reversed
// by string replacement alone. These helpers resolve an ID back to the real ZFS
// name by matching it against the names that actually exist on the host.

// encodeZfsID flattens a full ZFS name into the resource ID used by the API.
func encodeZfsID(pool, fullName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(fullName, pool+"/"), "/", "-")
}

// matchZfsName picks the full ZFS name for id among existing names.
// Exact relative-path matches win over flattened-ID matches; when several
// names flatten to the same ID the shallowest (fewest '/') wins for stability.
// Returns "" when nothing matches.
func matchZfsName(names []string, pool, id string) string {
	best := ""
	for _, name := range names {
		if !strings.HasPrefix(name, pool+"/") {
			continue
		}
		rel := strings.TrimPrefix(name, pool+"/")
		if rel == id {
			return name
		}
		if encodeZfsID(pool, name) == id {
			if best == "" || strings.Count(name, "/") < strings.Count(best, "/") {
				best = name
			}
		}
	}
	return best
}

// resolveZfsPath converts a dataset/volume resource ID into the full ZFS name.
// zfsType is "filesystem" or "volume".
func (a *Adapter) resolveZfsPath(ctx context.Context, cleanPool, id, zfsType string) string {
	id = strings.TrimPrefix(id, "datasets/")
	id = strings.TrimPrefix(id, "volumes/")
	if id == "" || (zfsType == "filesystem" && id == "root") || id == cleanPool {
		return cleanPool
	}
	if strings.Contains(id, "/") {
		if strings.HasPrefix(id, cleanPool+"/") {
			return id
		}
		return fmt.Sprintf("%s/%s", cleanPool, id)
	}

	out, err := exec.CommandContext(ctx, "zfs", "list", "-H", "-o", "name", "-r", "-t", zfsType, cleanPool).Output()
	if err == nil {
		if name := matchZfsName(strings.Split(strings.TrimSpace(string(out)), "\n"), cleanPool, id); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%s/%s", cleanPool, id)
}
