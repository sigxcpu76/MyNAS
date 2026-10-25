package cli

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type StartOptions struct {
	HTTPAddr   string
	GRPCAddr   string
	HybridMode bool
	StateDir   string
	JWTSecret  string
}

func newStartCmd(rootOpts *RootOptions) *cobra.Command {
	opts := &StartOptions{
		HTTPAddr:   ":8080",
		GRPCAddr:   ":9090",
		HybridMode: false,
		StateDir:   "/tmp/nas-state",
		JWTSecret:  "nas-production-secret-2026",
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the NAS management daemon (Embedded Web UI, gRPC/HTTP Gateway, WebDAV & noVNC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), rootOpts, opts)
		},
	}

	cmd.Flags().StringVar(&opts.HTTPAddr, "http-addr", opts.HTTPAddr, "HTTP/WebUI/gRPC-Gateway/WebDAV/noVNC listen address")
	cmd.Flags().StringVar(&opts.GRPCAddr, "grpc-addr", opts.GRPCAddr, "Native gRPC API server listen address")
	cmd.Flags().BoolVar(&opts.HybridMode, "hybrid", opts.HybridMode, "Enable hybrid sandbox fallback when host tools or libraries are missing")
	cmd.Flags().StringVar(&opts.StateDir, "state-dir", opts.StateDir, "Directory for NAS share & config state")
	cmd.Flags().StringVar(&opts.JWTSecret, "jwt-secret", opts.JWTSecret, "HMAC secret for JWT issuance")

	return cmd
}

