package identity

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/sigxcpu/nas/internal/domain"
)

// PasswordVerifier checks a username/password pair, returning nil on success.
type PasswordVerifier func(username, password string) error

type Adapter struct {
	mu             sync.RWMutex
	users          map[string]domain.User
	groups         map[string]domain.Group
	verifyPassword PasswordVerifier
	nextUID        int32
	nextGID        int32
}

func NewAdapter() *Adapter {
	a := &Adapter{
		users:   make(map[string]domain.User),
		groups:  make(map[string]domain.Group),
		nextUID: 1000,
		nextGID: 1000,
		verifyPassword: func(username, password string) error {
			return AuthenticatePAM("", username, password)
		},
	}
	a.loadFromLinuxSystem()
	a.loadSambaUsers()
	return a
}

// SetPasswordVerifier replaces the PAM password check (used by tests).
func (a *Adapter) SetPasswordVerifier(fn PasswordVerifier) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.verifyPassword = fn
}

func (a *Adapter) loadSambaUsers() {
	if _, err := exec.LookPath("pdbedit"); err != nil {
		return
	}
	out, err := exec.Command("pdbedit", "-L").Output()
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) > 0 {
			username := parts[0]
			if u, ok := a.users[username]; ok {
				u.SMBEnabled = true
				a.users[username] = u
			}
		}
	}
}

func (a *Adapter) loadFromLinuxSystem() {
	gidToName := make(map[int32]string)
	if f, err := os.Open("/etc/group"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, ":")
			if len(parts) < 4 {
				continue
			}
			gid, err := strconv.Atoi(parts[2])
			if err != nil {
				continue
			}
			var members []string
			if parts[3] != "" {
				members = strings.Split(parts[3], ",")
			} else {
				members = []string{}
			}
			isSystemGroup := gid < 1000 || gid > 10000
			g := domain.Group{
				ID:          parts[0],
				GroupName:   parts[0],
				GID:         int32(gid),
				Members:     members,
				SystemGroup: isSystemGroup,
			}
			a.groups[g.ID] = g
			gidToName[int32(gid)] = g.GroupName
			if int32(gid) >= a.nextGID && gid <= 10000 {
				a.nextGID = int32(gid) + 1
			}
		}
	}

	if f, err := os.Open("/etc/passwd"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, ":")
			if len(parts) < 7 {
				continue
			}
			uid, err1 := strconv.Atoi(parts[2])
			gid, err2 := strconv.Atoi(parts[3])
			if err1 != nil || err2 != nil {
				continue
			}
			username := parts[0]
			gecos := strings.Split(parts[4], ",")[0]
			homeDir := parts[5]
			shell := parts[6]
			isSystem := uid < 1000 || uid > 10000
			sshEnabled := !strings.HasSuffix(shell, "nologin") && !strings.HasSuffix(shell, "false")

			var suppGroups []string
			for _, grp := range a.groups {
				for _, m := range grp.Members {
					if m == username {
						suppGroups = append(suppGroups, grp.GroupName)
					}
				}
			}
			if suppGroups == nil {
				suppGroups = []string{}
			}

			isAdmin := uid == 0 ||
				containsStr(suppGroups, "wheel") ||
				containsStr(suppGroups, "sudo") ||
				containsStr(suppGroups, "adm") ||
				gidToName[int32(gid)] == "wheel" ||
				gidToName[int32(gid)] == "sudo"

			keys := readAuthorizedKeys(homeDir)
			u := domain.User{
				ID:                  username,
				Username:            username,
				UID:                 int32(uid),
				PrimaryGID:          int32(gid),
				PrimaryGroup:        gidToName[int32(gid)],
				SupplementaryGroups: suppGroups,
				FullName:            gecos,
				HomeDirectory:       homeDir,
				Shell:               shell,
				SSHEnabled:          sshEnabled,
				SSHPasswordAuth:     sshEnabled,
				SSHAuthorizedKeys:   keys,
				Locked:              false,
				SystemAccount:       isSystem,
				IsAdmin:             isAdmin,
			}
			a.users[username] = u
			if int32(uid) >= a.nextUID && uid <= 10000 {
				a.nextUID = int32(uid) + 1
			}
		}
	}
}

