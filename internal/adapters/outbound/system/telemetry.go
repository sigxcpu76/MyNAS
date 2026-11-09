package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type cpuSample struct {
	total uint64
	idle  uint64
	time  time.Time
}

type netSample struct {
	rxBytes uint64
	txBytes uint64
	time    time.Time
}

type TelemetryCollector struct {
	mu          sync.Mutex
	lastCpu     *cpuSample
	lastNet     map[string]netSample
	cachedGpus  []string
	gpuScanTime time.Time

	powerIntervalSet  bool
	powerIntervalTry  time.Time
	lastDcmiPower     *domain.PowerMeterTelemetry
	lastDcmiPowerTime time.Time
	dcmiUnavailableAt time.Time
}

func NewTelemetryCollector() *TelemetryCollector {
	return &TelemetryCollector{
		lastNet: make(map[string]netSample),
	}
}

func (c *TelemetryCollector) Collect(ctx context.Context) (*domain.SystemTelemetry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cpu := c.collectCPU()
	mem := c.collectMemory()
	host := c.collectHost(ctx)
	netIfaces := c.collectNetwork()
	sensors := c.collectSensors(ctx)

	c.enrichCpuTemperatures(&cpu, &sensors)

	return &domain.SystemTelemetry{
		Cpu:     cpu,
		Memory:  mem,
		Host:    host,
		Network: netIfaces,
		Sensors: sensors,
	}, nil
}

func (c *TelemetryCollector) collectCPU() domain.CpuTelemetry {
	res := domain.CpuTelemetry{}

	// 1. Model & core counts from /proc/cpuinfo
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		scanner := bufio.NewScanner(f)
		physicalMap := make(map[string]bool)
		logicalCount := 0
		var model string
		currPhysicalId := "0"

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && model == "" {
					model = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "processor") {
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
					// Unique key is physical socket + core id to properly count cores across multi-socket systems
					key := fmt.Sprintf("%s:%s", currPhysicalId, coreId)
					physicalMap[key] = true
				}
			}
		}
		_ = f.Close()

		res.Model = model
		res.LogicalCores = int32(logicalCount)
		if len(physicalMap) > 0 {
			res.PhysicalCores = int32(len(physicalMap))
		} else {
			res.PhysicalCores = res.LogicalCores
		}
	}

	if res.LogicalCores == 0 {
		res.LogicalCores = int32(runtime.NumCPU())
		res.PhysicalCores = res.LogicalCores
	}

	// 2. Load average from /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			res.Load1, _ = strconv.ParseFloat(fields[0], 64)
			res.Load5, _ = strconv.ParseFloat(fields[1], 64)
			res.Load15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// 3. CPU usage percentage from /proc/stat
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					var total uint64
					var idle uint64
					for i := 1; i < len(fields); i++ {
						val, _ := strconv.ParseUint(fields[i], 10, 64)
						total += val
						if i == 4 { // idle is the 4th metric (index 4 in fields)
							idle = val
						}
					}

					now := time.Now()
					if c.lastCpu != nil && total > c.lastCpu.total {
						deltaTotal := total - c.lastCpu.total
						deltaIdle := idle - c.lastCpu.idle
						if deltaTotal > 0 && deltaTotal >= deltaIdle {
							res.UsagePercent = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100.0
						}
					}
					c.lastCpu = &cpuSample{total: total, idle: idle, time: now}
				}
				break
			}
		}
	}

	// 4. CPU temperature
	res.TemperatureCelsius, res.PackageTemperatures, res.CoreTemperatures = readCPUTemperatures()

	return res
}

