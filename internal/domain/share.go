package domain

import (
	"fmt"
	"strings"
)

type CifsShare struct {
	ID            string
	ShareName     string
	Path          string
	Comment       string
	ReadOnly      bool
	GuestOk       bool
	Browseable    bool
	ValidUsers    []string
	ValidGroups   []string
	CreateMask    string
	DirectoryMask string
	Enabled       bool
}

func (s *CifsShare) Validate() error {
	if err := ValidateResourceID(s.ID, "cifs_share_id"); err != nil {
		return err
	}
	if !strings.HasPrefix(s.Path, "/") {
		return fmt.Errorf("%w: path %q must be an absolute filesystem path", ErrInvalidArgument, s.Path)
	}
	if s.CreateMask == "" {
		s.CreateMask = "0664"
	}
	if s.DirectoryMask == "" {
		s.DirectoryMask = "0775"
	}
	return nil
}

type WebdavShare struct {
	ID            string
	ShareName     string
	Path          string
	URLPrefix     string
	Description   string
	ReadOnly      bool
	AllowedUsers  []string
	AllowedGroups []string
	Enabled       bool
}

func (s *WebdavShare) Validate() error {
	if err := ValidateResourceID(s.ID, "webdav_share_id"); err != nil {
		return err
	}
	if !strings.HasPrefix(s.Path, "/") {
		return fmt.Errorf("%w: path %q must be an absolute filesystem path", ErrInvalidArgument, s.Path)
	}
	s.URLPrefix = "/dav/" + s.ID
	return nil
}