func runServer(parentCtx context.Context, rootOpts *RootOptions, opts *StartOptions) error {
	ctx, stop := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	logger := rootOpts.Logger

	// Dedicated signal listener for secondary force-shutdown
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigCh)

	go func() {
		count := 0
		for range sigCh {
			count++
			if count >= 2 {
				logger.Warn("forcing immediate termination on secondary signal")
				os.Exit(1)
			}
		}
	}()

	// 1. Outbound Prerequisite Checker & Module Availability Evaluation
	prereqAdapter := prereq.NewSystemChecker(opts.HybridMode)

	zfsHasHost := prereqAdapter.HasHostPrerequisites("zfs")
	kvmHasHost := prereqAdapter.HasHostPrerequisites("kvm")
	dockerHasHost := prereqAdapter.HasHostPrerequisites("docker")
	lxcHasHost := prereqAdapter.HasHostPrerequisites("lxc")
	cifsHasHost := prereqAdapter.HasHostPrerequisites("cifs")

	zfsEnabled, zfsReason := prereqAdapter.IsModuleEnabled("zfs")
	kvmEnabled, kvmReason := prereqAdapter.IsModuleEnabled("kvm")
	dockerEnabled, dockerReason := prereqAdapter.IsModuleEnabled("docker")
	lxcEnabled, lxcReason := prereqAdapter.IsModuleEnabled("lxc")
	cifsEnabled, cifsReason := prereqAdapter.IsModuleEnabled("cifs")

	for _, mod := range []struct {
		name    string
		enabled bool
		live    bool
		reason  string
	}{
		{"zfs", zfsEnabled, zfsHasHost, zfsReason},
		{"kvm", kvmEnabled, kvmHasHost, kvmReason},
		{"docker", dockerEnabled, dockerHasHost, dockerReason},
		{"lxc", lxcEnabled, lxcHasHost, lxcReason},
		{"cifs", cifsEnabled, cifsHasHost, cifsReason},
	} {
		if mod.enabled {
			mode := "live"
			if !mod.live {
				mode = "hybrid-sandbox"
			}
			logger.Info("module enabled", "module", mod.name, "mode", mode, "details", mod.reason)
		} else {
			logger.Warn("module disabled due to missing prerequisites", "module", mod.name, "details", mod.reason)
		}
	}

	// 1b. Initialize Filesystem State Database for Units (Docker Compose, LXC, KVM, CIFS)
	stateDir := resolveStateDir(opts.StateDir)
	dockerUnitsDir := filepath.Join(stateDir, "docker")
	lxcUnitsDir := filepath.Join(stateDir, "lxc")
	vmUnitsDir := filepath.Join(stateDir, "vm")
	sharesDir := filepath.Join(stateDir, "shares")
	_ = os.MkdirAll(dockerUnitsDir, 0755)
	_ = os.MkdirAll(lxcUnitsDir, 0755)
	_ = os.MkdirAll(vmUnitsDir, 0755)
	_ = os.MkdirAll(sharesDir, 0755)

	logger.Info("filesystem unit state store initialized", "path", stateDir)

	// An adapter runs in hybrid/simulation mode ONLY when host prerequisites are missing
	zfsHybrid := !zfsHasHost
	kvmHybrid := !kvmHasHost
	dockerHybrid := !dockerHasHost
	lxcHybrid := !lxcHasHost
	cifsHybrid := !cifsHasHost

	zfsAdapter := zfs.NewAdapter(zfsHybrid)
	kvmAdapter := kvm.NewAdapterWithStateDir(kvmHybrid, vmUnitsDir)
	kvmAdapter.SetLogger(logger.With("subsystem", "libvirt"))
	dockerAdapter := docker.NewAdapterWithStateDir(dockerHybrid, dockerUnitsDir)
	dockerAdapter.SetLogger(logger.With("subsystem", "docker"))
	lxcAdapter := lxc.NewAdapter(lxcHybrid)
	cifsAdapter := share.NewCifsAdapter(cifsHybrid, sharesDir)
	webdavAdapter := share.NewWebdavAdapter(true)
	identityAdapter := identity.NewAdapter(true)
	catalogCacheDir := filepath.Join(stateDir, "catalog")
	catalogAdapter := catalog.NewAdapter(catalogCacheDir)

	dockerAdapter.SetZfsPort(zfsAdapter)
	if cfg, err := dockerAdapter.GetConfig(ctx); err == nil && cfg != nil && cfg.Pool != "" {
		catalogAdapter.SetAppsPool(cfg.Pool)
	}

	// 2. Event Bus & Real-Time Notification Stream
	eventBus := application.NewEventBus()
	dockerAdapter.StartEventWatcher(ctx, func(evt domain.Event) {
		logger.Debug("docker event captured", "action", evt.Action, "resource", evt.Resource)
		eventBus.Publish(evt)
	})

	// 2b. Application Services (Hexagonal Core Use-Cases)
	prereqSvc := application.NewPrerequisiteService(prereqAdapter)
	diskAdapter := disk.NewAdapter(zfsAdapter, !zfsHasHost)
	diskSvc := application.NewDiskService(diskAdapter)
	zfsSvc := application.NewZfsService(zfsAdapter)
	kvmSvc := application.NewKvmService(kvmAdapter)
	dockerSvc := application.NewDockerService(dockerAdapter)
	dockerAppSvc := application.NewDockerAppService(dockerAdapter)
	lxcSvc := application.NewLxcService(lxcAdapter)
	shareSvc := application.NewShareService(cifsAdapter, webdavAdapter)
	identitySvc := application.NewIdentityService(identityAdapter, opts.JWTSecret)
	catalogSvc := application.NewCatalogService(catalogAdapter)

	// 3. Inbound Adapters (Driving Ports: gRPC + gRPC-Gateway + noVNC WS + WebDAV + Embedded Vue UI + SSE Events)
	nasGRPC := grpcadapter.NewServer(prereqSvc, diskSvc, zfsSvc, kvmSvc, dockerSvc, dockerAppSvc, lxcSvc, shareSvc, identitySvc, catalogSvc)

	rawGRPCServer := grpc.NewServer(
		grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			resp, err = handler(ctx, req)
			if err != nil {
				logger.Error("grpc request error", "method", info.FullMethod, "error", err)
			}
			return resp, err
		}),
	)
	nasv1.RegisterNasServiceServer(rawGRPCServer, nasGRPC)
	reflection.Register(rawGRPCServer)

	uiFS, err := nas.WebUIFS()
	if err != nil {
		return err
	}

	gateway, err := httpadapter.NewGateway(ctx, nasGRPC, kvmSvc, webdavAdapter, uiFS, eventBus)
	if err != nil {
		return err
	}

	if opts.GRPCAddr != "" {
		lis, err := net.Listen("tcp", opts.GRPCAddr)
		if err != nil {
			logger.Warn("could not bind gRPC address", "grpc_addr", opts.GRPCAddr, "error", err)
		} else {
			go func() {
				logger.Info("gRPC API server listening", "grpc_addr", opts.GRPCAddr)
				if err := rawGRPCServer.Serve(lis); err != nil {
					logger.Debug("gRPC server stopped", "error", err)
				}
			}()
		}
	}

	httpServer := &http.Server{
		Addr:              opts.HTTPAddr,
		Handler:           gateway,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("nasctl server started",
			"http_addr", opts.HTTPAddr,
			"grpc_addr", opts.GRPCAddr,
			"hybrid_fallback", opts.HybridMode,
			"tty_mode", logging.IsTTY(rootOpts.ErrOut),
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down nasctl server")
	case sig := <-sigCh:
		logger.Info("received termination signal, shutting down nasctl server", "signal", sig.String())
	case err := <-errCh:
		logger.Error("server encountered fatal error", "error", err)
		return err
	}

	// 1. Immediately terminate active SSE and WebSocket streaming connections
	if gateway != nil {
		gateway.Close()
	}

	// 2. Shut down HTTP listener with bounded 2-second timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
	}

	// 3. Gracefully stop gRPC server with bounded 1-second timeout
	grpcDone := make(chan struct{})
	go func() {
		rawGRPCServer.GracefulStop()
		close(grpcDone)
	}()
	select {
	case <-grpcDone:
	case <-time.After(1 * time.Second):
		rawGRPCServer.Stop()
	}

	logger.Info("nasctl server stopped cleanly")
	return nil
}

func resolveStateDir(preferred string) string {
	if preferred == "" {
		preferred = "/var/lib/nasctl"
	}
	if err := os.MkdirAll(preferred, 0755); err == nil {
		return preferred
	}
	if home, err := os.UserHomeDir(); err == nil {
		userDir := filepath.Join(home, ".local", "share", "nasctl")
		if err := os.MkdirAll(userDir, 0755); err == nil {
			return userDir
		}
	}
	tmpDir := filepath.Join(os.TempDir(), "nasctl-state")
	_ = os.MkdirAll(tmpDir, 0755)
	return tmpDir
}
