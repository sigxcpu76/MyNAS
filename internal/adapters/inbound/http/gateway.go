package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"golang.org/x/net/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Gateway struct {
	mux           *http.ServeMux
	grpcServer    nasv1.NasServiceServer
	verifier      grpcadapter.TokenVerifier
	innerServer   *grpc.Server
	innerConn     *grpc.ClientConn
	kvmService    *application.KvmService
	dockerService *application.DockerService
	lxcService    *application.LxcService
	storageRepo   *application.StorageRepoService
	webdavHandler http.Handler
	uiFS          fs.FS
	eventBus      *application.EventBus
	shutdownCh    chan struct{}
	shutdownOnce  sync.Once
}

// maxGatewayMsgSize bounds REST payloads relayed through the in-process gRPC server.
const maxGatewayMsgSize = 256 << 20

func NewGateway(
	ctx context.Context,
	grpcServer nasv1.NasServiceServer,
	verifier grpcadapter.TokenVerifier,
	kvmService *application.KvmService,
	dockerService *application.DockerService,
	lxcService *application.LxcService,
	storageRepo *application.StorageRepoService,
	webdavHandler http.Handler,
	uiFS fs.FS,
	eventBus *application.EventBus,
) (*Gateway, error) {
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		}),
	)

	type eventBusSetter interface {
		SetEventBus(bus *application.EventBus)
	}
	if setter, ok := grpcServer.(eventBusSetter); ok && eventBus != nil {
		setter.SetEventBus(eventBus)
	}

	// REST calls are relayed through an in-process gRPC server so they pass the
	// same authentication and ID validation interceptors as native gRPC clients.
	innerServer := grpc.NewServer(append(grpcadapter.ServerOptions(verifier),
		grpc.MaxRecvMsgSize(maxGatewayMsgSize),
		grpc.MaxSendMsgSize(maxGatewayMsgSize),
	)...)
	nasv1.RegisterNasServiceServer(innerServer, grpcServer)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = innerServer.Serve(lis) }()
	innerConn, err := grpc.NewClient("passthrough:///nas-gateway",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGatewayMsgSize),
			grpc.MaxCallSendMsgSize(maxGatewayMsgSize),
		),
	)
	if err != nil {
		innerServer.Stop()
		return nil, err
	}
	if err := nasv1.RegisterNasServiceHandlerClient(ctx, gwMux, nasv1.NewNasServiceClient(innerConn)); err != nil {
		_ = innerConn.Close()
		innerServer.Stop()
		return nil, err
	}

	mux := http.NewServeMux()
	g := &Gateway{
		mux:           mux,
		grpcServer:    grpcServer,
		verifier:      verifier,
		innerServer:   innerServer,
		innerConn:     innerConn,
		kvmService:    kvmService,
		dockerService: dockerService,
		lxcService:    lxcService,
		storageRepo:   storageRepo,
		webdavHandler: webdavHandler,
		uiFS:          uiFS,
		eventBus:      eventBus,
		shutdownCh:    make(chan struct{}),
	}

	// 1. Storage Repositories (ISOs & LXC Templates)
	mux.Handle("/v1/storage/isos/upload", g.requireAuth(http.HandlerFunc(g.handleUploadISO)))
	mux.Handle("/v1/storage/isos", g.requireAuth(http.HandlerFunc(g.handleISOs)))
	mux.Handle("/v1/storage/isos/", g.requireAuth(http.HandlerFunc(g.handleISOs)))
	mux.Handle("/v1/storage/lxc-templates/upload", g.requireAuth(http.HandlerFunc(g.handleUploadLxcTemplate)))
	mux.Handle("/v1/storage/lxc-templates", g.requireAuth(http.HandlerFunc(g.handleLxcTemplates)))
	mux.Handle("/v1/storage/lxc-templates/", g.requireAuth(http.HandlerFunc(g.handleLxcTemplates)))

	// 2. Real-Time System Notification Stream (Server-Sent Events)
	mux.Handle("/v1/events", g.requireAuth(http.HandlerFunc(g.handleEventsSSE)))
	mux.Handle("/events", g.requireAuth(http.HandlerFunc(g.handleEventsSSE)))

	// 2b. Reactive gRPC Server-Side Streaming / Watch Handlers over HTTP
	mux.Handle("/v1/dockerApps:watch", g.requireAuth(http.HandlerFunc(g.handleWatchDockerApps)))
	mux.Handle("/v1/dockerContainers:watch", g.requireAuth(http.HandlerFunc(g.handleWatchDockerContainers)))
	mux.Handle("/v1/virtualMachines:watch", g.requireAuth(http.HandlerFunc(g.handleWatchVirtualMachines)))
	mux.Handle("/v1/lxcContainers:watch", g.requireAuth(http.HandlerFunc(g.handleWatchLxcContainers)))
	mux.Handle("/v1/system:telemetryStream", g.requireAuth(http.HandlerFunc(g.handleStreamSystemTelemetry)))
	mux.Handle("/v1/resources:watch", g.requireAuth(http.HandlerFunc(g.handleWatchResources)))

	// 3. Google AIP REST/JSON API via gRPC-Gateway (authenticated by the gRPC interceptors)
	mux.Handle("/v1/", noStoreMiddleware(gwMux))

	// 4. KVM noVNC WebSocket-to-RFB Proxy (/ws/novnc/{vmName}?token={token}).
	// Authorized by the single-VM console token minted by the authenticated :openConsole call.
	mux.Handle("/ws/novnc/", websocket.Server{
		Handshake: sameOriginHandshake,
		Handler:   g.handleNoVNCWebSocket,
	})

	// 5. Docker Container Interactive Terminal WebSocket (/ws/docker/terminal/{containerId}?shell=/bin/sh&access_token=...)
	mux.Handle("/ws/docker/terminal/", g.requireAuth(websocket.Server{
		Handshake: sameOriginHandshake,
		Handler:   g.handleDockerTerminalWebSocket,
	}))

	// 6. LXC Container Interactive Terminal WebSocket (/ws/lxc/terminal/{containerId}?shell=/bin/sh&access_token=...)
	mux.Handle("/ws/lxc/terminal/", g.requireAuth(websocket.Server{
		Handshake: sameOriginHandshake,
		Handler:   g.handleLxcTerminalWebSocket,
	}))

	// 7. Embedded WebDAV File Sharing Server (/dav/{share_name}/...), protected by HTTP Basic Auth
	if webdavHandler != nil {
		mux.Handle("/dav/", webdavHandler)
	}

	// 8. Embedded Vue.js Single-Page Application (SPA)
	mux.HandleFunc("/", g.handleSPA)

	return g, nil
}

