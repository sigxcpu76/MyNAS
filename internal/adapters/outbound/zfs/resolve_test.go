package zfs

import "testing"

func TestMatchZfsName(t *testing.T) {
	names := []string{
		"tank",
		"tank/vms",
		"tank/vms/arch",
		"tank/vms/debian-server",
		"tank/media",
		"tank/a-b",
		"tank/a/b",
	}

	cases := map[string]string{
		"vms-arch":          "tank/vms/arch",
		"vms-debian-server": "tank/vms/debian-server", // naive '-'->'/' would yield tank/vms/debian/server
		"media":             "tank/media",
		"vms":               "tank/vms",
		"a-b":               "tank/a-b", // exact relative match wins over flattened tank/a/b
		"missing":           "",
	}
	for id, want := range cases {
		if got := matchZfsName(names, "tank", id); got != want {
			t.Errorf("matchZfsName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestEncodeZfsID(t *testing.T) {
	if got := encodeZfsID("tank", "tank/vms/debian-server"); got != "vms-debian-server" {
		t.Fatalf("got %q", got)
	}
}
