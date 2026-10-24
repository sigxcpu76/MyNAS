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
	vm.VMName = vm.ID
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

func (s *KvmService) RebootVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	if err := domain.ValidateResourceID(id, "virtual_machine"); err != nil {
		return nil, err
	}
	return s.hypervisor.RebootVM(ctx, id)
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
