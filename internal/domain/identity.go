package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var linuxUsernameRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type User struct {
	ID                  string
	Username            string
	UID                 int32
	PrimaryGID          int32
	PrimaryGroup        string
	SupplementaryGroups []string
	FullName            string
	HomeDirectory       string
	Shell               string
	SSHEnabled          bool
	SSHPasswordAuth     bool
	SSHAuthorizedKeys   []string
	Locked              bool
	SystemAccount       bool
	Password            string
	IsAdmin             bool
	SMBEnabled          bool
	SMBPassword         string
}

func (u *User) Validate() error {
	if !linuxUsernameRegex.MatchString(u.ID) {
		return fmt.Errorf("%w: username %q must match Linux account pattern ^[a-z_][a-z0-9_-]{0,31}$", ErrInvalidArgument, u.ID)
	}
	if u.HomeDirectory == "" {
		u.HomeDirectory = "/home/" + u.ID
	}
	if u.Shell == "" {
		if u.SSHEnabled {
			u.Shell = "/bin/bash"
		} else {
			u.Shell = "/usr/sbin/nologin"
		}
	}
	for _, key := range u.SSHAuthorizedKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" && !strings.HasPrefix(trimmed, "ssh-") && !strings.HasPrefix(trimmed, "ecdsa-") && !strings.HasPrefix(trimmed, "sk-") {
			return fmt.Errorf("%w: invalid SSH public key format %q", ErrInvalidArgument, trimmed)
		}
	}
	return nil
}

type Group struct {
	ID          string
	GroupName   string
	GID         int32
	Members     []string
	SystemGroup bool
}

func (g *Group) Validate() error {
	if !linuxUsernameRegex.MatchString(g.ID) {
		return fmt.Errorf("%w: group_name %q must match Linux group pattern ^[a-z_][a-z0-9_-]{0,31}$", ErrInvalidArgument, g.ID)
	}
	return nil
}