func readAuthorizedKeys(homeDir string) []string {
	if homeDir == "" || homeDir == "/" || homeDir == "/nonexistent" {
		return []string{}
	}
	path := filepath.Join(homeDir, ".ssh", "authorized_keys")
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			keys = append(keys, line)
		}
	}
	if keys == nil {
		return []string{}
	}
	return keys
}

func (a *Adapter) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	// Errors are deliberately identical for unknown users, wrong passwords and
	// locked accounts so the login endpoint cannot be used to enumerate users.
	invalid := fmt.Errorf("%w: invalid username or password", domain.ErrUnauthorized)
	if username == "" || password == "" {
		return nil, invalid
	}

	a.mu.RLock()
	verify := a.verifyPassword
	a.mu.RUnlock()
	if verify == nil || verify(username, password) != nil {
		return nil, invalid
	}

	a.mu.RLock()
	user, ok := a.users[username]
	if !ok || user.Locked {
		a.mu.RUnlock()
		return nil, invalid
	}

	cp := user
	cp.Password = ""
	a.mu.RUnlock()

	// Automatically synchronize Samba password upon successful authentication
	if os.Geteuid() == 0 && password != "" {
		a.syncSambaUser(username, password)
	}

	return &cp, nil
}

func (a *Adapter) syncSambaUser(username, password string) {
	if _, err := exec.LookPath("smbpasswd"); err != nil {
		return
	}
	cmd := exec.Command("smbpasswd", "-a", "-s", username)
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s\n%s\n", password, password))
	if err := cmd.Run(); err == nil {
		_ = exec.Command("smbpasswd", "-e", username).Run()
		a.mu.Lock()
		if u, exists := a.users[username]; exists {
			u.SMBEnabled = true
			a.users[username] = u
		}
		a.mu.Unlock()
	}
}

func (a *Adapter) findNextAvailableUIDLocked() int32 {
	used := make(map[int32]bool, len(a.users))
	for _, u := range a.users {
		used[u.UID] = true
	}
	for uid := int32(1000); uid <= 10000; uid++ {
		if !used[uid] {
			return uid
		}
	}
	var maxUID int32 = 1000
	for uid := range used {
		if uid > maxUID && uid < 60000 {
			maxUID = uid
		}
	}
	return maxUID + 1
}

func (a *Adapter) findNextAvailableGIDLocked() int32 {
	used := make(map[int32]bool, len(a.groups))
	for _, g := range a.groups {
		used[g.GID] = true
	}
	for gid := int32(1000); gid <= 10000; gid++ {
		if !used[gid] {
			return gid
		}
	}
	var maxGID int32 = 1000
	for gid := range used {
		if gid > maxGID && gid < 60000 {
			maxGID = gid
		}
	}
	return maxGID + 1
}

func (a *Adapter) ListUsers(ctx context.Context, includeSystem bool) ([]domain.User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var out []domain.User
	for _, u := range a.users {
		if !includeSystem && (u.UID < 1000 || u.UID > 10000 || u.SystemAccount) {
			continue
		}
		cp := u
		cp.Password = ""
		out = append(out, cp)
	}
	return out, nil
}

func (a *Adapter) GetUser(ctx context.Context, username string) (*domain.User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	u, ok := a.users[username]
	if !ok {
		return nil, fmt.Errorf("%w: linux user %q", domain.ErrNotFound, username)
	}
	cp := u
	cp.Password = ""
	return &cp, nil
}

