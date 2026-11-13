package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"github.com/sigxcpu/nas"
	grpcadapter "github.com/sigxcpu/nas/internal/adapters/inbound/grpc"
	httpadapter "github.com/sigxcpu/nas/internal/adapters/inbound/http"
	"github.com/sigxcpu/nas/internal/adapters/outbound/catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/cron"
	"github.com/sigxcpu/nas/internal/adapters/outbound/disk"
	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/adapters/outbound/identity"
	"github.com/sigxcpu/nas/internal/adapters/outbound/kvm"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc"
	"github.com/sigxcpu/nas/internal/adapters/outbound/lxc_catalog"
	"github.com/sigxcpu/nas/internal/adapters/outbound/preferences"
	"github.com/sigxcpu/nas/internal/adapters/outbound/prereq"
	"github.com/sigxcpu/nas/internal/adapters/outbound/share"
	"github.com/sigxcpu/nas/internal/adapters/outbound/system"
	"github.com/sigxcpu/nas/internal/adapters/outbound/zfs"
	"github.com/sigxcpu/nas/internal/application"
	"github.com/sigxcpu/nas/internal/certutil"
	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/sigxcpu/nas/internal/logging"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type StartOptions struct {
	BindAddr    string
	HTTPPort    int
	HTTPSPort   int
	TLSCertFile string
	TLSKeyFile  string
	GRPCPort    int
	WebdavPort  int
	StateDir    string
	JWTSecret   string
}

