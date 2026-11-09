package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

const (
	// maxPowerAverageIntervalMs is the longest averaging window we accept for
	// ACPI power meters. Firmware often defaults to 300s, which makes the
	// reported wattage far too sluggish to be useful in the UI.
	maxPowerAverageIntervalMs = 5000

	// dcmiPowerPollInterval is the caching window for `ipmitool dcmi power reading`
	// so instantaneous BMC power readings refresh every 5 seconds.
	dcmiPowerPollInterval = 5 * time.Second

	// powerIntervalRetryPeriod rate-limits sysfs/IPMI probing when no power meter
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

func hasLocalIpmiDevice() bool {
	for _, dev := range []string{"/dev/ipmi0", "/dev/ipmi/0", "/dev/ipmidev/0"} {
		if _, err := os.Stat(dev); err == nil {
			return true
		}
	}
	return false
}

func findIpmitoolBinary() string {
	if bin, err := exec.LookPath("ipmitool"); err == nil && bin != "" {
		return bin
	}
	for _, p := range []string{"/usr/bin/ipmitool", "/bin/ipmitool", "/usr/local/bin/ipmitool"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// collectDcmiPowerMeter reads instantaneous server power via `ipmitool dcmi power reading`,
// caching the result for dcmiPowerPollInterval (5s). Must be called with c.mu held.
func (c *TelemetryCollector) collectDcmiPowerMeter(ctx context.Context, res *domain.HardwareSensorsTelemetry) {
	if c.lastDcmiPower != nil && time.Since(c.lastDcmiPowerTime) < dcmiPowerPollInterval {
		applyPowerMeterReading(res, *c.lastDcmiPower)
		return
	}
	if !c.dcmiUnavailableAt.IsZero() && time.Since(c.dcmiUnavailableAt) < powerIntervalRetryPeriod {
		if c.lastDcmiPower != nil {
			applyPowerMeterReading(res, *c.lastDcmiPower)
		}
		return
	}

	if !hasLocalIpmiDevice() {
		c.dcmiUnavailableAt = time.Now()
		return
	}
	ipmitoolBin := findIpmitoolBinary()
	if ipmitoolBin == "" {
		c.dcmiUnavailableAt = time.Now()
		return
	}

	tctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(tctx, ipmitoolBin, "dcmi", "power", "reading").CombinedOutput()
	if err != nil && len(out) == 0 {
		c.dcmiUnavailableAt = time.Now()
		if c.lastDcmiPower != nil {
			applyPowerMeterReading(res, *c.lastDcmiPower)
		}
		return
	}

	pm, ok := parseDcmiPowerReading(string(out))
	if !ok {
		c.dcmiUnavailableAt = time.Now()
		if c.lastDcmiPower != nil {
			applyPowerMeterReading(res, *c.lastDcmiPower)
		}
		return
	}

	c.lastDcmiPower = pm
	c.lastDcmiPowerTime = time.Now()
	c.dcmiUnavailableAt = time.Time{}
	applyPowerMeterReading(res, *pm)
}

func applyPowerMeterReading(res *domain.HardwareSensorsTelemetry, pm domain.PowerMeterTelemetry) {
	cp := pm
	res.PowerMeter = &cp

	replaced := false
	for i := range res.Temperatures {
		if res.Temperatures[i].Category == "POWER" {
			res.Temperatures[i] = domain.HardwareSensorItem{
				Chip:         pm.DeviceName,
				Adapter:      pm.Adapter,
				Label:        fmt.Sprintf("Instantaneous Power (%s)", pm.DeviceName),
				Category:     "POWER",
				CurrentValue: pm.PowerWatts,
				Unit:         "W",
			}
			replaced = true
			break
		}
	}
	if !replaced {
		res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
			Chip:         pm.DeviceName,
			Adapter:      pm.Adapter,
			Label:        fmt.Sprintf("Instantaneous Power (%s)", pm.DeviceName),
			Category:     "POWER",
			CurrentValue: pm.PowerWatts,
			Unit:         "W",
		})
	}
}

// parseDcmiPowerReading parses the stdout of `ipmitool dcmi power reading` and
// extracts the instantaneous power reading in Watts.
func parseDcmiPowerReading(output string) (*domain.PowerMeterTelemetry, bool) {
	var instWatts float64
	var hasInst bool
	activated := true

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "instantaneous power reading:") {
			valPart := strings.TrimSpace(line[len("Instantaneous power reading:"):])
			fields := strings.Fields(valPart)
			if len(fields) > 0 {
				if v, err := strconv.ParseFloat(fields[0], 64); err == nil && v > 0 {
					instWatts = v
					hasInst = true
				}
			}
		} else if strings.HasPrefix(lower, "power reading state is:") {
			statePart := strings.TrimSpace(lower[len("power reading state is:"):])
			if statePart != "" && !strings.HasPrefix(statePart, "activated") {
				activated = false
			}
		}
	}

	if !hasInst || !activated || instWatts <= 0 {
		return nil, false
	}

	return &domain.PowerMeterTelemetry{
		DeviceName:      "ipmi-dcmi",
		Adapter:         "IPMI DCMI",
		PowerWatts:      instWatts,
		IntervalSeconds: dcmiPowerPollInterval.Seconds(),
	}, true
}