func readCPUTemperatures() (float64, []float64, []float64) {
	var coreTemps []float64
	var packageTemp float64
	pkgMap := make(map[int]float64)
	var fallbackPkgTemps []float64

	// Check hwmon directory
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hwmon := range matches {
		nameData, err := os.ReadFile(filepath.Join(hwmon, "name"))
		if err != nil {
			continue
		}
		sensorName := strings.TrimSpace(string(nameData))
		isCPU := strings.Contains(sensorName, "coretemp") ||
			strings.Contains(sensorName, "k10temp") ||
			strings.Contains(sensorName, "cpu") ||
			strings.Contains(sensorName, "zenpower")

		if !isCPU {
			continue
		}

		// Read temp*_input files
		tempInputs, _ := filepath.Glob(filepath.Join(hwmon, "temp*_input"))
		var hwmonPrimaryTemp float64
		foundPkgInHwmon := false

		for _, tf := range tempInputs {
			valBytes, err := os.ReadFile(tf)
			if err != nil {
				continue
			}
			millidegrees, err := strconv.ParseFloat(strings.TrimSpace(string(valBytes)), 64)
			if err != nil {
				continue
			}
			deg := millidegrees / 1000.0
			if deg <= 0 || deg >= 150 {
				continue
			}

			// Check corresponding label file, e.g. temp1_label for temp1_input
			base := strings.TrimSuffix(tf, "_input")
			labelBytes, _ := os.ReadFile(base + "_label")
			label := strings.TrimSpace(string(labelBytes))
			labelLower := strings.ToLower(label)

			if strings.Contains(labelLower, "package") {
				// e.g. "Package id 0", "Package id 1"
				pkgIdx := len(pkgMap)
				for _, part := range strings.Fields(label) {
					if n, err := strconv.Atoi(part); err == nil && n >= 0 && n < 32 {
						pkgIdx = n
						break
					}
				}
				pkgMap[pkgIdx] = deg
				foundPkgInHwmon = true
				if packageTemp == 0 || deg > packageTemp {
					packageTemp = deg
				}
			} else if strings.Contains(labelLower, "tdie") || strings.Contains(labelLower, "tctl") {
				if hwmonPrimaryTemp == 0 {
					hwmonPrimaryTemp = deg
				}
				if packageTemp == 0 || deg > packageTemp {
					packageTemp = deg
				}
			} else {
				if strings.Contains(labelLower, "core") {
					coreTemps = append(coreTemps, deg)
				}
				if hwmonPrimaryTemp == 0 {
					hwmonPrimaryTemp = deg
				}
			}
		}

		if !foundPkgInHwmon && hwmonPrimaryTemp > 0 {
			fallbackPkgTemps = append(fallbackPkgTemps, hwmonPrimaryTemp)
			if packageTemp == 0 || hwmonPrimaryTemp > packageTemp {
				packageTemp = hwmonPrimaryTemp
			}
		}
	}

	var packageTemps []float64
	if len(pkgMap) > 0 {
		maxID := -1
		for id := range pkgMap {
			if id > maxID {
				maxID = id
			}
		}
		for i := 0; i <= maxID; i++ {
			if temp, ok := pkgMap[i]; ok {
				packageTemps = append(packageTemps, temp)
			}
		}
		if len(packageTemps) < len(pkgMap) {
			packageTemps = nil
			var keys []int
			for k := range pkgMap {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				packageTemps = append(packageTemps, pkgMap[k])
			}
		}
	} else if len(fallbackPkgTemps) > 0 {
		packageTemps = fallbackPkgTemps
	}

	// Fallback to /sys/class/thermal/thermal_zone*
	if packageTemp == 0 {
		thermalZones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
		for _, tz := range thermalZones {
			if valBytes, err := os.ReadFile(filepath.Join(tz, "temp")); err == nil {
				if millidegrees, err := strconv.ParseFloat(strings.TrimSpace(string(valBytes)), 64); err == nil {
					deg := millidegrees / 1000.0
					if deg > 0 && deg < 150 {
						packageTemp = deg
						coreTemps = append(coreTemps, deg)
						packageTemps = append(packageTemps, deg)
						break
					}
				}
			}
		}
	}

	if len(packageTemps) == 0 && packageTemp > 0 {
		packageTemps = []float64{packageTemp}
	}

	return packageTemp, packageTemps, coreTemps
}

