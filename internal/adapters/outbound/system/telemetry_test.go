package system

import (
	"context"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestTelemetryCollector_SensorsParsing(t *testing.T) {
	collector := NewTelemetryCollector()

	// 1. Live collect test
	telem, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if telem == nil {
		t.Fatalf("expected non-nil telemetry")
	}

	t.Logf("Sensors: lm_sensors_installed=%v, power_meter=%v, temps_count=%d, fans_count=%d, gpus_count=%d",
		telem.Sensors.LmSensorsInstalled,
		telem.Sensors.PowerMeter != nil,
		len(telem.Sensors.Temperatures),
		len(telem.Sensors.Fans),
		len(telem.Sensors.Gpus),
	)

	// 2. Mock JSON parsing test (including HP power meter, coretemp, tg3, ixgbe)
	mockJSON := `{
		"tg3-pci-0200": {
			"Adapter": "PCI adapter",
			"temp1": {
				"temp1_input": 52.0,
				"temp1_max": 100.0,
				"temp1_crit": 110.0
			}
		},
		"coretemp-isa-0000": {
			"Adapter": "ISA adapter",
			"Package id 0": {
				"temp1_input": 35.0,
				"temp1_max": 93.0,
				"temp1_crit": 103.0
			},
			"Core 0": {
				"temp2_input": 30.0,
				"temp2_max": 93.0
			},
			"Core 1": {
				"temp3_input": 28.0,
				"temp3_max": 93.0
			}
		},
		"ixgbe-pci-0401": {
			"Adapter": "PCI adapter",
			"loc2": {
				"temp1_input": 33.0,
				"temp1_max": 105.0
			}
		},
		"power_meter-acpi-0": {
			"Adapter": "ACPI interface",
			"power1": {
				"power1_average": 165.0,
				"power1_average_interval": 300.0
			}
		},
		"it8696-isa-0a40": {
			"Adapter": "ISA adapter",
			"fan1": {
				"fan1_input": 1073.0,
				"fan1_min": 0.0
			}
		}
	}`

	var res domain.HardwareSensorsTelemetry
	res.Temperatures = make([]domain.HardwareSensorItem, 0)
	res.Fans = make([]domain.HardwareSensorItem, 0)
	res.Gpus = make([]domain.GpuTelemetry, 0)

	// Inject and test parsing
	collector.parseLmSensorsJSON([]byte(mockJSON), &res)

	// Verify Power Meter
	if res.PowerMeter == nil {
		t.Fatalf("expected power meter to be parsed")
	}
	if res.PowerMeter.PowerWatts != 165.0 {
		t.Errorf("expected 165.0W, got %f", res.PowerMeter.PowerWatts)
	}
	if res.PowerMeter.IntervalSeconds != 300.0 {
		t.Errorf("expected 300.0s interval, got %f", res.PowerMeter.IntervalSeconds)
	}
	if res.PowerMeter.DeviceName != "power_meter-acpi-0" {
		t.Errorf("expected power_meter-acpi-0, got %s", res.PowerMeter.DeviceName)
	}

	// Verify Temperatures
	foundTg3 := false
	foundIxgbe := false
	foundPackage := false
	for _, item := range res.Temperatures {
		if item.Chip == "tg3-pci-0200" && item.Category == "PCI" && item.CurrentValue == 52.0 {
			foundTg3 = true
		}
		if item.Chip == "ixgbe-pci-0401" && item.Category == "PCI" && item.CurrentValue == 33.0 {
			foundIxgbe = true
		}
		if item.Chip == "coretemp-isa-0000" && item.Category == "CPU" && item.CurrentValue == 35.0 {
			foundPackage = true
			if item.Label != "CPU 0" {
				t.Errorf("expected CPU 0 label, got %s", item.Label)
			}
			if item.AverageValue != 29.0 {
				t.Errorf("expected average 29.0, got %f", item.AverageValue)
			}
			if len(item.Cores) != 2 {
				t.Fatalf("expected 2 cores, got %d", len(item.Cores))
			}
			if item.Cores[0].Label != "Core 0" || item.Cores[0].CurrentValue != 30.0 {
				t.Errorf("expected Core 0 with 30.0C, got %+v", item.Cores[0])
			}
			if item.Cores[1].Label != "Core 1" || item.Cores[1].CurrentValue != 28.0 {
				t.Errorf("expected Core 1 with 28.0C, got %+v", item.Cores[1])
			}
		}
	}
	if !foundTg3 {
		t.Errorf("expected tg3 PCI temperature to be parsed")
	}
	if !foundIxgbe {
		t.Errorf("expected ixgbe PCI temperature to be parsed")
	}
	if !foundPackage {
		t.Errorf("expected coretemp Package id 0 temperature to be parsed")
	}

	// Verify Fans
	if len(res.Fans) != 1 || res.Fans[0].CurrentValue != 1073.0 {
		t.Errorf("expected 1 fan with 1073 RPM, got %+v", res.Fans)
	}
}

func TestCpuInfo_MultiSocketPhysicalCores(t *testing.T) {
	// Simulate /proc/cpuinfo with 2 sockets, each with 20 cores (total 40 physical, 80 logical)
	mockCpuInfo := ""
	for socket := 0; socket < 2; socket++ {
		for core := 0; core < 20; core++ {
			for thread := 0; thread < 2; thread++ {
				procNum := socket*40 + core*2 + thread
				mockCpuInfo += "processor\t: " + string(rune('0'+procNum)) + "\n"
				mockCpuInfo += "model name\t: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\n"
				mockCpuInfo += "physical id\t: " + string(rune('0'+socket)) + "\n"
				mockCpuInfo += "core id\t\t: " + string(rune('0'+core)) + "\n\n"
			}
		}
	}

	physicalMap := make(map[string]bool)
	logicalCount := 0
	currPhysicalId := "0"

	for _, line := range strings.Split(mockCpuInfo, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "processor") {
			logicalCount++
		} else if strings.HasPrefix(line, "physical id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currPhysicalId = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "core id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				coreId := strings.TrimSpace(parts[1])
				key := currPhysicalId + ":" + coreId
				physicalMap[key] = true
			}
		}
	}

	if logicalCount != 80 {
		t.Errorf("expected 80 logical threads, got %d", logicalCount)
	}
	if len(physicalMap) != 40 {
		t.Errorf("expected 40 physical cores across 2 sockets, got %d", len(physicalMap))
	}
}

