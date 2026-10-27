package docker

import (
	"context"
	"strings"
	"testing"
)

// TCP and UDP bindings of the same host port are distinct and must not be reported as a conflict,
// in any compose port syntax (short, long, or TrueNAS-rendered JSON).
func TestComposePortsSamePortTCPAndUDPIsNotConflict(t *testing.T) {
	a := NewAdapterWithClient(nil, "http://x")
	a.hasSocket = false
	cases := map[string]string{
		"short": "services:\n  t:\n    image: x\n    ports:\n      - \"51413:51413\"\n      - \"51413:51413/udp\"\n",
		"long":  "services:\n  t:\n    image: x\n    ports:\n      - {published: 51413, target: 51413, protocol: tcp}\n      - {published: 51413, target: 51413, protocol: udp}\n",
		"json":  `{"services":{"t":{"image":"x","ports":[{"mode":"ingress","protocol":"tcp","published":51413,"target":51413},{"mode":"ingress","protocol":"udp","published":51413,"target":51413}]}}}`,
	}
	for name, y := range cases {
		n := a.NormalizeComposePorts(context.Background(), y)
		if err := a.checkComposeHostPortsAvailable(context.Background(), n, "t"); err != nil && strings.Contains(err.Error(), "mapped multiple times") {
			t.Errorf("%s: tcp+udp on same port reported as duplicate: %v", name, err)
		}
	}
}

func TestComposePortsRealDuplicateStillDetected(t *testing.T) {
	a := NewAdapterWithClient(nil, "http://x")
	a.hasSocket = false
	y := "services:\n  t:\n    image: x\n    ports:\n      - \"51413:51413/udp\"\n      - \"51413:6881/udp\"\n"
	err := a.checkComposeHostPortsAvailable(context.Background(), y, "t")
	if err == nil || !strings.Contains(err.Error(), "51413/udp is mapped multiple times") {
		t.Fatalf("expected udp duplicate error, got %v", err)
	}
}
