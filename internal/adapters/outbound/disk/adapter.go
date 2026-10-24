package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type Adapter struct {
	mu        sync.RWMutex
	sysFS     string
	zfsEngine ports.ZfsEnginePort
	fallback  bool
}

func NewAdapter(zfsEngine ports.ZfsEnginePort, fallback bool) *Adapter {
	return &Adapter{
		sysFS:     "/sys/block",
		zfsEngine: zfsEngine,
		fallback:  fallback,
	}
}

func (a *Adapter) ListDisks(ctx context.Context) ([]domain.Disk, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var disks []domain.Disk

	// 1. Fetch ZFS Pool vdev mappings to match owning pools
	poolDeviceMap := a.getPoolDeviceMap(ctx)

	// 2. Discover block devices in /sys/block
	entries, err := os.ReadDir(a.sysFS)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()

			// Skip virtual loop, ram, and device-mapper devices
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
				continue
			}

			disk, err := a.inspectSysfsDisk(name, poolDeviceMap)
			if err == nil && disk != nil {
				disks = append(disks, *disk)
			}
		}
	}

	// 3. Fallback if no disks discovered (e.g. restricted container or test environment)
	if len(disks) == 0 && a.fallback {
		disks = a.getSimulatedDisks(poolDeviceMap)
	}

	return disks, nil
}