func (c *TelemetryCollector) collectMemory() domain.MemoryTelemetry {
	res := domain.MemoryTelemetry{}

	if f, err := os.Open("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				key := strings.TrimSuffix(parts[0], ":")
				valKb, _ := strconv.ParseInt(parts[1], 10, 64)
				bytesVal := valKb * 1024

				switch key {
				case "MemTotal":
					res.TotalBytes = bytesVal
				case "MemFree":
					res.FreeBytes = bytesVal
				case "MemAvailable":
					res.AvailableBytes = bytesVal
				case "Buffers":
					res.BuffersBytes = bytesVal
				case "Cached":
					res.CachedBytes = bytesVal
				}
			}
		}
		_ = f.Close()
	}

	// Read ZFS ARC stats if available
	if arcData, err := os.ReadFile("/proc/spl/kstat/zfs/arcstats"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(arcData)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 3 {
				if fields[0] == "size" {
					res.ZfsArcBytes, _ = strconv.ParseInt(fields[2], 10, 64)
				} else if fields[0] == "c_max" {
					res.ZfsArcMaxBytes, _ = strconv.ParseInt(fields[2], 10, 64)
				}
			}
		}
	}

	if res.AvailableBytes > 0 {
		res.UsedBytes = res.TotalBytes - res.AvailableBytes
	} else {
		res.UsedBytes = res.TotalBytes - res.FreeBytes - res.BuffersBytes - res.CachedBytes
	}
	if res.UsedBytes < 0 {
		res.UsedBytes = 0
	}

	// Services memory = Total Used memory minus ZFS ARC cache
	if res.UsedBytes > res.ZfsArcBytes {
		res.ServicesBytes = res.UsedBytes - res.ZfsArcBytes
	} else {
		res.ServicesBytes = res.UsedBytes
	}

	return res
}

func (c *TelemetryCollector) collectHost(ctx context.Context) domain.HostHardwareInfo {
	res := domain.HostHardwareInfo{
		Architecture: runtime.GOARCH,
	}

	if h, err := os.Hostname(); err == nil {
		res.Hostname = h
	}

	// 1. OS & Kernel
	if osData, err := os.ReadFile("/etc/os-release"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(osData)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				res.OsName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			} else if strings.HasPrefix(line, "VERSION_ID=") && res.OsVersion == "" {
				res.OsVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}
	if res.OsName == "" {
		res.OsName = fmt.Sprintf("Linux (%s)", runtime.GOOS)
	}

	if kData, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		res.KernelVersion = strings.TrimSpace(string(kData))
	}

	// 2. Uptime
	if upData, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(upData))
		if len(fields) > 0 {
			if upSecs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				res.UptimeSeconds = int64(upSecs)
			}
		}
	}

	// 3. Motherboard & BIOS from /sys/class/dmi/id
	readDMI := func(filename string) string {
		p := filepath.Join("/sys/class/dmi/id", filename)
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
		return ""
	}

	res.MotherboardVendor = readDMI("board_vendor")
	if res.MotherboardVendor == "" {
		res.MotherboardVendor = readDMI("sys_vendor")
	}
	res.MotherboardName = readDMI("board_name")
	if res.MotherboardName == "" {
		res.MotherboardName = readDMI("product_name")
	}
	res.MotherboardVersion = readDMI("board_version")
	if res.MotherboardVersion == "" {
		res.MotherboardVersion = readDMI("product_version")
	}

	res.BiosVendor = readDMI("bios_vendor")
	res.BiosVersion = readDMI("bios_version")
	res.BiosDate = readDMI("bios_date")

	// 4. GPUs (cache for 10 minutes since hardware does not change frequently)
	if len(c.cachedGpus) == 0 || time.Since(c.gpuScanTime) > 10*time.Minute {
		c.cachedGpus = scanGPUs(ctx)
		c.gpuScanTime = time.Now()
	}
	res.Gpus = c.cachedGpus

	return res
}

