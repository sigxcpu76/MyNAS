package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigxcpu/nas"
	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	httpadapter "github.com/sigxcpu/nas/internal/adapters/inbound/http"
	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/kvm"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	"golang.org/x/net/websocket"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()

	prereqSvc := application.NewPrerequisiteService(prereq.NewSystemChecker(true))
	zfsAdapter := zfs.NewAdapter(true)
	zfsSvc := application.NewZfsService(zfsAdapter)
	diskSvc := application.NewDiskService(disk.NewAdapter(zfsAdapter, true))
	kvmSvc := application.NewKvmService(kvm.NewAdapter(true))
	dockerAdapter := docker.NewAdapter(true)
	dockerSvc := application.NewDockerService(dockerAdapter)
	dockerAppSvc := application.NewDockerAppService(dockerAdapter)
	lxcSvc := application.NewLxcService(lxc.NewAdapter(true))
	webdavAdapter := share.NewWebdavAdapter(true)
	shareSvc := application.NewShareService(share.NewCifsAdapter(true, t.TempDir()), webdavAdapter)
	identitySvc := application.NewIdentityService(identity.NewAdapter(true), "test-jwt-secret")
	catalogSvc := application.NewCatalogService(catalog.NewAdapter(t.TempDir()))

	grpcSrv := grpcadapter.NewServer(prereqSvc, diskSvc, zfsSvc, kvmSvc, dockerSvc, dockerAppSvc, lxcSvc, shareSvc, identitySvc, catalogSvc)
	uiFS, err := nas.WebUIFS()
	if err != nil {
		t.Fatalf("failed to load embedded UI FS: %v", err)
	}
	eventBus := application.NewEventBus()
	gw, err := httpadapter.NewGateway(ctx, grpcSrv, kvmSvc, webdavAdapter, uiFS, eventBus)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}
	return httptest.NewServer(gw)
}

