package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	osuser "os/user"
	"strings"
	"testing"
	"time"

	"github.com/sigxcpu/nas"
	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	httpadapter "github.com/sigxcpu/nas/internal/adapters/inbound/http"
	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
	"golang.org/x/net/websocket"
)

type stubZfsPort struct {
	ports.ZfsEnginePort
	pools    map[string]domain.ZfsPool
	datasets map[string]domain.ZfsDataset
}

func (s *stubZfsPort) GetPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	p, ok := s.pools[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &p, nil
}

func (s *stubZfsPort) ListPools(ctx context.Context) ([]domain.ZfsPool, error) {
	var res []domain.ZfsPool
	for _, p := range s.pools {
		res = append(res, p)
	}
	return res, nil
}

func (s *stubZfsPort) ScrubPool(ctx context.Context, id string) (*domain.ZfsPool, error) {
	p, ok := s.pools[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	p.LastScrubStatus = "Scrub completed"
	s.pools[id] = p
	return &p, nil
}

func (s *stubZfsPort) GetDataset(ctx context.Context, poolID, datasetID string) (*domain.ZfsDataset, error) {
	key := poolID + "/" + datasetID
	ds, ok := s.datasets[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &ds, nil
}

func (s *stubZfsPort) SnapshotDataset(ctx context.Context, poolID, datasetID, snapName string) (*domain.ZfsDataset, error) {
	key := poolID + "/" + datasetID
	ds, ok := s.datasets[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	ds.SnapshotsCount++
	s.datasets[key] = ds
	return &ds, nil
}

func (s *stubZfsPort) UpdateDataset(ctx context.Context, dataset domain.ZfsDataset, fields []string) (*domain.ZfsDataset, error) {
	key := dataset.PoolID + "/" + dataset.ID
	s.datasets[key] = dataset
	return &dataset, nil
}

type stubKvmPort struct {
	ports.KvmHypervisorPort
	vms      map[string]domain.VirtualMachine
	sessions map[string]domain.ConsoleSession
}

func (k *stubKvmPort) OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error) {
	token := "valid-token"
	sess := domain.ConsoleSession{
		VMName:       id,
		Token:        token,
		WebsocketURL: fmt.Sprintf("/ws/novnc/%s?token=%s", id, token),
		ExpiresAt:    time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	}
	k.sessions[id] = sess
	return &sess, nil
}

func (k *stubKvmPort) ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error) {
	sess, ok := k.sessions[vmID]
	if !ok || sess.Token != token {
		return nil, domain.ErrPermissionDenied
	}
	vm, ok := k.vms[vmID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &vm, nil
}

func (k *stubKvmPort) DialVNC(ctx context.Context, vmID string) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = server.Write([]byte("RFB 003.008\n"))
	}()
	return client, nil
}

func (k *stubKvmPort) RebootVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	vm, ok := k.vms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	vm.State = "RUNNING"
	k.vms[id] = vm
	return &vm, nil
}

func (k *stubKvmPort) StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	vm, ok := k.vms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	vm.State = "STOPPED"
	k.vms[id] = vm
	return &vm, nil
}

func (k *stubKvmPort) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	var res []domain.VirtualMachine
	for _, vm := range k.vms {
		res = append(res, vm)
	}
	return res, nil
}

type stubDockerPort struct {
	ports.DockerRuntimePort
	ports.DockerAppPort
	ports.DockerConfigPort
	containers map[string]domain.DockerContainer
	apps       map[string]domain.DockerApp
	config     domain.DockerConfig
}

func (d *stubDockerPort) ListContainers(ctx context.Context) ([]domain.DockerContainer, error) {
	var res []domain.DockerContainer
	for _, c := range d.containers {
		res = append(res, c)
	}
	return res, nil
}

func (d *stubDockerPort) ListApps(ctx context.Context) ([]domain.DockerApp, error) {
	var res []domain.DockerApp
	for _, a := range d.apps {
		res = append(res, a)
	}
	return res, nil
}

func (d *stubDockerPort) GetApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	if a, ok := d.apps[id]; ok {
		return &a, nil
	}
	return nil, domain.ErrNotFound
}

func (d *stubDockerPort) CreateApp(ctx context.Context, app domain.DockerApp) (*domain.DockerApp, error) {
	app.State = "RUNNING"
	d.apps[app.ID] = app
	return &app, nil
}

