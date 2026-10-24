package domain

import "fmt"

type VirtualMachine struct {
	ID            string
	VMName        string
	UUID          string
	State         string // RUNNING, STOPPED, PAUSED
	VCPUs         int32
	MemoryMiB     int64
	DiskPath      string
	DiskSizeGiB   int64
	ISOImage      string
	NetworkBridge string
	VNCPort       int32
	VNCDisplay    string
	Autostart     bool
	OSVariant     string
	Description   string
}

func (vm *VirtualMachine) Validate() error {
	if err := ValidateResourceID(vm.ID, "virtual_machine_id"); err != nil {
		return err
	}
	if vm.VCPUs <= 0 {
		return fmt.Errorf("%w: vcpus must be >= 1", ErrInvalidArgument)
	}
	if vm.MemoryMiB < 128 {
		return fmt.Errorf("%w: memory_mib must be >= 128", ErrInvalidArgument)
	}
	if vm.NetworkBridge == "" {
		vm.NetworkBridge = "virbr0"
	}
	if vm.OSVariant == "" {
		vm.OSVariant = "linux2024"
	}
	return nil
}

type ConsoleSession struct {
	VMName       string
	Token        string
	WebsocketURL string
	VNCPort      int32
	Resolution   string
	ExpiresAt    string
}
