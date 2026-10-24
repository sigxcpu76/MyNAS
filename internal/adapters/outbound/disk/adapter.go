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

			// Skip virtual loop, ram, device-mapper, and ZFS volume (zvol) block devices
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "zd") {
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
	} else if strings.HasPrefix(devName, "zd") || transport == "zvol" {
		// ZFS volume (zvol) block device - skip, managed under Storage Volumes
		return nil, nil
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

	devPath := "/dev/" + devName

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
		SmartStatus:        "UNKNOWN",
		HealthPercentage:   100,
		OwningZfsPool:      owningPool,
		Partitions:         partitions,
	}

	readIOs, writeIOs := readSysfsStat(devDir, disk)
	a.enrichSmartData(devPath, disk, readIOs, writeIOs)

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

func readSysfsStat(devDir string, d *domain.Disk) (readIOs, writeIOs int64) {
	data := readFileTrim(filepath.Join(devDir, "stat"))
	if data == "" {
		return 0, 0
	}
	fields := strings.Fields(data)
	if len(fields) >= 7 {
		rIO, _ := strconv.ParseInt(fields[0], 10, 64)
		rSec, _ := strconv.ParseInt(fields[2], 10, 64)
		wIO, _ := strconv.ParseInt(fields[4], 10, 64)
		wSec, _ := strconv.ParseInt(fields[6], 10, 64)
		if d.TotalBytesRead == 0 && rSec > 0 {
			d.TotalBytesRead = rSec * 512
		}
		if d.TotalBytesWritten == 0 && wSec > 0 {
			d.TotalBytesWritten = wSec * 512
		}
		return rIO, wIO
	}
	return 0, 0
}

func (a *Adapter) enrichSmartData(devPath string, d *domain.Disk, readIOs, writeIOs int64) {
	if d.Type == "VIRTUAL" {
		d.SmartStatus = "N/A"
		d.SmartParameters = []domain.SmartParameter{
			{
				ID:          0,
				Name:        "Virtual_Block_Device",
				Value:       "N/A",
				Worst:       "N/A",
				Threshold:   "N/A",
				RawValue:    "Virtual / zvol device (SMART not supported)",
				Status:      "OK",
				Type:        "Virtual",
				Description: "Virtual disk or ZFS volume pass-through device",
			},
		}
		return
	}

	// 1. Try smartctl json (-j -x or -j -a)
	if smartctlPath, err := exec.LookPath("smartctl"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, smartctlPath, "-j", "-x", devPath)
		out, err := cmd.Output()
		if err != nil || len(out) == 0 {
			cmd = exec.CommandContext(ctx, smartctlPath, "-j", "-a", devPath)
			out, _ = cmd.Output()
		}
		if len(out) > 0 {
			if a.parseSmartctlJSON(out, d) {
				return
			}
		}
	}

	// 2. Try nvme-cli for NVMe devices
	if d.Type == "NVME" {
		if nvmePath, err := exec.LookPath("nvme"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, nvmePath, "log", "smart", devPath, "-o", "json")
			out, err := cmd.Output()
			if err != nil || len(out) == 0 {
				cmd = exec.CommandContext(ctx, nvmePath, "smart-log", devPath, "-o", "json")
				out, _ = cmd.Output()
			}
			if len(out) > 0 {
				if a.parseNvmeSmartJSON(out, d) {
					return
				}
			}
		}
	}

	// If we got temperature or active state from sysfs, default to PASSED
	if d.SmartStatus == "UNKNOWN" {
		if d.TemperatureCelsius > 0 || d.State == "live" || d.State == "running" {
			d.SmartStatus = "PASSED"
		}
	}

	// If no vendor SMART parameters could be extracted (e.g. unprivileged execution in container/shell),
	// provide sysfs diagnostic parameters
	if len(d.SmartParameters) == 0 {
		a.populateSysfsDiagnosticParameters(d, readIOs, writeIOs)
	}
}