func (a *Adapter) CreateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.users[user.ID]; exists {
		return nil, fmt.Errorf("%w: linux user %q", domain.ErrAlreadyExists, user.ID)
	}

	if user.UID <= 0 {
		user.UID = a.findNextAvailableUIDLocked()
	} else {
		for _, existing := range a.users {
			if existing.UID == user.UID {
				return nil, fmt.Errorf("%w: UID %d is already in use by user %q", domain.ErrAlreadyExists, user.UID, existing.Username)
			}
		}
	}
	if user.PrimaryGroup == "" {
		user.PrimaryGroup = "users"
	}
	if grp, ok := a.groups[user.PrimaryGroup]; ok {
		user.PrimaryGID = grp.GID
	} else {
		user.PrimaryGID = 100
	}
	if user.SupplementaryGroups == nil {
		user.SupplementaryGroups = []string{}
	}
	if user.SSHAuthorizedKeys == nil {
		user.SSHAuthorizedKeys = []string{}
	}
	if user.Password != "" {
		if user.SMBPassword == "" {
			user.SMBPassword = user.Password
			user.SMBEnabled = true
		}
	}

	if os.Geteuid() == 0 {
		args := []string{"-m", "-d", user.HomeDirectory, "-s", user.Shell}
		if user.FullName != "" {
			args = append(args, "-c", user.FullName)
		}
		if len(user.SupplementaryGroups) > 0 {
			args = append(args, "-G", strings.Join(user.SupplementaryGroups, ","))
		}
		args = append(args, user.ID)
		_ = exec.CommandContext(ctx, "useradd", args...).Run()
		if user.Password != "" {
			cmd := exec.CommandContext(ctx, "chpasswd")
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", user.ID, user.Password))
			_ = cmd.Run()
		}
		if len(user.SSHAuthorizedKeys) > 0 {
			writeAuthorizedKeys(user.HomeDirectory, user.SSHAuthorizedKeys)
		}
		if user.SMBPassword != "" {
			cmd := exec.CommandContext(ctx, "smbpasswd", "-a", "-s", user.ID)
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s\n%s\n", user.SMBPassword, user.SMBPassword))
			_ = cmd.Run()
			if !user.SMBEnabled {
				_ = exec.CommandContext(ctx, "smbpasswd", "-d", user.ID).Run()
			}
		}
	}
	if user.SMBPassword != "" {
		user.SMBEnabled = true
	}

	user.Password = ""
	user.SMBPassword = ""
	a.users[user.ID] = user
	cp := user
	return &cp, nil
}

