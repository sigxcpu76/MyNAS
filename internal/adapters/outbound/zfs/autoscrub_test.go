package zfs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
)

func TestShouldDisablePeriodicScrubProperty(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"-", true},
		{"auto", true},
		{"enable", true},
		{"AUTO\n", true},
		{"disable", false},
		{"DISABLE", false},
		{"", false},
		{"   ", false},
	}

	for _, tc := range cases {
		if got := zfs.ShouldDisablePeriodicScrubProperty(tc.val); got != tc.want {
			t.Errorf("ShouldDisablePeriodicScrubProperty(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestDisableZfsCronScrubLine(t *testing.T) {
	debianCron := `PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin

# TRIM the first Sunday of every month.
24 0 1-7 * * root if [ $(date +\%w) -eq 0 ] && [ -x /usr/lib/zfs-linux/trim ]; then /usr/lib/zfs-linux/trim; fi

# Scrub the second Sunday of every month.
24 0 8-14 * * root if [ $(date +\%w) -eq 0 ] && [ -x /usr/lib/zfs-linux/scrub ]; then /usr/lib/zfs-linux/scrub; fi
`
	updated, changed := zfs.DisableZfsCronScrubLine(debianCron)
	if !changed {
		t.Fatal("expected DisableZfsCronScrubLine to report changed = true")
	}
	if !strings.Contains(updated, "# Disabled by nasctl: 24 0 8-14 * * root") {
		t.Errorf("expected scrub line to be commented out, got:\n%s", updated)
	}
	// Ensure TRIM line is left intact
	if !strings.Contains(updated, "\n24 0 1-7 * * root if [ $(date +\\%w) -eq 0 ] && [ -x /usr/lib/zfs-linux/trim ];") {
		t.Errorf("expected trim line to remain untouched, got:\n%s", updated)
	}

	// Idempotency check
	updated2, changed2 := zfs.DisableZfsCronScrubLine(updated)
	if changed2 || updated2 != updated {
		t.Errorf("expected second call to be idempotent (changed=%v)", changed2)
	}
}

func TestDisableCronAutoScrubFile(t *testing.T) {
	tmpDir := t.TempDir()
	cronPath := filepath.Join(tmpDir, "zfsutils-linux")

	// Missing file should not error
	changed, err := zfs.DisableCronAutoScrubFile(cronPath)
	if err != nil || changed {
		t.Fatalf("expected (false, nil) for missing file, got (%v, %v)", changed, err)
	}

	initial := "# Scrub\n24 0 8-14 * * root /usr/lib/zfs-linux/scrub\n"
	if err := os.WriteFile(cronPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err = zfs.DisableCronAutoScrubFile(cronPath)
	if err != nil || !changed {
		t.Fatalf("expected (true, nil), got (%v, %v)", changed, err)
	}

	data, err := os.ReadFile(cronPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Disabled by nasctl: 24 0 8-14 * * root /usr/lib/zfs-linux/scrub") {
		t.Fatalf("unexpected file content: %s", string(data))
	}
}

func TestDiscoverEnabledSystemdScrubUnits(t *testing.T) {
	tmpDir := t.TempDir()
	timersWants := filepath.Join(tmpDir, "timers.target.wants")
	if err := os.MkdirAll(timersWants, 0755); err != nil {
		t.Fatal(err)
	}

	_ = os.WriteFile(filepath.Join(timersWants, "zfs-scrub-monthly@tank.timer"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(timersWants, "zfs-scrub-weekly@rpool.timer"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(timersWants, "apt-daily.timer"), []byte(""), 0644)

	units := zfs.DiscoverEnabledSystemdScrubUnits(tmpDir)
	if len(units) != 2 {
		t.Fatalf("expected 2 scrub timer units, got %v", units)
	}
}

func TestPoolSystemdScrubTimers(t *testing.T) {
	timers := zfs.PoolSystemdScrubTimers("zfsPools/storage")
	if len(timers) != 3 {
		t.Fatalf("expected 3 candidate timers, got %v", timers)
	}
	if timers[0] != "zfs-scrub-monthly@storage.timer" ||
		timers[1] != "zfs-scrub-weekly@storage.timer" ||
		timers[2] != "zfs-scrub@storage.timer" {
		t.Errorf("unexpected timer names: %v", timers)
	}
}