func (g *Gateway) handleISOs(w http.ResponseWriter, r *http.Request) {
	if g.storageRepo == nil {
		http.Error(w, `{"error":"storage repository service unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		isos, err := g.storageRepo.ListISOs(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"isos": isos})

	case http.MethodDelete:
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			clean := strings.TrimPrefix(r.URL.Path, "/v1/storage/isos/")
			clean = strings.TrimPrefix(clean, "/v1/storage/isos")
			filename = strings.Trim(clean, "/")
		}
		if filename == "" {
			http.Error(w, `{"error":"filename query parameter or path is required"}`, http.StatusBadRequest)
			return
		}
		if err := g.storageRepo.DeleteISO(r.Context(), filename); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleUploadISO(w http.ResponseWriter, r *http.Request) {
	if g.storageRepo == nil {
		http.Error(w, `{"error":"storage repository service unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Support up to 64GB streaming uploads with 32MB in-memory buffer
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse multipart form: %v"}`, err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"'file' field is required in multipart form"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	iso, err := g.storageRepo.SaveISO(r.Context(), header.Filename, file)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"iso": iso})
}

func (g *Gateway) handleLxcTemplates(w http.ResponseWriter, r *http.Request) {
	if g.storageRepo == nil {
		http.Error(w, `{"error":"storage repository service unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		templates, err := g.storageRepo.ListLxcTemplates(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"templates": templates})

	case http.MethodDelete:
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			clean := strings.TrimPrefix(r.URL.Path, "/v1/storage/lxc-templates/")
			clean = strings.TrimPrefix(clean, "/v1/storage/lxc-templates")
			filename = strings.Trim(clean, "/")
		}
		if filename == "" {
			http.Error(w, `{"error":"filename query parameter or path is required"}`, http.StatusBadRequest)
			return
		}
		if err := g.storageRepo.DeleteLxcTemplate(r.Context(), filename); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleUploadLxcTemplate(w http.ResponseWriter, r *http.Request) {
	if g.storageRepo == nil {
		http.Error(w, `{"error":"storage repository service unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse multipart form: %v"}`, err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"'file' field is required in multipart form"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpl, err := g.storageRepo.SaveLxcTemplate(r.Context(), header.Filename, file)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"template": tmpl})
}

func (g *Gateway) handleEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Handshake event
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	if g.eventBus == nil {
		<-r.Context().Done()
		return
	}

	eventsCh := g.eventBus.Subscribe(r.Context())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-g.shutdownCh:
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case evt, ok := <-eventsCh:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mux.ServeHTTP(w, r)
}

// Close gracefully terminates all active SSE and streaming connections.
func (g *Gateway) Close() {
	if g == nil {
		return
	}
	g.shutdownOnce.Do(func() {
		close(g.shutdownCh)
		if g.innerConn != nil {
			_ = g.innerConn.Close()
		}
		if g.innerServer != nil {
			g.innerServer.Stop()
		}
	})
	if g.eventBus != nil {
		g.eventBus.Close()
	}
}

func (g *Gateway) handleNoVNCWebSocket(ws *websocket.Conn) {
	defer ws.Close()
	req := ws.Request()
	vmID := strings.TrimPrefix(req.URL.Path, "/ws/novnc/")
	vmID = strings.Trim(vmID, "/")
	if domain.ValidateResourceID(vmID, "virtual machine") != nil {
		return
	}
	token := req.URL.Query().Get("token")

	if _, err := g.kvmService.ValidateConsoleToken(req.Context(), vmID, token); err != nil {
		return
	}
	vncConn, err := g.kvmService.DialVNC(req.Context(), vmID)
	if err != nil {
		return
	}
	defer vncConn.Close()

	ws.PayloadType = websocket.BinaryFrame
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(ws, vncConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(vncConn, ws)
		done <- struct{}{}
	}()
	<-done
}

func (g *Gateway) handleDockerTerminalWebSocket(ws *websocket.Conn) {
	defer ws.Close()
	req := ws.Request()
	containerID := strings.TrimPrefix(req.URL.Path, "/ws/docker/terminal/")
	containerID = strings.Trim(containerID, "/")
	if err := domain.ValidateResourceID(containerID, "container"); err != nil {
		_, _ = ws.Write([]byte(err.Error() + "\r\n"))
		return
	}

	shell := req.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
	}
	if !shellPathRegex.MatchString(shell) {
		_, _ = ws.Write([]byte("Invalid shell path\r\n"))
		return
	}

	ws.PayloadType = websocket.BinaryFrame

	if g.dockerService == nil {
		_, _ = ws.Write([]byte("Docker service unavailable\r\n"))
		return
	}

	conn, execID, err := g.dockerService.ExecAttach(req.Context(), containerID, []string{shell})
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("Failed to exec into container %s (%s): %v\r\n", containerID, shell, err)))
		return
	}
	defer conn.Close()

	done := make(chan struct{}, 2)

	// Pipe container output (stdout/stderr) to WebSocket
	go func() {
		_, _ = io.Copy(ws, conn)
		done <- struct{}{}
	}()

	// Pipe WebSocket client input to container stdin and handle terminal resize
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
					_ = g.dockerService.ResizeExec(req.Context(), execID, resizeMsg.Rows, resizeMsg.Cols)
					continue
				}
			}
			if _, err := conn.Write(data); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
}

func (g *Gateway) handleLxcTerminalWebSocket(ws *websocket.Conn) {
	defer ws.Close()
	req := ws.Request()
	containerID := strings.TrimPrefix(req.URL.Path, "/ws/lxc/terminal/")
	containerID = strings.Trim(containerID, "/")
	if err := domain.ValidateResourceID(containerID, "container"); err != nil {
		_, _ = ws.Write([]byte(err.Error() + "\r\n"))
		return
	}

	shell := req.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
	}
	if !shellPathRegex.MatchString(shell) {
		_, _ = ws.Write([]byte("Invalid shell path\r\n"))
		return
	}

	ws.PayloadType = websocket.BinaryFrame

	if g.lxcService == nil {
		_, _ = ws.Write([]byte("LXC service unavailable\r\n"))
		return
	}

	conn, execID, err := g.lxcService.ExecAttach(req.Context(), containerID, []string{shell})
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("Failed to attach to LXC container %s (%s): %v\r\n", containerID, shell, err)))
		return
	}
	defer conn.Close()

	done := make(chan struct{}, 2)

	// Pipe container output (stdout/stderr) to WebSocket
	go func() {
		_, _ = io.Copy(ws, conn)
		done <- struct{}{}
	}()

	// Pipe WebSocket client input to container stdin and handle terminal resize
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
					_ = g.lxcService.ResizeExec(req.Context(), execID, resizeMsg.Rows, resizeMsg.Cols)
					continue
				}
			}
			if _, err := conn.Write(data); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()

	<-done
}

func (g *Gateway) handleSPA(w http.ResponseWriter, r *http.Request) {
	if g.uiFS == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("NAS Management API is running. Embedded UI not attached."))
		return
	}
	cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleanPath == "" || cleanPath == "index.html" {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		cleanPath = "index.html"
	}
	// Check if file exists in embedded FS; otherwise serve index.html for Vue Router SPA navigation
	if f, err := g.uiFS.Open(cleanPath); err == nil {
		info, statErr := f.Stat()
		_ = f.Close()
		if statErr == nil && !info.IsDir() {
			if strings.HasPrefix(cleanPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.FileServer(http.FS(g.uiFS)).ServeHTTP(w, r)
			return
		}
	}
	indexBytes, err := fs.ReadFile(g.uiFS, "index.html")
	if err != nil {
		http.Error(w, "UI index.html not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexBytes)
}

// shellPathRegex restricts terminal shells to plain absolute paths such as /bin/bash.
var shellPathRegex = regexp.MustCompile(`^/[a-zA-Z0-9/_.-]{1,127}$`)

// noStoreMiddleware stops browsers and proxies from caching API responses.
// No CORS headers are sent: the embedded UI is same-origin, and cross-origin
// pages must not be able to drive the API.
func noStoreMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// requireAuth rejects requests without a valid bearer token. Browsers cannot set
// headers on EventSource or WebSocket requests, so GET requests may instead pass
// the token as the access_token query parameter.
func (g *Gateway) requireAuth(next http.Handler) http.Handler {
	return noStoreMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" && r.Method == http.MethodGet {
			token = r.URL.Query().Get("access_token")
		}
		if token == "" || g.verifier == nil {
			writeUnauthenticated(w)
			return
		}
		if _, err := g.verifier.VerifyToken(r.Context(), token); err != nil {
			writeUnauthenticated(w)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func writeUnauthenticated(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"code":16,"message":"authentication required"}`))
}

// sameOriginHandshake blocks cross-site WebSocket hijacking: a browser always sends
// Origin, and it must match the host the UI was served from. Non-browser clients
// that omit Origin are allowed (they cannot ride on a victim's session).
func sameOriginHandshake(cfg *websocket.Config, req *http.Request) error {
	if origin := req.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, req.Host) {
			return fmt.Errorf("websocket origin %q not allowed", origin)
		}
	}
	// Accept binary or base64 subprotocols requested by noVNC / RFB clients
	if len(cfg.Protocol) > 0 {
		cfg.Protocol = cfg.Protocol[:1]
	}
	return nil
}

// =============================================================================
// HTTP Streaming Bridge for gRPC Server Streaming (AIP-162 / AIP-151)
// =============================================================================

type httpServerStream[T any] struct {
	ctx     context.Context
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *httpServerStream[T]) Send(msg T) error {
	var data []byte
	var err error
	if pm, ok := any(msg).(proto.Message); ok {
		data, err = protojson.Marshal(pm)
	} else {
		data, err = json.Marshal(msg)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "{\"result\":%s}\n", data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *httpServerStream[T]) Context() context.Context {
	return s.ctx
}

func (s *httpServerStream[T]) SetHeader(metadata.MD) error  { return nil }
func (s *httpServerStream[T]) SendHeader(metadata.MD) error { return nil }
func (s *httpServerStream[T]) SetTrailer(metadata.MD)       {}
func (s *httpServerStream[T]) SendMsg(m any) error          { return nil }
func (s *httpServerStream[T]) RecvMsg(m any) error          { return nil }

func (g *Gateway) handleWatchDockerApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.WatchDockerAppsResponse]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.WatchDockerApps(&nasv1.WatchDockerAppsRequest{}, stream)
}

