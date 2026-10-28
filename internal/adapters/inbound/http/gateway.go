package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/sigxcpu/nas/internal/application"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"golang.org/x/net/websocket"
	"google.golang.org/protobuf/encoding/protojson"
)

type Gateway struct {
	mux           *http.ServeMux
	grpcServer    nasv1.NasServiceServer
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

func NewGateway(
	ctx context.Context,
	grpcServer nasv1.NasServiceServer,
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
	if err := nasv1.RegisterNasServiceHandlerServer(ctx, gwMux, grpcServer); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	g := &Gateway{
		mux:           mux,
		grpcServer:    grpcServer,
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
	mux.Handle("/v1/storage/isos/upload", corsMiddleware(http.HandlerFunc(g.handleUploadISO)))
	mux.Handle("/v1/storage/isos", corsMiddleware(http.HandlerFunc(g.handleISOs)))
	mux.Handle("/v1/storage/isos/", corsMiddleware(http.HandlerFunc(g.handleISOs)))
	mux.Handle("/v1/storage/lxc-templates/upload", corsMiddleware(http.HandlerFunc(g.handleUploadLxcTemplate)))
	mux.Handle("/v1/storage/lxc-templates", corsMiddleware(http.HandlerFunc(g.handleLxcTemplates)))
	mux.Handle("/v1/storage/lxc-templates/", corsMiddleware(http.HandlerFunc(g.handleLxcTemplates)))

	// 2. Real-Time System Notification Stream (Server-Sent Events)
	mux.HandleFunc("/v1/events", g.handleEventsSSE)
	mux.HandleFunc("/events", g.handleEventsSSE)

	// 3. Google AIP REST/JSON API via gRPC-Gateway
	mux.Handle("/v1/", corsMiddleware(gwMux))

	// 4. KVM noVNC WebSocket-to-RFB Proxy (/ws/novnc/{vmName}?token={token})
	mux.Handle("/ws/novnc/", websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			// Accept binary or base64 subprotocols requested by noVNC / RFB clients
			if len(cfg.Protocol) > 0 {
				cfg.Protocol = cfg.Protocol[:1]
			}
			return nil
		},
		Handler: g.handleNoVNCWebSocket,
	})

	// 5. Docker Container Interactive Terminal WebSocket (/ws/docker/terminal/{containerId}?shell=/bin/sh)
	mux.Handle("/ws/docker/terminal/", websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			if len(cfg.Protocol) > 0 {
				cfg.Protocol = cfg.Protocol[:1]
			}
			return nil
		},
		Handler: g.handleDockerTerminalWebSocket,
	})

	// 6. LXC Container Interactive Terminal WebSocket (/ws/lxc/terminal/{containerId}?shell=/bin/sh)
	mux.Handle("/ws/lxc/terminal/", websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			if len(cfg.Protocol) > 0 {
				cfg.Protocol = cfg.Protocol[:1]
			}
			return nil
		},
		Handler: g.handleLxcTerminalWebSocket,
	})

	// 7. Embedded WebDAV File Sharing Server (/dav/{share_name}/...)
	if webdavHandler != nil {
		mux.Handle("/dav/", corsMiddleware(webdavHandler))
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	if containerID == "" {
		return
	}

	shell := req.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
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
	if containerID == "" {
		return
	}

	shell := req.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, PROPFIND, MKCOL, MOVE, COPY")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Depth, Destination, Overwrite")
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
