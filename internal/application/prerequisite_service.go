package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type PrerequisiteService struct {
	checker ports.PrerequisiteCheckerPort
}

func NewPrerequisiteService(checker ports.PrerequisiteCheckerPort) *PrerequisiteService {
	return &PrerequisiteService{checker: checker}
}

func (s *PrerequisiteService) SetMainPool(pool string) {
	s.checker.SetMainPool(pool)
}

func (s *PrerequisiteService) GetMainPool() string {
	return s.checker.GetMainPool()
}

func (s *PrerequisiteService) IsModuleEnabled(module string) (bool, string) {
	return s.checker.IsModuleEnabled(module)
}

func (s *PrerequisiteService) HasHostPrerequisites(module string) bool {
	return s.checker.HasHostPrerequisites(module)
}

func (s *PrerequisiteService) RequireModule(module string) error {
	enabled, reason := s.checker.IsModuleEnabled(module)
	if !enabled {
		return fmt.Errorf("%w: %s", domain.ErrFailedPrecondition, reason)
	}
	return nil
}

func (s *PrerequisiteService) ListPrerequisites(ctx context.Context, filter string) ([]domain.Prerequisite, error) {
	items, err := s.checker.ListPrerequisites(ctx)
	if err != nil {
		return nil, err
	}
	if filter == "" {
		return items, nil
	}
	f := strings.ToLower(strings.TrimSpace(filter))
	var filtered []domain.Prerequisite
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Category), f) ||
			strings.Contains(strings.ToLower(string(item.Status)), f) ||
			strings.Contains(strings.ToLower(item.ID), f) ||
			strings.Contains(strings.ToLower(item.DisplayName), f) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *PrerequisiteService) GetPrerequisite(ctx context.Context, id string) (*domain.Prerequisite, error) {
	if err := domain.ValidateResourceID(id, "prerequisite"); err != nil {
		return nil, err
	}
	return s.checker.GetPrerequisite(ctx, id)
}

func (s *PrerequisiteService) CheckPrerequisites(ctx context.Context) ([]domain.Prerequisite, int32, int32, bool, error) {
	items, hybrid, err := s.checker.CheckAll(ctx)
	if err != nil {
		return nil, 0, 0, false, err
	}
	var installed, missing int32
	for _, p := range items {
		if p.Status == domain.StatusInstalled {
			installed++
		} else {
			missing++
		}
	}
	return items, installed, missing, hybrid, nil
}

func (s *PrerequisiteService) GetReport(ctx context.Context) (domain.PrerequisitesReport, error) {
	return s.checker.GetReport(ctx)
}

func (s *PrerequisiteService) GetLinuxFlavor() domain.LinuxFlavor {
	return s.checker.GetLinuxFlavor()
}
