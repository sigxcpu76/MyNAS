package domain_test

import (
	"sync"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestToZfsSafeSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Nextcloud Hub 28!", "nextcloud-hub-28"},
		{"My Ubuntu VM (v2.0)", "my-ubuntu-vm-v2-0"},
		{"---hello---world---", "hello-world"},
		{".hidden.app.", "hidden-app"},
		{"My_VM_01", "my_vm_01"},
		{"  Custom App Title  ", "custom-app-title"},
		{"", ""},
		{"   ", ""},
		{"1761411800123456789", "1761411800123456789"},
	}

	for _, tc := range tests {
		got := domain.ToZfsSafeSlug(tc.input)
		if got != tc.expected {
			t.Errorf("ToZfsSafeSlug(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestGenerateNanosecondID(t *testing.T) {
	// 1. Basic format
	id := domain.GenerateNanosecondID()
	if id == "" {
		t.Fatalf("expected non-empty ID")
	}
	if err := domain.ValidateResourceID(id, "test_id"); err != nil {
		t.Fatalf("generated nanosecond ID failed validation: %v", err)
	}

	// 2. Concurrency and Uniqueness Test
	const count = 1000
	ids := make(map[string]bool, count)
	var mu sync.Mutex
	var wg sync.WaitGroup

	t0 := time.Now().UnixNano()
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newID := domain.GenerateNanosecondID()
			mu.Lock()
			if ids[newID] {
				t.Errorf("collision detected for ID %s", newID)
			}
			ids[newID] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	t1 := time.Now().UnixNano()

	if len(ids) != count {
		t.Fatalf("expected %d unique IDs, got %d", count, len(ids))
	}
	t.Logf("Generated %d unique nanosecond-level IDs in %v", count, time.Duration(t1-t0))

	// 3. Disk naming helper
	vmID := "my-ubuntu-vm"
	diskName := domain.GenerateVMDiskName(vmID, 0)
	expectedDiskName := "vm-my-ubuntu-vm-01"
	if diskName != expectedDiskName {
		t.Fatalf("expected %q, got %q", expectedDiskName, diskName)
	}

	zvolPath := domain.GenerateZvolDiskPath("mypool", vmID, 0)
	expectedZvol := "/dev/zvol/mypool/vms/my-ubuntu-vm-01"
	if zvolPath != expectedZvol {
		t.Fatalf("expected %q, got %q", expectedZvol, zvolPath)
	}

	zvolPath1 := domain.GenerateZvolDiskPath("mypool", vmID, 1)
	expectedZvol1 := "/dev/zvol/mypool/vms/my-ubuntu-vm-02"
	if zvolPath1 != expectedZvol1 {
		t.Fatalf("expected %q, got %q", expectedZvol1, zvolPath1)
	}

	vmFilePath := domain.GenerateVMDiskFilePath("mypool", vmID, 0)
	expectedFilePath := "/mnt/mypool/vms/my-ubuntu-vm/disk-01.raw"
	if vmFilePath != expectedFilePath {
		t.Fatalf("expected %q, got %q", expectedFilePath, vmFilePath)
	}

	// 4. LXC containers rootfs helper
	lxcRootfs := domain.GenerateLxcRootfsPath("mypool", "debian-noble")
	expectedLxc := "/mnt/mypool/containers/debian-noble/rootfs"
	if lxcRootfs != expectedLxc {
		t.Fatalf("expected %q, got %q", expectedLxc, lxcRootfs)
	}

	// 5. Docker apps helper
	appdataPath := domain.GenerateAppdataPath("mypool", "Nextcloud Hub")
	expectedAppdata := "/mnt/mypool/apps/nextcloud-hub"
	if appdataPath != expectedAppdata {
		t.Fatalf("expected %q, got %q", expectedAppdata, appdataPath)
	}
}