func (d *stubDockerPort) GetConfig(ctx context.Context) (*domain.DockerConfig, error) {
	return &d.config, nil
}

func (d *stubDockerPort) ListDirectories(ctx context.Context, reqPath string) (*domain.DirectoryList, error) {
	return &domain.DirectoryList{
		CurrentPath: reqPath,
		Entries: []domain.DirectoryEntry{
			{Name: "appdata", Path: reqPath + "/appdata", IsDir: true},
		},
	}, nil
}

func (d *stubDockerPort) ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error) {
	c1, c2 := net.Pipe()
	go func() {
		defer c2.Close()
		_, _ = c2.Write([]byte("stub container shell session\r\n$ "))
	}()
	return c1, "stub-exec-id", nil
}

func (d *stubDockerPort) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	return nil
}

const testPassword = "test-password"

// bearerTransport attaches the session token to every request, like the web UI does.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func testLoginUsername() string {
	if curr, err := osuser.Current(); err == nil && curr.Username != "" {
		return curr.Username
	}
	return "root"
}

// login authenticates against the test server and returns the access token.
func login(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	payload := fmt.Sprintf(`{"username":%q,"password":%q}`, testLoginUsername(), testPassword)
	resp, err := srv.Client().Post(srv.URL+"/v1/auth:login", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on :login, got %d", resp.StatusCode)
	}
	var data struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || data.AccessToken == "" {
		t.Fatalf("expected non-empty accessToken: %v", err)
	}
	return data.AccessToken
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()

	prereqChecker := prereq.NewSystemChecker()
	prereqChecker.SetAllModulesInstalledForTest()
	prereqChecker.SetMainPool("tank")
	prereqSvc := application.NewPrerequisiteService(prereqChecker)

	zfsPort := &stubZfsPort{
		pools: map[string]domain.ZfsPool{
			"tank": {ID: "tank", PoolName: "tank", State: "ONLINE", Mountpoint: "/mnt/tank"},
		},
		datasets: map[string]domain.ZfsDataset{
			"tank/media": {PoolID: "tank", ID: "media", DatasetPath: "tank/media", Mountpoint: "/mnt/tank/media"},
		},
	}
	systemSvc := application.NewSystemService(system.NewAdapter(zfsPort, prereqChecker))
	zfsSvc := application.NewZfsService(zfsPort)
	diskSvc := application.NewDiskService(disk.NewAdapter(zfsPort))

	kvmPort := &stubKvmPort{
		vms: map[string]domain.VirtualMachine{
			"ubuntu-24-lts": {ID: "ubuntu-24-lts", VMName: "Ubuntu 24.04 LTS", State: "RUNNING"},
		},
		sessions: make(map[string]domain.ConsoleSession),
	}
	kvmSvc := application.NewKvmService(kvmPort)

	dockerPort := &stubDockerPort{
		containers: make(map[string]domain.DockerContainer),
		apps:       make(map[string]domain.DockerApp),
		config: domain.DockerConfig{
			Pool:            "tank",
			Dataset:         "tank/apps",
			AppdataPath:     "/mnt/tank/apps",
			DatasetSelected: true,
		},
	}
	dockerSvc := application.NewDockerService(dockerPort)
	dockerAppSvc := application.NewDockerAppService(dockerPort)
	lxcAdapter := lxc.NewAdapter()
	_, _ = lxcAdapter.CreateContainer(ctx, domain.LxcContainer{
		ID:            "alpine-linux",
		ContainerName: "Alpine Linux",
		Distribution:  "alpine",
		Release:       "3.19",
		Architecture:  "amd64",
		State:         "RUNNING",
	})
	lxcSvc := application.NewLxcService(lxcAdapter)
	webdavAdapter := share.NewWebdavAdapter()
	webdavDir := t.TempDir()
	_, _ = webdavAdapter.CreateShare(ctx, domain.WebdavShare{
		ID:      "documents",
		Path:    webdavDir,
		Enabled: true,
	})
	shareSvc := application.NewShareService(share.NewCifsAdapter(t.TempDir()), webdavAdapter)
	identityAdapter := identity.NewAdapter()
	identityAdapter.SetPasswordVerifier(func(username, password string) error {
		if password != testPassword {
			return fmt.Errorf("bad password")
		}
		return nil
	})
	identitySvc := application.NewIdentityService(identityAdapter, "test-jwt-secret")
	catalogSvc := application.NewCatalogService(catalog.NewAdapter(t.TempDir()))

	grpcSrv := grpcadapter.NewServer(systemSvc, prereqSvc, diskSvc, zfsSvc, kvmSvc, dockerSvc, dockerAppSvc, lxcSvc, shareSvc, identitySvc, catalogSvc)
	uiFS, err := nas.WebUIFS()
	if err != nil {
		t.Fatalf("failed to load embedded UI FS: %v", err)
	}
	eventBus := application.NewEventBus()
	storageRepoSvc := application.NewStorageRepoService(nil, t.TempDir())
	gw, err := httpadapter.NewGateway(ctx, grpcSrv, identitySvc, kvmSvc, dockerSvc, lxcSvc, storageRepoSvc, webdavAdapter, uiFS, eventBus)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}
	return httptest.NewServer(gw)
}

