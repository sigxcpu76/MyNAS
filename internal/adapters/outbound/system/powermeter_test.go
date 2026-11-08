package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
