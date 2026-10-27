package share

import (
	"bufio"
	"bytes"
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

type CifsAdapter struct {
	mu           sync.RWMutex
	configPath   string
	shares       map[string]domain.CifsShare
	globalConfig domain.CifsGlobalConfig
}

func NewCifsAdapter(stateDir string) *CifsAdapter {
	cfgPath := "/etc/samba/smb.nas.conf"
	if os.Geteuid() != 0 && stateDir != "" {
		_ = os.MkdirAll(stateDir, 0o755)
		cfgPath = filepath.Join(stateDir, "smb.nas.conf")
	}

	a := &CifsAdapter{
		configPath:   cfgPath,
		shares:       make(map[string]domain.CifsShare),
		globalConfig: domain.DefaultCifsGlobalConfig(),
	}

	a.loadSambaConfig()
	if os.Geteuid() == 0 {
		a.ensureIncludeInSmbConf()
	}
	return a
}

func (a *CifsAdapter) ensureIncludeInSmbConf() {
	mainConf := "/etc/samba/smb.conf"
	data, err := os.ReadFile(mainConf)
	if err != nil {
		return
	}
	content := string(data)
	target := fmt.Sprintf("include = %s", a.configPath)

	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		if strings.Contains(line, a.configPath) || strings.Contains(line, "Auto-included by NAS Daemon") {
			continue
		}
		cleaned = append(cleaned, line)
	}

	// Insert right before the first non-global section header
	var result []string
	inserted := false
	for _, line := range cleaned {
		trimmed := strings.TrimSpace(line)
		if !inserted && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && !strings.EqualFold(trimmed, "[global]") {
			result = append(result, "   # Auto-included by NAS Daemon", fmt.Sprintf("   %s", target), "")
			inserted = true
		}
		result = append(result, line)
	}
	if !inserted {
		result = append(result, "", "   # Auto-included by NAS Daemon", fmt.Sprintf("   %s", target))
	}

	_ = os.WriteFile(mainConf, []byte(strings.Join(result, "\n")), 0o644)
}