func TestGoogleAIPGatewayEndToEnd(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	// 1. Auth custom method POST /v1/auth:login; every later call carries the token
	token := login(t, srv)
	client := &http.Client{Transport: &bearerTransport{token: token, base: srv.Client().Transport}}

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

	// 5b. KVM Virtual Machine :reboot and :stop with forced option
	rebootResp, err := client.Post(srv.URL+"/v1/virtualMachines/ubuntu-24-lts:reboot", "application/json", strings.NewReader(`{"force": true}`))
	if err != nil || rebootResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rebootResp.Body)
		t.Fatalf("reboot with force failed: status=%v body=%s err=%v", rebootResp.StatusCode, string(body), err)
	}
	_ = rebootResp.Body.Close()

	stopResp, err := client.Post(srv.URL+"/v1/virtualMachines/ubuntu-24-lts:stop", "application/json", strings.NewReader(`{"force": true}`))
	if err != nil || stopResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(stopResp.Body)
		t.Fatalf("stop with force failed: status=%v body=%s err=%v", stopResp.StatusCode, string(body), err)
	}
	_ = stopResp.Body.Close()

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

	// 6c. Real-Time gRPC Server-Side Streaming Watch API (/v1/dockerApps:watch)
	watchResp, err := client.Get(srv.URL + "/v1/dockerApps:watch")
	if err != nil || watchResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/dockerApps:watch failed: status=%v err=%v", watchResp.StatusCode, err)
	}
	watchBuf := make([]byte, 512)
	wn, _ := watchResp.Body.Read(watchBuf)
	_ = watchResp.Body.Close()
	if !strings.Contains(string(watchBuf[:wn]), "SNAPSHOT") && !strings.Contains(string(watchBuf[:wn]), "result") {
		t.Fatalf("expected initial SNAPSHOT chunk from watch stream, got: %s", string(watchBuf[:wn]))
	}

	// 6d. Real-Time Multiplexed Watch Stream (/v1/resources:watch)
	resWatchResp, err := client.Get(srv.URL + "/v1/resources:watch?resourceTypes=dockerApps,virtualMachines,telemetry")
	if err != nil || resWatchResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/resources:watch failed: status=%v err=%v", resWatchResp.StatusCode, err)
	}
	resWatchBuf := make([]byte, 512)
	rwn, _ := resWatchResp.Body.Read(resWatchBuf)
	_ = resWatchResp.Body.Close()
	if !strings.Contains(string(resWatchBuf[:rwn]), "SNAPSHOT") {
		t.Fatalf("expected initial SNAPSHOT chunk from resources watch stream, got: %s", string(resWatchBuf[:rwn]))
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

	// 8. Docker Interactive Terminal WebSocket (/ws/docker/terminal/{containerId})
	dockerWsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/docker/terminal/c1?shell=/bin/sh&access_token=" + token
	wsConfig, wsErr := websocket.NewConfig(dockerWsURL, srv.URL)
	if wsErr != nil {
		t.Fatalf("failed to create ws config: %v", wsErr)
	}
	var wsConn *websocket.Conn
	wsConn, wsErr = websocket.DialConfig(wsConfig)
	if wsErr != nil {
		t.Fatalf("failed to connect to docker terminal ws: %v", wsErr)
	}
	defer wsConn.Close()

	wsBuf := make([]byte, 256)
	_ = wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	wsN, err := wsConn.Read(wsBuf)
	if err != nil {
		t.Fatalf("failed to read from docker terminal ws: %v", err)
	}
	if !strings.Contains(string(wsBuf[:wsN]), "stub container shell") {
		t.Fatalf("expected stub shell prompt, got: %s", string(wsBuf[:wsN]))
	}

	// Test sending resize JSON frame
	_, err = wsConn.Write([]byte(`{"type":"resize","cols":120,"rows":35}`))
	if err != nil {
		t.Fatalf("failed to send resize to ws: %v", err)
	}

	// 8b. LXC Interactive Terminal WebSocket (/ws/lxc/terminal/{containerId})
	// Test connecting with display name containing spaces (e.g. "Alpine Linux") to verify resolution to technical ID "alpine-linux"
	lxcWsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/lxc/terminal/Alpine%20Linux?shell=/bin/sh&access_token=" + token
	lxcWsConfig, lxcWsErr := websocket.NewConfig(lxcWsURL, srv.URL)
	if lxcWsErr != nil {
		t.Fatalf("failed to create lxc ws config: %v", lxcWsErr)
	}
	lxcWsConn, lxcWsErr := websocket.DialConfig(lxcWsConfig)
	if lxcWsErr != nil {
		t.Fatalf("failed to connect to lxc terminal ws with spaces: %v", lxcWsErr)
	}
	defer lxcWsConn.Close()

	lxcWsBuf := make([]byte, 256)
	_ = lxcWsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	lxcWsN, err := lxcWsConn.Read(lxcWsBuf)
	if err != nil {
		t.Fatalf("failed to read from lxc terminal ws: %v", err)
	}
	if !strings.Contains(string(lxcWsBuf[:lxcWsN]), "alpine-linux") {
		t.Fatalf("expected resolved lxc container alpine-linux in output, got: %s", string(lxcWsBuf[:lxcWsN]))
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

	// 9. ISOs Repository API
	isoListResp, err := client.Get(srv.URL + "/v1/storage/isos")
	if err != nil || isoListResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/storage/isos failed: status=%v err=%v", isoListResp.StatusCode, err)
	}
	_ = isoListResp.Body.Close()

	// Upload ISO
	var isoBody bytes.Buffer
	isoWriter := multipart.NewWriter(&isoBody)
	part, err := isoWriter.CreateFormFile("file", "debian-12.8.0.iso")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("fake-iso-bytes-data"))
	_ = isoWriter.Close()

	uploadReq, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/storage/isos/upload", &isoBody)
	if err != nil {
		t.Fatalf("failed to create upload request: %v", err)
	}
	uploadReq.Header.Set("Content-Type", isoWriter.FormDataContentType())
	uploadResp, err := client.Do(uploadReq)
	if err != nil || uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/storage/isos/upload failed: status=%v err=%v", uploadResp.StatusCode, err)
	}
	_ = uploadResp.Body.Close()

	// Verify listed
	isoListResp2, err := client.Get(srv.URL + "/v1/storage/isos")
	if err != nil || isoListResp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/storage/isos after upload failed: status=%v err=%v", isoListResp2.StatusCode, err)
	}
	var isoListResult struct {
		ISOs []domain.StorageImage `json:"isos"`
	}
	_ = json.NewDecoder(isoListResp2.Body).Decode(&isoListResult)
	_ = isoListResp2.Body.Close()
	if len(isoListResult.ISOs) == 0 || isoListResult.ISOs[0].Name != "debian-12.8.0.iso" {
		t.Fatalf("expected debian-12.8.0.iso in list, got %+v", isoListResult.ISOs)
	}

	// Delete ISO
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/storage/isos?filename=debian-12.8.0.iso", nil)
	delResp, err := client.Do(delReq)
	if err != nil || delResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /v1/storage/isos failed: status=%v err=%v", delResp.StatusCode, err)
	}
	_ = delResp.Body.Close()

	// 10. LXC Templates Repository API
	tmplListResp, err := client.Get(srv.URL + "/v1/storage/lxc-templates")
	if err != nil || tmplListResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/storage/lxc-templates failed: status=%v err=%v", tmplListResp.StatusCode, err)
	}
	_ = tmplListResp.Body.Close()

	// Upload LXC Template
	var tmplBody bytes.Buffer
	tmplWriter := multipart.NewWriter(&tmplBody)
	tPart, err := tmplWriter.CreateFormFile("file", "ubuntu-24.04-rootfs.tar.xz")
	if err != nil {
		t.Fatalf("failed to create tmpl form file: %v", err)
	}
	_, _ = tPart.Write([]byte("fake-tar-xz-bytes-data"))
	_ = tmplWriter.Close()

	tmplUpReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/storage/lxc-templates/upload", &tmplBody)
	tmplUpReq.Header.Set("Content-Type", tmplWriter.FormDataContentType())
	tmplUpResp, err := client.Do(tmplUpReq)
	if err != nil || tmplUpResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/storage/lxc-templates/upload failed: status=%v err=%v", tmplUpResp.StatusCode, err)
	}
	_ = tmplUpResp.Body.Close()

	// Verify template listed
	tmplListResp2, err := client.Get(srv.URL + "/v1/storage/lxc-templates")
	if err != nil || tmplListResp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/storage/lxc-templates after upload failed: status=%v err=%v", tmplListResp2.StatusCode, err)
	}
	var tmplListResult struct {
		Templates []domain.StorageImage `json:"templates"`
	}
	_ = json.NewDecoder(tmplListResp2.Body).Decode(&tmplListResult)
	_ = tmplListResp2.Body.Close()
	if len(tmplListResult.Templates) == 0 || tmplListResult.Templates[0].Name != "ubuntu-24.04-rootfs.tar.xz" {
		t.Fatalf("expected ubuntu-24.04-rootfs.tar.xz in list, got %+v", tmplListResult.Templates)
	}

	// Delete LXC Template
	tDelReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/storage/lxc-templates?filename=ubuntu-24.04-rootfs.tar.xz", nil)
	tDelResp, err := client.Do(tDelReq)
	if err != nil || tDelResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /v1/storage/lxc-templates failed: status=%v err=%v", tDelResp.StatusCode, err)
	}
	_ = tDelResp.Body.Close()
}