func (a *Adapter) UpdateUser(ctx context.Context, user domain.User, fields []string) (*domain.User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.users[user.ID]
	if !ok {
		return nil, fmt.Errorf("%w: linux user %q", domain.ErrNotFound, user.ID)
	}

	if user.UID > 0 && user.UID != existing.UID {
		for _, other := range a.users {
			if other.ID != user.ID && other.UID == user.UID {
				return nil, fmt.Errorf("%w: UID %d is already in use by user %q", domain.ErrAlreadyExists, user.UID, other.Username)
			}
		}
		existing.UID = user.UID
	}

	if user.FullName != "" {
		existing.FullName = user.FullName
	}
	if user.HomeDirectory != "" {
		existing.HomeDirectory = user.HomeDirectory
	}
	if user.Shell != "" {
		existing.Shell = user.Shell
	}
	if user.PrimaryGroup != "" {
		existing.PrimaryGroup = user.PrimaryGroup
	}
	if user.SupplementaryGroups != nil {
		existing.SupplementaryGroups = user.SupplementaryGroups
	}
	existing.SSHEnabled = user.SSHEnabled
	existing.SSHPasswordAuth = user.SSHPasswordAuth
	if user.SSHAuthorizedKeys != nil {
		existing.SSHAuthorizedKeys = user.SSHAuthorizedKeys
	}
	existing.Locked = user.Locked
	existing.IsAdmin = user.IsAdmin
	if user.Password != "" {
		if user.SMBPassword == "" {
			user.SMBPassword = user.Password
		}
		if os.Geteuid() == 0 {
			cmd := exec.CommandContext(ctx, "chpasswd")
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", user.ID, user.Password))
			_ = cmd.Run()
		}
	}
	if user.SMBPassword != "" {
		existing.SMBEnabled = true
		if os.Geteuid() == 0 {
			cmd := exec.CommandContext(ctx, "smbpasswd", "-a", "-s", user.ID)
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%s\n%s\n", user.SMBPassword, user.SMBPassword))
			_ = cmd.Run()
		}
	}
	if user.SMBEnabled != existing.SMBEnabled {
		existing.SMBEnabled = user.SMBEnabled
		if os.Geteuid() == 0 {
			if existing.SMBEnabled {
				_ = exec.CommandContext(ctx, "smbpasswd", "-e", user.ID).Run()
			} else {
				_ = exec.CommandContext(ctx, "smbpasswd", "-d", user.ID).Run()
			}
		}
	}
	if os.Geteuid() == 0 && len(existing.SSHAuthorizedKeys) > 0 {
		writeAuthorizedKeys(existing.HomeDirectory, existing.SSHAuthorizedKeys)
	}
	a.users[user.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteUser(ctx context.Context, username string, removeHome bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.users[username]; !exists {
		return fmt.Errorf("%w: linux user %q", domain.ErrNotFound, username)
	}
	if os.Geteuid() == 0 {
		_ = exec.CommandContext(ctx, "smbpasswd", "-x", username).Run()
		args := []string{}
		if removeHome {
			args = append(args, "-r")
		}
		args = append(args, username)
		_ = exec.CommandContext(ctx, "userdel", args...).Run()
	}
	delete(a.users, username)
	return nil
}

func (a *Adapter) ListGroups(ctx context.Context, includeSystem bool) ([]domain.Group, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var out []domain.Group
	for _, g := range a.groups {
		if !includeSystem && (g.GID < 1000 || g.GID > 10000 || g.SystemGroup) {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}

func (a *Adapter) GetGroup(ctx context.Context, groupName string) (*domain.Group, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	g, ok := a.groups[groupName]
	if !ok {
		return nil, fmt.Errorf("%w: linux group %q", domain.ErrNotFound, groupName)
	}
	cp := g
	return &cp, nil
}

func (a *Adapter) CreateGroup(ctx context.Context, group domain.Group) (*domain.Group, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.groups[group.ID]; exists {
		return nil, fmt.Errorf("%w: linux group %q", domain.ErrAlreadyExists, group.ID)
	}

	if group.GID <= 0 {
		group.GID = a.findNextAvailableGIDLocked()
	} else {
		for _, existing := range a.groups {
			if existing.GID == group.GID {
				return nil, fmt.Errorf("%w: GID %d is already in use by group %q", domain.ErrAlreadyExists, group.GID, existing.GroupName)
			}
		}
	}

	if group.Members == nil {
		group.Members = []string{}
	}
	if os.Geteuid() == 0 {
		_ = exec.CommandContext(ctx, "groupadd", "-g", strconv.Itoa(int(group.GID)), group.ID).Run()
	}
	a.groups[group.ID] = group
	cp := group
	return &cp, nil
}

func (a *Adapter) UpdateGroup(ctx context.Context, group domain.Group, fields []string) (*domain.Group, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.groups[group.ID]
	if !ok {
		return nil, fmt.Errorf("%w: linux group %q", domain.ErrNotFound, group.ID)
	}

	if group.GID > 0 && group.GID != existing.GID {
		for _, other := range a.groups {
			if other.ID != group.ID && other.GID == group.GID {
				return nil, fmt.Errorf("%w: GID %d is already in use by group %q", domain.ErrAlreadyExists, group.GID, other.GroupName)
			}
		}
		existing.GID = group.GID
	}

	if group.Members != nil {
		existing.Members = group.Members
	}
	a.groups[group.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteGroup(ctx context.Context, groupName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.groups[groupName]; !exists {
		return fmt.Errorf("%w: linux group %q", domain.ErrNotFound, groupName)
	}
	if os.Geteuid() == 0 {
		_ = exec.CommandContext(ctx, "groupdel", groupName).Run()
	}
	delete(a.groups, groupName)
	return nil
}

func writeAuthorizedKeys(homeDir string, keys []string) {
	sshDir := filepath.Join(homeDir, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)
	content := strings.Join(keys, "\n") + "\n"
	_ = os.WriteFile(filepath.Join(sshDir, "authorized_keys"), []byte(content), 0o600)
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