func (g *Gateway) handleWatchDockerContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.WatchDockerContainersResponse]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.WatchDockerContainers(&nasv1.WatchDockerContainersRequest{}, stream)
}

func (g *Gateway) handleWatchVirtualMachines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.WatchVirtualMachinesResponse]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.WatchVirtualMachines(&nasv1.WatchVirtualMachinesRequest{}, stream)
}

func (g *Gateway) handleWatchLxcContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.WatchLxcContainersResponse]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.WatchLxcContainers(&nasv1.WatchLxcContainersRequest{}, stream)
}

func (g *Gateway) handleStreamSystemTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var interval int32 = 2
	if s := r.URL.Query().Get("intervalSeconds"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			interval = int32(n)
		}
	}

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.SystemTelemetry]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.StreamSystemTelemetry(&nasv1.StreamSystemTelemetryRequest{IntervalSeconds: interval}, stream)
}

func (g *Gateway) handleWatchResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var rTypes []string
	for _, raw := range r.URL.Query()["resourceTypes"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				rTypes = append(rTypes, part)
			}
		}
	}

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-streamCtx.Done():
		case <-g.shutdownCh:
			cancel()
		}
	}()

	stream := &httpServerStream[*nasv1.WatchResourceChange]{
		ctx:     streamCtx,
		w:       w,
		flusher: flusher,
	}
	_ = g.grpcServer.WatchResources(&nasv1.WatchResourcesRequest{ResourceTypes: rTypes}, stream)
}