func scanGPUs(ctx context.Context) []string {
	var gpus []string

	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 1. Try nvidia-smi for precise NVIDIA GPU and VRAM info if available
	ncmd := exec.CommandContext(tctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader")
	if nout, err := ncmd.Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(nout)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				gpus = append(gpus, line)
			}
		}
		if len(gpus) > 0 {
			return gpus
		}
	}

	// 2. Fall back to lspci scanning
	cmd := exec.CommandContext(tctx, "lspci")
	if out, err := cmd.Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			lower := strings.ToLower(line)
			if strings.Contains(lower, "vga compatible controller") ||
				strings.Contains(lower, "3d controller") ||
				strings.Contains(lower, "display controller") {
				// Strip PCI address prefix like "01:00.0 "
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) == 2 {
					gpus = append(gpus, strings.TrimSpace(parts[1]))
				} else {
					gpus = append(gpus, strings.TrimSpace(line))
				}
			}
		}
	}

	return gpus
}

func (c *TelemetryCollector) collectNetwork() []domain.NetworkInterfaceTelemetry {
	netDevStats := readProcNetDev()
	defaultIface := getDefaultRouteInterface()
	now := time.Now()

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var results []domain.NetworkInterfaceTelemetry

	for _, ifi := range interfaces {
		// Skip loopback interfaces if they have no IP or are lo
		if ifi.Name == "lo" {
			continue
		}

		var ips []string
		if addrs, err := ifi.Addrs(); err == nil {
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if ok && ipNet.IP != nil {
					ips = append(ips, ipNet.IP.String())
				}
			}
		}

		isUp := (ifi.Flags & net.FlagUp) != 0
		stats := netDevStats[ifi.Name]

		var rxRate int64
		var txRate int64

		if prev, ok := c.lastNet[ifi.Name]; ok && now.After(prev.time) {
			elapsedSec := now.Sub(prev.time).Seconds()
			if elapsedSec > 0.05 {
				if stats.rx >= prev.rxBytes {
					rxRate = int64(float64(stats.rx-prev.rxBytes) / elapsedSec)
				}
				if stats.tx >= prev.txBytes {
					txRate = int64(float64(stats.tx-prev.txBytes) / elapsedSec)
				}
			}
		}

		c.lastNet[ifi.Name] = netSample{
			rxBytes: stats.rx,
			txBytes: stats.tx,
			time:    now,
		}

		speed := readInterfaceSpeed(ifi.Name)
		_, bridgeErr := os.Stat(filepath.Join("/sys/class/net", ifi.Name, "bridge"))

		results = append(results, domain.NetworkInterfaceTelemetry{
			Name:              ifi.Name,
			IpAddresses:       ips,
			MacAddress:        ifi.HardwareAddr.String(),
			IsUp:              isUp,
			SpeedMbps:         speed,
			RxBytes:           int64(stats.rx),
			TxBytes:           int64(stats.tx),
			RxRateBytesPerSec: rxRate,
			TxRateBytesPerSec: txRate,
			IsDefault:         ifi.Name == defaultIface,
			IsBridge:          bridgeErr == nil,
		})
	}

	return results
}

func getDefaultRouteInterface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

func readInterfaceSpeed(name string) int64 {
	data, err := os.ReadFile(filepath.Join("/sys/class/net", name, "speed"))
	if err != nil {
		return 0
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

type devStats struct {
	rx uint64
	tx uint64
}

func readProcNetDev() map[string]devStats {
	m := make(map[string]devStats)
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return m
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) >= 9 {
			rx, _ := strconv.ParseUint(fields[0], 10, 64)
			tx, _ := strconv.ParseUint(fields[8], 10, 64)
			m[iface] = devStats{rx: rx, tx: tx}
		}
	}
	return m
}

