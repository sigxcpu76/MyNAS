package system

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	return &domain.SystemTelemetry{
		Cpu:     cpu,
		Memory:  mem,
		Host:    host,
		Network: netIfaces,
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

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && model == "" {
					model = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "processor") {
				logicalCount++
			} else if strings.HasPrefix(line, "core id") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					physicalMap[strings.TrimSpace(parts[1])] = true
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
	res.TemperatureCelsius, res.CoreTemperatures = readCPUTemperatures()

	return res
}

func readCPUTemperatures() (float64, []float64) {
	var coreTemps []float64
	var packageTemp float64

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
		for _, tf := range tempInputs {
			if valBytes, err := os.ReadFile(tf); err == nil {
				if millidegrees, err := strconv.ParseFloat(strings.TrimSpace(string(valBytes)), 64); err == nil {
					deg := millidegrees / 1000.0
					if deg > 0 && deg < 150 {
						coreTemps = append(coreTemps, deg)
						if packageTemp == 0 || deg > packageTemp {
							packageTemp = deg
						}
					}
				}
			}
		}

		if packageTemp > 0 {
			break
		}
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
						break
					}
				}
			}
		}
	}

	return packageTemp, coreTemps
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
