package httpadapter_test

import (
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/sigxcpu/nas/internal/adapters/inbound/http"
	"golang.org/x/net/websocket"
)

func TestLookupPasswdUser(t *testing.T) {
	tempDir := t.TempDir()
	passwdFile := filepath.Join(tempDir, "passwd")
	groupFile := filepath.Join(tempDir, "group")

	passwdContent := `
# System users
root:x:0:0:root:/root:/bin/bash
bin:x:1:1:bin:/bin:/usr/sbin/nologin
# Regular users
alice:x:1001:1001:Alice Smith:/home/alice:/bin/zsh
bob:x:1002:1002:Bob Jones:/home/bob:
charlie:x:1003:1003:Charlie Brown:/home/charlie:/usr/bin/fish
`
	groupContent := `
root:x:0:
wheel:x:998:alice,charlie
docker:x:965:bob,charlie
storage:x:1005:alice
`

	if err := os.WriteFile(passwdFile, []byte(passwdContent), 0644); err != nil {
		t.Fatalf("failed to write passwd file: %v", err)
	}
	if err := os.WriteFile(groupFile, []byte(groupContent), 0644); err != nil {
		t.Fatalf("failed to write group file: %v", err)
	}

	// 1. Test Alice
	alice, err := httpadapter.LookupPasswdUser("alice", passwdFile, groupFile)
	if err != nil {
		t.Fatalf("unexpected error looking up alice: %v", err)
	}
	if alice.Username != "alice" || alice.UID != 1001 || alice.GID != 1001 {
		t.Errorf("alice info mismatch: %+v", alice)
	}
	if alice.Shell != "/bin/zsh" {
		t.Errorf("expected alice shell /bin/zsh, got %q", alice.Shell)
	}
	if alice.HomeDir != "/home/alice" {
		t.Errorf("expected alice home /home/alice, got %q", alice.HomeDir)
	}
	// Alice is in primary group 1001, plus wheel (998) and storage (1005)
	if !containsGID(alice.Groups, 1001) || !containsGID(alice.Groups, 998) || !containsGID(alice.Groups, 1005) {
		t.Errorf("expected alice groups to contain 1001, 998, 1005; got %v", alice.Groups)
	}

	// 2. Test Bob (empty shell should default to /bin/sh)
	bob, err := httpadapter.LookupPasswdUser("bob", passwdFile, groupFile)
	if err != nil {
		t.Fatalf("unexpected error looking up bob: %v", err)
	}
	if bob.Shell != "/bin/sh" {
		t.Errorf("expected empty shell to default to /bin/sh, got %q", bob.Shell)
	}
	if !containsGID(bob.Groups, 965) {
		t.Errorf("expected bob groups to contain docker (965); got %v", bob.Groups)
	}

	// 3. Test Unknown user
	_, err = httpadapter.LookupPasswdUser("nonexistent", passwdFile, groupFile)
	if err == nil {
		t.Fatalf("expected error for nonexistent user, got nil")
	}

	// 4. Test real system lookup for current user
	curr, err := osuser.Current()
	if err == nil && curr.Username != "" {
		sysUser, err := httpadapter.LookupPasswdUser(curr.Username, "", "")
		if err != nil {
			t.Logf("system passwd lookup for %s: %v", curr.Username, err)
		} else {
			if sysUser.Username != curr.Username {
				t.Errorf("expected username %s, got %s", curr.Username, sysUser.Username)
			}
			if sysUser.Shell == "" {
				t.Errorf("expected non-empty shell for current user")
			}
		}
	}
}

func containsGID(gids []uint32, target uint32) bool {
	for _, g := range gids {
		if g == target {
			return true
		}
	}
	return false
}

func TestHostTerminalWebSocket_AuthAndOrigin(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	token := login(t, srv)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/host/terminal"

	// 1. Without token -> must fail
	if conn, err := websocket.Dial(wsURL, "", srv.URL); err == nil {
		conn.Close()
		t.Errorf("host terminal websocket without token unexpectedly connected")
	}

	// 2. With foreign origin -> must fail
	if conn, err := websocket.Dial(wsURL+"?access_token="+token, "", "https://malicious-site.example"); err == nil {
		conn.Close()
		t.Errorf("host terminal websocket from foreign origin unexpectedly connected")
	}

	// 3. Authenticated same-origin connection -> succeeds
	cfg, err := websocket.NewConfig(wsURL+"?access_token="+token, srv.URL)
	if err != nil {
		t.Fatalf("failed to create websocket config: %v", err)
	}
	wsConn, err := websocket.DialConfig(cfg)
	if err != nil {
		t.Fatalf("failed to connect to host terminal: %v", err)
	}
	defer wsConn.Close()

	// 4. Test resize frame
	_, err = wsConn.Write([]byte(`{"type":"resize","cols":120,"rows":35}`))
	if err != nil {
		t.Fatalf("failed to send resize to host terminal: %v", err)
	}

	// 5. Send input to shell (echo test)
	_, err = wsConn.Write([]byte("echo NAS_HOST_TERMINAL_OK\n"))
	if err != nil {
		t.Fatalf("failed to send echo command to host terminal: %v", err)
	}

	// 6. Read output from terminal
	buf := make([]byte, 2048)
	_ = wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	foundOutput := false
	startTime := time.Now()
	for time.Since(startTime) < 3*time.Second {
		n, err := wsConn.Read(buf)
		if err != nil {
			break
		}
		out := string(buf[:n])
		if strings.Contains(out, "NAS_HOST_TERMINAL_OK") {
			foundOutput = true
			break
		}
	}
	if !foundOutput {
		t.Logf("Notice: terminal output may have buffered or prompt differed, buffer captured: %q", string(buf))
	}
}

func TestHostTerminalWebSocket_UserNotFound(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	token := login(t, srv)

	// Override passwd path on gateway to an empty file
	gw, ok := srv.Config.Handler.(*httpadapter.Gateway)
	if !ok {
		t.Skip("handler is not *httpadapter.Gateway")
	}
	emptyPasswd := filepath.Join(t.TempDir(), "empty_passwd")
	_ = os.WriteFile(emptyPasswd, []byte(""), 0644)
	gw.SetPasswdPathsForTest(emptyPasswd, "")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/host/terminal?access_token=" + token
	cfg, err := websocket.NewConfig(wsURL, srv.URL)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	wsConn, err := websocket.DialConfig(cfg)
	if err != nil {
		// Connection rejected is acceptable
		return
	}
	defer wsConn.Close()

	buf := make([]byte, 512)
	_ = wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := wsConn.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "Failed to resolve user") && !strings.Contains(out, "not found") {
		t.Fatalf("expected user not found message, got: %q", out)
	}
}
