package zfs

import (
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestEncodeDecodeZfsIDBijection(t *testing.T) {
	samples := []string{
		"media",
		".nas-system",
		".system/cores",
		"vms/debian-server",
		"a-b",
		"a/b",
		"a_sb",
		"a_esb",
		"a_eesb",
		"a_/b",
		"a_e/b",
		"a_s/b",
		"my_data_01",
		"deep/nested_s/path_e/item-1",
	}

	seen := make(map[string]string, len(samples))
	for _, rel := range samples {
		full := "tank/" + rel
		enc := encodeZfsID("tank", full)
		if prev, exists := seen[enc]; exists {
			t.Fatalf("collision: %q and %q both encoded to %q", prev, rel, enc)
		}
		seen[enc] = rel

		dec := decodeZfsID(enc)
		if dec != rel {
			t.Fatalf("round-trip mismatch for %q: encoded=%q decoded=%q", rel, enc, dec)
		}
	}
}

func TestMatchZfsName(t *testing.T) {
	names := []string{
		"tank",
		"tank/vms",
		"tank/vms/arch",
		"tank/vms/debian-server",
		"tank/media",
		"tank/a-b",
		"tank/a/b",
		"tank/a_sb",
		"tank/x-y/z",
		"tank/x/y-z",
	}

	cases := map[string]string{
		"vms_sarch":          "tank/vms/arch",
		"vms_sdebian-server": "tank/vms/debian-server",
		"vms-arch":           "tank/vms/arch",          // unambiguous legacy ID still resolves
		"vms-debian-server":  "tank/vms/debian-server", // unambiguous legacy ID with hyphens resolves
		"media":              "tank/media",
		"vms":                "tank/vms",
		"a-b":                "tank/a-b",  // exact flat dataset
		"a_sb":               "tank/a/b",  // encoded nested dataset does NOT collide with tank/a-b or tank/a_sb
		"a_esb":              "tank/a_sb", // encoded literal "a_sb" dataset
		"x-y-z":              "",          // ambiguous legacy '-' collision refuses to guess
		"missing":            "",
	}
	for id, want := range cases {
		if got := matchZfsName(names, "tank", id); got != want {
			t.Errorf("matchZfsName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestEncodeZfsID(t *testing.T) {
	if got := encodeZfsID("tank", "tank/vms/debian-server"); got != "vms_sdebian-server" {
		t.Fatalf("got %q, want vms_sdebian-server", got)
	}
	if got := encodeZfsID("tank", "tank/.nas-system"); got != ".nas-system" {
		t.Fatalf("got %q, want .nas-system", got)
	}
	if got := encodeZfsID("tank", "tank"); got != "root" {
		t.Fatalf("got %q, want root", got)
	}
}

func TestFormatScanStatsAndPoolHealth(t *testing.T) {
	if got := formatScanStats(nil); got != "Never scrubbed" {
		t.Fatalf("expected Never scrubbed, got %q", got)
	}
	if got := formatScanStats(map[string]any{
		"function":   "SCRUB",
		"state":      "SCANNING",
		"examined":   "50G",
		"to_examine": "200G",
		"errors":     "0",
	}); got != "Scrub in progress (25.0%, 0 errors)" {
		t.Fatalf("unexpected scanning status: %q", got)
	}
	if got := formatScanStats(map[string]any{
		"function": "SCRUB",
		"state":    "FINISHED",
		"errors":   "0",
		"end_time": "Sun Nov 10 04:12:00 2026",
	}); got != "Scrub completed with 0 errors on Sun Nov 10 04:12:00 2026" {
		t.Fatalf("unexpected finished status: %q", got)
	}
	if got := formatScanStats(map[string]any{
		"function": "RESILVER",
		"state":    "FINISHED",
		"errors":   "2",
	}); got != "Resilver completed with 2 errors" {
		t.Fatalf("unexpected resilver status: %q", got)
	}

	vdevs := []domain.ZfsVdev{
		{
			Name:  "mirror-0",
			State: "DEGRADED",
			Children: []domain.ZfsVdev{
				{Name: "sda", State: "ONLINE"},
				{Name: "sdb", State: "OFFLINE"},
			},
		},
		{
			Name:  "spares",
			State: "AVAIL",
			Children: []domain.ZfsVdev{
				{Name: "sdc", State: "AVAIL"},
			},
		},
	}
	if got := formatPoolHealth("DEGRADED", "0", vdevs); got != "Pool DEGRADED: 1 device(s) degraded/offline; 0 data errors" {
		t.Fatalf("unexpected degraded health string: %q", got)
	}
}