func (a *CifsAdapter) loadSambaConfig() {
	if a.configPath == "" {
		return
	}
	f, err := os.Open(a.configPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	currentSection := ""
	currentShare := domain.CifsShare{
		CreateMask:         "0664",
		DirectoryMask:      "0775",
		ForceCreateMode:    "0664",
		ForceDirectoryMode: "0775",
		Browseable:         true,
		Enabled:            true,
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Save previous share if was a share
			if currentSection != "" && currentSection != "global" && currentShare.ShareName != "" {
				currentShare.ID = currentShare.ShareName
				a.shares[currentShare.ID] = currentShare
			}

			currentSection = strings.Trim(line, "[]")
			currentShare = domain.CifsShare{
				ShareName:          currentSection,
				CreateMask:         "0664",
				DirectoryMask:      "0775",
				ForceCreateMode:    "0664",
				ForceDirectoryMode: "0775",
				Browseable:         true,
				Enabled:            true,
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		if currentSection == "global" {
			switch key {
			case "workgroup":
				a.globalConfig.Workgroup = val
			case "server string":
				a.globalConfig.ServerString = val
			case "security":
				a.globalConfig.Security = val
			case "map to guest":
				a.globalConfig.MapToGuest = val
			case "guest account":
				a.globalConfig.GuestAccount = val
			case "max log size":
				if n, err := strconv.Atoi(val); err == nil {
					a.globalConfig.MaxLogSize = int32(n)
				}
			case "disable spoolss":
				a.globalConfig.DisableSpoolss = parseBoolVal(val)
			case "fruit:aapl":
				a.globalConfig.FruitAAPL = parseBoolVal(val)
			}
		} else if currentSection != "" {
			switch key {
			case "path":
				currentShare.Path = val
			case "comment":
				currentShare.Comment = val
			case "read only":
				currentShare.ReadOnly = parseBoolVal(val)
			case "guest ok", "public":
				currentShare.GuestOk = parseBoolVal(val)
			case "guest only":
				currentShare.GuestOnly = parseBoolVal(val)
			case "guest account":
				currentShare.GuestAccount = val
			case "browseable":
				currentShare.Browseable = parseBoolVal(val)
			case "create mask":
				currentShare.CreateMask = val
			case "directory mask":
				currentShare.DirectoryMask = val
			case "force create mode":
				currentShare.ForceCreateMode = val
			case "force directory mode":
				currentShare.ForceDirectoryMode = val
			case "force user":
				currentShare.ForceUser = val
			case "force group":
				currentShare.ForceGroup = val
			case "valid users":
				var users, groups []string
				for _, p := range strings.Fields(val) {
					if strings.HasPrefix(p, "@") {
						groups = append(groups, strings.TrimPrefix(p, "@"))
					} else {
						users = append(users, p)
					}
				}
				currentShare.ValidUsers = users
				currentShare.ValidGroups = groups
			case "read list":
				currentShare.ReadList = strings.Fields(val)
			case "write list":
				currentShare.WriteList = strings.Fields(val)
			case "vfs objects":
				if strings.Contains(val, "shadow_copy2") {
					currentShare.ShadowCopy = true
				}
				if strings.Contains(val, "fruit") {
					currentShare.TimeMachine = true
				}
				if strings.Contains(val, "recycle") {
					currentShare.RecycleBin = true
				}
			}
		}
	}

	if currentSection != "" && currentSection != "global" && currentShare.ShareName != "" {
		currentShare.ID = currentShare.ShareName
		a.shares[currentShare.ID] = currentShare
	}
}

func parseBoolVal(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "yes" || v == "true" || v == "1"
}

func (a *CifsAdapter) ListShares(ctx context.Context) ([]domain.CifsShare, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.CifsShare, 0, len(a.shares))
	for _, s := range a.shares {
		out = append(out, s)
	}
	return out, nil
}

func (a *CifsAdapter) GetShare(ctx context.Context, id string) (*domain.CifsShare, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s, ok := a.shares[id]
	if !ok {
		return nil, fmt.Errorf("%w: cifs share %q", domain.ErrNotFound, id)
	}
	cp := s
	return &cp, nil
}

func (a *CifsAdapter) CreateShare(ctx context.Context, share domain.CifsShare) (*domain.CifsShare, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.shares[share.ID]; exists {
		return nil, fmt.Errorf("%w: cifs share %q", domain.ErrAlreadyExists, share.ID)
	}
	if share.Path != "" {
		_ = os.MkdirAll(share.Path, 0o775)
	}
	a.shares[share.ID] = share
	_ = a.syncSambaConfig()
	cp := share
	return &cp, nil
}

func (a *CifsAdapter) UpdateShare(ctx context.Context, share domain.CifsShare, fields []string) (*domain.CifsShare, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.shares[share.ID]
	if !ok {
		return nil, fmt.Errorf("%w: cifs share %q", domain.ErrNotFound, share.ID)
	}
	if share.Path != "" {
		existing.Path = share.Path
	}
	if share.Comment != "" {
		existing.Comment = share.Comment
	}
	existing.ReadOnly = share.ReadOnly
	existing.GuestOk = share.GuestOk
	existing.GuestOnly = share.GuestOnly
	if share.GuestAccount != "" {
		existing.GuestAccount = share.GuestAccount
	}
	existing.Browseable = share.Browseable
	existing.Enabled = share.Enabled
	if share.ValidUsers != nil {
		existing.ValidUsers = share.ValidUsers
	}
	if share.ValidGroups != nil {
		existing.ValidGroups = share.ValidGroups
	}
	if share.ReadList != nil {
		existing.ReadList = share.ReadList
	}
	if share.WriteList != nil {
		existing.WriteList = share.WriteList
	}
	if share.ForceUser != "" {
		existing.ForceUser = share.ForceUser
	}
	if share.ForceGroup != "" {
		existing.ForceGroup = share.ForceGroup
	}
	if share.CreateMask != "" {
		existing.CreateMask = share.CreateMask
	}
	if share.DirectoryMask != "" {
		existing.DirectoryMask = share.DirectoryMask
	}
	if share.ForceCreateMode != "" {
		existing.ForceCreateMode = share.ForceCreateMode
	}
	if share.ForceDirectoryMode != "" {
		existing.ForceDirectoryMode = share.ForceDirectoryMode
	}
	existing.ShadowCopy = share.ShadowCopy
	existing.TimeMachine = share.TimeMachine
	existing.RecycleBin = share.RecycleBin

	a.shares[share.ID] = existing
	_ = a.syncSambaConfig()
	cp := existing
	return &cp, nil
}

func (a *CifsAdapter) DeleteShare(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.shares[id]; !exists {
		return fmt.Errorf("%w: cifs share %q", domain.ErrNotFound, id)
	}
	delete(a.shares, id)
	_ = a.syncSambaConfig()
	return nil
}

func (a *CifsAdapter) GetGlobalConfig(ctx context.Context) (*domain.CifsGlobalConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cp := a.globalConfig
	return &cp, nil
}

func (a *CifsAdapter) UpdateGlobalConfig(ctx context.Context, config domain.CifsGlobalConfig, fields []string) (*domain.CifsGlobalConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if config.Workgroup != "" {
		a.globalConfig.Workgroup = config.Workgroup
	}
	if config.ServerString != "" {
		a.globalConfig.ServerString = config.ServerString
	}
	if config.Security != "" {
		a.globalConfig.Security = config.Security
	}
	if config.MapToGuest != "" {
		a.globalConfig.MapToGuest = config.MapToGuest
	}
	if config.GuestAccount != "" {
		a.globalConfig.GuestAccount = config.GuestAccount
	}
	if config.MaxLogSize > 0 {
		a.globalConfig.MaxLogSize = config.MaxLogSize
	}
	a.globalConfig.DisableSpoolss = config.DisableSpoolss
	a.globalConfig.FruitAAPL = config.FruitAAPL
	a.globalConfig.WSDDEnabled = config.WSDDEnabled

	_ = a.syncSambaConfig()
	cp := a.globalConfig
	return &cp, nil
}

func (a *CifsAdapter) syncSambaConfig() error {
	var buf bytes.Buffer
	buf.WriteString("# Auto-generated by NAS Management Daemon\n")
	buf.WriteString("# Do not edit manually; managed through NAS UI\n\n")

	// [global] Section
	buf.WriteString("[global]\n")
	workgroup := a.globalConfig.Workgroup
	if workgroup == "" {
		workgroup = "WORKGROUP"
	}
	fmt.Fprintf(&buf, "   workgroup = %s\n", workgroup)
	if a.globalConfig.ServerString != "" {
		fmt.Fprintf(&buf, "   server string = %s\n", a.globalConfig.ServerString)
	}
	security := a.globalConfig.Security
	if security == "" {
		security = "user"
	}
	fmt.Fprintf(&buf, "   security = %s\n", security)
	mapToGuest := a.globalConfig.MapToGuest
	if mapToGuest == "" {
		mapToGuest = "Bad User"
	}
	fmt.Fprintf(&buf, "   map to guest = %s\n", mapToGuest)
	guestAcc := a.globalConfig.GuestAccount
	if guestAcc == "" {
		guestAcc = "nobody"
	}
	fmt.Fprintf(&buf, "   guest account = %s\n", guestAcc)
	maxLog := a.globalConfig.MaxLogSize
	if maxLog <= 0 {
		maxLog = 50
	}
	fmt.Fprintf(&buf, "   max log size = %d\n", maxLog)
	if a.globalConfig.DisableSpoolss {
		buf.WriteString("   load printers = no\n")
		buf.WriteString("   printing = bsd\n")
		buf.WriteString("   printcap name = /dev/null\n")
		buf.WriteString("   disable spoolss = yes\n")
	}

	// Sensible default masks for group collaboration across shares
	buf.WriteString("   create mask = 0664\n")
	buf.WriteString("   directory mask = 0775\n")
	buf.WriteString("   force create mode = 0664\n")
	buf.WriteString("   force directory mode = 0775\n")

	// Global Fruit / macOS support
	if a.globalConfig.FruitAAPL {
		buf.WriteString("   fruit:aapl = yes\n")
		buf.WriteString("   fruit:model = Macmini\n")
		buf.WriteString("   fruit:metadata = stream\n")
		buf.WriteString("   fruit:veto_appledouble = no\n")
		buf.WriteString("   fruit:posix_rename = yes\n")
		buf.WriteString("   fruit:zero_file_id = yes\n")
		buf.WriteString("   fruit:wipe_intentionally_left_blank_rfork = yes\n")
		buf.WriteString("   fruit:delete_empty_adfiles = yes\n")
	}
	buf.WriteString("\n")

	// Individual Share Sections
	for _, s := range a.shares {
		if !s.Enabled {
			continue
		}
		fmt.Fprintf(&buf, "[%s]\n", s.ShareName)
		fmt.Fprintf(&buf, "   path = %s\n", s.Path)
		if s.Comment != "" {
			fmt.Fprintf(&buf, "   comment = %s\n", s.Comment)
		}
		fmt.Fprintf(&buf, "   read only = %s\n", yesNo(s.ReadOnly))
		fmt.Fprintf(&buf, "   guest ok = %s\n", yesNo(s.GuestOk))
		if s.GuestOnly {
			fmt.Fprintf(&buf, "   guest only = yes\n")
		}
		fmt.Fprintf(&buf, "   browseable = %s\n", yesNo(s.Browseable))

		// Permissions & Access Lists
		var principals []string
		principals = append(principals, s.ValidUsers...)
		for _, g := range s.ValidGroups {
			principals = append(principals, "@"+strings.TrimPrefix(g, "@"))
		}
		if len(principals) > 0 {
			fmt.Fprintf(&buf, "   valid users = %s\n", strings.Join(principals, " "))
		}
		if len(s.ReadList) > 0 {
			fmt.Fprintf(&buf, "   read list = %s\n", strings.Join(s.ReadList, " "))
		}
		if len(s.WriteList) > 0 {
			fmt.Fprintf(&buf, "   write list = %s\n", strings.Join(s.WriteList, " "))
		}
		if s.ForceUser != "" {
			fmt.Fprintf(&buf, "   force user = %s\n", s.ForceUser)
		}
		if s.ForceGroup != "" {
			fmt.Fprintf(&buf, "   force group = %s\n", s.ForceGroup)
		}

		// Masks
		createMask := s.CreateMask
		if createMask == "" {
			createMask = "0664"
		}
		fmt.Fprintf(&buf, "   create mask = %s\n", createMask)

		dirMask := s.DirectoryMask
		if dirMask == "" {
			dirMask = "0775"
		}
		fmt.Fprintf(&buf, "   directory mask = %s\n", dirMask)

		forceCreate := s.ForceCreateMode
		if forceCreate == "" {
			forceCreate = "0664"
		}
		fmt.Fprintf(&buf, "   force create mode = %s\n", forceCreate)

		forceDir := s.ForceDirectoryMode
		if forceDir == "" {
			forceDir = "0775"
		}
		fmt.Fprintf(&buf, "   force directory mode = %s\n", forceDir)

		// VFS modules
		var vfs []string
		if s.TimeMachine || a.globalConfig.FruitAAPL {
			vfs = append(vfs, "catia", "fruit", "streams_xattr")
		}
		if s.ShadowCopy {
			vfs = append(vfs, "shadow_copy2")
		}
		if s.RecycleBin {
			vfs = append(vfs, "recycle")
		}

		if len(vfs) > 0 {
			fmt.Fprintf(&buf, "   vfs objects = %s\n", strings.Join(vfs, " "))
		}

		if s.TimeMachine {
			fmt.Fprintf(&buf, "   fruit:time machine = yes\n")
			fmt.Fprintf(&buf, "   fruit:time machine max size = 0\n")
		}

		if s.ShadowCopy {
			fmt.Fprintf(&buf, "   shadow:snapdir = .zfs/snapshot\n")
			fmt.Fprintf(&buf, "   shadow:sort = desc\n")
			fmt.Fprintf(&buf, "   shadow:format = @%%Y%%m%%d_%%H%%M%%S\n")
			fmt.Fprintf(&buf, "   shadow:localtime = yes\n")
		}

		if s.RecycleBin {
			fmt.Fprintf(&buf, "   recycle:repository = .recycle/%%U\n")
			fmt.Fprintf(&buf, "   recycle:keeptree = yes\n")
			fmt.Fprintf(&buf, "   recycle:versions = yes\n")
			fmt.Fprintf(&buf, "   recycle:touch = yes\n")
			fmt.Fprintf(&buf, "   recycle:directory_mode = 0775\n")
			fmt.Fprintf(&buf, "   recycle:subdir_mode = 0775\n")
		}

		buf.WriteString("\n")
	}

	if a.configPath != "" {
		_ = os.WriteFile(a.configPath, buf.Bytes(), 0o644)
	}

	if os.Geteuid() == 0 {
		if _, err := exec.LookPath("smbcontrol"); err == nil {
			_ = exec.Command("smbcontrol", "all", "reload-config").Run()
		}
		a.syncAvahiDiscovery()
		a.syncWSDD()
	}
	return nil
}

func (a *CifsAdapter) syncAvahiDiscovery() {
	avahiDir := "/etc/avahi/services"
	if _, err := os.Stat(avahiDir); err != nil {
		return
	}

	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" standalone='no'?>\n")
	buf.WriteString("<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n")
	buf.WriteString("<service-group>\n")
	buf.WriteString("  <name replace-wildcards=\"yes\">%h (Samba)</name>\n")
	buf.WriteString("  <service>\n")
	buf.WriteString("    <type>_smb._tcp</type>\n")
	buf.WriteString("    <port>445</port>\n")
	buf.WriteString("  </service>\n")
	buf.WriteString("  <service>\n")
	buf.WriteString("    <type>_device-info._tcp</type>\n")
	buf.WriteString("    <port>0</port>\n")
	buf.WriteString("    <txt-record>model=RackMac</txt-record>\n")
	buf.WriteString("  </service>\n")

	var tmShares []string
	for _, s := range a.shares {
		if s.Enabled && s.TimeMachine {
			tmShares = append(tmShares, s.ShareName)
		}
	}
	if len(tmShares) > 0 {
		buf.WriteString("  <service>\n")
		buf.WriteString("    <type>_adisk._tcp</type>\n")
		buf.WriteString("    <port>9</port>\n")
		buf.WriteString("    <txt-record>sys=waMa=0,adVF=0x100</txt-record>\n")
		for i, tm := range tmShares {
			fmt.Fprintf(&buf, "    <txt-record>dk%d=adVN=%s,adVF=0x82</txt-record>\n", i, tm)
		}
		buf.WriteString("  </service>\n")
	}

	buf.WriteString("</service-group>\n")
	_ = os.WriteFile(filepath.Join(avahiDir, "smb.service"), buf.Bytes(), 0o644)

	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "try-reload-or-restart", "avahi-daemon").Run()
	}
}