func (c *TelemetryCollector) collectSensors(ctx context.Context) domain.HardwareSensorsTelemetry {
	res := domain.HardwareSensorsTelemetry{
		LmSensorsInstalled: false,
		Temperatures:       make([]domain.HardwareSensorItem, 0),
		Fans:               make([]domain.HardwareSensorItem, 0),
		Gpus:               make([]domain.GpuTelemetry, 0),
	}

	sensorsBin, err := exec.LookPath("sensors")
	if err != nil {
		if _, statErr := os.Stat("/usr/bin/sensors"); statErr == nil {
			sensorsBin = "/usr/bin/sensors"
		} else if _, statErr := os.Stat("/bin/sensors"); statErr == nil {
			sensorsBin = "/bin/sensors"
		}
	}

	// Cap ACPI power meter averaging window (firmware default is often 300s).
	c.ensurePowerMeterInterval()

	if sensorsBin != "" {
		res.LmSensorsInstalled = true
		c.parseLmSensorsOutput(ctx, sensorsBin, &res)
	}

	// If no power meter was found or the ACPI power meter is locked by firmware
	// to a slow averaging window (> 5s, e.g. 300s on HP servers), prefer the
	// instantaneous reading from `ipmitool dcmi power reading` (sampled every 5s).
	if res.PowerMeter == nil || res.PowerMeter.IntervalSeconds > float64(maxPowerAverageIntervalMs)/1000.0 {
		c.collectDcmiPowerMeter(ctx, &res)
	}

	// Always collect GPU telemetry (via nvidia-smi if available, plus amdgpu from sensors/sysfs)
	gpus := c.collectGpus(ctx)
	if len(gpus) > 0 {
		res.Gpus = append(res.Gpus, gpus...)
		// Also add GPU temperatures to Temperatures list if not already present
		for _, g := range gpus {
			if g.TemperatureCelsius > 0 {
				res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
					Chip:         fmt.Sprintf("gpu-%d", g.Index),
					Adapter:      "PCI adapter",
					Label:        fmt.Sprintf("%s (GPU %d)", g.Name, g.Index),
					Category:     "GPU",
					CurrentValue: g.TemperatureCelsius,
					Unit:         "°C",
				})
			}
		}
	}

	// Deterministically sort temperatures: CPU -> GPU -> PCI -> STORAGE -> MOTHERBOARD -> others
	categoryPriority := func(cat string) int {
		switch strings.ToUpper(cat) {
		case "CPU":
			return 1
		case "GPU":
			return 2
		case "PCI":
			return 3
		case "STORAGE":
			return 4
		case "MOTHERBOARD":
			return 5
		case "SYSTEM":
			return 6
		default:
			return 99
		}
	}

	sort.SliceStable(res.Temperatures, func(i, j int) bool {
		prioI := categoryPriority(res.Temperatures[i].Category)
		prioJ := categoryPriority(res.Temperatures[j].Category)
		if prioI != prioJ {
			return prioI < prioJ
		}
		return res.Temperatures[i].Label < res.Temperatures[j].Label
	})

	sort.SliceStable(res.Fans, func(i, j int) bool {
		return res.Fans[i].Label < res.Fans[j].Label
	})

	return res
}

func (c *TelemetryCollector) parseLmSensorsOutput(ctx context.Context, sensorsBin string, res *domain.HardwareSensorsTelemetry) {
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, sensorsBin, "-j")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return
	}

	c.parseLmSensorsJSON(out, res)
}