func newStartCmd(rootOpts *RootOptions) *cobra.Command {
	opts := &StartOptions{
		HTTPPort:   8080,
		HTTPSPort:  -1,
		GRPCPort:   9090,
		WebdavPort: 8081,
		StateDir:   "/tmp/nas-state",
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the NAS management daemon (Embedded Web UI, gRPC/HTTP Gateway, WebDAV & noVNC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), rootOpts, opts)
		},
	}

	cmd.Flags().IntVar(&opts.HTTPPort, "http-port", opts.HTTPPort, "HTTP/WebUI/gRPC-Gateway/WebDAV/noVNC listen port")
	cmd.Flags().IntVar(&opts.HTTPPort, "port", opts.HTTPPort, "Alias for --http-port")
	cmd.Flags().IntVar(&opts.HTTPSPort, "https-port", opts.HTTPSPort, "HTTPS/TLS listen port (default 8443, 0 to disable HTTPS)")
	cmd.Flags().StringVar(&opts.TLSCertFile, "tls-cert", "", "Path to custom TLS certificate file (optional)")
	cmd.Flags().StringVar(&opts.TLSKeyFile, "tls-key", "", "Path to custom TLS private key file (optional)")
	cmd.Flags().StringVar(&opts.BindAddr, "bind", "", "Bind interface/address for HTTP, HTTPS, gRPC, and WebDAV listeners (default 0.0.0.0)")
	cmd.Flags().IntVar(&opts.GRPCPort, "grpc-port", opts.GRPCPort, "Native gRPC API server listen port")
	cmd.Flags().IntVar(&opts.WebdavPort, "webdav-port", opts.WebdavPort, "Dedicated WebDAV server listen port (default 8081, 0 to disable dedicated port)")
	cmd.Flags().StringVar(&opts.StateDir, "state-dir", opts.StateDir, "Directory for NAS share & config state")
	cmd.Flags().StringVar(&opts.JWTSecret, "jwt-secret", opts.JWTSecret, "HMAC secret for JWT issuance (default: random secret persisted in the state dir)")

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
	if strings.TrimSpace(opts.BindAddr) != "" {
		cfg.Server.BindAddr = strings.TrimSpace(opts.BindAddr)
	}
	if strings.TrimSpace(cfg.Server.BindAddr) == "" {
		cfg.Server.BindAddr = "0.0.0.0"
	}
	if opts.HTTPPort != 8080 || cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = opts.HTTPPort
	}
	if opts.HTTPSPort >= 0 {
		cfg.Server.HTTPSPort = opts.HTTPSPort
	}
	if opts.TLSCertFile != "" {
		cfg.Server.TLSCertFile = opts.TLSCertFile
	}
	if opts.TLSKeyFile != "" {
		cfg.Server.TLSKeyFile = opts.TLSKeyFile
	}
	if opts.GRPCPort != 9090 || cfg.Server.GRPCPort == 0 {
		cfg.Server.GRPCPort = opts.GRPCPort
	}
	if opts.WebdavPort != 8081 || cfg.Server.WebdavPort == 0 {
		cfg.Server.WebdavPort = opts.WebdavPort
	}
	if opts.StateDir != "/tmp/nas-state" && opts.StateDir != "" {
		cfg.Server.StateDir = opts.StateDir
	}
	if opts.JWTSecret != "" {
		cfg.Server.JWTSecret = opts.JWTSecret
	}

	// 1. Outbound Prerequisite Checker & ZFS Engine
	prereqAdapter := prereq.NewSystemChecker()
	zfsAdapter := zfs.NewAdapter()
	zfsAdapter.SetLogger(logger.With("subsystem", "zfs"))

	// 1b. Outbound System Adapter for Main Pool & Dataset Configuration
	systemAdapter := system.NewAdapter(zfsAdapter, prereqAdapter)
	systemSvc := application.NewSystemService(systemAdapter)

	// 0. Ensure /etc/zfs exists and configure systemd ZFS boot-import services to use altroot /mnt
	_ = os.MkdirAll("/etc/zfs", 0755)
	ensureZfsBootAltrootConfig(logger)
	ensureLibvirtGuestsConfig(logger)
	ensureNouveauBlacklist(logger)
	_, _ = system.EnsureDockerMountDependencies(logger, cfg.System.MainPool)

	// Step A: Auto-Import Configured Main Pool on Boot / Reboot
	if cfg.System.MainPool != "" {
		cleanMain := strings.TrimPrefix(strings.TrimSpace(cfg.System.MainPool), "zfsPools/")
		if cleanMain != "" {
			if _, getErr := zfsAdapter.GetPool(ctx, cleanMain); getErr != nil {
				logger.Warn("configured main storage pool is not currently imported, attempting boot-time import", "main_pool", cleanMain)
				if impPool, impErr := zfsAdapter.ImportPool(ctx, cleanMain, true, "/mnt"); impErr == nil && impPool != nil {
					logger.Info("successfully imported main storage pool on startup", "main_pool", cleanMain)
					prereqAdapter.SetMainPool(cleanMain)
				} else {
					logger.Error("failed to auto-import configured main storage pool", "main_pool", cleanMain, "error", impErr)
				}
			} else {
				prereqAdapter.SetMainPool(cleanMain)
			}
		}
	}

	// Step B: Auto-Discovery on Reinstall / Boot:
	// If main pool is still not selected/imported, scan discovered pools for existing .nas-system
	if prereqAdapter.GetMainPool() == "" && cfg.System.AutoDiscover {
		if discovered, discErr := systemAdapter.DiscoverPools(ctx); discErr == nil {
			for _, dp := range discovered {
				cleanDP := strings.TrimPrefix(dp.PoolName, "zfsPools/")
				if !dp.IsImported {
					logger.Info("attempting to import discovered pool to check for NAS system dataset", "pool", cleanDP)
					if _, impErr := zfsAdapter.ImportPool(ctx, cleanDP, true, "/mnt"); impErr != nil {
						logger.Debug("could not import discovered pool for inspection", "pool", cleanDP, "error", impErr)
						continue
					}
				}
				if _, selErr := systemAdapter.SelectMainPool(ctx, cleanDP, true); selErr == nil {
					logger.Info("re-discovered and restored NAS configuration from pool", "main_pool", cleanDP)
					break
				}
			}
		}
	}

	// Step C: Ensure /etc/zfs/zpool.cache is synchronized for all active pools and mount all datasets
	_ = zfsAdapter.SyncCacheFile(ctx)
	_ = exec.CommandContext(ctx, "zfs", "mount", "-a").Run()

	// Verify imported data pools use altroot /mnt
	if pools, err := zfsAdapter.ListPools(ctx); err == nil {
		for _, p := range pools {
			cleanName := strings.TrimPrefix(p.ID, "zfsPools/")
			if cleanName == "rpool" || cleanName == "bpool" {
				continue
			}
			out, err := exec.CommandContext(ctx, "zpool", "get", "-H", "-o", "value", "altroot", cleanName).Output()
			if err == nil {
				val := strings.TrimSpace(string(out))
				if val == "-" || val == "" {
					logger.Warn("ZFS storage pool is mounted directly without altroot (/mnt); rebooting with configured systemd altroot will ensure /mnt mounts", "pool", cleanName)
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
		_, _ = system.EnsureDockerMountDependencies(logger, activeMainPool)
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
	cronUnitsDir := filepath.Join(stateDir, "cron")
	_ = os.MkdirAll(dockerUnitsDir, 0755)
	_ = os.MkdirAll(lxcUnitsDir, 0755)
	_ = os.MkdirAll(vmUnitsDir, 0755)
	_ = os.MkdirAll(sharesDir, 0755)
	_ = os.MkdirAll(cronUnitsDir, 0755)

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
	webdavAdapter := share.NewWebdavAdapter(sharesDir)
	identityAdapter := identity.NewAdapter()
	webdavAdapter.SetAuthenticator(identityAdapter.Authenticate)
	step("webdav and identity adapters initialized")
	catalogCacheDir := filepath.Join(stateDir, "catalog")
	catalogAdapter := catalog.NewAdapter(catalogCacheDir)
	step("catalog adapter initialized (cache load)")

	cronAdapter := cron.NewAdapter(cronUnitsDir)
	cronAdapter.SetLogger(logger.With("subsystem", "cron"))
	step("cron adapter initialized")

	dockerAdapter.SetZfsPort(zfsAdapter)
	lxcAdapter.SetZfsPort(zfsAdapter)
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
	kvmAdapter.StartEventWatcher(ctx, func(evt domain.Event) {
		logger.Debug("kvm event captured", "action", evt.Action, "resource", evt.Resource)
		eventBus.Publish(evt)
	})

	step("docker and kvm event watchers started")

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
	jwtSecret, err := resolveJWTSecret(cfg.Server.JWTSecret, stateDir)
	if err != nil {
		return fmt.Errorf("resolving JWT secret: %w", err)
	}
	identitySvc := application.NewIdentityService(identityAdapter, jwtSecret)
	if !identity.PAMSupported {
		logger.Warn("this build has no PAM support (CGO disabled); web UI and API logins will be rejected")
	}
	catalogSvc := application.NewCatalogService(catalogAdapter)

	preferencesAdapter := preferences.NewFileAdapter()
	preferencesSvc := application.NewPreferencesService(preferencesAdapter)

	cronAdapter.SetEventCallback(func(action, jobID, message string) {
		eventBus.Publish(domain.NewEvent("cron", action, domain.FormatCronJobResourceName(jobID), message))
	})
	cronSvc := application.NewCronJobService(cronAdapter)
	if err := cronSvc.Start(ctx); err != nil {
		logger.Warn("could not start cron scheduler service", "error", err)
	}
	defer cronSvc.Stop()

	step("application services constructed")

	// 3. Inbound Adapters (Driving Ports: gRPC + gRPC-Gateway + noVNC WS + WebDAV + Embedded Vue UI + SSE Events)
	nasGRPC := grpcadapter.NewServer(systemSvc, prereqSvc, diskSvc, zfsSvc, kvmSvc, dockerSvc, dockerAppSvc, lxcSvc, shareSvc, identitySvc, catalogSvc)
	nasGRPC.SetRegistryService(application.NewDockerRegistryService(dockerAdapter))
	nasGRPC.SetCronJobService(cronSvc)
	nasGRPC.SetPreferencesService(preferencesSvc)
	nasGRPC.SetEventBus(eventBus)

	lxcCatalogCacheDir := filepath.Join(stateDir, "lxc-catalog")
	lxcCatalogAdapter := lxc_catalog.NewAdapter(lxcCatalogCacheDir)
	lxcCatalogSvc := application.NewLxcCatalogService(lxcCatalogAdapter)
	nasGRPC.SetLxcCatalogService(lxcCatalogSvc)

	rawGRPCServer := grpc.NewServer(append(grpcadapter.ServerOptions(identitySvc),
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			resp, err = handler(ctx, req)
			if err != nil {
				logger.Error("grpc request error", "method", info.FullMethod, "error", err)
			}
			return resp, err
		}),
	)...)
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

	gateway, err := httpadapter.NewGateway(ctx, nasGRPC, identitySvc, kvmSvc, dockerSvc, lxcSvc, storageRepoSvc, webdavAdapter, uiFS, eventBus)
	if err != nil {
		return err
	}

	step("http gateway created")

	bindHost := cfg.Server.BindAddr
	if cfg.Server.GRPCPort > 0 {
		grpcAddr := net.JoinHostPort(bindHost, strconv.Itoa(cfg.Server.GRPCPort))
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			logger.Warn("could not bind gRPC port", "grpc_port", cfg.Server.GRPCPort, "addr", grpcAddr, "error", err)
		} else {
			go func() {
				logger.Info("gRPC API server listening", "grpc_port", cfg.Server.GRPCPort, "addr", grpcAddr)
				if err := rawGRPCServer.Serve(lis); err != nil {
					logger.Debug("gRPC server stopped", "error", err)
				}
			}()
		}
	}

	httpAddr := net.JoinHostPort(bindHost, strconv.Itoa(cfg.Server.HTTPPort))
	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           gateway,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var httpsServer *http.Server
	if cfg.Server.HTTPSPort > 0 {
		certDir := filepath.Join(stateDir, "certs")
		tlsCfg, caPEM, err := certutil.EnsureTLSConfig(certDir, cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		if err != nil {
			logger.Error("failed to configure TLS for HTTPS server", "error", err)
		} else {
			if len(caPEM) > 0 {
				gateway.SetCACert(caPEM)
			}
			httpsAddr := net.JoinHostPort(bindHost, strconv.Itoa(cfg.Server.HTTPSPort))
			httpsServer = &http.Server{
				Addr:              httpsAddr,
				Handler:           gateway,
				TLSConfig:         tlsCfg,
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				logger.Info("HTTPS server listening", "https_port", cfg.Server.HTTPSPort, "addr", httpsAddr)
				if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					logger.Warn("HTTPS server stopped", "error", err)
				}
			}()
		}
	}

	var webdavServer *http.Server
	if cfg.Server.WebdavPort > 0 {
		webdavAddr := net.JoinHostPort(bindHost, strconv.Itoa(cfg.Server.WebdavPort))
		webdavServer = &http.Server{
			Addr:              webdavAddr,
			Handler:           webdavAdapter,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			logger.Info("WebDAV dedicated server listening", "webdav_port", cfg.Server.WebdavPort, "addr", webdavAddr)
			if err := webdavServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Warn("WebDAV dedicated server stopped", "error", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("nasctl server started",
			"startup_total", time.Since(startupBegin).Round(time.Millisecond).String(),
			"bind_addr", bindHost,
			"http_port", cfg.Server.HTTPPort,
			"https_port", cfg.Server.HTTPSPort,
			"grpc_port", cfg.Server.GRPCPort,
			"webdav_port", cfg.Server.WebdavPort,
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

	// 2. Shut down HTTP & HTTPS listeners with bounded 2-second timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if httpsServer != nil {
		_ = httpsServer.Shutdown(shutdownCtx)
	}
	if webdavServer != nil {
		_ = webdavServer.Shutdown(shutdownCtx)
	}
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

// legacyJWTSecrets were hardcoded defaults in earlier releases and may still be
// present in saved config files; they are public and must never sign tokens.
var legacyJWTSecrets = map[string]bool{
	"nas-production-secret-2026":       true,
	"nas-default-hmac-secret-key-2026": true,
}

// resolveJWTSecret returns the configured secret, or a random one generated once
// and persisted (0600) in the state directory so sessions survive restarts.
func resolveJWTSecret(configured, stateDir string) (string, error) {
	if configured != "" && !legacyJWTSecrets[configured] {
		return configured, nil
	}
	path := filepath.Join(stateDir, "jwt.secret")
	if data, err := os.ReadFile(path); err == nil {
		if secret := strings.TrimSpace(string(data)); len(secret) >= 32 {
			return secret, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

// ensureZfsBootAltrootConfig configures systemd ZFS import services to import
// non-root storage pools with -R /mnt on boot, matching the TrueNAS SCALE storage layout.
func ensureZfsBootAltrootConfig(logger *slog.Logger) {
	if os.Geteuid() != 0 {
		return
	}
	systemdDir := "/etc/systemd/system"
	if fi, err := os.Stat(systemdDir); err != nil || !fi.IsDir() {
		return
	}

	cacheDir := filepath.Join(systemdDir, "zfs-import-cache.service.d")
	scanDir := filepath.Join(systemdDir, "zfs-import-scan.service.d")
	_ = os.MkdirAll(cacheDir, 0755)
	_ = os.MkdirAll(scanDir, 0755)

	cacheDropin := filepath.Join(cacheDir, "altroot.conf")
	scanDropin := filepath.Join(scanDir, "altroot.conf")

	cacheContent := `[Service]
ExecStart=
ExecStart=/usr/sbin/zpool import -c /etc/zfs/zpool.cache -aN -R /mnt
`
	scanContent := `[Service]
ExecStart=
ExecStart=/usr/sbin/zpool import -aN -d /dev/disk/by-id -o cachefile=none -R /mnt
`

	modified := false
	if current, err := os.ReadFile(cacheDropin); err != nil || string(current) != cacheContent {
		if err := os.WriteFile(cacheDropin, []byte(cacheContent), 0644); err == nil {
			modified = true
		}
	}
	if current, err := os.ReadFile(scanDropin); err != nil || string(current) != scanContent {
		if err := os.WriteFile(scanDropin, []byte(scanContent), 0644); err == nil {
			modified = true
		}
	}

	defaultZfsPath := "/etc/default/zfs"
	if data, err := os.ReadFile(defaultZfsPath); err == nil {
		content := string(data)
		if strings.Contains(content, `ZPOOL_IMPORT_OPTS="-R /mnt"`) ||
			strings.Contains(content, `ZPOOL_IMPORT_OPTS='-R /mnt'`) {
			re := regexp.MustCompile(`(?m)^ZPOOL_IMPORT_OPTS=.*$`)
			newContent := re.ReplaceAllString(content, `ZPOOL_IMPORT_OPTS=""`)
			_ = os.WriteFile(defaultZfsPath, []byte(newContent), 0644)
		}
	}

	if modified {
		logger.Info("configured systemd ZFS import services with altroot (-R /mnt) for boot-time persistence")
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}
}

// ensureLibvirtGuestsConfig configures libvirt-guests service shutdown parameters
// to prevent hangs on host poweroff/reboot when VMs ignore ACPI shutdown.
func ensureLibvirtGuestsConfig(logger *slog.Logger) {
	if os.Geteuid() != 0 {
		return
	}
	libvirtGuestsPath := "/etc/default/libvirt-guests"
	data, err := os.ReadFile(libvirtGuestsPath)
	if err != nil {
		return
	}
	content := string(data)
	modified := false

	// Ensure ON_SHUTDOWN=shutdown
	if strings.Contains(content, "#ON_SHUTDOWN=shutdown") || strings.Contains(content, "ON_SHUTDOWN=suspend") {
		re := regexp.MustCompile(`(?m)^#?ON_SHUTDOWN=.*$`)
		content = re.ReplaceAllString(content, `ON_SHUTDOWN=shutdown`)
		modified = true
	}
	// Ensure PARALLEL_SHUTDOWN=1
	if strings.Contains(content, "#PARALLEL_SHUTDOWN=0") || strings.Contains(content, "PARALLEL_SHUTDOWN=0") {
		re := regexp.MustCompile(`(?m)^#?PARALLEL_SHUTDOWN=.*$`)
		content = re.ReplaceAllString(content, `PARALLEL_SHUTDOWN=1`)
		modified = true
	}
	// Ensure SHUTDOWN_TIMEOUT=15 (instead of 300 seconds default)
	if strings.Contains(content, "#SHUTDOWN_TIMEOUT=300") || strings.Contains(content, "SHUTDOWN_TIMEOUT=300") {
		re := regexp.MustCompile(`(?m)^#?SHUTDOWN_TIMEOUT=.*$`)
		content = re.ReplaceAllString(content, `SHUTDOWN_TIMEOUT=15`)
		modified = true
	}

	if modified {
		if err := os.WriteFile(libvirtGuestsPath, []byte(content), 0644); err == nil {
			logger.Info("configured libvirt-guests with reduced shutdown timeout (15s, parallel)", "path", libvirtGuestsPath)
		}
	}
}

// ensureNouveauBlacklist ensures nouveau and nvidiafb are blacklisted when NVIDIA GPUs are present
// to prevent dual-driver conflicts that freeze host display or lock up kernel power-off.
func ensureNouveauBlacklist(logger *slog.Logger) {
	if os.Geteuid() != 0 {
		return
	}
	modprobeDir := "/etc/modprobe.d"
	if fi, err := os.Stat(modprobeDir); err != nil || !fi.IsDir() {
		return
	}
	blacklistPath := filepath.Join(modprobeDir, "blacklist-nouveau.conf")
	expectedContent := "blacklist nouveau\nblacklist nvidiafb\noptions nouveau modeset=0\n"
	current, err := os.ReadFile(blacklistPath)
	if err != nil || string(current) != expectedContent {
		if err := os.WriteFile(blacklistPath, []byte(expectedContent), 0644); err == nil {
			logger.Info("configured GPU blacklist for nouveau and nvidiafb", "path", blacklistPath)
		}
	}
}