func (a *CifsAdapter) syncWSDD() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}
	wsddBin, err := exec.LookPath("wsdd")
	serviceName := "wsdd"
	if err != nil {
		if bin2, err2 := exec.LookPath("wsdd2"); err2 == nil {
			wsddBin = bin2
			serviceName = "wsdd2"
		} else {
			return
		}
	}

	if serviceName == "wsdd" {
		servicePath := "/etc/systemd/system/wsdd.service"
		if _, err := os.Stat(servicePath); err != nil {
			unitContent := `[Unit]
Description=Web Services Dynamic Discovery host daemon
After=network.target

[Service]
Type=simple
ExecStart=` + wsddBin + ` -s
Restart=on-failure
User=nobody
Group=nogroup

[Install]
WantedBy=multi-user.target
`
			if err := os.WriteFile(servicePath, []byte(unitContent), 0o644); err == nil {
				_ = exec.Command("systemctl", "daemon-reload").Run()
				_ = exec.Command("systemctl", "enable", "wsdd").Run()
			}
		}
	} else if serviceName == "wsdd2" {
		servicePath := "/etc/systemd/system/wsdd2.service"
		// If neither /etc/systemd/system/wsdd2.service nor /lib/systemd/system/wsdd2.service exists, create one
		_, errEtc := os.Stat(servicePath)
		_, errLib := os.Stat("/lib/systemd/system/wsdd2.service")
		_, errUsrLib := os.Stat("/usr/lib/systemd/system/wsdd2.service")
		if errEtc != nil && errLib != nil && errUsrLib != nil {
			unitContent := `[Unit]
Description=Web Services Dynamic Discovery host daemon (wsdd2)
After=network.target

[Service]
Type=simple
ExecStart=` + wsddBin + ` -d
Restart=on-failure

[Install]
WantedBy=multi-user.target
`
			if err := os.WriteFile(servicePath, []byte(unitContent), 0o644); err == nil {
				_ = exec.Command("systemctl", "daemon-reload").Run()
			}
		}
		_ = exec.Command("systemctl", "enable", "wsdd2").Run()
	}

	if a.globalConfig.WSDDEnabled {
		_ = exec.Command("systemctl", "start", serviceName).Run()
	} else {
		_ = exec.Command("systemctl", "stop", serviceName).Run()
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
