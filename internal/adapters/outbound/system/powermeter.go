package system

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// maxPowerAverageIntervalMs is the longest averaging window we accept for
	// ACPI power meters. Firmware often defaults to 300s, which makes the
	// reported wattage far too sluggish to be useful in the UI.
	maxPowerAverageIntervalMs = 5000

	// powerIntervalRetryPeriod rate-limits sysfs probing when no power meter
	// has been configured yet (e.g. the acpi_power_meter module loads late).
	powerIntervalRetryPeriod = time.Minute
)

// ensurePowerMeterInterval caps the averaging interval of every ACPI
// power_meter hwmon device to at most maxPowerAverageIntervalMs. It is cheap
// to call on every collection: once a meter has been configured it is a no-op.
// Must be called with c.mu held.
func (c *TelemetryCollector) ensurePowerMeterInterval() {
	if c.powerIntervalSet || time.Since(c.powerIntervalTry) < powerIntervalRetryPeriod {
		return
	}
	c.powerIntervalTry = time.Now()
	if configurePowerMeterIntervals("/sys/class/hwmon", maxPowerAverageIntervalMs) {
		c.powerIntervalSet = true
	}
}

// configurePowerMeterIntervals walks hwmonRoot for power_meter devices and
// lowers power*_average_interval (milliseconds) to at most maxMs, honouring
// the device's power*_average_interval_min/max bounds. Returns true if at
// least one power meter now reports an interval <= maxMs (or the closest
// value the hardware allows).
func configurePowerMeterIntervals(hwmonRoot string, maxMs int64) bool {
	devices, _ := filepath.Glob(filepath.Join(hwmonRoot, "hwmon*"))
	configured := false
	for _, dev := range devices {
		name, err := os.ReadFile(filepath.Join(dev, "name"))
		if err != nil || strings.TrimSpace(string(name)) != "power_meter" {
			continue
		}

		intervals, _ := filepath.Glob(filepath.Join(dev, "power*_average_interval"))
		for _, intervalPath := range intervals {
			cur, ok := readSysfsInt(intervalPath)
			if !ok {
				continue
			}

			target := maxMs
			if lo, ok := readSysfsInt(intervalPath + "_min"); ok && lo > target {
				target = lo
			}
			if hi, ok := readSysfsInt(intervalPath + "_max"); ok && hi > 0 && hi < target {
				target = hi
			}

			if cur <= target {
				configured = true
				continue
			}

			if err := os.WriteFile(intervalPath, []byte(strconv.FormatInt(target, 10)), 0o644); err != nil {
				continue
			}
			if v, ok := readSysfsInt(intervalPath); ok && v <= target {
				configured = true
			}
		}
	}
	return configured
}

func readSysfsInt(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
