package system

import (
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestIsZvolDevice(t *testing.T) {
	zvols := []string{
		"zd0",
		"zd16",
		"zd64p2",
		"/dev/zd128",
		"/dev/zd544",
		"/dev/zvol/tank/vm-100-disk-0",
		"/dev/zvol/rpool/data/subvol-101-disk-0",
	}
	for _, dev := range zvols {
		if !domain.IsZvolDevice(dev) {
			t.Errorf("expected IsZvolDevice(%q) == true, got false", dev)
		}
	}

	nonZvols := []string{
		"sda",
		"/dev/sda",
		"/dev/sdb1",
		"/dev/nvme0n1",
		"/dev/nvme0n1p3",
		"/dev/disk/by-id/wwn-0x50026b7380d15799-part3",
		"/dev/disk/by-path/pci-0000:00:11.4-ata-2.0-part3",
	}
	for _, dev := range nonZvols {
		if domain.IsZvolDevice(dev) {
			t.Errorf("expected IsZvolDevice(%q) == false, got true", dev)
		}
	}
}

func TestParseUnimportedPools_IgnoresZvols(t *testing.T) {
	rawOutput := `  pool: boot-pool
    id: 18206301404569400741
 state: ONLINE
status: Some supported features are not enabled on the pool.
action: The pool can be imported using its name or numeric identifier.
config:

	boot-pool   ONLINE
	  zd128     ONLINE

  pool: storage
    id: 2462744917453601801
 state: ONLINE
status: The pool was last accessed by another system.
action: The pool can be imported using its name or numeric identifier and the '-f' flag.
config:

	storage     ONLINE
	  zd64p2    ONLINE

  pool: oldrpool
    id: 9611962285660214094
 state: DEGRADED
status: The pool was last accessed by another system.
action: The pool can be imported despite missing or damaged devices.
config:

	oldrpool                            DEGRADED
	  mirror-0                          DEGRADED
	    pci-0000:00:11.4-ata-2.0-part3  ONLINE
	    5514801849470982002             UNAVAIL

  pool: data-vault
    id: 1234567890123456789
 state: ONLINE
config:

	data-vault  ONLINE
	  mirror-0  ONLINE
	    sda     ONLINE
	    sdb     ONLINE
`

	pools := parseUnimportedPools(rawOutput)

	// boot-pool and storage must be filtered out because they are inside zd128 and zd64p2
	// oldrpool and data-vault must be kept because they reside on physical disks
	poolMap := make(map[string]bool)
	for _, p := range pools {
		poolMap[p] = true
	}

	if poolMap["boot-pool"] {
		t.Errorf("expected boot-pool (on zd128) to be ignored, but it was returned")
	}
	if poolMap["storage"] {
		t.Errorf("expected storage (on zd64p2) to be ignored, but it was returned")
	}
	if !poolMap["oldrpool"] {
		t.Errorf("expected oldrpool to be returned, but it was missing: %v", pools)
	}
	if !poolMap["data-vault"] {
		t.Errorf("expected data-vault to be returned, but it was missing: %v", pools)
	}
	if len(pools) != 2 {
		t.Errorf("expected exactly 2 pools, got %d: %v", len(pools), pools)
	}
}

func TestParseUnimportedPoolsNoPoolsAvailable(t *testing.T) {
	outputs := []string{
		"no pools available to import\n",
		"no pools available to import",
		"NO POOLS AVAILABLE TO IMPORT",
		"",
		"   \n",
		"random text without pool prefix",
	}

	for _, out := range outputs {
		pools := parseUnimportedPools(out)
		if len(pools) != 0 {
			t.Errorf("expected 0 pools for output %q, got: %v", out, pools)
		}
	}
}
