package application

import (
	"context"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type PreferencesService struct {
	port ports.PreferencesPort
}

func NewPreferencesService(port ports.PreferencesPort) *PreferencesService {
	return &PreferencesService{port: port}
}

func (s *PreferencesService) GetPreferences(ctx context.Context, user *domain.User) (*domain.UserPreferences, error) {
	return s.port.GetPreferences(ctx, user)
}

func (s *PreferencesService) UpdatePreferences(ctx context.Context, user *domain.User, prefs *domain.UserPreferences) (*domain.UserPreferences, error) {
	return s.port.UpdatePreferences(ctx, user, prefs)
}