func TestGoogleAIPGatewayEndToEnd(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	client := srv.Client()

	// 1. Auth custom method POST /v1/auth:login
	loginResp, err := client.Post(srv.URL+"/v1/auth:login", "application/json", strings.NewReader(`{"username":"sigxcpu","password":"admin123"}`))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on :login, got %d", loginResp.StatusCode)
	}
	var loginData map[string]any
	if err := json.NewDecoder(loginResp.Body).Decode(&loginData); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginData["accessToken"] == "" {
		t.Fatalf("expected non-empty accessToken")
	}

	// 2. Prerequisites List & :check custom method
	prereqResp, err := client.Post(srv.URL+"/v1/prerequisites:check", "application/json", strings.NewReader(`{}`))
	if err != nil || prereqResp.StatusCode != http.StatusOK {
		t.Fatalf("prerequisites:check failed: status=%v err=%v", prereqResp.StatusCode, err)
	}
	_ = prereqResp.Body.Close()

	// 3. ZFS Pool Scrub custom method & Dataset Snapshot custom method
	scrubResp, err := client.Post(srv.URL+"/v1/zfsPools/tank:scrub", "application/json", strings.NewReader(`{}`))
	if err != nil || scrubResp.StatusCode != http.StatusOK {
		t.Fatalf("zfsPools/tank:scrub failed: status=%v err=%v", scrubResp.StatusCode, err)
	}
	_ = scrubResp.Body.Close()

	snapResp, err := client.Post(srv.URL+"/v1/zfsPools/tank/datasets/media:snapshot", "application/json", strings.NewReader(`{"snapshotName":"e2e-snap"}`))
	if err != nil || snapResp.StatusCode != http.StatusOK {
		t.Fatalf("datasets/media:snapshot failed: status=%v err=%v", snapResp.StatusCode, err)
	}
	_ = snapResp.Body.Close()

	// 4. ZFS Dataset PATCH with FieldMask
	patchReq, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/zfsPools/tank/datasets/media?updateMask=compression", strings.NewReader(`{"compression":"zstd"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := client.Do(patchReq)
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH dataset failed: status=%v err=%v", patchResp.StatusCode, err)
	}
	_ = patchResp.Body.Close()

	// 4b. Docker App creation (verifying protojson accepts clean camelCase JSON payload without duplicate field error)
	appBody := `{"title":"Nextcloud Hub","composeYaml":"services:\n  app:\n    image: nextcloud:stable\n"}`
	appResp, err := client.Post(srv.URL+"/v1/dockerApps?dockerAppId=nextcloud", "application/json", strings.NewReader(appBody))
	if err != nil || appResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(appResp.Body)
		t.Fatalf("createDockerApp failed: status=%v body=%s err=%v", appResp.StatusCode, string(body), err)
	}
	_ = appResp.Body.Close()

	// 5. KVM Virtual Machine :openConsole & WebSocket RFB 003.008 noVNC handshake
	consoleResp, err := client.Post(srv.URL+"/v1/virtualMachines/ubuntu-24-lts:openConsole", "application/json", strings.NewReader(`{}`))
	if err != nil || consoleResp.StatusCode != http.StatusOK {
		t.Fatalf("openConsole failed: status=%v err=%v", consoleResp.StatusCode, err)
	}
	var consoleData struct {
		Token        string `json:"token"`
		WebsocketURL string `json:"websocketUrl"`
	}
	_ = json.NewDecoder(consoleResp.Body).Decode(&consoleData)
	_ = consoleResp.Body.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + consoleData.WebsocketURL
	ws, err := websocket.Dial(wsURL, "", srv.URL)
	if err != nil {
		t.Fatalf("failed to dial noVNC websocket %s: %v", wsURL, err)
	}
	defer ws.Close()
	rfbBanner := make([]byte, 12)
	if _, err := io.ReadFull(ws, rfbBanner); err != nil {
		t.Fatalf("failed to read RFB banner from noVNC websocket: %v", err)
	}
	if string(rfbBanner) != "RFB 003.008\n" {
		t.Fatalf("expected RFB 003.008\\n banner, got %q", string(rfbBanner))
	}

	// 6. WebDAV live file upload & download (/dav/documents/hello.txt)
	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/dav/documents/hello.txt", bytes.NewBufferString("Hello from NAS WebDAV!"))
	putResp, err := client.Do(putReq)
	if err != nil || (putResp.StatusCode != http.StatusCreated && putResp.StatusCode != http.StatusNoContent) {
		t.Fatalf("WebDAV PUT failed: status=%v err=%v", putResp.StatusCode, err)
	}
	_ = putResp.Body.Close()

	getDavResp, err := client.Get(srv.URL + "/dav/documents/hello.txt")
	if err != nil || getDavResp.StatusCode != http.StatusOK {
		t.Fatalf("WebDAV GET failed: status=%v err=%v", getDavResp.StatusCode, err)
	}
	davContent, _ := io.ReadAll(getDavResp.Body)
	_ = getDavResp.Body.Close()
	if string(davContent) != "Hello from NAS WebDAV!" {
		t.Fatalf("unexpected WebDAV content: %q", string(davContent))
	}

	// 6b. Real-Time Server-Sent Events (SSE) notification stream
	eventsResp, err := client.Get(srv.URL + "/v1/events")
	if err != nil || eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/events SSE failed: status=%v err=%v", eventsResp.StatusCode, err)
	}
	buf := make([]byte, 128)
	n, _ := eventsResp.Body.Read(buf)
	_ = eventsResp.Body.Close()
	if !strings.Contains(string(buf[:n]), "connected") {
		t.Fatalf("expected SSE connected handshake, got %q", string(buf[:n]))
	}

	// 7. Embedded Single-Binary Vue.js SPA served at /
	spaResp, err := client.Get(srv.URL + "/")
	if err != nil || spaResp.StatusCode != http.StatusOK {
		t.Fatalf("SPA GET / failed: status=%v err=%v", spaResp.StatusCode, err)
	}
	spaHTML, _ := io.ReadAll(spaResp.Body)
	_ = spaResp.Body.Close()
	if !strings.Contains(string(spaHTML), "<div id=\"app\"></div>") {
		t.Fatalf("expected embedded Vue index.html, got: %s", string(spaHTML))
	}

	// 8. Docker storage config & filesystem directories API
	cfgResp, err := client.Get(srv.URL + "/v1/dockerConfig")
	if err != nil || cfgResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/dockerConfig failed: status=%v err=%v", cfgResp.StatusCode, err)
	}
	_ = cfgResp.Body.Close()

	fsResp, err := client.Get(srv.URL + "/v1/filesystem:directories?path=/tank")
	if err != nil || fsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/filesystem:directories failed: status=%v err=%v", fsResp.StatusCode, err)
	}
	_ = fsResp.Body.Close()
}