func TestParseProcDiskstats(t *testing.T) {
	sample := `   8       0 sda 372 0 100 147 50 0 200 10 2 81 147 0 0 0 0 0 0
   8       1 sda1 121 0 50 66 0 0 0 0 0 50 66 0 0 0 0 0 0
 259       0 nvme0n1 1000 10 4096 500 2000 20 8192 600 1 900 1100 0 0 0 0 0 0
`
	stats := parseProcDiskstats(sample, func(name string) bool {
		return name == "sda" || name == "nvme0n1"
	})
	if len(stats) != 2 {
		t.Fatalf("expected 2 whole disk entries, got %d", len(stats))
	}
	if stats[0].name != "sda" || stats[0].reads != 372 || stats[0].readBytes != 100*512 || stats[0].writes != 50 || stats[0].writeBytes != 200*512 || stats[0].ioInProgress != 2 || stats[0].ioTimeMs != 81 {
		t.Errorf("unexpected sda stats: %+v", stats[0])
	}
	if stats[1].name != "nvme0n1" || stats[1].reads != 1000 || stats[1].readBytes != 4096*512 || stats[1].writes != 2000 || stats[1].writeBytes != 8192*512 || stats[1].ioInProgress != 1 || stats[1].ioTimeMs != 900 {
		t.Errorf("unexpected nvme0n1 stats: %+v", stats[1])
	}
}

func TestParseZpoolKstatIO(t *testing.T) {
	sample := `31 1 0x01 1 192 524288 123456789
nread    nwritten reads    writes   wtime    wlentime wupdate  rtime    rlentime rupdate  wcnt     rcnt
1048576  2097152  128      256      1000     2000     3000     4000     5000     6000     3        7
`
	stat, ok := parseZpoolKstatIO("tank", sample)
	if !ok {
		t.Fatalf("expected zpool kstat io to parse")
	}
	if stat.poolName != "tank" || stat.nread != 1048576 || stat.nwritten != 2097152 || stat.reads != 128 || stat.writes != 256 || stat.wcnt != 3 || stat.rcnt != 7 {
		t.Errorf("unexpected zpool io stat: %+v", stat)
	}
}

func TestParseZpoolKstatIOStats(t *testing.T) {
	sample := `428 1 0x01 26 7072 6349198886 676910813623990
name                            type data
arc_read_count                  4    1946460
arc_read_bytes                  4    37815981056
arc_write_count                 4    9984639
arc_write_bytes                 4    75464344979
direct_read_count               4    10
direct_read_bytes               4    40960
direct_write_count              4    20
direct_write_bytes              4    81920
`
	stat, ok := parseZpoolKstatIOStats("fast", sample)
	if !ok {
		t.Fatalf("expected zpool kstat iostats to parse")
	}
	if stat.poolName != "fast" ||
		stat.nread != 37815981056+40960 ||
		stat.nwritten != 75464344979+81920 ||
		stat.reads != 1946460+10 ||
		stat.writes != 9984639+20 {
		t.Errorf("unexpected zpool iostats: %+v", stat)
	}
}
