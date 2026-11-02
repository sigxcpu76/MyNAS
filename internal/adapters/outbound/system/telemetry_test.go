package system

import (
	"context"
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