func (c *TelemetryCollector) parseLmSensorsJSON(out []byte, res *domain.HardwareSensorsTelemetry) {
	raw := string(out)
	idx := strings.Index(raw, "{")
	if idx < 0 {
		return
	}
	jsonStr := raw[idx:]

	var parsed map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return
	}

	for chipName, chipData := range parsed {
		adapter := ""
		if a, ok := chipData["Adapter"].(string); ok {
			adapter = a
		}

		lowerChip := strings.ToLower(chipName)
		defaultCat := "SYSTEM"
		if strings.Contains(lowerChip, "coretemp") || strings.Contains(lowerChip, "k10temp") ||
			strings.Contains(lowerChip, "zenpower") || strings.Contains(lowerChip, "cpu") {
			defaultCat = "CPU"
		} else if strings.Contains(lowerChip, "amdgpu") || strings.Contains(lowerChip, "nvidia") ||
			strings.Contains(lowerChip, "nouveau") || strings.Contains(lowerChip, "radeon") {
			defaultCat = "GPU"
		} else if strings.Contains(lowerChip, "nvme") || strings.Contains(lowerChip, "drivetemp") ||
			strings.Contains(lowerChip, "disk") || strings.Contains(lowerChip, "sata") {
			defaultCat = "STORAGE"
		} else if strings.Contains(strings.ToLower(adapter), "pci") || strings.Contains(lowerChip, "pci") {
			defaultCat = "PCI"
		} else if strings.Contains(lowerChip, "it86") || strings.Contains(lowerChip, "nct") ||
			strings.Contains(lowerChip, "w83") || strings.Contains(lowerChip, "superio") ||
			strings.Contains(lowerChip, "acpi") {
			defaultCat = "MOTHERBOARD"
		}

		type parsedTempFeat struct {
			name  string
			val   float64
			max   float64
			crit  float64
			alarm bool
			cat   string
		}
		var tempFeats []parsedTempFeat

		for featureName, featureVal := range chipData {
			if featureName == "Adapter" {
				continue
			}

			subMap, ok := featureVal.(map[string]interface{})
			if !ok {
				continue
			}

			// 1. Check for Power Meter (e.g. power_meter-acpi-0, power1: {power1_average: 152.0, power1_average_interval: 300.0})
			isPowerChip := strings.Contains(lowerChip, "power_meter") || strings.HasPrefix(strings.ToLower(featureName), "power")
			var powerAvg, powerInterval float64
			for subKey, subVal := range subMap {
				num, isNum := toFloat64(subVal)
				if !isNum {
					continue
				}
				if strings.Contains(subKey, "power") && (strings.HasSuffix(subKey, "_average") || strings.HasSuffix(subKey, "_input")) {
					powerAvg = num
				} else if strings.Contains(subKey, "average_interval") || strings.Contains(subKey, "interval") {
					powerInterval = num
				}
			}

			if isPowerChip && powerAvg > 0 {
				if res.PowerMeter == nil {
					res.PowerMeter = &domain.PowerMeterTelemetry{
						DeviceName:      chipName,
						Adapter:         adapter,
						PowerWatts:      powerAvg,
						IntervalSeconds: powerInterval,
					}
				}
				res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
					Chip:         chipName,
					Adapter:      adapter,
					Label:        fmt.Sprintf("%s (%s)", featureName, chipName),
					Category:     "POWER",
					CurrentValue: powerAvg,
					Unit:         "W",
				})
				continue
			}

			// 2. Check for Fan
			if strings.HasPrefix(strings.ToLower(featureName), "fan") {
				var fanRpm, fanMin, fanMax float64
				var fanAlarm bool
				for subKey, subVal := range subMap {
					num, isNum := toFloat64(subVal)
					if !isNum {
						continue
					}
					if strings.HasSuffix(subKey, "_input") {
						fanRpm = num
					} else if strings.HasSuffix(subKey, "_min") {
						fanMin = num
					} else if strings.HasSuffix(subKey, "_max") {
						fanMax = num
					} else if strings.HasSuffix(subKey, "_alarm") {
						fanAlarm = (num > 0)
					}
				}
				_ = fanMin
				res.Fans = append(res.Fans, domain.HardwareSensorItem{
					Chip:          chipName,
					Adapter:       adapter,
					Label:         fmt.Sprintf("%s (%s)", featureName, chipName),
					Category:      "FAN",
					CurrentValue:  fanRpm,
					Unit:          "RPM",
					HighThreshold: fanMax,
					Alarm:         fanAlarm,
				})
				continue
			}

			// 3. Collect Temperature feature
			var tempVal, tempMax, tempCrit float64
			var tempAlarm bool
			hasTemp := false

			for subKey, subVal := range subMap {
				num, isNum := toFloat64(subVal)
				if !isNum {
					continue
				}
				if strings.HasSuffix(subKey, "_input") && strings.HasPrefix(subKey, "temp") {
					tempVal = num
					hasTemp = true
				} else if strings.HasSuffix(subKey, "_max") || strings.HasSuffix(subKey, "_high") {
					tempMax = num
				} else if strings.HasSuffix(subKey, "_crit") {
					tempCrit = num
				} else if strings.HasSuffix(subKey, "_alarm") {
					tempAlarm = (num > 0)
				}
			}

			if hasTemp && tempVal > -50 && tempVal < 200 {
				cat := defaultCat
				lowerFeat := strings.ToLower(featureName)
				if strings.Contains(lowerFeat, "core") || strings.Contains(lowerFeat, "package") ||
					strings.Contains(lowerFeat, "tctl") || strings.Contains(lowerFeat, "tccd") {
					cat = "CPU"
				} else if strings.Contains(lowerFeat, "edge") || strings.Contains(lowerFeat, "junction") || strings.Contains(lowerFeat, "mem") && defaultCat == "GPU" {
					cat = "GPU"
				}

				tempFeats = append(tempFeats, parsedTempFeat{
					name:  featureName,
					val:   tempVal,
					max:   tempMax,
					crit:  tempCrit,
					alarm: tempAlarm,
					cat:   cat,
				})
			}
		}

		if len(tempFeats) > 0 {
			var coreFeats []parsedTempFeat
			var pkgFeat *parsedTempFeat
			var otherFeats []parsedTempFeat

			for _, f := range tempFeats {
				lowerName := strings.ToLower(f.name)
				if strings.HasPrefix(lowerName, "core") || (f.cat == "CPU" && strings.HasPrefix(lowerName, "tccd")) {
					coreFeats = append(coreFeats, f)
				} else if strings.Contains(lowerName, "package") || strings.EqualFold(f.name, "tdie") || strings.EqualFold(f.name, "tctl") {
					if pkgFeat == nil {
						pkgCopy := f
						pkgFeat = &pkgCopy
					} else {
						otherFeats = append(otherFeats, f)
					}
				} else {
					otherFeats = append(otherFeats, f)
				}
			}

			if len(coreFeats) > 0 {
				sort.Slice(coreFeats, func(i, j int) bool {
					idxI := extractCoreIndex(coreFeats[i].name)
					idxJ := extractCoreIndex(coreFeats[j].name)
					if idxI != idxJ {
						return idxI < idxJ
					}
					return coreFeats[i].name < coreFeats[j].name
				})

				var sum float64
				cores := make([]domain.CoreSensorItem, 0, len(coreFeats))
				for _, c := range coreFeats {
					sum += c.val
					cores = append(cores, domain.CoreSensorItem{
						Label:         c.name,
						CurrentValue:  c.val,
						HighThreshold: c.max,
						CritThreshold: c.crit,
					})
				}
				coreAvg := sum / float64(len(coreFeats))

				label := ""
				currentVal := coreAvg
				highThresh := coreFeats[0].max
				critThresh := coreFeats[0].crit
				alarm := false

				if pkgFeat != nil {
					label = pkgFeat.name
					// Rename "Package id X" to "CPU X", and "Package id" to "CPU"
					rePkg := regexp.MustCompile(`(?i)\bPackage\s*id\s*(\d+)`)
					if rePkg.MatchString(label) {
						label = rePkg.ReplaceAllString(label, "CPU $1")
					} else {
						rePkgBare := regexp.MustCompile(`(?i)\bPackage\s*id\b`)
						label = rePkgBare.ReplaceAllString(label, "CPU")
					}
					currentVal = pkgFeat.val
					highThresh = pkgFeat.max
					critThresh = pkgFeat.crit
					alarm = pkgFeat.alarm
				} else {
					// Fallback when no explicit package sensor exists
					reChipIdx := regexp.MustCompile(`\d+$`)
					if m := reChipIdx.FindString(chipName); m != "" {
						if n, err := strconv.Atoi(m); err == nil {
							label = fmt.Sprintf("CPU %d", n)
						} else {
							label = "CPU"
						}
					} else {
						label = "CPU"
					}
					for _, c := range coreFeats {
						if c.alarm {
							alarm = true
							break
						}
					}
				}

				if !strings.HasPrefix(label, "CPU") && !strings.Contains(label, chipName) {
					label = fmt.Sprintf("%s (%s)", label, chipName)
				}

				res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
					Chip:          chipName,
					Adapter:       adapter,
					Label:         label,
					Category:      "CPU",
					CurrentValue:  currentVal,
					AverageValue:  coreAvg,
					Unit:          "°C",
					HighThreshold: highThresh,
					CritThreshold: critThresh,
					Alarm:         alarm,
					Cores:         cores,
				})

				for _, of := range otherFeats {
					dispLabel := of.name
					if !strings.Contains(dispLabel, chipName) {
						dispLabel = fmt.Sprintf("%s: %s", chipName, of.name)
					}
					res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
						Chip:          chipName,
						Adapter:       adapter,
						Label:         dispLabel,
						Category:      of.cat,
						CurrentValue:  of.val,
						Unit:          "°C",
						HighThreshold: of.max,
						CritThreshold: of.crit,
						Alarm:         of.alarm,
					})
				}
			} else {
				for _, f := range tempFeats {
					displayLabel := f.name
					if !strings.Contains(displayLabel, chipName) {
						displayLabel = fmt.Sprintf("%s: %s", chipName, f.name)
					}
					res.Temperatures = append(res.Temperatures, domain.HardwareSensorItem{
						Chip:          chipName,
						Adapter:       adapter,
						Label:         displayLabel,
						Category:      f.cat,
						CurrentValue:  f.val,
						Unit:          "°C",
						HighThreshold: f.max,
						CritThreshold: f.crit,
						Alarm:         f.alarm,
					})
				}
			}
		}
	}
}

