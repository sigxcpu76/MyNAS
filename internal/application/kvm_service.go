package application

import (
	"context"
	"net"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type KvmService struct {
	hypervisor ports.KvmHypervisorPort
}

func NewKvmService(hypervisor ports.KvmHypervisorPort) *KvmService {
	return &KvmService{hypervisor: hypervisor}
}

func (s *KvmService) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	return s.hypervisor.ListVMs(ctx)
}

func (s *KvmService) GetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.GetVM(ctx, id)
}

func (s *KvmService) CreateVM(ctx context.Context, vm domain.VirtualMachine) (*domain.VirtualMachine, error) {
	if vm.ID == "" {
		vm.ID = domain.GenerateNanosecondID()
	}
	if vm.VMName == "" {
		vm.VMName = vm.ID
	}
	if err := vm.Validate(); err != nil {
		return nil, err
	}
	return s.hypervisor.CreateVM(ctx, vm)
}

func (s *KvmService) UpdateVM(ctx context.Context, vm domain.VirtualMachine, fields []string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(vm.ID, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.UpdateVM(ctx, vm, fields)
}

func (s *KvmService) DeleteVM(ctx context.Context, id string) error {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return err
	}
	return s.hypervisor.DeleteVM(ctx, id)
}

func (s *KvmService) StartVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.StartVM(ctx, id)
}

func (s *KvmService) StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.StopVM(ctx, id, force)
}

func (s *KvmService) RebootVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.RebootVM(ctx, id, force)
}

func (s *KvmService) ResetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.ResetVM(ctx, id)
}

func (s *KvmService) PauseVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.PauseVM(ctx, id)
}

func (s *KvmService) ResumeVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.ResumeVM(ctx, id)
}

func (s *KvmService) CloneVM(ctx context.Context, id, newID, newName, targetPool string, linkedClone bool) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	if newID == "" {
		newID = domain.GenerateNanosecondID()
	}
	if newName == "" {
		newName = newID
	}
	if err := domain.ValidateResourceID(newID, "virtual_machine_id"); err != nil {
		return nil, err
	}
	return s.hypervisor.CloneVM(ctx, id, newID, newName, targetPool, linkedClone)
}

func (s *KvmService) AttachMedia(ctx context.Context, id, deviceID, isoPath string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.AttachMedia(ctx, id, deviceID, isoPath)
}

func (s *KvmService) EjectMedia(ctx context.Context, id, deviceID string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.EjectMedia(ctx, id, deviceID)
}

func (s *KvmService) ListSnapshots(ctx context.Context, vmID string) ([]domain.VirtualMachineSnapshot, error) {
	if err := domain.ValidateResourceID(vmID, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.ListSnapshots(ctx, vmID)
}

func (s *KvmService) CreateSnapshot(ctx context.Context, vmID string, snap domain.VirtualMachineSnapshot) (*domain.VirtualMachineSnapshot, error) {
	if err := domain.ValidateResourceID(vmID, "virtual_machine"); err != nil {
		return nil, err
	}
	if snap.SnapshotID == "" {
		snap.SnapshotID = domain.GenerateNanosecondID()
	}
	return s.hypervisor.CreateSnapshot(ctx, vmID, snap)
}

func (s *KvmService) RollbackSnapshot(ctx context.Context, vmID, snapID string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(vmID, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.RollbackSnapshot(ctx, vmID, snapID)
}

func (s *KvmService) DeleteSnapshot(ctx context.Context, vmID, snapID string) error {
	if err := domain.ValidateResourceID(vmID, "virtual_machine"); err != nil {
		return err
	}
	return s.hypervisor.DeleteSnapshot(ctx, vmID, snapID)
}

func (s *KvmService) OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.OpenConsole(ctx, id)
}

func (s *KvmService) ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error) {
	return s.hypervisor.ValidateConsoleToken(ctx, vmID, token)
}

func (s *KvmService) DialVNC(ctx context.Context, vmID string) (net.Conn, error) {
	return s.hypervisor.DialVNC(ctx, vmID)
}
