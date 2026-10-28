package disk

import (
	"testing"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestParseSmartctlJSON_ScsiSas(t *testing.T) {
	a := &Adapter{}
	d := &domain.Disk{
		DiskID: "sda",
		Type:   "HDD",
	}

	scsiJSON := []byte(`{
		"json_format_version": [1, 0],
		"smart_status": {
			"passed": true
		},
		"temperature": {
			"current": 32,
			"drive_trip": 60
		},
		"power_on_time": {
			"hours": 48321,
			"minutes": 33
		},
		"scsi_grown_defect_list": 4,
		"scsi_start_stop_cycle_counter": {
			"accumulated_start_stop_cycles": 196,
			"specified_cycle_count_over_device_lifetime": 10000,
			"accumulated_load_unload_cycles": 3185,
			"specified_load_unload_count_over_device_lifetime": 300000
		},
		"scsi_error_counter_log": {
			"read": {
				"errors_corrected_by_eccfast": 865097963,
				"total_errors_corrected": 865097964,
				"gigabytes_processed": "72988.565",
				"total_uncorrected_errors": 0
			},
			"write": {
				"errors_corrected_by_eccfast": 0,
				"total_errors_corrected": 0,
				"gigabytes_processed": "616839.816",
				"total_uncorrected_errors": 0
			}
		}
	}`)

	success := a.parseSmartctlJSON(scsiJSON, d)
	if !success {
		t.Fatalf("expected parseSmartctlJSON to return true for SCSI SAS disk")
	}

	if d.SmartStatus != "PASSED" {
		t.Errorf("expected SmartStatus PASSED, got %s", d.SmartStatus)
	}
	if d.TemperatureCelsius != 32 {
		t.Errorf("expected Temperature 32, got %f", d.TemperatureCelsius)
	}
	if d.PowerOnHours != 48321 {
		t.Errorf("expected PowerOnHours 48321, got %d", d.PowerOnHours)
	}
	if d.PowerCycleCount != 196 {
		t.Errorf("expected PowerCycleCount 196, got %d", d.PowerCycleCount)
	}
	if d.ReallocatedSectors != 4 {
		t.Errorf("expected ReallocatedSectors (Grown Defect List) 4, got %d", d.ReallocatedSectors)
	}

	// Verify SMART parameters
	paramMap := make(map[string]domain.SmartParameter)
	for _, p := range d.SmartParameters {
		paramMap[p.Name] = p
	}

	if p, ok := paramMap["Elements_In_Grown_Defect_List"]; !ok || p.Value != "4" || p.Status != "OK" {
		t.Errorf("expected Elements_In_Grown_Defect_List=4 with status OK, got %+v", p)
	}
	if p, ok := paramMap["Read_Total_Uncorrected_Errors"]; !ok || p.Value != "0" || p.Status != "OK" {
		t.Errorf("expected Read_Total_Uncorrected_Errors=0 with status OK, got %+v", p)
	}
	if p, ok := paramMap["Write_Total_Uncorrected_Errors"]; !ok || p.Value != "0" || p.Status != "OK" {
		t.Errorf("expected Write_Total_Uncorrected_Errors=0 with status OK, got %+v", p)
	}
	if p, ok := paramMap["Load_Cycle_Count"]; !ok || p.Value != "3185" {
		t.Errorf("expected Load_Cycle_Count=3185, got %+v", p)
	}
	if p, ok := paramMap["Temperature_Celsius"]; !ok || p.Value != "32" {
		t.Errorf("expected Temperature_Celsius=32, got %+v", p)
	}
}

func TestParseSmartctlJSON_ScsiSas_UncorrectedErrorsFailed(t *testing.T) {
	a := &Adapter{}
	d := &domain.Disk{
		DiskID: "sda",
		Type:   "HDD",
	}

	scsiFailingJSON := []byte(`{
		"smart_status": {
			"passed": true
		},
		"scsi_grown_defect_list": 250,
		"scsi_error_counter_log": {
			"read": {
				"total_uncorrected_errors": 5
			},
			"write": {
				"total_uncorrected_errors": 0
			}
		}
	}`)

	success := a.parseSmartctlJSON(scsiFailingJSON, d)
	if !success {
		t.Fatalf("expected parseSmartctlJSON to return true")
	}

	if d.SmartStatus != "FAILED" {
		t.Errorf("expected SmartStatus FAILED due to uncorrected read errors, got %s", d.SmartStatus)
	}

	paramMap := make(map[string]domain.SmartParameter)
	for _, p := range d.SmartParameters {
		paramMap[p.Name] = p
	}

	if p, ok := paramMap["Read_Total_Uncorrected_Errors"]; !ok || p.Status != "FAILED" {
		t.Errorf("expected Read_Total_Uncorrected_Errors status FAILED, got %+v", p)
	}
	if p, ok := paramMap["Elements_In_Grown_Defect_List"]; !ok || p.Status != "FAILED" {
		t.Errorf("expected Elements_In_Grown_Defect_List status FAILED (>200 defects), got %+v", p)
	}
}

func TestDetectCcissTarget_CcissDevicePath(t *testing.T) {
	idx, ctrl := detectCcissTarget("/dev/cciss/c0d0")
	if idx != 0 || ctrl != "/dev/cciss/c0d0" {
		t.Errorf("expected (0, /dev/cciss/c0d0), got (%d, %s)", idx, ctrl)
	}

	idx, ctrl = detectCcissTarget("/dev/cciss/c0d5p2")
	if idx != 5 || ctrl != "/dev/cciss/c0d5p2" {
		t.Errorf("expected (5, /dev/cciss/c0d5p2), got (%d, %s)", idx, ctrl)
	}

	idx, _ = detectCcissTarget("/dev/nvme0n1")
	if idx != -1 {
		t.Errorf("expected -1 for nvme0n1, got %d", idx)
	}
}