func (c *TelemetryCollector) collectGpus(ctx context.Context) []domain.GpuTelemetry {
	var results []domain.GpuTelemetry

	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, "nvidia-smi",
		"--query-gpu=index,name,driver_version,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,fan.speed",
		"--format=csv,noheader,nounits")
	if out, err := cmd.Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) >= 7 {
				idx, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				name := strings.TrimSpace(parts[1])
				driverVer := strings.TrimSpace(parts[2])
				temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
				util, _ := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
				memUsedMiB, _ := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
				memTotalMiB, _ := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64)
				var power float64
				if len(parts) >= 8 {
					power, _ = strconv.ParseFloat(strings.TrimSpace(parts[7]), 64)
				}
				var fan int
				if len(parts) >= 9 {
					fan, _ = strconv.Atoi(strings.TrimSpace(parts[8]))
				}

				results = append(results, domain.GpuTelemetry{
					Index:              int32(idx),
					Name:               name,
					DriverVersion:      driverVer,
					TemperatureCelsius: temp,
					UtilizationPercent: util,
					MemoryUsedBytes:    int64(memUsedMiB * 1024 * 1024),
					MemoryTotalBytes:   int64(memTotalMiB * 1024 * 1024),
					PowerWatts:         power,
					FanSpeedPercent:    int32(fan),
					Vendor:             "NVIDIA",
				})
			}
		}
	}

	return results
}

func (c *TelemetryCollector) enrichCpuTemperatures(cpu *domain.CpuTelemetry, sensors *domain.HardwareSensorsTelemetry) {
	var pkgTemps []float64
	for _, item := range sensors.Temperatures {
		if item.Category == "CPU" && strings.Contains(strings.ToLower(item.Label), "package") && item.CurrentValue > 0 {
			pkgTemps = append(pkgTemps, item.CurrentValue)
		}
	}
	if len(pkgTemps) > len(cpu.PackageTemperatures) {
		cpu.PackageTemperatures = pkgTemps
		maxTemp := 0.0
		for _, t := range pkgTemps {
			if t > maxTemp {
				maxTemp = t
			}
		}
		if maxTemp > 0 {
			cpu.TemperatureCelsius = maxTemp
		}
	}
}

func toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func extractCoreIndex(label string) int {
	parts := strings.Fields(label)
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 9999
}
