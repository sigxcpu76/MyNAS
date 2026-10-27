package cli

import (
	"context"
	"fmt"
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
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type StartOptions struct {
	HTTPPort  int
	GRPCPort  int
	StateDir  string
	JWTSecret string
}

func newStartCmd(rootOpts *RootOptions) *cobra.Command {
	opts := &StartOptions{
		HTTPPort:  8080,
		GRPCPort:  9090,
		StateDir:  "/tmp/nas-state",
		JWTSecret: "nas-production-secret-2026",
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the NAS management daemon (Embedded Web UI, gRPC/HTTP Gateway, WebDAV & noVNC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), rootOpts, opts)
		},
	}

	cmd.Flags().IntVar(&opts.HTTPPort, "http-port", opts.HTTPPort, "HTTP/WebUI/gRPC-Gateway/WebDAV/noVNC listen port")
	cmd.Flags().IntVar(&opts.GRPCPort, "grpc-port", opts.GRPCPort, "Native gRPC API server listen port")
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

	// 0. Load Configuration via spf13/viper from config.yaml
	cfg, cfgPath, err := config.Load(rootOpts.ConfigFile)
	if err != nil {
		logger.Warn("could not load configuration file, falling back to defaults", "error", err)
		cfg = config.DefaultConfig()
	} else {
		logger.Info("loaded configuration from viper config.yaml", "path", cfgPath)
	}

	// Override with CLI flags if explicitly provided
	if opts.HTTPPort != 8080 || cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = opts.HTTPPort
	}
	if opts.GRPCPort != 9090 || cfg.Server.GRPCPort == 0 {
		cfg.Server.GRPCPort = opts.GRPCPort
	}
	if opts.StateDir != "/tmp/nas-state" && opts.StateDir != "" {
		cfg.Server.StateDir = opts.StateDir
	}
	if opts.JWTSecret != "nas-production-secret-2026" && opts.JWTSecret != "" {
		cfg.Server.JWTSecret = opts.JWTSecret
	}

	// 1. Outbound Prerequisite Checker & ZFS Engine
	prereqAdapter := prereq.NewSystemChecker()
	zfsAdapter := zfs.NewAdapter()

	// 1b. Outbound System Adapter for Main Pool & Dataset Configuration
	systemAdapter := system.NewAdapter(zfsAdapter, prereqAdapter)
	systemSvc := application.NewSystemService(systemAdapter)

	// Auto-Discovery on Reinstall / Boot:
	// If main pool is not selected, scan imported pools for existing .nas-system
	if cfg.System.MainPool == "" && cfg.System.AutoDiscover {
		if discovered, discErr := systemAdapter.DiscoverPools(ctx); discErr == nil {
			for _, dp := range discovered {
				if dp.IsImported && dp.HasNasSystem {
					logger.Info("auto-discovering existing NAS system dataset on imported pool", "pool", dp.PoolName, "dataset", dp.DatasetPath)
					if _, selErr := systemAdapter.SelectMainPool(ctx, dp.PoolName, true); selErr == nil {
						logger.Info("re-discovered and restored NAS configuration from imported pool", "main_pool", dp.PoolName)
						break
					}
				}
			}
		}
	}

	// Resolve State Storage Location:
	// If a main pool is selected, use its persistent .nas-system dataset; otherwise fallback to stateDir
	activeMainPool := prereqAdapter.GetMainPool()
	stateDir := resolveStateDir(cfg.Server.StateDir)
	if activeMainPool != "" {
		candidates := []string{
			filepath.Join("/mnt", activeMainPool, cfg.System.SystemDataset),
			filepath.Join("/nas", activeMainPool, cfg.System.SystemDataset),
			filepath.Join("/" + activeMainPool, cfg.System.SystemDataset),
		}
		for _, poolSysDir := range candidates {
			if info, sErr := os.Stat(poolSysDir); sErr == nil && info.IsDir() {
				stateDir = poolSysDir
				logger.Info("using persistent ZFS system dataset for all unit state stores", "path", stateDir, "main_pool", activeMainPool)
				break
			}
		}
	}

	zfsEnabled, zfsReason := prereqAdapter.IsModuleEnabled("zfs")
	kvmEnabled, kvmReason := prereqAdapter.IsModuleEnabled("kvm")
	dockerEnabled, dockerReason := prereqAdapter.IsModuleEnabled("docker")
	lxcEnabled, lxcReason := prereqAdapter.IsModuleEnabled("lxc")
	cifsEnabled, cifsReason := prereqAdapter.IsModuleEnabled("cifs")

	for _, mod := range []struct {
		name    string
		enabled bool
		reason  string
	}{
		{"zfs", zfsEnabled, zfsReason},
		{"kvm", kvmEnabled, kvmReason},
		{"docker", dockerEnabled, dockerReason},
		{"lxc", lxcEnabled, lxcReason},
		{"cifs", cifsEnabled, cifsReason},
	} {
		if mod.enabled {
			logger.Info("module enabled", "module", mod.name, "details", mod.reason)
		} else {
			logger.Warn("module disabled or paused", "module", mod.name, "details", mod.reason)
		}
	}

	// 1c. Initialize Filesystem State Database for Units (Docker Compose, LXC, KVM, CIFS)
	dockerUnitsDir := filepath.Join(stateDir, "docker")
	lxcUnitsDir := filepath.Join(stateDir, "lxc")
	vmUnitsDir := filepath.Join(stateDir, "vm")
	sharesDir := filepath.Join(stateDir, "shares")
	_ = os.MkdirAll(dockerUnitsDir, 0755)
	_ = os.MkdirAll(lxcUnitsDir, 0755)
	_ = os.MkdirAll(vmUnitsDir, 0755)
	_ = os.MkdirAll(sharesDir, 0755)

	// Clean up any legacy seeded mock units from older hybrid mode runs
	for _, legacyApp := range []string{"jellyfin", "nextcloud"} {
		legacyPath := filepath.Join(dockerUnitsDir, legacyApp)
		if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
			_ = os.RemoveAll(legacyPath)
		}
	}

	logger.Info("filesystem unit state store initialized", "path", stateDir)

	startupBegin := time.Now()
	stepStart := time.Now()
	// step logs how long the previous startup phase took, then restarts the phase timer.
	step := func(name string) {
		logger.Info("startup progress", "step", name, "took", time.Since(stepStart).Round(time.Millisecond).String(), "total", time.Since(startupBegin).Round(time.Millisecond).String())
		stepStart = time.Now()
	}

	kvmAdapter := kvm.NewAdapterWithStateDir(vmUnitsDir)
	kvmAdapter.SetLogger(logger.With("subsystem", "libvirt"))
	step("kvm adapter initialized")
	dockerAdapter := docker.NewAdapterWithStateDir(dockerUnitsDir)
	dockerAdapter.SetLogger(logger.With("subsystem", "docker"))
	step("docker adapter initialized (compose detection, persisted units)")
	lxcAdapter := lxc.NewAdapter()
	step("lxc adapter initialized")
	cifsAdapter := share.NewCifsAdapter(sharesDir)
	step("cifs adapter initialized (samba config load)")
	webdavAdapter := share.NewWebdavAdapter()
	identityAdapter := identity.NewAdapter()
	step("webdav and identity adapters initialized")
	catalogCacheDir := filepath.Join(stateDir, "catalog")
	catalogAdapter := catalog.NewAdapter(catalogCacheDir)
	step("catalog adapter initialized (cache load)")

	dockerAdapter.SetZfsPort(zfsAdapter)
	if activeMainPool != "" {
		dockerAdapter.SetMainPool(activeMainPool)
		lxcAdapter.SetMainPool(activeMainPool)
		kvmAdapter.SetMainPool(activeMainPool)
	}
	if cfg, err := dockerAdapter.GetConfig(ctx); err == nil && cfg != nil && cfg.Pool != "" {
		catalogAdapter.SetAppsPool(cfg.Pool)
	}
	step("docker config resolved (zfs pool/dataset lookup)")

	// 2. Event Bus & Real-Time Notification Stream
	eventBus := application.NewEventBus()
	dockerAdapter.StartEventWatcher(ctx, func(evt domain.Event) {
		logger.Debug("docker event captured", "action", evt.Action, "resource", evt.Resource)
		eventBus.Publish(evt)
	})

	step("docker event watcher started")

	// 2b. Application Services (Hexagonal Core Use-Cases)
	prereqSvc := application.NewPrerequisiteService(prereqAdapter)
	diskAdapter := disk.NewAdapter(zfsAdapter)
	diskSvc := application.NewDiskService(diskAdapter)
	zfsSvc := application.NewZfsService(zfsAdapter)
	kvmSvc := application.NewKvmService(kvmAdapter)
	dockerSvc := application.NewDockerService(dockerAdapter)
	dockerAppSvc := application.NewDockerAppService(dockerAdapter)
	lxcSvc := application.NewLxcService(lxcAdapter)
	shareSvc := application.NewShareService(cifsAdapter, webdavAdapter)
	identitySvc := application.NewIdentityService(identityAdapter, opts.JWTSecret)
	catalogSvc := application.NewCatalogService(catalogAdapter)

	step("application services constructed")

	// 3. Inbound Adapters (Driving Ports: gRPC + gRPC-Gateway + noVNC WS + WebDAV + Embedded Vue UI + SSE Events)
	nasGRPC := grpcadapter.NewServer(systemSvc, prereqSvc, diskSvc, zfsSvc, kvmSvc, dockerSvc, dockerAppSvc, lxcSvc, shareSvc, identitySvc, catalogSvc)
	nasGRPC.SetRegistryService(application.NewDockerRegistryService(dockerAdapter))

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

	step("grpc server and web UI assets ready")

	storageRepoSvc := application.NewStorageRepoService(zfsAdapter, "")
	if activeMainPool != "" {
		storageRepoSvc.SetMainPool(activeMainPool)
		_ = storageRepoSvc.EnsureRepositories(ctx)
	}
	step("storage repositories ensured (isos, lxc-templates datasets)")

	gateway, err := httpadapter.NewGateway(ctx, nasGRPC, kvmSvc, dockerSvc, storageRepoSvc, webdavAdapter, uiFS, eventBus)
	if err != nil {
		return err
	}

	step("http gateway created")

	if cfg.Server.GRPCPort > 0 {
		grpcAddr := fmt.Sprintf(":%d", cfg.Server.GRPCPort)
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			logger.Warn("could not bind gRPC port", "grpc_port", cfg.Server.GRPCPort, "error", err)
		} else {
			go func() {
				logger.Info("gRPC API server listening", "grpc_port", cfg.Server.GRPCPort, "addr", grpcAddr)
				if err := rawGRPCServer.Serve(lis); err != nil {
					logger.Debug("gRPC server stopped", "error", err)
				}
			}()
		}
	}

	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           gateway,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("nasctl server started",
			"startup_total", time.Since(startupBegin).Round(time.Millisecond).String(),
			"http_port", cfg.Server.HTTPPort,
			"grpc_port", cfg.Server.GRPCPort,
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
