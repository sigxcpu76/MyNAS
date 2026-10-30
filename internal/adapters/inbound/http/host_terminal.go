package httpadapter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	"golang.org/x/net/websocket"
)

// PasswdUser holds user account information read directly from /etc/passwd and /etc/group.
type PasswdUser struct {
	Username string
	UID      uint32
	GID      uint32
	HomeDir  string
	Shell    string
	Groups   []uint32
}

// LookupPasswdUser looks up a Linux user account in the specified passwd and group files
// (defaulting to /etc/passwd and /etc/group when empty).
func LookupPasswdUser(username, passwdPath, groupPath string) (*PasswdUser, error) {
	if passwdPath == "" {
		passwdPath = "/etc/passwd"
	}
	if groupPath == "" {
		groupPath = "/etc/group"
	}

	pf, err := os.Open(passwdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open passwd file %s: %w", passwdPath, err)
	}
	defer pf.Close()

	var userEntry *PasswdUser
	scanner := bufio.NewScanner(pf)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		if parts[0] == username {
			uid64, err := strconv.ParseUint(parts[2], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid uid %q for user %s: %w", parts[2], username, err)
			}
			gid64, err := strconv.ParseUint(parts[3], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid gid %q for user %s: %w", parts[3], username, err)
			}
			shell := parts[6]
			if shell == "" {
				shell = "/bin/sh"
			}
			userEntry = &PasswdUser{
				Username: username,
				UID:      uint32(uid64),
				GID:      uint32(gid64),
				HomeDir:  parts[5],
				Shell:    shell,
				Groups:   []uint32{uint32(gid64)},
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed reading passwd file: %w", err)
	}
	if userEntry == nil {
		return nil, fmt.Errorf("user %q not found in %s", username, passwdPath)
	}

	// Supplementary groups from group file
	if gf, err := os.Open(groupPath); err == nil {
		defer gf.Close()
		gScanner := bufio.NewScanner(gf)
		for gScanner.Scan() {
			gLine := strings.TrimSpace(gScanner.Text())
			if gLine == "" || strings.HasPrefix(gLine, "#") {
				continue
			}
			gParts := strings.Split(gLine, ":")
			if len(gParts) < 4 {
				continue
			}
			gid64, err := strconv.ParseUint(gParts[2], 10, 32)
			if err != nil {
				continue
			}
			gid := uint32(gid64)
			members := strings.Split(gParts[3], ",")
			for _, m := range members {
				if strings.TrimSpace(m) == username {
					addUniqueGID(userEntry, gid)
					break
				}
			}
		}
	}

	// Also consult OS user lookup if available on the current host
	if sysUser, err := osuser.Lookup(username); err == nil {
		if gids, err := sysUser.GroupIds(); err == nil {
			for _, gStr := range gids {
				if gVal, err := strconv.ParseUint(gStr, 10, 32); err == nil {
					addUniqueGID(userEntry, uint32(gVal))
				}
			}
		}
	}

	return userEntry, nil
}

func addUniqueGID(u *PasswdUser, gid uint32) {
	for _, existing := range u.Groups {
		if existing == gid {
			return
		}
	}
	u.Groups = append(u.Groups, gid)
}

func (g *Gateway) handleHostTerminalWebSocket(ws *websocket.Conn) {
	defer ws.Close()
	req := ws.Request()

	caller := grpcadapter.CallerFromContext(req.Context())
	if caller == nil || caller.Username == "" {
		_, _ = ws.Write([]byte("Authentication required\r\n"))
		return
	}

	passwdPath := g.passwdPath
	if passwdPath == "" {
		passwdPath = "/etc/passwd"
	}
	groupPath := g.groupPath
	if groupPath == "" {
		groupPath = "/etc/group"
	}

	passwdUser, err := LookupPasswdUser(caller.Username, passwdPath, groupPath)
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("Failed to resolve user %s in %s: %v\r\n", caller.Username, passwdPath, err)))
		return
	}

	shell := passwdUser.Shell
	if _, err := exec.LookPath(shell); err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("User shell %q not found or not executable: %v\r\n", shell, err)))
		return
	}

	shellBase := filepath.Base(shell)
	var cmdArgs []string
	if shellBase == "bash" || shellBase == "sh" || shellBase == "zsh" || shellBase == "fish" || shellBase == "csh" || shellBase == "tcsh" || shellBase == "ksh" {
		cmdArgs = []string{shell, "-l"}
	} else {
		cmdArgs = []string{shell}
	}

	cmd := exec.CommandContext(req.Context(), cmdArgs[0], cmdArgs[1:]...)

	homeDir := passwdUser.HomeDir
	if homeDir != "" {
		if fi, err := os.Stat(homeDir); err == nil && fi.IsDir() {
			cmd.Dir = homeDir
		} else {
			cmd.Dir = "/"
		}
	} else {
		cmd.Dir = "/"
	}

	cmd.Env = []string{
		"TERM=xterm-256color",
		"HOME=" + homeDir,
		"USER=" + passwdUser.Username,
		"LOGNAME=" + passwdUser.Username,
		"SHELL=" + shell,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
	}

	// Drop privileges to the authenticated Linux user
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid:    passwdUser.UID,
				Gid:    passwdUser.GID,
				Groups: passwdUser.Groups,
			},
		}
	} else if uint32(os.Geteuid()) != passwdUser.UID {
		_, _ = ws.Write([]byte(fmt.Sprintf("Permission error: daemon running as UID %d cannot impersonate user %s (UID %d)\r\n", os.Geteuid(), passwdUser.Username, passwdUser.UID)))
		return
	}

	ws.PayloadType = websocket.BinaryFrame

	ptmx, err := pty.Start(cmd)
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("Failed to spawn host shell (%s): %v\r\n", shell, err)))
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	done := make(chan struct{}, 2)

	// Pipe shell output to WebSocket
	go func() {
		_, _ = io.Copy(ws, ptmx)
		done <- struct{}{}
	}()

	// Pipe WebSocket client input to shell PTY and handle terminal resize
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				break
			}
			data := buf[:n]
			if bytes.HasPrefix(data, []byte(`{"type":"resize"`)) {
				var resizeMsg struct {
					Cols uint `json:"cols"`
					Rows uint `json:"rows"`
				}
				if err := json.Unmarshal(data, &resizeMsg); err == nil && resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Rows: uint16(resizeMsg.Rows),
						Cols: uint16(resizeMsg.Cols),
					})
					continue
				}
			}
			if _, err := ptmx.Write(data); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
}