func (a *Adapter) parseSmartctlJSON(data []byte, d *domain.Disk) bool {
	var root struct {
		SmartStatus *struct {
			Passed bool `json:"passed"`
		} `json:"smart_status"`
		Temperature struct {
			Current float64 `json:"current"`
		} `json:"temperature"`
		PowerOnTime struct {
			Hours int64 `json:"hours"`
		} `json:"power_on_time"`
		PowerCycleCount    int64 `json:"power_cycle_count"`
		AtaSmartAttributes *struct {
			Table []struct {
				ID         int32  `json:"id"`
				Name       string `json:"name"`
				Value      int    `json:"value"`
				Worst      int    `json:"worst"`
				Thresh     int    `json:"thresh"`
				WhenFailed string `json:"when_failed"`
				Flags      struct {
					Prefailure bool   `json:"prefailure"`
					String     string `json:"string"`
				} `json:"flags"`
				Raw struct {
					Value  int64  `json:"value"`
					String string `json:"string"`
				} `json:"raw"`
			} `json:"table"`
		} `json:"ata_smart_attributes"`
		NvmeSmartHealthLog *struct {
			CriticalWarning         int64   `json:"critical_warning"`
			Temperature             float64 `json:"temperature"`
			AvailableSpare          int32   `json:"available_spare"`
			AvailableSpareThreshold int32   `json:"available_spare_threshold"`
			PercentageUsed          int32   `json:"percentage_used"`
			DataUnitsRead           int64   `json:"data_units_read"`
			DataUnitsWritten        int64   `json:"data_units_written"`
			HostReads               int64   `json:"host_reads"`
			HostWrites              int64   `json:"host_writes"`
			ControllerBusyTime      int64   `json:"controller_busy_time"`
			PowerCycles             int64   `json:"power_cycles"`
			PowerOnHours            int64   `json:"power_on_hours"`
			UnsafeShutdowns         int64   `json:"unsafe_shutdowns"`
			MediaErrors             int64   `json:"media_errors"`
			NumErrLogEntries        int64   `json:"num_err_log_entries"`
		} `json:"nvme_smart_health_information_log"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}

	if root.SmartStatus != nil {
		if root.SmartStatus.Passed {
			d.SmartStatus = "PASSED"
		} else {
			d.SmartStatus = "FAILED"
		}
	}

	if d.TemperatureCelsius == 0 && root.Temperature.Current > 0 {
		d.TemperatureCelsius = root.Temperature.Current
	}
	if root.PowerOnTime.Hours > 0 {
		d.PowerOnHours = root.PowerOnTime.Hours
	}
	if root.PowerCycleCount > 0 {
		d.PowerCycleCount = root.PowerCycleCount
	}

	// 1. ATA SMART Attributes
	if root.AtaSmartAttributes != nil && len(root.AtaSmartAttributes.Table) > 0 {
		for _, attr := range root.AtaSmartAttributes.Table {
			status := "OK"
			if attr.Thresh > 0 && attr.Value <= attr.Thresh {
				status = "FAILED"
			} else if attr.Thresh > 0 && attr.Value <= attr.Thresh+5 {
				status = "WARNING"
			}

			attrType := "Old_age"
			if attr.Flags.Prefailure {
				attrType = "Pre-fail"
			}

			rawStr := attr.Raw.String
			if rawStr == "" {
				rawStr = strconv.FormatInt(attr.Raw.Value, 10)
			}

			switch attr.ID {
			case 5:
				d.ReallocatedSectors = attr.Raw.Value
			case 197:
				d.PendingSectors = attr.Raw.Value
			case 9:
				if d.PowerOnHours == 0 {
					d.PowerOnHours = attr.Raw.Value
				}
			case 12:
				if d.PowerCycleCount == 0 {
					d.PowerCycleCount = attr.Raw.Value
				}
			case 194:
				if d.TemperatureCelsius == 0 {
					d.TemperatureCelsius = float64(attr.Raw.Value)
				}
			}

			d.SmartParameters = append(d.SmartParameters, domain.SmartParameter{
				ID:          attr.ID,
				Name:        attr.Name,
				Value:       strconv.Itoa(attr.Value),
				Worst:       strconv.Itoa(attr.Worst),
				Threshold:   strconv.Itoa(attr.Thresh),
				RawValue:    rawStr,
				Status:      status,
				Type:        attrType,
				Description: getAtaAttributeDescription(attr.Name),
			})
		}
		return true
	}

	// 2. NVMe SMART Health Log from smartctl
	if log := root.NvmeSmartHealthLog; log != nil {
		d.PowerOnHours = log.PowerOnHours
		d.PowerCycleCount = log.PowerCycles
		d.UnsafeShutdowns = log.UnsafeShutdowns
		d.MediaErrors = log.MediaErrors
		if log.PercentageUsed >= 0 {
			d.HealthPercentage = 100 - log.PercentageUsed
			if d.HealthPercentage < 0 {
				d.HealthPercentage = 0
			}
		}
		if log.DataUnitsRead > 0 {
			d.TotalBytesRead = log.DataUnitsRead * 512000
		}
		if log.DataUnitsWritten > 0 {
			d.TotalBytesWritten = log.DataUnitsWritten * 512000
		}
		if d.TemperatureCelsius == 0 && log.Temperature > 0 {
			d.TemperatureCelsius = log.Temperature
		}

		a.populateNvmeParameters(d, log.CriticalWarning, log.Temperature, log.AvailableSpare, log.AvailableSpareThreshold, log.PercentageUsed, log.DataUnitsRead, log.DataUnitsWritten, log.HostReads, log.HostWrites, log.ControllerBusyTime, log.PowerCycles, log.PowerOnHours, log.UnsafeShutdowns, log.MediaErrors, log.NumErrLogEntries)
		return true
	}

	return root.SmartStatus != nil
}

func (a *Adapter) parseNvmeSmartJSON(data []byte, d *domain.Disk) bool {
	var nvmeSmart struct {
		Error           string  `json:"error"`
		CriticalWarning *int64  `json:"critical_warning"`
		Temperature     float64 `json:"temperature"`
		PercentUsed     int32   `json:"percent_used"`
		AvailSpare      int32   `json:"avail_spare"`
		SpareThresh     int32   `json:"spare_thresh"`
		DataUnitsRead   int64   `json:"data_units_read"`
		DataUnitsWrite  int64   `json:"data_units_written"`
		HostReads       int64   `json:"host_read_commands"`
		HostWrites      int64   `json:"host_write_commands"`
		BusyTime        int64   `json:"controller_busy_time"`
		PowerCycles     int64   `json:"power_cycles"`
		PowerOnHours    int64   `json:"power_on_hours"`
		UnsafeShutdowns int64   `json:"unsafe_shutdowns"`
		MediaErrors     int64   `json:"media_errors"`
		NumErrEntries   int64   `json:"num_err_log_entries"`
	}

	if err := json.Unmarshal(data, &nvmeSmart); err != nil || nvmeSmart.Error != "" || nvmeSmart.CriticalWarning == nil {
		return false
	}

	if *nvmeSmart.CriticalWarning == 0 {
		d.SmartStatus = "PASSED"
	} else {
		d.SmartStatus = "FAILED"
	}

	if nvmeSmart.PercentUsed >= 0 {
		d.HealthPercentage = 100 - nvmeSmart.PercentUsed
		if d.HealthPercentage < 0 {
			d.HealthPercentage = 0
		}
	}

	temp := nvmeSmart.Temperature
	if temp > 200 { // Kelvin
		temp -= 273.15
	}
	if d.TemperatureCelsius == 0 && temp > 0 {
		d.TemperatureCelsius = temp
	}

	d.PowerOnHours = nvmeSmart.PowerOnHours
	d.PowerCycleCount = nvmeSmart.PowerCycles
	d.UnsafeShutdowns = nvmeSmart.UnsafeShutdowns
	d.MediaErrors = nvmeSmart.MediaErrors
	if nvmeSmart.DataUnitsRead > 0 {
		d.TotalBytesRead = nvmeSmart.DataUnitsRead * 512000
	}
	if nvmeSmart.DataUnitsWrite > 0 {
		d.TotalBytesWritten = nvmeSmart.DataUnitsWrite * 512000
	}

	a.populateNvmeParameters(d, *nvmeSmart.CriticalWarning, temp, nvmeSmart.AvailSpare, nvmeSmart.SpareThresh, nvmeSmart.PercentUsed, nvmeSmart.DataUnitsRead, nvmeSmart.DataUnitsWrite, nvmeSmart.HostReads, nvmeSmart.HostWrites, nvmeSmart.BusyTime, nvmeSmart.PowerCycles, nvmeSmart.PowerOnHours, nvmeSmart.UnsafeShutdowns, nvmeSmart.MediaErrors, nvmeSmart.NumErrEntries)
	return true
}

func (a *Adapter) populateNvmeParameters(
	d *domain.Disk,
	critWarn int64,
	temp float64,
	availSpare, spareThresh, pctUsed int32,
	unitsRead, unitsWritten, hostReads, hostWrites, busyTime, cycles, hours, shutdowns, mediaErr, numErr int64,
) {
	critStatus := "OK"
	if critWarn != 0 {
		critStatus = "WARNING"
	}
	spareStatus := "OK"
	if spareThresh > 0 && availSpare < spareThresh {
		spareStatus = "WARNING"
	}
	wearStatus := "OK"
	if pctUsed >= 90 {
		wearStatus = "WARNING"
	}
	errStatus := "OK"
	if mediaErr > 0 {
		errStatus = "WARNING"
	}

	d.SmartParameters = []domain.SmartParameter{
		{ID: 1, Name: "Critical_Warning", Value: fmt.Sprintf("0x%02x", critWarn), Worst: "0x00", Threshold: "0x00", RawValue: fmt.Sprintf("0x%02x", critWarn), Status: critStatus, Type: "NVMe", Description: "Device critical warning flags (temperature, spare, reliability)"},
		{ID: 2, Name: "Composite_Temperature", Value: fmt.Sprintf("%.1f°C", temp), Worst: "-", Threshold: "85°C", RawValue: fmt.Sprintf("%.1f°C", temp), Status: "OK", Type: "NVMe", Description: "Current composite controller and flash temperature"},
		{ID: 3, Name: "Available_Spare", Value: fmt.Sprintf("%d%%", availSpare), Worst: "100%", Threshold: fmt.Sprintf("%d%%", spareThresh), RawValue: fmt.Sprintf("%d%%", availSpare), Status: spareStatus, Type: "NVMe", Description: "Normalized percentage of remaining spare flash capacity"},
		{ID: 4, Name: "Percentage_Used", Value: fmt.Sprintf("%d%%", pctUsed), Worst: "100%", Threshold: "100%", RawValue: fmt.Sprintf("%d%%", pctUsed), Status: wearStatus, Type: "NVMe", Description: "Vendor-estimated percentage of NVM subsystem life consumed"},
		{ID: 5, Name: "Data_Units_Read", Value: domain.FormatBytes(unitsRead * 512000), Worst: "-", Threshold: "-", RawValue: fmt.Sprintf("%d units (%s)", unitsRead, domain.FormatBytes(unitsRead*512000)), Status: "OK", Type: "NVMe", Description: "Total 512K data units read by host system"},
		{ID: 6, Name: "Data_Units_Written", Value: domain.FormatBytes(unitsWritten * 512000), Worst: "-", Threshold: "-", RawValue: fmt.Sprintf("%d units (%s)", unitsWritten, domain.FormatBytes(unitsWritten*512000)), Status: "OK", Type: "NVMe", Description: "Total 512K data units written to flash (TBW endurance tracking)"},
		{ID: 7, Name: "Host_Read_Commands", Value: strconv.FormatInt(hostReads, 10), Worst: "-", Threshold: "-", RawValue: strconv.FormatInt(hostReads, 10), Status: "OK", Type: "NVMe", Description: "Number of read commands completed by controller"},
		{ID: 8, Name: "Host_Write_Commands", Value: strconv.FormatInt(hostWrites, 10), Worst: "-", Threshold: "-", RawValue: strconv.FormatInt(hostWrites, 10), Status: "OK", Type: "NVMe", Description: "Number of write commands completed by controller"},
		{ID: 9, Name: "Controller_Busy_Time", Value: fmt.Sprintf("%d min", busyTime), Worst: "-", Threshold: "-", RawValue: fmt.Sprintf("%d minutes", busyTime), Status: "OK", Type: "NVMe", Description: "Time controller was busy processing I/O commands"},
		{ID: 10, Name: "Power_Cycles", Value: strconv.FormatInt(cycles, 10), Worst: "-", Threshold: "-", RawValue: strconv.FormatInt(cycles, 10), Status: "OK", Type: "NVMe", Description: "Total host power on/off cycles"},
		{ID: 11, Name: "Power_On_Hours", Value: fmt.Sprintf("%d hrs", hours), Worst: "-", Threshold: "-", RawValue: fmt.Sprintf("%d hours (%.1f days)", hours, float64(hours)/24.0), Status: "OK", Type: "NVMe", Description: "Cumulative operating lifetime in hours"},
		{ID: 12, Name: "Unsafe_Shutdowns", Value: strconv.FormatInt(shutdowns, 10), Worst: "-", Threshold: "-", RawValue: strconv.FormatInt(shutdowns, 10), Status: "OK", Type: "NVMe", Description: "Loss of power events without prior host shutdown notification"},
		{ID: 13, Name: "Media_Errors", Value: strconv.FormatInt(mediaErr, 10), Worst: "0", Threshold: "0", RawValue: strconv.FormatInt(mediaErr, 10), Status: errStatus, Type: "NVMe", Description: "Unrecovered data integrity and media ECC errors"},
		{ID: 14, Name: "Error_Log_Entries", Value: strconv.FormatInt(numErr, 10), Worst: "-", Threshold: "-", RawValue: strconv.FormatInt(numErr, 10), Status: "OK", Type: "NVMe", Description: "Number of entries in the controller error information log"},
	}
}

func (a *Adapter) populateSysfsDiagnosticParameters(d *domain.Disk, readIOs, writeIOs int64) {
	d.SmartParameters = []domain.SmartParameter{
		{
			ID:          1,
			Name:        "Device_Operational_State",
			Value:       d.State,
			Worst:       "running",
			Threshold:   "-",
			RawValue:    d.State,
			Status:      "OK",
			Type:        "Kernel_Sysfs",
			Description: "Linux block layer device state (/sys/block/<dev>/device/state)",
		},
		{
			ID:          2,
			Name:        "Temperature_Sensor",
			Value:       fmt.Sprintf("%.1f°C", d.TemperatureCelsius),
			Worst:       "-",
			Threshold:   "80.0°C",
			RawValue:    fmt.Sprintf("%.1f°C", d.TemperatureCelsius),
			Status:      "OK",
			Type:        "Kernel_Hwmon",
			Description: "Hardware thermal monitoring input (/sys/class/hwmon)",
		},
		{
			ID:          3,
			Name:        "Rotational_Media",
			Value:       fmt.Sprintf("%v", d.Rotational),
			Worst:       "-",
			Threshold:   "-",
			RawValue:    map[bool]string{true: "Rotational (HDD)", false: "Solid State (Flash/NVMe)"}[d.Rotational],
			Status:      "OK",
			Type:        "Kernel_Sysfs",
			Description: "Rotational vs solid state flash media flag",
		},
		{
			ID:          4,
			Name:        "Sector_Geometry",
			Value:       fmt.Sprintf("%dB / %dB", d.LogicalBlockSize, d.PhysicalBlockSize),
			Worst:       "-",
			Threshold:   "-",
			RawValue:    fmt.Sprintf("Logical: %d bytes, Physical: %d bytes", d.LogicalBlockSize, d.PhysicalBlockSize),
			Status:      "OK",
			Type:        "Kernel_Sysfs",
			Description: "Hardware logical and physical sector formatting geometry",
		},
		{
			ID:          5,
			Name:        "Host_Read_Volume",
			Value:       domain.FormatBytes(d.TotalBytesRead),
			Worst:       "-",
			Threshold:   "-",
			RawValue:    fmt.Sprintf("%s (%d read operations)", domain.FormatBytes(d.TotalBytesRead), readIOs),
			Status:      "OK",
			Type:        "Kernel_Sysfs_Stat",
			Description: "Cumulative volume read from block device since host boot",
		},
		{
			ID:          6,
			Name:        "Host_Written_Volume",
			Value:       domain.FormatBytes(d.TotalBytesWritten),
			Worst:       "-",
			Threshold:   "-",
			RawValue:    fmt.Sprintf("%s (%d write operations)", domain.FormatBytes(d.TotalBytesWritten), writeIOs),
			Status:      "OK",
			Type:        "Kernel_Sysfs_Stat",
			Description: "Cumulative volume written to block device since host boot",
		},
	}
}

func getAtaAttributeDescription(name string) string {
	switch strings.ToLower(name) {
	case "raw_read_error_rate":
		return "Frequency of errors that occurred while reading raw data from the disk."
	case "spin_up_time":
		return "Average time of spindle spin up (from zero RPM to full speed)."
	case "start_stop_count":
		return "A tally of spindle start/stop cycles."
	case "reallocated_sector_ct":
		return "Count of reallocated sectors. When the drive finds a read/write error, it marks the sector as reallocated and transfers data to a spare sector."
	case "seek_error_rate":
		return "Rate of seek errors of the magnetic heads."
	case "power_on_hours":
		return "Count of hours in power-on state."
	case "spin_retry_count":
		return "Count of retry of spin start attempts if the first attempt failed."
	case "power_cycle_count":
		return "Count of full hard disk power on/off cycles."
	case "temperature_celsius", "temperature":
		return "Current internal temperature of the drive."
	case "current_pending_sector":
		return "Count of unstable sectors waiting to be remapped."
	case "offline_uncorrectable":
		return "Count of uncorrectable errors when reading/writing a sector."
	case "udma_crc_error_count":
		return "Number of errors in data transfer via interface cable as determined by CRC."
	default:
		return "S.M.A.R.T. disk health attribute"
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
			PowerOnHours:       1420,
			PowerCycleCount:    86,
			UnsafeShutdowns:    4,
			MediaErrors:        0,
			TotalBytesRead:     14850000000000,
			TotalBytesWritten:  28400000000000,
			Partitions: []domain.DiskPartition{
				{Name: "nvme0n1p1", DevicePath: "/dev/nvme0n1p1", SizeBytes: 1073741824, Mountpoint: "/boot"},
				{Name: "nvme0n1p2", DevicePath: "/dev/nvme0n1p2", SizeBytes: 2047334506496, Mountpoint: "/"},
			},
			SmartParameters: []domain.SmartParameter{
				{ID: 1, Name: "Critical_Warning", Value: "0x00", Worst: "0x00", Threshold: "0x00", RawValue: "0x00", Status: "OK", Type: "NVMe", Description: "Device critical warning flags (temperature, spare, reliability)"},
				{ID: 2, Name: "Composite_Temperature", Value: "38.5°C", Worst: "-", Threshold: "85°C", RawValue: "38.5°C", Status: "OK", Type: "NVMe", Description: "Current composite controller and flash temperature"},
				{ID: 3, Name: "Available_Spare", Value: "100%", Worst: "100%", Threshold: "10%", RawValue: "100%", Status: "OK", Type: "NVMe", Description: "Normalized percentage of remaining spare flash capacity"},
				{ID: 4, Name: "Percentage_Used", Value: "1%", Worst: "100%", Threshold: "100%", RawValue: "1%", Status: "OK", Type: "NVMe", Description: "Vendor-estimated percentage of NVM subsystem life consumed"},
				{ID: 5, Name: "Data_Units_Read", Value: "13.5 TiB", Worst: "-", Threshold: "-", RawValue: "29003906 units (13.5 TiB)", Status: "OK", Type: "NVMe", Description: "Total 512K data units read by host system"},
				{ID: 6, Name: "Data_Units_Written", Value: "25.8 TiB", Worst: "-", Threshold: "-", RawValue: "55468750 units (25.8 TiB)", Status: "OK", Type: "NVMe", Description: "Total 512K data units written to flash (TBW endurance tracking)"},
				{ID: 7, Name: "Host_Read_Commands", Value: "124591023", Worst: "-", Threshold: "-", RawValue: "124591023", Status: "OK", Type: "NVMe", Description: "Number of read commands completed by controller"},
				{ID: 8, Name: "Host_Write_Commands", Value: "210482901", Worst: "-", Threshold: "-", RawValue: "210482901", Status: "OK", Type: "NVMe", Description: "Number of write commands completed by controller"},
				{ID: 9, Name: "Controller_Busy_Time", Value: "1280 min", Worst: "-", Threshold: "-", RawValue: "1280 minutes", Status: "OK", Type: "NVMe", Description: "Time controller was busy processing I/O commands"},
				{ID: 10, Name: "Power_Cycles", Value: "86", Worst: "-", Threshold: "-", RawValue: "86", Status: "OK", Type: "NVMe", Description: "Total host power on/off cycles"},
				{ID: 11, Name: "Power_On_Hours", Value: "1420 hrs", Worst: "-", Threshold: "-", RawValue: "1420 hours (59.2 days)", Status: "OK", Type: "NVMe", Description: "Cumulative operating lifetime in hours"},
				{ID: 12, Name: "Unsafe_Shutdowns", Value: "4", Worst: "-", Threshold: "-", RawValue: "4", Status: "OK", Type: "NVMe", Description: "Loss of power events without prior host shutdown notification"},
				{ID: 13, Name: "Media_Errors", Value: "0", Worst: "0", Threshold: "0", RawValue: "0", Status: "OK", Type: "NVMe", Description: "Unrecovered data integrity and media ECC errors"},
				{ID: 14, Name: "Error_Log_Entries", Value: "0", Worst: "-", Threshold: "-", RawValue: "0", Status: "OK", Type: "NVMe", Description: "Number of entries in the controller error information log"},
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
			PowerOnHours:       8420,
			PowerCycleCount:    215,
			UnsafeShutdowns:    2,
			MediaErrors:        0,
			TotalBytesRead:     45200000000000,
			TotalBytesWritten:  38900000000000,
			ReallocatedSectors: 0,
			PendingSectors:     0,
			SmartParameters: []domain.SmartParameter{
				{ID: 1, Name: "Raw_Read_Error_Rate", Value: "100", Worst: "100", Threshold: "51", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Frequency of errors that occurred while reading raw data from the disk."},
				{ID: 3, Name: "Spin_Up_Time", Value: "180", Worst: "175", Threshold: "21", RawValue: "4120", Status: "OK", Type: "Pre-fail", Description: "Average time of spindle spin up (from zero RPM to full speed)."},
				{ID: 4, Name: "Start_Stop_Count", Value: "100", Worst: "100", Threshold: "0", RawValue: "215", Status: "OK", Type: "Old_age", Description: "A tally of spindle start/stop cycles."},
				{ID: 5, Name: "Reallocated_Sector_Ct", Value: "100", Worst: "100", Threshold: "140", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Count of reallocated sectors. When the drive finds a read/write error, it marks the sector as reallocated and transfers data to a spare sector."},
				{ID: 7, Name: "Seek_Error_Rate", Value: "100", Worst: "100", Threshold: "51", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Rate of seek errors of the magnetic heads."},
				{ID: 9, Name: "Power_On_Hours", Value: "89", Worst: "89", Threshold: "0", RawValue: "8420", Status: "OK", Type: "Old_age", Description: "Count of hours in power-on state."},
				{ID: 10, Name: "Spin_Retry_Count", Value: "100", Worst: "100", Threshold: "51", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Count of retry of spin start attempts if the first attempt failed."},
				{ID: 12, Name: "Power_Cycle_Count", Value: "100", Worst: "100", Threshold: "0", RawValue: "215", Status: "OK", Type: "Old_age", Description: "Count of full hard disk power on/off cycles."},
				{ID: 194, Name: "Temperature_Celsius", Value: "118", Worst: "110", Threshold: "0", RawValue: "32", Status: "OK", Type: "Old_age", Description: "Current internal temperature of the drive."},
				{ID: 197, Name: "Current_Pending_Sector", Value: "100", Worst: "100", Threshold: "0", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Count of unstable sectors waiting to be remapped."},
				{ID: 198, Name: "Offline_Uncorrectable", Value: "100", Worst: "100", Threshold: "0", RawValue: "0", Status: "OK", Type: "Pre-fail", Description: "Count of uncorrectable errors when reading/writing a sector."},
				{ID: 199, Name: "UDMA_CRC_Error_Count", Value: "200", Worst: "200", Threshold: "0", RawValue: "0", Status: "OK", Type: "Old_age", Description: "Number of errors in data transfer via interface cable as determined by CRC."},
			},
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