func (a *Adapter) GetDisk(ctx context.Context, id string) (*domain.Disk, error) {
	cleanID := strings.TrimPrefix(id, "disks/")
	disks, err := a.ListDisks(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range disks {
		if d.DiskID == cleanID {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("disk %q not found", cleanID)
}

func (a *Adapter) inspectSysfsDisk(devName string, poolMap map[string]string) (*domain.Disk, error) {
	devDir := filepath.Join(a.sysFS, devName)

	// Read size in 512-byte sectors
	sizeSectorsStr := readFileTrim(filepath.Join(devDir, "size"))
	if sizeSectorsStr == "" {
		return nil, fmt.Errorf("cannot read size for %s", devName)
	}
	sectors, _ := strconv.ParseInt(sizeSectorsStr, 10, 64)
	sizeBytes := sectors * 512

	// Read rotational flag: 1 = HDD, 0 = SSD/Flash
	rotationalStr := readFileTrim(filepath.Join(devDir, "queue", "rotational"))
	rotational := rotationalStr == "1"

	// Read logical & physical block sizes
	logicalBlockSize, _ := strconv.ParseInt(readFileTrim(filepath.Join(devDir, "queue", "logical_block_size")), 10, 64)
	if logicalBlockSize == 0 {
		logicalBlockSize = 512
	}
	physicalBlockSize, _ := strconv.ParseInt(readFileTrim(filepath.Join(devDir, "queue", "physical_block_size")), 10, 64)
	if physicalBlockSize == 0 {
		physicalBlockSize = logicalBlockSize
	}

	// Removable
	removable := readFileTrim(filepath.Join(devDir, "removable")) == "1"

	// Hardware properties from /sys/block/<dev>/device/
	model := readFileTrim(filepath.Join(devDir, "device", "model"))
	serial := readFileTrim(filepath.Join(devDir, "device", "serial"))
	vendor := readFileTrim(filepath.Join(devDir, "device", "vendor"))
	firmwareRev := readFileTrim(filepath.Join(devDir, "device", "firmware_rev"))
	if firmwareRev == "" {
		firmwareRev = readFileTrim(filepath.Join(devDir, "device", "rev"))
	}
	state := readFileTrim(filepath.Join(devDir, "device", "state"))
	if state == "" {
		state = "running"
	}
	transport := strings.ToLower(readFileTrim(filepath.Join(devDir, "device", "transport")))

	// Classify device type & transport
	diskType := "SSD"
	if strings.HasPrefix(devName, "nvme") {
		diskType = "NVME"
		if transport == "" {
			transport = "nvme"
		}
	} else if strings.HasPrefix(devName, "vd") {
		diskType = "VIRTUAL"
		if transport == "" {
			transport = "virtio"
		}
	} else if strings.HasPrefix(devName, "zd") {
		diskType = "VIRTUAL"
		if transport == "" {
			transport = "zvol"
		}
	} else if rotational {
		diskType = "HDD"
		if transport == "" {
			transport = "sata"
		}
	} else {
		if transport == "" {
			transport = "sata"
		}
	}

	// Read temperature from hwmon
	temp := a.readSysfsTemperature(devDir, devName)

	// S.M.A.R.T. status & Health percentage
	smartStatus := "UNKNOWN"
	healthPct := int32(100)

	// Try querying smartctl / nvme-cli if available
	devPath := "/dev/" + devName
	a.enrichSmartData(devPath, diskType, &smartStatus, &healthPct, &temp)

	// Owning ZFS Pool resolution
	owningPool := poolMap[devName]
	if owningPool == "" {
		owningPool = poolMap[devPath]
	}

	// Partitions discovery
	partitions := a.inspectSysfsPartitions(devDir, devName, poolMap)

	// If any child partition is owned by a ZFS pool, disk is also associated
	if owningPool == "" {
		for _, p := range partitions {
			if p.OwningZfsPool != "" {
				owningPool = p.OwningZfsPool
				break
			}
		}
	}

	disk := &domain.Disk{
		Name:               "disks/" + devName,
		DiskID:             devName,
		DevicePath:         devPath,
		SizeBytes:          sizeBytes,
		Model:              model,
		Serial:             serial,
		Vendor:             vendor,
		Transport:          transport,
		Rotational:         rotational,
		Type:               diskType,
		FirmwareRev:        firmwareRev,
		State:              state,
		Removable:          removable,
		LogicalBlockSize:   logicalBlockSize,
		PhysicalBlockSize:  physicalBlockSize,
		TemperatureCelsius: temp,
		SmartStatus:        smartStatus,
		HealthPercentage:   healthPct,
		OwningZfsPool:      owningPool,
		Partitions:         partitions,
	}

	return disk, nil
}

func (a *Adapter) inspectSysfsPartitions(devDir, devName string, poolMap map[string]string) []domain.DiskPartition {
	var parts []domain.DiskPartition

	entries, err := os.ReadDir(devDir)
	if err != nil {
		return parts
	}

	mounts := parseProcMounts()

	for _, e := range entries {
		subName := e.Name()
		subDir := filepath.Join(devDir, subName)
		if _, err := os.Stat(filepath.Join(subDir, "partition")); err == nil {
			sizeSectorsStr := readFileTrim(filepath.Join(subDir, "size"))
			sec, _ := strconv.ParseInt(sizeSectorsStr, 10, 64)
			partPath := "/dev/" + subName

			owningPool := poolMap[subName]
			if owningPool == "" {
				owningPool = poolMap[partPath]
			}

			parts = append(parts, domain.DiskPartition{
				Name:          subName,
				DevicePath:    partPath,
				SizeBytes:     sec * 512,
				Mountpoint:    mounts[partPath],
				OwningZfsPool: owningPool,
			})
		}
	}

	return parts
}

func (a *Adapter) readSysfsTemperature(devDir, devName string) float64 {
	// 1. Check under /sys/block/<dev>/device/hwmon*/temp*_input
	matches, _ := filepath.Glob(filepath.Join(devDir, "device", "hwmon*", "temp*_input"))
	for _, m := range matches {
		valStr := readFileTrim(m)
		if val, err := strconv.ParseFloat(valStr, 64); err == nil && val > 0 {
			if val > 1000 {
				return val / 1000.0
			}
			return val
		}
	}

	// 2. Check /sys/class/hwmon/ for nvme or matching device name
	hwMatches, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hw := range hwMatches {
		name := readFileTrim(filepath.Join(hw, "name"))
		if strings.Contains(name, devName) || (strings.HasPrefix(devName, "nvme") && strings.HasPrefix(name, "nvme")) {
			valStr := readFileTrim(filepath.Join(hw, "temp1_input"))
			if val, err := strconv.ParseFloat(valStr, 64); err == nil && val > 0 {
				if val > 1000 {
					return val / 1000.0
				}
				return val
			}
		}
	}

	return 0.0
}

func (a *Adapter) enrichSmartData(devPath, diskType string, smartStatus *string, healthPct *int32, temp *float64) {
	if diskType == "VIRTUAL" {
		*smartStatus = "N/A"
		return
	}

	// Try smartctl json
	if smartctlPath, err := exec.LookPath("smartctl"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, smartctlPath, "-j", "-H", "-A", devPath)
		out, _ := cmd.Output()
		if len(out) > 0 {
			var parsed struct {
				ExitStatus  int `json:"exit_status"`
				SmartStatus *struct {
					Passed bool `json:"passed"`
				} `json:"smart_status"`
				Temperature struct {
					Current float64 `json:"current"`
				} `json:"temperature"`
				PowerOnTime struct {
					Hours int64 `json:"hours"`
				} `json:"power_on_time"`
			}
			if err := json.Unmarshal(out, &parsed); err == nil && parsed.SmartStatus != nil {
				if parsed.SmartStatus.Passed {
					*smartStatus = "PASSED"
				} else {
					*smartStatus = "FAILED"
				}
				if *temp == 0.0 && parsed.Temperature.Current > 0 {
					*temp = parsed.Temperature.Current
				}
				return
			}
		}
	}

	// Try nvme-cli for NVMe
	if diskType == "NVME" {
		if nvmePath, err := exec.LookPath("nvme"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			// try `nvme log smart` then fallback to `nvme smart-log`
			cmd := exec.CommandContext(ctx, nvmePath, "log", "smart", devPath, "-o", "json")
			out, err := cmd.Output()
			if err != nil || len(out) == 0 {
				cmd = exec.CommandContext(ctx, nvmePath, "smart-log", devPath, "-o", "json")
				out, _ = cmd.Output()
			}
			if len(out) > 0 {
				var nvmeSmart struct {
					Error           string  `json:"error"`
					CriticalWarning *int32  `json:"critical_warning"`
					Temperature     float64 `json:"temperature"`
					PercentUsed     int32   `json:"percent_used"`
					AvailSpare      int32   `json:"avail_spare"`
				}
				if err := json.Unmarshal(out, &nvmeSmart); err == nil && nvmeSmart.Error == "" && nvmeSmart.CriticalWarning != nil {
					if *nvmeSmart.CriticalWarning == 0 {
						*smartStatus = "PASSED"
					} else {
						*smartStatus = "FAILED"
					}
					if nvmeSmart.PercentUsed > 0 {
						*healthPct = 100 - nvmeSmart.PercentUsed
					}
					if *temp == 0.0 && nvmeSmart.Temperature > 0 {
						if nvmeSmart.Temperature > 200 { // reported in Kelvin
							*temp = nvmeSmart.Temperature - 273.15
						} else {
							*temp = nvmeSmart.Temperature
						}
					}
					return
				}
			}
		}
	}

	// If we got temperature or active state from sysfs, mark as PASSED
	if *temp > 0 {
		*smartStatus = "PASSED"
	}
}

func (a *Adapter) getPoolDeviceMap(ctx context.Context) map[string]string {
	poolMap := make(map[string]string)

	// 1. Query ZFS engine port if available
	if a.zfsEngine != nil {
		pools, err := a.zfsEngine.ListPools(ctx)
		if err == nil {
			for _, p := range pools {
				poolName := p.PoolName
				if poolName == "" {
					poolName = p.ID
				}
				for _, dev := range p.Devices {
					poolMap[dev] = poolName
					poolMap[filepath.Base(dev)] = poolName
				}
				for _, v := range p.Vdevs {
					matchDevsInVdev(v, poolName, poolMap)
				}
			}
		}
	}

	// 2. Query zpool status -P directly if available
	if zpoolPath, err := exec.LookPath("zpool"); err == nil {
		cmd := exec.CommandContext(ctx, zpoolPath, "status", "-P")
		out, err := cmd.Output()
		if err == nil {
			parseZpoolStatusDevices(string(out), poolMap)
		}
	}

	// 3. Resolve symlinks from /dev/disk/by-id to map to underlying disks
	byIDMatches, _ := filepath.Glob("/dev/disk/by-id/*")
	for _, byIDPath := range byIDMatches {
		target, err := filepath.EvalSymlinks(byIDPath)
		if err == nil {
			devName := filepath.Base(target)
			if pool, exists := poolMap[byIDPath]; exists {
				poolMap[devName] = pool
				poolMap[target] = pool
			}
		}
	}

	return poolMap
}

func matchDevsInVdev(v domain.ZfsVdev, poolName string, poolMap map[string]string) {
	if v.Path != "" {
		poolMap[v.Path] = poolName
		poolMap[filepath.Base(v.Path)] = poolName
	}
	for _, child := range v.Children {
		matchDevsInVdev(child, poolName, poolMap)
	}
}

func parseZpoolStatusDevices(statusOut string, poolMap map[string]string) {
	lines := strings.Split(statusOut, "\n")
	var currentPool string
	inConfig := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pool:") {
			currentPool = strings.TrimSpace(strings.TrimPrefix(trimmed, "pool:"))
			inConfig = false
			continue
		}
		if trimmed == "config:" {
			inConfig = true
			continue
		}
		if inConfig && currentPool != "" {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				name := fields[0]
				if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "zd") {
					poolMap[name] = currentPool
					poolMap[filepath.Base(name)] = currentPool
				}
			}
		}
	}
}

