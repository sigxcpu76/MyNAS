package zfs

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
)

const (
	debianPeriodicScrubProp = "org.debian:periodic-scrub"
	zfsCronFilePath         = "/etc/cron.d/zfsutils-linux"
	systemdSystemDir        = "/etc/systemd/system"
)

// SetLogger attaches a structured logger to the ZFS adapter.
func (a *Adapter) SetLogger(logger *slog.Logger) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logger = logger
}

func (a *Adapter) logInfo(msg string, args ...any) {
	if a.logger != nil {
		a.logger.Info(msg, args...)
	}
}

// ShouldDisablePeriodicScrubProperty reports whether the given value of
// org.debian:periodic-scrub indicates that host periodic scrubbing is enabled
// (e.g. "-", "auto", "enable") and should be set to "disable".
func ShouldDisablePeriodicScrubProperty(val string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(val))
	if trimmed == "" || trimmed == "disable" {
		return false
	}
	return true
}

// DisableZfsCronScrubLine comments out any active cron schedule line in
// /etc/cron.d/zfsutils-linux that invokes /usr/lib/zfs-linux/scrub.
// It returns the updated file content and true if any line was modified.
func DisableZfsCronScrubLine(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	modified := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "/usr/lib/zfs-linux/scrub") || strings.Contains(trimmed, "zfs-linux/scrub") {
			lines[i] = "# Disabled by nasctl: " + line
			modified = true
		}
	}
	if !modified {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

// DisableCronAutoScrubFile inspects cronFilePath (typically /etc/cron.d/zfsutils-linux)
// and comments out any enabled ZFS scrub schedule.
func DisableCronAutoScrubFile(cronFilePath string) (bool, error) {
	data, err := os.ReadFile(cronFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	updated, changed := DisableZfsCronScrubLine(string(data))
	if !changed {
		return false, nil
	}
	if err := os.WriteFile(cronFilePath, []byte(updated), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// DiscoverEnabledSystemdScrubUnits scans systemd target.wants directories under systemdDir
// for any enabled zfs-scrub*.timer or zfs-scrub*.service unit links.
func DiscoverEnabledSystemdScrubUnits(systemdDir string) []string {
	wantDirs := []string{
		filepath.Join(systemdDir, "timers.target.wants"),
		filepath.Join(systemdDir, "multi-user.target.wants"),
		filepath.Join(systemdDir, "zfs.target.wants"),
	}
	seen := make(map[string]bool)
	var units []string
	for _, dir := range wantDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "zfs-scrub") &&
				(strings.HasSuffix(name, ".timer") || strings.HasSuffix(name, ".service")) {
				if !seen[name] {
					seen[name] = true
					units = append(units, name)
				}
			}
		}
	}
	return units
}

// PoolSystemdScrubTimers returns the candidate systemd scrub timer unit names for a pool.
func PoolSystemdScrubTimers(poolName string) []string {
	clean := strings.TrimPrefix(strings.TrimSpace(poolName), "zfsPools/")
	if clean == "" {
		return nil
	}
	return []string{
		"zfs-scrub-monthly@" + clean + ".timer",
		"zfs-scrub-weekly@" + clean + ".timer",
		"zfs-scrub@" + clean + ".timer",
	}
}

// disablePoolAutoScrub stops and disables host ZFS auto-scrubbing for a single pool:
//  1. Sets org.debian:periodic-scrub=disable on the pool root dataset if not already disabled.
//  2. Stops and disables any active/enabled systemd zfs-scrub timers or service for the pool.
func (a *Adapter) disablePoolAutoScrub(ctx context.Context, poolName string) {
	cleanID := strings.TrimPrefix(strings.TrimSpace(poolName), "zfsPools/")
	if cleanID == "" || domain.IsZvolDevice(cleanID) {
		return
	}

	// 1. Check and disable org.debian:periodic-scrub property on pool root dataset
	if out, err := exec.CommandContext(ctx, "zfs", "get", "-H", "-o", "value", debianPeriodicScrubProp, cleanID).Output(); err == nil {
		val := strings.TrimSpace(string(out))
		if ShouldDisablePeriodicScrubProperty(val) {
			if err := exec.CommandContext(ctx, "zfs", "set", debianPeriodicScrubProp+"=disable", cleanID).Run(); err == nil {
				a.logInfo("disabled host ZFS periodic auto-scrub property on pool", "pool", cleanID, "property", debianPeriodicScrubProp, "previous", val)
			}
		}
	}

	// 2. Check and stop/disable systemd scrub timers and service for this pool
	if os.Geteuid() != 0 {
		return
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}

	reloaded := false
	for _, timerUnit := range PoolSystemdScrubTimers(cleanID) {
		isEnabled := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", timerUnit).Run() == nil
		isActive := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", timerUnit).Run() == nil
		removedLink := removeSystemdScrubWantLinks(systemdSystemDir, timerUnit)

		if isEnabled || isActive || removedLink {
			_ = exec.CommandContext(ctx, "systemctl", "disable", "--now", timerUnit).Run()
			reloaded = true
			a.logInfo("stopped and disabled host ZFS auto-scrub systemd timer", "pool", cleanID, "unit", timerUnit)
		}
	}

	svcUnit := "zfs-scrub@" + cleanID + ".service"
	if exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", svcUnit).Run() == nil {
		_ = exec.CommandContext(ctx, "systemctl", "stop", svcUnit).Run()
		a.logInfo("stopped active host ZFS auto-scrub systemd service", "pool", cleanID, "unit", svcUnit)
	}

	if reloaded {
		_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	}
}

func removeSystemdScrubWantLinks(systemdDir, unitName string) bool {
	wantDirs := []string{
		filepath.Join(systemdDir, "timers.target.wants"),
		filepath.Join(systemdDir, "multi-user.target.wants"),
		filepath.Join(systemdDir, "zfs.target.wants"),
	}
	removed := false
	for _, dir := range wantDirs {
		linkPath := filepath.Join(dir, unitName)
		if _, err := os.Lstat(linkPath); err == nil {
			if os.Remove(linkPath) == nil {
				removed = true
			}
		}
	}
	return removed
}

// disableHostAutoScrubGlobals disables host-wide ZFS auto-scrubbing schedulers:
//   - Comments out the periodic scrub job in /etc/cron.d/zfsutils-linux
//   - Disables any zfs-scrub*.timer symlinks found in /etc/systemd/system/*.target.wants
func (a *Adapter) disableHostAutoScrubGlobals(ctx context.Context) {
	if os.Geteuid() != 0 {
		return
	}

	if changed, err := DisableCronAutoScrubFile(zfsCronFilePath); err == nil && changed {
		a.logInfo("disabled host ZFS auto-scrub cron schedule", "path", zfsCronFilePath)
	}

	units := DiscoverEnabledSystemdScrubUnits(systemdSystemDir)
	if len(units) > 0 {
		for _, unit := range units {
			removeSystemdScrubWantLinks(systemdSystemDir, unit)
			if _, err := exec.LookPath("systemctl"); err == nil {
				_ = exec.CommandContext(ctx, "systemctl", "disable", "--now", unit).Run()
			}
			a.logInfo("disabled host ZFS auto-scrub systemd unit", "unit", unit)
		}
		if _, err := exec.LookPath("systemctl"); err == nil {
			_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
		}
	}
}
