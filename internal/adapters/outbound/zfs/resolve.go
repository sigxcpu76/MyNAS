package zfs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// encodeZfsID converts a full ZFS dataset/volume name (e.g. "tank/vms/debian-server")
// into a single-segment AIP-122-safe resource ID using a prefix-free bijection:
//   - "_e" -> "_ee"
//   - "_s" -> "_es"
//   - "/"  -> "_s"
// Flat names that do not contain "/", "_e", or "_s" remain unchanged.
func encodeZfsID(pool, fullName string) string {
	rel := strings.TrimPrefix(fullName, pool+"/")
	if rel == fullName && fullName == pool {
		return "root"
	}
	var b strings.Builder
	b.Grow(len(rel) + 4)
	for i := 0; i < len(rel); {
		if rel[i] == '/' {
			b.WriteString("_s")
			i++
		} else if rel[i] == '_' && i+1 < len(rel) && (rel[i+1] == 's' || rel[i+1] == 'e') {
			b.WriteString("_e")
			b.WriteByte(rel[i+1])
			i += 2
		} else {
			b.WriteByte(rel[i])
			i++
		}
	}
	return b.String()
}

// decodeZfsID reverses encodeZfsID back into the pool-relative ZFS path.
func decodeZfsID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for i := 0; i < len(id); {
		if id[i] == '_' && i+1 < len(id) {
			if id[i+1] == 's' {
				b.WriteByte('/')
				i += 2
				continue
			}
			if id[i+1] == 'e' && i+2 < len(id) && (id[i+2] == 's' || id[i+2] == 'e') {
				b.WriteByte('_')
				b.WriteByte(id[i+2])
				i += 3
				continue
			}
		}
		b.WriteByte(id[i])
		i++
	}
	return b.String()
}

// matchZfsName resolves an API resource ID back to an existing full ZFS name:
//  1. Bijective encoded ID match (encodeZfsID(pool, name) == id).
//  2. Exact relative name match (rel == id).
//  3. Legacy '/' -> '-' match ONLY when unambiguous (exactly one candidate).
//
// Returns "" when nothing matches or when a legacy '-' ID is ambiguous.
func matchZfsName(names []string, pool, id string) string {
	exactRel := ""
	legacyMatch := ""
	legacyCount := 0

	for _, name := range names {
		if !strings.HasPrefix(name, pool+"/") {
			continue
		}
		rel := strings.TrimPrefix(name, pool+"/")
		if encodeZfsID(pool, name) == id {
			return name
		}
		if rel == id && exactRel == "" {
			exactRel = name
		}
		if strings.ReplaceAll(rel, "/", "-") == id {
			legacyMatch = name
			legacyCount++
		}
	}
	if exactRel != "" {
		return exactRel
	}
	if legacyCount == 1 {
		return legacyMatch
	}
	return ""
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
	return fmt.Sprintf("%s/%s", cleanPool, decodeZfsID(id))
}