func (a *Adapter) getSimulatedDisks(poolMap map[string]string) []domain.Disk {
	pool1 := poolMap["nvme0n1"]
	if pool1 == "" {
		pool1 = poolMap["/dev/nvme0n1"]
	}
	pool2 := poolMap["sda"]
	if pool2 == "" {
		pool2 = poolMap["/dev/sda"]
	}

	return []domain.Disk{
		{
			Name:               "disks/nvme0n1",
			DiskID:             "nvme0n1",
			DevicePath:         "/dev/nvme0n1",
			SizeBytes:          2048408248320,
			Model:              "Micron 7450 PRO Enterprise NVMe",
			Serial:             "242749B56EA0",
			Vendor:             "Micron",
			Transport:          "nvme",
			Rotational:         false,
			Type:               "NVME",
			FirmwareRev:        "E203",
			State:              "live",
			Removable:          false,
			LogicalBlockSize:   512,
			PhysicalBlockSize:  4096,
			TemperatureCelsius: 38.5,
			SmartStatus:        "PASSED",
			HealthPercentage:   99,
			OwningZfsPool:      pool1,
			Partitions: []domain.DiskPartition{
				{Name: "nvme0n1p1", DevicePath: "/dev/nvme0n1p1", SizeBytes: 1073741824, Mountpoint: "/boot"},
				{Name: "nvme0n1p2", DevicePath: "/dev/nvme0n1p2", SizeBytes: 2047334506496, Mountpoint: "/"},
			},
		},
		{
			Name:               "disks/sda",
			DiskID:             "sda",
			DevicePath:         "/dev/sda",
			SizeBytes:          4000787030016,
			Model:              "WDC WD40EFRX-68N32N0",
			Serial:             "WD-WCC7K1122334",
			Vendor:             "Western Digital",
			Transport:          "sata",
			Rotational:         true,
			Type:               "HDD",
			FirmwareRev:        "82.00A82",
			State:              "running",
			Removable:          false,
			LogicalBlockSize:   512,
			PhysicalBlockSize:  4096,
			TemperatureCelsius: 32.0,
			SmartStatus:        "PASSED",
			HealthPercentage:   100,
			OwningZfsPool:      pool2,
		},
	}
}

func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func parseProcMounts() map[string]string {
	res := make(map[string]string)
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return res
	}
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		parts := strings.Fields(l)
		if len(parts) >= 2 {
			res[parts[0]] = parts[1]
		}
	}
	return res
}
