package httpadapter

import (
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
		webdavHandler: webdavHandler,
		uiFS:          uiFS,
		eventBus:      eventBus,
		shutdownCh:    make(chan struct{}),
	}

	// 1. Real-Time System Notification Stream (Server-Sent Events)
	mux.HandleFunc("/v1/events", g.handleEventsSSE)
	mux.HandleFunc("/events", g.handleEventsSSE)

	// 2. Google AIP REST/JSON API via gRPC-Gateway
	mux.Handle("/v1/", corsMiddleware(gwMux))

	// 3. KVM noVNC WebSocket-to-RFB Proxy (/ws/novnc/{vmName}?token={token})
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

	// 4. Embedded WebDAV File Sharing Server (/dav/{share_name}/...)
	if webdavHandler != nil {
		mux.Handle("/dav/", corsMiddleware(webdavHandler))
	}

	// 5. Embedded Vue.js Single-Page Application (SPA)
	mux.HandleFunc("/", g.handleSPA)

	return g, nil
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

func (g *Gateway) handleSPA(w http.ResponseWriter, r *http.Request) {
	if g.uiFS == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("NAS Management API is running. Embedded UI not attached."))
		return
	}
	cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleanPath == "" {
		cleanPath = "index.html"
	}
	// Check if file exists in embedded FS; otherwise serve index.html for Vue Router SPA navigation
	if f, err := g.uiFS.Open(cleanPath); err == nil {
		info, statErr := f.Stat()
		_ = f.Close()
		if statErr == nil && !info.IsDir() {
			http.FileServer(http.FS(g.uiFS)).ServeHTTP(w, r)
			return
		}
	}
	indexBytes, err := fs.ReadFile(g.uiFS, "index.html")
	if err != nil {
		http.Error(w, "UI index.html not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexBytes)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, PROPFIND, MKCOL, MOVE, COPY")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Depth, Destination, Overwrite")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
