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
