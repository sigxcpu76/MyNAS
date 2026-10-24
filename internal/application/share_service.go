package application

import (
	"context"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type ShareService struct {
	cifs   ports.CifsSharePort
	webdav ports.WebdavSharePort
}

func NewShareService(cifs ports.CifsSharePort, webdav ports.WebdavSharePort) *ShareService {
	return &ShareService{cifs: cifs, webdav: webdav}
}

func (s *ShareService) ListCifsShares(ctx context.Context) ([]domain.CifsShare, error) {
	return s.cifs.ListShares(ctx)
}

func (s *ShareService) GetCifsShare(ctx context.Context, id string) (*domain.CifsShare, error) {
	if err := domain.ValidateResourceID(id, "cifs_share"); err != nil {
		return nil, err
	}
	return s.cifs.GetShare(ctx, id)
}

func (s *ShareService) CreateCifsShare(ctx context.Context, share domain.CifsShare) (*domain.CifsShare, error) {
	share.ShareName = share.ID
	if err := share.Validate(); err != nil {
		return nil, err
	}
	return s.cifs.CreateShare(ctx, share)
}

func (s *ShareService) UpdateCifsShare(ctx context.Context, share domain.CifsShare, fields []string) (*domain.CifsShare, error) {
	if err := domain.ValidateResourceID(share.ID, "cifs_share"); err != nil {
		return nil, err
	}
	return s.cifs.UpdateShare(ctx, share, fields)
}

func (s *ShareService) DeleteCifsShare(ctx context.Context, id string) error {
	if err := domain.ValidateResourceID(id, "cifs_share"); err != nil {
		return err
	}
	return s.cifs.DeleteShare(ctx, id)
}

func (s *ShareService) ListWebdavShares(ctx context.Context) ([]domain.WebdavShare, error) {
	return s.webdav.ListShares(ctx)
}

func (s *ShareService) GetWebdavShare(ctx context.Context, id string) (*domain.WebdavShare, error) {
	if err := domain.ValidateResourceID(id, "webdav_share"); err != nil {
		return nil, err
	}
	return s.webdav.GetShare(ctx, id)
}

func (s *ShareService) CreateWebdavShare(ctx context.Context, share domain.WebdavShare) (*domain.WebdavShare, error) {
	share.ShareName = share.ID
	if err := share.Validate(); err != nil {
		return nil, err
	}
	return s.webdav.CreateShare(ctx, share)
}

func (s *ShareService) UpdateWebdavShare(ctx context.Context, share domain.WebdavShare, fields []string) (*domain.WebdavShare, error) {
	if err := domain.ValidateResourceID(share.ID, "webdav_share"); err != nil {
		return nil, err
	}
	return s.webdav.UpdateShare(ctx, share, fields)
}

func (s *ShareService) DeleteWebdavShare(ctx context.Context, id string) error {
	if err := domain.ValidateResourceID(id, "webdav_share"); err != nil {
		return err
	}
	return s.webdav.DeleteShare(ctx, id)
}
