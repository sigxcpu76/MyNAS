package docker

import (
	"strings"
	"testing"
)

func TestPrettifyComposeYAMLConvertsJSON(t *testing.T) {
	raw := strings.Repeat("\n", 52) + `{"networks": {"proxy_net": {"external": true}}, "services": {"rustfs": {"cap_drop": ["ALL"], "group_add": [568], "image": "rustfs/rustfs:1"}}}`
	got := prettifyComposeYAML(raw)
	if strings.HasPrefix(got, "\n") || strings.Contains(got, "{") {
		t.Fatalf("expected block YAML, got:\n%s", got)
	}
	for _, want := range []string{"networks:\n  proxy_net:\n    external: true", "services:\n  rustfs:", "- ALL", "- 568"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	// ordering preserved
	if strings.Index(got, "networks:") > strings.Index(got, "services:") {
		t.Fatalf("key order changed:\n%s", got)
	}
}

func TestPrettifyComposeYAMLLeavesBlockYAMLAlone(t *testing.T) {
	in := "services:\n  web:\n    image: nginx # keep comment\n"
	if got := prettifyComposeYAML(in); got != in {
		t.Fatalf("block yaml modified: %q", got)
	}
}