func TestWatchStreamingWithDeltas(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	client := &http.Client{Transport: &bearerTransport{token: login(t, srv), base: srv.Client().Transport}}

	// 1. Open DockerApps watch stream
	watchResp, err := client.Get(srv.URL + "/v1/dockerApps:watch")
	if err != nil || watchResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/dockerApps:watch failed: status=%v err=%v", watchResp.StatusCode, err)
	}
	defer watchResp.Body.Close()

	dec := json.NewDecoder(watchResp.Body)

	// Read initial SNAPSHOT message
	var snapMsg struct {
		Result struct {
			EventType string `json:"eventType"`
		} `json:"result"`
	}
	if err := dec.Decode(&snapMsg); err != nil {
		t.Fatalf("failed to decode initial snapshot: %v", err)
	}
	if snapMsg.Result.EventType != "SNAPSHOT" {
		t.Fatalf("expected SNAPSHOT eventType, got %q", snapMsg.Result.EventType)
	}

	// 2. Trigger creation of a new app in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		appBody := `{"title":"Stream Test","composeYaml":"services:\n  test:\n    image: alpine\n"}`
		_, _ = client.Post(srv.URL+"/v1/dockerApps?dockerAppId=stream-test", "application/json", strings.NewReader(appBody))
	}()

	// 3. Read delta event from stream
	var deltaMsg struct {
		Result struct {
			EventType string `json:"eventType"`
			DockerApp struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			} `json:"dockerApp"`
		} `json:"result"`
	}
	if err := dec.Decode(&deltaMsg); err != nil {
		t.Fatalf("failed to decode delta event: %v", err)
	}
	if deltaMsg.Result.EventType != "ADDED" && deltaMsg.Result.EventType != "MODIFIED" {
		t.Fatalf("expected ADDED/MODIFIED eventType, got %q", deltaMsg.Result.EventType)
	}
	if deltaMsg.Result.DockerApp.Name != "dockerApps/stream-test" {
		t.Fatalf("expected dockerApps/stream-test, got %q", deltaMsg.Result.DockerApp.Name)
	}
}
