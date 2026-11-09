package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func writeHwmon(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readTrim(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestConfigurePowerMeterIntervals(t *testing.T) {
	root := t.TempDir()
	meter := filepath.Join(root, "hwmon3")
	other := filepath.Join(root, "hwmon0")
	writeHwmon(t, meter, map[string]string{
		"name":                        "power_meter",
		"power1_average":              "152000000",
		"power1_average_interval":     "300000",
		"power1_average_interval_min": "1000",
		"power1_average_interval_max": "600000",
	})
	writeHwmon(t, other, map[string]string{
		"name":                    "nvme",
		"power1_average_interval": "300000",
	})

	if !configurePowerMeterIntervals(root, 5000) {
		t.Fatal("expected power meter to be configured")
	}
	if got := readTrim(t, filepath.Join(meter, "power1_average_interval")); got != "5000" {
		t.Errorf("power meter interval = %s, want 5000", got)
	}
	if got := readTrim(t, filepath.Join(other, "power1_average_interval")); got != "300000" {
		t.Errorf("non power_meter device modified: %s", got)
	}
}

func TestConfigurePowerMeterIntervals_RespectsMinBound(t *testing.T) {
	root := t.TempDir()
	meter := filepath.Join(root, "hwmon1")
	writeHwmon(t, meter, map[string]string{
		"name":                        "power_meter",
		"power1_average_interval":     "300000",
		"power1_average_interval_min": "10000",
	})

	if !configurePowerMeterIntervals(root, 5000) {
		t.Fatal("expected power meter to be configured")
	}
	if got := readTrim(t, filepath.Join(meter, "power1_average_interval")); got != "10000" {
		t.Errorf("interval = %s, want 10000 (hardware minimum)", got)
	}
}

func TestConfigurePowerMeterIntervals_AlreadyShort(t *testing.T) {
	root := t.TempDir()
	meter := filepath.Join(root, "hwmon1")
	writeHwmon(t, meter, map[string]string{
		"name":                    "power_meter",
		"power1_average_interval": "1000",
	})

	if !configurePowerMeterIntervals(root, 5000) {
		t.Fatal("expected already-short interval to count as configured")
	}
	if got := readTrim(t, filepath.Join(meter, "power1_average_interval")); got != "1000" {
		t.Errorf("interval changed to %s, want 1000", got)
	}
}

func TestConfigurePowerMeterIntervals_NoMeter(t *testing.T) {
	if configurePowerMeterIntervals(t.TempDir(), 5000) {
		t.Fatal("expected false with no power meter present")
	}
}

func TestParseDcmiPowerReading(t *testing.T) {
	raw := `
    Instantaneous power reading:                   174 Watts
    Minimum during sampling period:                174 Watts
    Maximum during sampling period:                236 Watts
    Average power reading over sample period:      181 Watts
    IPMI timestamp:                           11/08/2026 11:48:36 PM EET    Sampling period:                          00000300 Seconds.
    Power reading state is:                   activated
`
	pm, ok := parseDcmiPowerReading(raw)
	if !ok || pm == nil {
		t.Fatal("expected DCMI power reading to parse")
	}
	if pm.PowerWatts != 174 {
		t.Errorf("PowerWatts = %v, want 174", pm.PowerWatts)
	}
	if pm.DeviceName != "ipmi-dcmi" {
		t.Errorf("DeviceName = %q, want ipmi-dcmi", pm.DeviceName)
	}
	if pm.Adapter != "IPMI DCMI" {
		t.Errorf("Adapter = %q, want IPMI DCMI", pm.Adapter)
	}
	if pm.IntervalSeconds != 5 {
		t.Errorf("IntervalSeconds = %v, want 5", pm.IntervalSeconds)
	}

	// Verify applyPowerMeterReading overrides slow 300s ACPI reading
	sensors := domain.HardwareSensorsTelemetry{
		PowerMeter: &domain.PowerMeterTelemetry{
			DeviceName:      "power_meter-acpi-0",
			Adapter:         "ACPI interface",
			PowerWatts:      181,
			IntervalSeconds: 300,
		},
		Temperatures: []domain.HardwareSensorItem{
			{
				Chip:         "power_meter-acpi-0",
				Adapter:      "ACPI interface",
				Label:        "power1 (power_meter-acpi-0)",
				Category:     "POWER",
				CurrentValue: 181,
				Unit:         "W",
			},
		},
	}
	applyPowerMeterReading(&sensors, *pm)
	if sensors.PowerMeter.PowerWatts != 174 || sensors.PowerMeter.DeviceName != "ipmi-dcmi" {
		t.Errorf("expected PowerMeter to be overridden by IPMI DCMI, got %+v", sensors.PowerMeter)
	}
	if len(sensors.Temperatures) != 1 || sensors.Temperatures[0].CurrentValue != 174 {
		t.Errorf("expected POWER sensor item to be updated to 174W, got %+v", sensors.Temperatures)
	}
}

func TestParseDcmiPowerReading_Deactivated(t *testing.T) {
	raw := `
    Instantaneous power reading:                   174 Watts
    Power reading state is:                   deactivated
`
	if _, ok := parseDcmiPowerReading(raw); ok {
		t.Fatal("expected deactivated DCMI power reading to be ignored")
	}
}
