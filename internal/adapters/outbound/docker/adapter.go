package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/config"
	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
	"gopkg.in/yaml.v3"
)

type containerStatsRecord struct {
	cpuPct  float64
	memMiB  int64
	netRx   int64
	netTx   int64
	rxRate  int64
	txRate  int64
	updated time.Time
}

type Adapter struct {
	mu           sync.RWMutex
	logger       *slog.Logger
	client       *http.Client
	socketPath   string
	baseEndpoint string
	unitsDir     string
	hasSocket    bool
	hasCompose   bool
	composeCmd   []string
	zfsPort      ports.ZfsEnginePort
	mainPool     string
	// fallback memory store when docker socket is unavailable
	containers   map[string]domain.DockerContainer
	statsCacheMu sync.RWMutex
	statsCache   map[string]containerStatsRecord
	statsInFlight map[string]bool

	deployingApps   map[string]string
	deployingErrors map[string]string
	deployCancel    map[string]context.CancelFunc

	// guards registry credential files (config.json / registries.json)
	registryMu sync.Mutex
	// registryVerify overrides `docker login` verification (tests only).
	registryVerify func(ctx context.Context, server, user, pass string) error
}

func (a *Adapter) SetLogger(l *slog.Logger) {
	if l != nil {
		a.logger = l
	}
}

func (a *Adapter) logError(op string, resource string, err error, details string) {
	if err == nil {
		return
	}
	l := a.logger
	if l == nil {
		l = slog.Default()
	}
	details = strings.TrimSpace(details)
	if details != "" {
		l.Error("docker operation failed", "subsystem", "docker", "operation", op, "resource", resource, "error", err, "details", details)
	} else {
		l.Error("docker operation failed", "subsystem", "docker", "operation", op, "resource", resource, "error", err)
	}
}

func (a *Adapter) baseURL() string {
	if a.baseEndpoint != "" {
		return a.baseEndpoint
	}
	return "http://docker"
}

// detectCompose checks if "docker compose" or standalone "docker-compose" is available on the system.
func detectCompose() (bool, []string) {
	if _, err := exec.LookPath("docker"); err == nil {
		cmd := exec.Command("docker", "compose", "version")
		if err := cmd.Run(); err == nil {
			return true, []string{"docker", "compose"}
		}
	}
	if p, err := exec.LookPath("docker-compose"); err == nil {
		return true, []string{p}
	}
	return false, nil
}

// NewAdapter creates a Docker runtime adapter connecting to /var/run/docker.sock with default units dir.
func NewAdapter() *Adapter {
	return NewAdapterWithStateDir("")
}

// NewAdapterWithStateDir creates a Docker adapter using a dedicated filesystem units directory for compose files.
func NewAdapterWithStateDir(unitsDir string) *Adapter {
	return NewAdapterFull("/var/run/docker.sock", unitsDir)
}

// NewAdapterWithSocketPath creates a Docker adapter with a custom socket path.
func NewAdapterWithSocketPath(socketPath string) *Adapter {
	return NewAdapterFull(socketPath, "")
}

// NewAdapterFull constructs the complete Docker adapter with socket path and units directory.
func NewAdapterFull(socketPath string, unitsDir string) *Adapter {
	if unitsDir == "" {
		unitsDir = filepath.Join(os.TempDir(), fmt.Sprintf("nasctl-docker-units-%d-%d", os.Getpid(), time.Now().UnixNano()))
	}
	_ = os.MkdirAll(unitsDir, 0755)

	hasSocket := false
	if fi, err := os.Stat(socketPath); err == nil && (fi.Mode()&os.ModeSocket) != 0 {
		hasSocket = true
	}

	hasCompose, composeCmd := detectCompose()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}

	a := &Adapter{
		logger: slog.Default().With("subsystem", "docker"),
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		socketPath:   socketPath,
		baseEndpoint: "http://docker",
		unitsDir:     unitsDir,
		hasSocket:    hasSocket,
		hasCompose:   hasCompose,
		composeCmd:   composeCmd,
		containers:      make(map[string]domain.DockerContainer),
		statsCache:      make(map[string]containerStatsRecord),
		statsInFlight:   make(map[string]bool),
		deployingApps:   make(map[string]string),
		deployingErrors: make(map[string]string),
		deployCancel:    make(map[string]context.CancelFunc),
	}

	a.loadPersistedUnits()
	a.seedRegistriesFromHome()
	return a
}

// NewAdapterWithClient allows injecting an HTTP client and base endpoint (for unit testing).
func NewAdapterWithClient(client *http.Client, baseEndpoint string) *Adapter {
	unitsDir := filepath.Join(os.TempDir(), fmt.Sprintf("nasctl-test-units-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(unitsDir, 0755)

	return &Adapter{
		logger:          slog.Default().With("subsystem", "docker"),
		client:          client,
		baseEndpoint:    baseEndpoint,
		unitsDir:        unitsDir,
		hasSocket:       true,
		hasCompose:      false,
		containers:      make(map[string]domain.DockerContainer),
		statsCache:      make(map[string]containerStatsRecord),
		statsInFlight:   make(map[string]bool),
		deployingApps:   make(map[string]string),
		deployingErrors: make(map[string]string),
		deployCancel:    make(map[string]context.CancelFunc),
	}
}


// loadPersistedUnits scans the units filesystem directory and loads existing compose.yaml files.
// It automatically purges legacy mock templates (e.g. jellyfin, nextcloud) that were pre-seeded in older builds
// and do not correspond to any active containers on the Docker host.
func (a *Adapter) loadPersistedUnits() {
	if a.unitsDir == "" {
		return
	}
	legacyMocks := []string{"jellyfin", "nextcloud"}
	for _, mockID := range legacyMocks {
		dir := filepath.Join(a.unitsDir, mockID)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			hasActive := false
			if a.hasSocket {
				live := a.fetchLiveContainers(context.Background())
				for _, c := range live {
					if c.Labels["com.docker.compose.project"] == mockID ||
						containsString(c.Names, "/"+mockID) ||
						containsString(c.Names, "/"+mockID+"-app") ||
						containsString(c.Names, "/"+mockID+"-media") {
						hasActive = true
						break
					}
				}
			}
			if !hasActive {
				a.logger.Info("purging uninstalled legacy mock app directory", "app", mockID, "path", dir)
				_ = os.RemoveAll(dir)
			}
		}
	}
}

// runCompose executes a docker compose subcommand for a specific unit directory.
func (a *Adapter) runCompose(ctx context.Context, unitDir string, projectName string, args ...string) error {
	if !a.hasCompose || len(a.composeCmd) == 0 {
		return fmt.Errorf("docker compose CLI is not available on host")
	}
	composeFile := filepath.Join(unitDir, "compose.yaml")
	if len(args) > 0 && args[0] == "up" {
		if data, err := os.ReadFile(composeFile); err == nil {
			EnsureVolumeHostPaths(string(data))
			a.ensureExternalNetworks(ctx, string(data))
		}
	}
	var cmdArgs []string
	if len(a.composeCmd) > 1 {
		// e.g. ["docker", "compose"] -> ["compose", "-f", file, "-p", project, ...args]
		cmdArgs = append([]string{a.composeCmd[1], "-f", composeFile, "-p", projectName}, args...)
	} else {
		// e.g. ["docker-compose"] -> ["-f", file, "-p", project, ...args]
		cmdArgs = append([]string{"-f", composeFile, "-p", projectName}, args...)
	}

	cmd := exec.CommandContext(ctx, a.composeCmd[0], cmdArgs...)
	cmd.Dir = unitDir
	cmd.Env = a.dockerCLIEnv()
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		details := filterComposeOutput(errBuf.String())
		op := "compose_" + strings.Join(args, "_")
		if isComposeCanceled(err, details) {
			a.logger.Info("docker compose operation canceled", "subsystem", "docker", "operation", op, "resource", projectName)
		} else {
			a.logError(op, projectName, err, details)
		}
		return fmt.Errorf("docker compose %s failed: %w (details: %s)", strings.Join(args, " "), err, details)
	}
	return nil
}

// GenerateComposeYAML produces a clean, standard Docker Compose specification for a container.
func GenerateComposeYAML(c domain.DockerContainer) string {
	var sb strings.Builder
	serviceName := c.ID
	if serviceName == "" {
		serviceName = "app"
	}
	sb.WriteString("services:\n")
	sb.WriteString(fmt.Sprintf("  %s:\n", serviceName))
	sb.WriteString(fmt.Sprintf("    container_name: %s\n", c.ID))
	sb.WriteString(fmt.Sprintf("    image: %s\n", c.Image))
	if c.RestartPolicy != "" {
		sb.WriteString(fmt.Sprintf("    restart: %q\n", c.RestartPolicy))
	} else {
		sb.WriteString("    restart: \"unless-stopped\"\n")
	}
	if c.NetworkMode != "" && c.NetworkMode != "bridge" {
		sb.WriteString(fmt.Sprintf("    network_mode: %q\n", c.NetworkMode))
	}
	if len(c.Ports) > 0 {
		sb.WriteString("    ports:\n")
		for _, p := range c.Ports {
			sb.WriteString(fmt.Sprintf("      - %q\n", p))
		}
	}
	if len(c.Volumes) > 0 {
		sb.WriteString("    volumes:\n")
		for _, v := range c.Volumes {
			sb.WriteString(fmt.Sprintf("      - %q\n", v))
		}
	}
	if len(c.Environment) > 0 {
		sb.WriteString("    environment:\n")
		for _, e := range c.Environment {
			sb.WriteString(fmt.Sprintf("      - %q\n", EscapeComposeEnvString(e)))
		}
	}
	if len(c.Command) > 0 {
		sb.WriteString("    command:\n")
		for _, arg := range c.Command {
			sb.WriteString(fmt.Sprintf("      - %q\n", arg))
		}
	}
	if c.MemoryLimitMiB > 0 {
		sb.WriteString(fmt.Sprintf("    mem_limit: %dm\n", c.MemoryLimitMiB))
	}
	if c.Privileged {
		sb.WriteString("    privileged: true\n")
	}
	return sb.String()
}

// =============================================================================
// Docker Engine REST API Data Models
// =============================================================================

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Ports   []dockerPort      `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

type dockerContainerInspect struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	State   struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
		Paused  bool   `json:"Paused"`
	} `json:"State"`
	Config struct {
		Image string   `json:"Image"`
		Cmd   []string `json:"Cmd"`
		Env   []string `json:"Env"`
	} `json:"Config"`
	HostConfig struct {
		Binds         []string                               `json:"Binds"`
		PortBindings  map[string][]struct{ HostPort string } `json:"PortBindings"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
		NetworkMode string `json:"NetworkMode"`
		Memory      int64  `json:"Memory"`
		Privileged  bool   `json:"Privileged"`
	} `json:"HostConfig"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

// =============================================================================
// Implementation of ports.DockerRuntimePort
// =============================================================================

func (a *Adapter) ListContainers(ctx context.Context) ([]domain.DockerContainer, error) {
	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL()+"/containers/json?all=1", nil)
		if err == nil {
			resp, err := a.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var rawList []dockerContainerSummary
					if err := json.NewDecoder(resp.Body).Decode(&rawList); err == nil {
						results := make([]domain.DockerContainer, 0, len(rawList))
						for _, r := range rawList {
							shortID := r.ID
							if len(shortID) > 12 {
								shortID = shortID[:12]
							}
							name := shortID
							if len(r.Names) > 0 {
								name = strings.TrimPrefix(r.Names[0], "/")
							}
							var portList []string
							for _, p := range r.Ports {
								var portStr string
								if p.PublicPort > 0 {
									if p.Type == "" || strings.EqualFold(p.Type, "tcp") {
										portStr = fmt.Sprintf("%d:%d", p.PublicPort, p.PrivatePort)
									} else {
										portStr = fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, strings.ToLower(p.Type))
									}
								} else if p.PrivatePort > 0 {
									if p.Type == "" || strings.EqualFold(p.Type, "tcp") {
										portStr = fmt.Sprintf("%d", p.PrivatePort)
									} else {
										portStr = fmt.Sprintf("%d/%s", p.PrivatePort, strings.ToLower(p.Type))
									}
								}
								portStr = normalizePortSpec(portStr)
								if portStr != "" && !slices.Contains(portList, portStr) {
									portList = append(portList, portStr)
								}
							}
							state := strings.ToUpper(r.State)
							c := domain.DockerContainer{
								ID:            name,
								ContainerID:   shortID,
								ContainerName: name,
								Image:         r.Image,
								State:         state,
								StatusText:    r.Status,
								Ports:         portList,
								CreatedAt:     time.Unix(r.Created, 0).UTC().Format(time.RFC3339),
							}
							// Read persisted compose.yaml if present
							if a.unitsDir != "" {
								composePath := filepath.Join(a.unitsDir, name, "compose.yaml")
								if content, err := os.ReadFile(composePath); err == nil {
									c.ComposeYAML = string(content)
								}
							}
							results = append(results, c)
						}
						return results, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("docker daemon is unreachable via %s", a.socketPath)
}

func (a *Adapter) GetContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/containers/%s/json", a.baseURL(), url.PathEscape(id)), nil)
		if err == nil {
			resp, err := a.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var inspect dockerContainerInspect
					if err := json.NewDecoder(resp.Body).Decode(&inspect); err == nil {
						shortID := inspect.ID
						if len(shortID) > 12 {
							shortID = shortID[:12]
						}
						name := strings.TrimPrefix(inspect.Name, "/")
						if name == "" {
							name = shortID
						}
						var portList []string
						for containerPort, bindings := range inspect.HostConfig.PortBindings {
							for _, b := range bindings {
								portList = append(portList, fmt.Sprintf("%s:%s", b.HostPort, containerPort))
							}
						}

						// Fetch live container stats (CPU% and Memory MiB)
						var cpuPct float64
						var memMiB int64
						if inspect.State.Running {
							cpuPct, memMiB, _, _, _, _ = a.getLiveStats(ctx, inspect.ID, true)
						}

						res := &domain.DockerContainer{
							ID:             name,
							ContainerID:    shortID,
							ContainerName:  name,
							Image:          inspect.Config.Image,
							State:          strings.ToUpper(inspect.State.Status),
							StatusText:     inspect.State.Status,
							Ports:          portList,
							Volumes:        inspect.HostConfig.Binds,
							Environment:    inspect.Config.Env,
							RestartPolicy:  inspect.HostConfig.RestartPolicy.Name,
							NetworkMode:    inspect.HostConfig.NetworkMode,
							CPUPercent:     cpuPct,
							MemoryUsageMiB: memMiB,
							CreatedAt:      inspect.Created,
							MemoryLimitMiB: inspect.HostConfig.Memory / (1024 * 1024),
							Command:        inspect.Config.Cmd,
							Privileged:     inspect.HostConfig.Privileged,
						}

						// Attach persisted compose.yaml from filesystem unit store
						if a.unitsDir != "" {
							composePath := filepath.Join(a.unitsDir, name, "compose.yaml")
							if content, err := os.ReadFile(composePath); err == nil {
								res.ComposeYAML = string(content)
							} else {
								res.ComposeYAML = GenerateComposeYAML(*res)
							}
						}

						return res, nil
					}
				}
				if resp.StatusCode == http.StatusNotFound {
					return nil, fmt.Errorf("%w: docker container %q", domain.ErrNotFound, id)
				}
			}
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.containers[id]
	if !ok {
		return nil, fmt.Errorf("%w: docker container %q", domain.ErrNotFound, id)
	}
	cp := c
	if cp.ComposeYAML == "" {
		cp.ComposeYAML = GenerateComposeYAML(cp)
	}
	return &cp, nil
}

func (a *Adapter) CreateContainer(ctx context.Context, c domain.DockerContainer) (*domain.DockerContainer, error) {
	// 1. Persist unit to filesystem database (<unitsDir>/<id>/compose.yaml)
	unitDir := filepath.Join(a.unitsDir, c.ID)
	_ = os.MkdirAll(unitDir, 0755)
	composePath := filepath.Join(unitDir, "compose.yaml")

	yamlContent := c.ComposeYAML
	if strings.TrimSpace(yamlContent) == "" {
		yamlContent = GenerateComposeYAML(c)
	}
	c.ComposeYAML = yamlContent

	if err := os.WriteFile(composePath, []byte(yamlContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to persist compose unit to %s: %w", composePath, err)
	}

	// 2. Deploy using Docker Compose if available
	if a.hasCompose && a.hasSocket {
		if err := a.runCompose(ctx, unitDir, c.ID, "up", "-d", "--remove-orphans"); err != nil {
			return nil, err
		}
		res, err := a.GetContainer(ctx, c.ID)
		if err == nil {
			res.ComposeYAML = yamlContent
			return res, nil
		}
	}

	// 3. Fallback to direct Docker Engine Socket API if compose binary is absent
	if a.hasSocket {
		a.ensureImage(ctx, c.Image)

		portBindings := make(map[string][]struct{ HostPort string })
		exposedPorts := make(map[string]struct{})
		for _, p := range c.Ports {
			parts := strings.Split(p, ":")
			if len(parts) == 2 {
				hostPort := parts[0]
				containerPort := parts[1]
				if !strings.Contains(containerPort, "/") {
					containerPort += "/tcp"
				}
				portBindings[containerPort] = append(portBindings[containerPort], struct{ HostPort string }{HostPort: hostPort})
				exposedPorts[containerPort] = struct{}{}
			}
		}

		hostConfig := map[string]any{
			"Binds":        c.Volumes,
			"PortBindings": portBindings,
			"RestartPolicy": map[string]string{
				"Name": c.RestartPolicy,
			},
			"NetworkMode": c.NetworkMode,
			"Privileged":  c.Privileged,
		}
		if c.MemoryLimitMiB > 0 {
			hostConfig["Memory"] = c.MemoryLimitMiB * 1024 * 1024
		}

		createPayload := map[string]any{
			"Image":        c.Image,
			"Cmd":          c.Command,
			"Env":          c.Environment,
			"ExposedPorts": exposedPorts,
			"HostConfig":   hostConfig,
		}

		bodyBytes, _ := json.Marshal(createPayload)
		createURL := fmt.Sprintf("%s/containers/create?name=%s", a.baseURL(), url.QueryEscape(c.ID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.client.Do(req)
		if err != nil {
			a.logError("create_request", c.ID, err, "")
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			err := fmt.Errorf("%w: container %q already exists in docker engine", domain.ErrAlreadyExists, c.ID)
			a.logError("create", c.ID, err, "")
			return nil, err
		}
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("docker create failed (%d): %s", resp.StatusCode, string(body))
			a.logError("create", c.ID, err, string(body))
			return nil, err
		}

		var createResp struct {
			ID string `json:"Id"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&createResp)

		startReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/containers/%s/start", a.baseURL(), createResp.ID), nil)
		startResp, startErr := a.client.Do(startReq)
		if startErr != nil {
			a.logError("start_after_create", c.ID, startErr, "")
		} else {
			defer startResp.Body.Close()
			if startResp.StatusCode >= 400 {
				body, _ := io.ReadAll(startResp.Body)
				a.logError("start_after_create", c.ID, fmt.Errorf("http %d", startResp.StatusCode), string(body))
			}
		}

		res, err := a.GetContainer(ctx, c.ID)
		if err == nil {
			res.ComposeYAML = yamlContent
			return res, nil
		}
	}

	return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
}

func (a *Adapter) UpdateContainer(ctx context.Context, c domain.DockerContainer, fields []string) (*domain.DockerContainer, error) {
	unitDir := filepath.Join(a.unitsDir, c.ID)
	_ = os.MkdirAll(unitDir, 0755)
	composePath := filepath.Join(unitDir, "compose.yaml")

	yamlContent := c.ComposeYAML
	if strings.TrimSpace(yamlContent) == "" {
		yamlContent = GenerateComposeYAML(c)
	}
	c.ComposeYAML = yamlContent

	if err := os.WriteFile(composePath, []byte(yamlContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to persist updated compose unit to %s: %w", composePath, err)
	}

	// 1. If Docker Compose is available, re-run "docker compose up -d" to smoothly recreate with new config
	if a.hasCompose && a.hasSocket {
		if err := a.runCompose(ctx, unitDir, c.ID, "up", "-d", "--remove-orphans"); err != nil {
			return nil, err
		}
		res, err := a.GetContainer(ctx, c.ID)
		if err == nil {
			res.ComposeYAML = yamlContent
			return res, nil
		}
	}

	// 2. Direct Docker Engine socket recreate fallback
	if a.hasSocket {
		orig, err := a.GetContainer(ctx, c.ID)
		if err != nil {
			return nil, err
		}

		wasRunning := orig.State == "RUNNING"
		backupName := fmt.Sprintf("%s-backup-%d", c.ID, time.Now().Unix())

		renameURL := fmt.Sprintf("%s/containers/%s/rename?name=%s", a.baseURL(), url.PathEscape(c.ID), url.QueryEscape(backupName))
		if rReq, err := http.NewRequestWithContext(ctx, http.MethodPost, renameURL, nil); err == nil {
			if resp, err := a.client.Do(rReq); err == nil {
				_ = resp.Body.Close()
			}
		}

		if wasRunning {
			_, _ = a.StopContainer(ctx, backupName)
		}

		if c.Image == "" {
			c.Image = orig.Image
		}
		if len(c.Ports) == 0 && !containsString(fields, "ports") {
			c.Ports = orig.Ports
		}
		if len(c.Volumes) == 0 && !containsString(fields, "volumes") {
			c.Volumes = orig.Volumes
		}
		if len(c.Environment) == 0 && !containsString(fields, "environment") {
			c.Environment = orig.Environment
		}
		if c.RestartPolicy == "" {
			c.RestartPolicy = orig.RestartPolicy
		}
		if c.NetworkMode == "" {
			c.NetworkMode = orig.NetworkMode
		}
		if c.MemoryLimitMiB == 0 && !containsString(fields, "memory_limit_mib") {
			c.MemoryLimitMiB = orig.MemoryLimitMiB
		}

		created, err := a.CreateContainer(ctx, c)
		if err != nil {
			_ = a.DeleteContainer(ctx, c.ID, true)
			restoreURL := fmt.Sprintf("%s/containers/%s/rename?name=%s", a.baseURL(), url.PathEscape(backupName), url.QueryEscape(c.ID))
			if rReq, rErr := http.NewRequestWithContext(ctx, http.MethodPost, restoreURL, nil); rErr == nil {
				if rResp, rErr := a.client.Do(rReq); rErr == nil {
					_ = rResp.Body.Close()
				}
			}
			if wasRunning {
				_, _ = a.StartContainer(ctx, c.ID)
			}
			return nil, fmt.Errorf("failed to recreate container %q: %w", c.ID, err)
		}

		_ = a.DeleteContainer(ctx, backupName, true)

		if wasRunning && created.State != "RUNNING" {
			created, _ = a.StartContainer(ctx, c.ID)
		}
		created.ComposeYAML = yamlContent
		return created, nil
	}

	return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
}

func (a *Adapter) StartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	unitDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		if _, err := os.Stat(filepath.Join(unitDir, "compose.yaml")); err == nil {
			_ = a.runCompose(ctx, unitDir, id, "start")
			return a.GetContainer(ctx, id)
		}
	}

	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/containers/%s/start", a.baseURL(), url.PathEscape(id)), nil)
		if err != nil {
			return nil, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			a.logError("start", id, err, "")
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("docker start failed (%d): %s", resp.StatusCode, string(body))
			a.logError("start", id, err, string(body))
			return nil, err
		}
		return a.GetContainer(ctx, id)
	}

	return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
}

func (a *Adapter) StopContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	unitDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		if _, err := os.Stat(filepath.Join(unitDir, "compose.yaml")); err == nil {
			_ = a.runCompose(ctx, unitDir, id, "stop")
			return a.GetContainer(ctx, id)
		}
	}

	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/containers/%s/stop?t=10", a.baseURL(), url.PathEscape(id)), nil)
		if err != nil {
			return nil, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			a.logError("stop", id, err, "")
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("docker stop failed (%d): %s", resp.StatusCode, string(body))
			a.logError("stop", id, err, string(body))
			return nil, err
		}
		return a.GetContainer(ctx, id)
	}

	return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
}

func (a *Adapter) RestartContainer(ctx context.Context, id string) (*domain.DockerContainer, error) {
	unitDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		if _, err := os.Stat(filepath.Join(unitDir, "compose.yaml")); err == nil {
			_ = a.runCompose(ctx, unitDir, id, "restart")
			return a.GetContainer(ctx, id)
		}
	}

	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/containers/%s/restart?t=10", a.baseURL(), url.PathEscape(id)), nil)
		if err != nil {
			return nil, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			a.logError("restart", id, err, "")
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("docker restart failed (%d): %s", resp.StatusCode, string(body))
			a.logError("restart", id, err, string(body))
			return nil, err
		}
		return a.GetContainer(ctx, id)
	}

	return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
}

func (a *Adapter) DeleteContainer(ctx context.Context, id string, force bool) error {
	unitDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		if _, err := os.Stat(filepath.Join(unitDir, "compose.yaml")); err == nil {
			_ = a.runCompose(ctx, unitDir, id, "down", "-v")
		}
	}
	_ = os.RemoveAll(unitDir)

	if !a.hasSocket {
		return fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}

	deleteURL := fmt.Sprintf("%s/containers/%s?force=%t&v=1", a.baseURL(), url.PathEscape(id), force)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.logError("delete", id, err, "")
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("docker delete failed (%d): %s", resp.StatusCode, string(body))
		a.logError("delete", id, err, string(body))
		return err
	}
	return nil
}

func (a *Adapter) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	if a.hasSocket {
		logsURL := fmt.Sprintf("%s/containers/%s/logs?stdout=1&stderr=1&tail=%d&timestamps=1", a.baseURL(), url.PathEscape(id), tail)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, logsURL, nil)
		if err != nil {
			return "", err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			a.logError("logs", id, err, "")
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("%w: docker container %q", domain.ErrNotFound, id)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("docker logs failed (%d): %s", resp.StatusCode, string(body))
			a.logError("logs", id, err, string(body))
			return "", err
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		return stripDockerLogHeaders(data), nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.containers[id]
	if !ok {
		return "", fmt.Errorf("%w: docker container %q", domain.ErrNotFound, id)
	}
	now := time.Now().UTC()
	return fmt.Sprintf("[%s] Container %s (%s) starting up...\n[%s] Configuration loaded, listening on configured ports\n[%s] Health check: OK, running smoothly\n",
		now.Add(-5*time.Minute).Format(time.RFC3339),
		c.ContainerName, c.Image,
		now.Add(-4*time.Minute).Format(time.RFC3339),
		now.Add(-1*time.Minute).Format(time.RFC3339)), nil
}

type dockerHijackedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *dockerHijackedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// ExecAttach creates an interactive exec instance in the specified container and connects raw I/O to it.
func (a *Adapter) ExecAttach(ctx context.Context, id string, cmd []string) (net.Conn, string, error) {
	if !a.hasSocket {
		c1, c2 := net.Pipe()
		go func() {
			defer c2.Close()
			_, _ = fmt.Fprintf(c2, "Connected to simulated container %s (docker socket not connected)\r\n$ ", id)
			buf := make([]byte, 256)
			for {
				n, err := c2.Read(buf)
				if err != nil {
					break
				}
				_, _ = c2.Write(buf[:n])
			}
		}()
		return c1, "mock-exec-" + id, nil
	}

	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	createPayload := map[string]any{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          cmd,
	}
	bodyBytes, err := json.Marshal(createPayload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal exec create payload: %w", err)
	}

	createURL := fmt.Sprintf("%s/containers/%s/exec", a.baseURL(), url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("create exec request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("docker exec create request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("docker exec create failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(out)))
	}

	var execResp struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
		return nil, "", fmt.Errorf("decode exec create response: %w", err)
	}
	if execResp.ID == "" {
		return nil, "", fmt.Errorf("docker daemon returned empty exec ID")
	}

	var dialer net.Dialer
	rawConn, err := dialer.DialContext(ctx, "unix", a.socketPath)
	if err != nil {
		return nil, "", fmt.Errorf("dial docker socket for exec hijack: %w", err)
	}

	startPayload := `{"Detach":false,"Tty":true}`
	reqStr := fmt.Sprintf("POST /exec/%s/start HTTP/1.1\r\nHost: docker\r\nContent-Type: application/json\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: %d\r\n\r\n%s",
		execResp.ID, len(startPayload), startPayload)

	if _, err := rawConn.Write([]byte(reqStr)); err != nil {
		_ = rawConn.Close()
		return nil, "", fmt.Errorf("failed to send exec start request: %w", err)
	}

	br := bufio.NewReader(rawConn)
	startResp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = rawConn.Close()
		return nil, "", fmt.Errorf("failed to read exec start response: %w", err)
	}
	if startResp.StatusCode != http.StatusOK && startResp.StatusCode != http.StatusSwitchingProtocols {
		_ = rawConn.Close()
		return nil, "", fmt.Errorf("docker exec start returned status %d", startResp.StatusCode)
	}

	return &dockerHijackedConn{Conn: rawConn, r: br}, execResp.ID, nil
}

// ResizeExec resizes the TTY session for a running exec instance.
func (a *Adapter) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	if !a.hasSocket {
		return nil
	}
	resizeURL := fmt.Sprintf("%s/exec/%s/resize?h=%d&w=%d", a.baseURL(), url.PathEscape(execID), height, width)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resizeURL, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("resize failed with status %d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ensureImage(ctx context.Context, image string) {
	inspectURL := fmt.Sprintf("%s/images/%s/json", a.baseURL(), url.PathEscape(image))
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, inspectURL, nil); err == nil {
		if resp, err := a.client.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
	}
	pullURL := fmt.Sprintf("%s/images/create?fromImage=%s", a.baseURL(), url.QueryEscape(image))
	if req, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, nil); err == nil {
		if auth := a.registryAuthHeader(image); auth != "" {
			req.Header.Set("X-Registry-Auth", auth)
		}
		if resp, err := a.client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func (a *Adapter) getLiveStats(ctx context.Context, id string, allowSync bool) (float64, int64, int64, int64, int64, int64) {
	a.statsCacheMu.RLock()
	cached, hasCache := a.statsCache[id]
	inFlight := false
	if a.statsInFlight != nil {
		inFlight = a.statsInFlight[id]
	}
	a.statsCacheMu.RUnlock()

	// If cache is fresh (< 3 seconds old), return it immediately
	if hasCache && time.Since(cached.updated) < 3*time.Second {
		return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
	}

	if !hasCache && allowSync && a.hasSocket {
		a.fetchStatsAsync(id)
		a.statsCacheMu.RLock()
		cached, hasCache = a.statsCache[id]
		a.statsCacheMu.RUnlock()
		if hasCache {
			return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
		}
		return 0, 0, 0, 0, 0, 0
	}

	// Trigger async background refresh if not already in flight so API callers never stall on Docker's CPU sampling
	if !inFlight && a.hasSocket {
		a.statsCacheMu.Lock()
		if a.statsInFlight == nil {
			a.statsInFlight = make(map[string]bool)
		}
		if !a.statsInFlight[id] {
			a.statsInFlight[id] = true
			go a.fetchStatsAsync(id)
		}
		a.statsCacheMu.Unlock()
	}

	if hasCache {
		return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
	}
	return 0, 0, 0, 0, 0, 0
}

func (a *Adapter) fetchStatsAsync(id string) {
	defer func() {
		a.statsCacheMu.Lock()
		if a.statsInFlight != nil {
			delete(a.statsInFlight, id)
		}
		a.statsCacheMu.Unlock()
	}()

	tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	statsURL := fmt.Sprintf("%s/containers/%s/stats?stream=false", a.baseURL(), url.PathEscape(id))
	req, err := http.NewRequestWithContext(tctx, http.MethodGet, statsURL, nil)
	if err != nil {
		return
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var stats dockerStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return
	}

	var cpuPercent float64
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage) - float64(stats.PreCPUStats.SystemCPUUsage)
	onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	memMiB := int64(stats.MemoryStats.Usage / (1024 * 1024))

	var netRx, netTx int64
	for _, n := range stats.Networks {
		netRx += int64(n.RxBytes)
		netTx += int64(n.TxBytes)
	}

	now := time.Now()
	a.statsCacheMu.Lock()
	cached, hasCache := a.statsCache[id]
	if hasCache {
		if memMiB == 0 && cached.memMiB > 0 {
			memMiB = cached.memMiB
		}
		if netRx == 0 && cached.netRx > 0 {
			netRx = cached.netRx
		}
		if netTx == 0 && cached.netTx > 0 {
			netTx = cached.netTx
		}
	}

	var rxRate, txRate int64
	if hasCache && !cached.updated.IsZero() {
		dt := now.Sub(cached.updated).Seconds()
		if dt >= 0.4 {
			if netRx >= cached.netRx {
				rxRate = int64(float64(netRx-cached.netRx) / dt)
			}
			if netTx >= cached.netTx {
				txRate = int64(float64(netTx-cached.netTx) / dt)
			}
		} else {
			rxRate = cached.rxRate
			txRate = cached.txRate
		}
	}

	a.statsCache[id] = containerStatsRecord{
		cpuPct:  cpuPercent,
		memMiB:  memMiB,
		netRx:   netRx,
		netTx:   netTx,
		rxRate:  rxRate,
		txRate:  txRate,
		updated: now,
	}
	a.statsCacheMu.Unlock()
}

func stripDockerLogHeaders(data []byte) string {
	if len(data) < 8 {
		return strings.ToValidUTF8(string(data), "")
	}
	if (data[0] == 1 || data[0] == 2) && data[1] == 0 && data[2] == 0 && data[3] == 0 {
		var sb strings.Builder
		r := bytes.NewReader(data)
		header := make([]byte, 8)
		for {
			_, err := io.ReadFull(r, header)
			if err != nil {
				break
			}
			size := binary.BigEndian.Uint32(header[4:8])
			frame := make([]byte, size)
			n, err := io.ReadFull(r, frame)
			if n > 0 {
				sb.Write(frame[:n])
			}
			if err != nil {
				break
			}
		}
		if sb.Len() > 0 {
			return strings.ToValidUTF8(sb.String(), "")
		}
	}
	return strings.ToValidUTF8(string(data), "")
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// =============================================================================
// TrueNAS SCALE Style Docker Compose Applications (DockerAppPort)
// =============================================================================

func (a *Adapter) fetchLiveContainers(ctx context.Context) []dockerContainerSummary {
	var liveContainers []dockerContainerSummary
	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, "GET", a.baseEndpoint+"/containers/json?all=1", nil)
		if err == nil {
			resp, err := a.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = json.NewDecoder(resp.Body).Decode(&liveContainers)
				}
			}
		}
	}
	return liveContainers
}

// ListApps returns all deployed Docker Compose Applications.
func (a *Adapter) ListApps(ctx context.Context) ([]domain.DockerApp, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	liveContainers := a.fetchLiveContainers(ctx)

	appIDs := make(map[string]bool)
	if entries, err := os.ReadDir(a.unitsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				// Don't list legacy mock units unless they actually have live containers
				if e.Name() == "jellyfin" || e.Name() == "nextcloud" {
					hasLive := false
					for _, c := range liveContainers {
						if c.Labels["com.docker.compose.project"] == e.Name() ||
							containsString(c.Names, "/"+e.Name()) ||
							containsString(c.Names, "/"+e.Name()+"-app") ||
							containsString(c.Names, "/"+e.Name()+"-media") {
							hasLive = true
							break
						}
					}
					if !hasLive {
						continue
					}
				}
				composePath := filepath.Join(a.unitsDir, e.Name(), "compose.yaml")
				if _, err := os.Stat(composePath); err == nil {
					appIDs[e.Name()] = true
				}
			}
		}
	}

	// Also discover any compose projects running on host docker
	for _, c := range liveContainers {
		if proj := c.Labels["com.docker.compose.project"]; proj != "" {
			appIDs[proj] = true
		}
	}

	var apps []domain.DockerApp
	for appID := range appIDs {
		app, err := a.buildApp(ctx, appID, liveContainers)
		if err == nil {
			apps = append(apps, *app)
		}
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].ID < apps[j].ID
	})

	return apps, nil
}

// GetApp retrieves a single Docker Compose Application by ID.
func (a *Adapter) GetApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	composePath := filepath.Join(a.unitsDir, id, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		// Check if it's running on host docker
		liveContainers := a.fetchLiveContainers(ctx)
		found := false
		for _, c := range liveContainers {
			if c.Labels["com.docker.compose.project"] == id {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("app %q not found", id)
		}
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, id, liveContainers)
}

// CreateApp deploys a new Docker Compose Application asynchronously in the background.
func (a *Adapter) CreateApp(ctx context.Context, app domain.DockerApp) (*domain.DockerApp, error) {
	a.mu.Lock()

	if app.ID == "" && app.Name != "" {
		app.ID = domain.ToZfsSafeSlug(app.Name)
	} else if app.ID != "" {
		app.ID = domain.ToZfsSafeSlug(app.ID)
	}
	if app.ID == "" {
		app.ID = "app-" + domain.GenerateNanosecondID()
	}
	if app.Name == "" {
		app.Name = app.ID
	}

	// When no host is specified in mapping, pick a random high port in 32000+ range
	app.ComposeYAML = a.NormalizeComposePorts(ctx, app.ComposeYAML)
	app.ComposeYAML = NormalizeComposeEnvironment(app.ComposeYAML)
	app.ComposeYAML = a.NormalizeComposeVolumes(app.ID, app.ComposeYAML)

	// Preemptively check if all published host ports are available
	if err := a.checkComposeHostPortsAvailable(ctx, app.ComposeYAML, app.ID); err != nil {
		a.mu.Unlock()
		return nil, err
	}

	appDir := filepath.Join(a.unitsDir, app.ID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}

	composeFile := filepath.Join(appDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(app.ComposeYAML), 0644); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to write compose.yaml: %w", err)
	}

	if !a.hasSocket {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	a.startDeploymentLocked(appDir, app.ID, "Deploying stack (pulling images & starting containers)...")

	liveContainers := a.fetchLiveContainers(ctx)
	built, err := a.buildApp(ctx, app.ID, liveContainers)
	a.mu.Unlock()

	return built, err
}

// UpdateApp reconfigures and redeploys an existing Docker Compose Application asynchronously in the background.
func (a *Adapter) UpdateApp(ctx context.Context, app domain.DockerApp, fields []string) (*domain.DockerApp, error) {
	a.mu.Lock()

	// When no host is specified in mapping, pick a random high port in 32000+ range
	app.ComposeYAML = a.NormalizeComposePorts(ctx, app.ComposeYAML)
	app.ComposeYAML = NormalizeComposeEnvironment(app.ComposeYAML)
	app.ComposeYAML = a.NormalizeComposeVolumes(app.ID, app.ComposeYAML)

	// Preemptively check if all published host ports are available
	if err := a.checkComposeHostPortsAvailable(ctx, app.ComposeYAML, app.ID); err != nil {
		a.mu.Unlock()
		return nil, err
	}

	appDir := filepath.Join(a.unitsDir, app.ID)
	composeFile := filepath.Join(appDir, "compose.yaml")

	if err := os.MkdirAll(appDir, 0755); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}

	if err := os.WriteFile(composeFile, []byte(app.ComposeYAML), 0644); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to update compose.yaml: %w", err)
	}

	if !a.hasSocket {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	a.startDeploymentLocked(appDir, app.ID, "Deploying stack (pulling images & restarting containers)...")

	liveContainers := a.fetchLiveContainers(ctx)
	built, err := a.buildApp(ctx, app.ID, liveContainers)
	a.mu.Unlock()

	return built, err
}

func (a *Adapter) waitForDeployment(id string, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		deploying := false
		if a.deployingApps != nil {
			_, deploying = a.deployingApps[id]
		}
		a.mu.RUnlock()
		if !deploying {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// DeleteApp tears down a Docker Compose Application and purges its persisted descriptor.
func (a *Adapter) DeleteApp(ctx context.Context, id string, deleteVolumes bool) error {
	a.cancelDeployment(id, 15*time.Second)
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.deployingApps, id)
	delete(a.deployingErrors, id)

	appDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		args := []string{"down"}
		if deleteVolumes {
			args = append(args, "-v", "--remove-orphans")
		}
		_ = a.runCompose(ctx, appDir, id, args...)
	}

	_ = os.RemoveAll(appDir)
	return nil
}

// StartApp starts a Docker Compose Application. `docker compose up` may need to pull images, which
// can take far longer than an HTTP/gRPC request deadline, so it runs in the background and the app
// reports state DEPLOYING until it finishes.
func (a *Adapter) StartApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasSocket {
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	appDir := filepath.Join(a.unitsDir, id)
	composePath := filepath.Join(appDir, "compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		if _, busy := a.deployingApps[id]; !busy {
			if content, err := os.ReadFile(composePath); err == nil {
				// Normalize compose volumes, environment, and ports to auto-heal any legacy or corrupted files on disk
				normalized := a.NormalizeComposeVolumes(id, string(content))
				normalized = NormalizeComposeEnvironment(normalized)
				if normalized != string(content) {
					_ = os.WriteFile(composePath, []byte(normalized), 0644)
					content = []byte(normalized)
				}
				if err := a.checkComposeHostPortsAvailable(ctx, string(content), id); err != nil {
					return nil, err
				}
			}
			a.startDeploymentLocked(appDir, id, "Starting stack (pulling images if needed)...")
		}
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, id, liveContainers)
}

// StopApp stops all containers in a Docker Compose Application.
func (a *Adapter) StopApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a.cancelDeployment(id, 15*time.Second)
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasSocket {
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	appDir := filepath.Join(a.unitsDir, id)
	composePath := filepath.Join(appDir, "compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		if err := a.runCompose(ctx, appDir, id, "stop"); err != nil {
			return nil, err
		}
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, id, liveContainers)
}

// RestartApp restarts all containers in a Docker Compose Application.
func (a *Adapter) RestartApp(ctx context.Context, id string) (*domain.DockerApp, error) {
	a.waitForDeployment(id, 5*time.Second)
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.hasSocket {
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	appDir := filepath.Join(a.unitsDir, id)
	composePath := filepath.Join(appDir, "compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		if err := a.runCompose(ctx, appDir, id, "restart"); err != nil {
			return nil, err
		}
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, id, liveContainers)
}

// UpgradeApp pulls latest images for all services in the compose stack and redeploys with --remove-orphans in the background.
// If takeSnapshot is true and a ZFS engine port is configured, it automatically takes a ZFS snapshot of any matching app datasets before upgrading.
func (a *Adapter) UpgradeApp(ctx context.Context, id string, takeSnapshot bool) (*domain.DockerApp, error) {
	a.mu.Lock()

	appDir := filepath.Join(a.unitsDir, id)
	composePath := filepath.Join(appDir, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("app %q not found or has no compose.yaml", id)
	}

	if !a.hasSocket {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	content, err := os.ReadFile(composePath)
	if err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to read compose.yaml: %w", err)
	}

	// Normalize compose volumes & environment to ensure health before upgrade
	normalized := a.NormalizeComposeVolumes(id, string(content))
	normalized = NormalizeComposeEnvironment(normalized)
	if normalized != string(content) {
		_ = os.WriteFile(composePath, []byte(normalized), 0644)
		content = []byte(normalized)
	}

	_, _, vols, _, _, _, _, _, _, _ := parseComposeYAML(string(content))

	if takeSnapshot {
		a.snapshotAppVolumes(ctx, id, vols)
	}

	a.startUpgradeLocked(appDir, id, "Upgrading stack (pulling latest images & restarting containers)...")

	liveContainers := a.fetchLiveContainers(ctx)
	built, err := a.buildApp(ctx, id, liveContainers)
	a.mu.Unlock()

	return built, err
}

// GetAppLogs retrieves aggregated logs for a Docker Compose Application.
func (a *Adapter) GetAppLogs(ctx context.Context, id string, tail int, serviceName string) (string, error) {
	appDir := filepath.Join(a.unitsDir, id)
	if a.hasCompose && a.hasSocket {
		args := []string{"logs", "--no-color", fmt.Sprintf("--tail=%d", tail)}
		if serviceName != "" {
			args = append(args, serviceName)
		}
		out, err := a.runComposeOutput(ctx, appDir, id, args...)
		if err == nil {
			return strings.ToValidUTF8(out, ""), nil
		}
	}

	return "", fmt.Errorf("no logs available for app %q", id)
}

func (a *Adapter) runComposeOutput(ctx context.Context, unitDir string, projectName string, args ...string) (string, error) {
	if !a.hasCompose || len(a.composeCmd) == 0 {
		return "", fmt.Errorf("docker compose CLI is not available on host")
	}
	composeFile := filepath.Join(unitDir, "compose.yaml")
	var cmdArgs []string
	if len(a.composeCmd) > 1 {
		cmdArgs = append([]string{a.composeCmd[1], "-f", composeFile, "-p", projectName}, args...)
	} else {
		cmdArgs = append([]string{"-f", composeFile, "-p", projectName}, args...)
	}

	cmd := exec.CommandContext(ctx, a.composeCmd[0], cmdArgs...)
	cmd.Dir = unitDir
	cmd.Env = a.dockerCLIEnv()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return outBuf.String(), fmt.Errorf("docker compose %s failed: %w (details: %s)", strings.Join(args, " "), err, filterComposeOutput(errBuf.String()))
	}
	return outBuf.String(), nil
}

type rawPortalEntry struct {
	Name     string `yaml:"name"`
	Scheme   string `yaml:"scheme"`
	Protocol string `yaml:"protocol"`
	Host     string `yaml:"host"`
	Port     any    `yaml:"port"`
	Path     string `yaml:"path"`
}

type composeDescriptor struct {
	ManagementPort any    `yaml:"x-nas-management-port"`
	CatalogID      string `yaml:"x-nas-catalog-id"`
	CatalogSource  string `yaml:"x-nas-catalog-source"`
	CatalogVersion string `yaml:"x-nas-catalog-version"`
	XPortals       any    `yaml:"x-portals"`
	Portals        any    `yaml:"portals"`
	Services       map[string]struct {
		Image         string `yaml:"image"`
		ContainerName string `yaml:"container_name"`
		Ports         []any  `yaml:"ports"`
		Volumes       []any  `yaml:"volumes"`
		Labels        any    `yaml:"labels"`
		Restart       string `yaml:"restart"`
	} `yaml:"services"`
}

func parseRawPortals(raw any) []rawPortalEntry {
	if raw == nil {
		return nil
	}
	var result []rawPortalEntry
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				p := rawPortalEntry{
					Name:     fmt.Sprintf("%v", m["name"]),
					Scheme:   fmt.Sprintf("%v", m["scheme"]),
					Protocol: fmt.Sprintf("%v", m["protocol"]),
					Host:     fmt.Sprintf("%v", m["host"]),
					Port:     m["port"],
					Path:     fmt.Sprintf("%v", m["path"]),
				}
				if p.Name == "<nil>" || p.Name == "" {
					p.Name = "Web UI"
				}
				if p.Scheme == "<nil>" || p.Scheme == "" {
					if p.Protocol != "<nil>" && p.Protocol != "" {
						p.Scheme = p.Protocol
					} else {
						p.Scheme = "http"
					}
				}
				if p.Path == "<nil>" || p.Path == "" {
					p.Path = "/"
				}
				result = append(result, p)
			}
		}
	case map[string]any:
		for k, item := range v {
			if m, ok := item.(map[string]any); ok {
				name := k
				if m["name"] != nil {
					name = fmt.Sprintf("%v", m["name"])
				}
				scheme := "http"
				if m["scheme"] != nil {
					scheme = fmt.Sprintf("%v", m["scheme"])
				} else if m["protocol"] != nil {
					scheme = fmt.Sprintf("%v", m["protocol"])
				}
				path := "/"
				if m["path"] != nil {
					path = fmt.Sprintf("%v", m["path"])
				}
				result = append(result, rawPortalEntry{
					Name:   name,
					Scheme: scheme,
					Port:   m["port"],
					Path:   path,
				})
			}
		}
	}
	return result
}

func normalizePortSpec(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	proto := "tcp"
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		proto = strings.ToLower(s[idx+1:])
		s = s[:idx]
	}

	// Strip default 0.0.0.0: or [::]: or ::: prefix
	if strings.HasPrefix(s, "0.0.0.0:") {
		s = strings.TrimPrefix(s, "0.0.0.0:")
	} else if strings.HasPrefix(s, "[::]:") {
		s = strings.TrimPrefix(s, "[::]:")
	} else if strings.HasPrefix(s, "::: ") {
		s = strings.TrimPrefix(s, "::: ")
	}

	// Only append protocol if it's NOT default tcp
	if proto != "" && proto != "tcp" {
		return s + "/" + proto
	}
	return s
}

func parsePortMapping(p string) (hostPort, containerPort string) {
	p = strings.TrimSpace(p)
	if idx := strings.LastIndex(p, "/"); idx != -1 {
		p = p[:idx]
	}
	// Check "->" format from live docker: e.g. "0.0.0.0:8080->80"
	if idx := strings.Index(p, "->"); idx != -1 {
		hostPart := strings.TrimSpace(p[:idx])
		containerPart := strings.TrimSpace(p[idx+2:])
		if colon := strings.LastIndex(hostPart, ":"); colon != -1 {
			hostPart = hostPart[colon+1:]
		}
		return hostPart, containerPart
	}
	parts := strings.Split(p, ":")
	if len(parts) == 1 {
		return "", parts[0]
	} else if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	} else if len(parts) == 3 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	}
	return "", ""
}

func parseComposeYAML(content string) (services []string, ports []string, volumes []string, managementPort string, rawPortals []rawPortalEntry, autoStart bool, catalogID, catalogSource, catalogVersion string, err error) {
	var c composeDescriptor
	if err := yaml.Unmarshal([]byte(content), &c); err != nil {
		return nil, nil, nil, "", nil, false, "", "", "", err
	}
	var mgmtPort string
	if c.ManagementPort != nil {
		mgmtPort = strings.TrimSpace(fmt.Sprintf("%v", c.ManagementPort))
	}
	catalogID = strings.TrimSpace(c.CatalogID)
	catalogSource = strings.TrimSpace(c.CatalogSource)
	catalogVersion = strings.TrimSpace(c.CatalogVersion)
	rawPortals = parseRawPortals(c.XPortals)
	if len(rawPortals) == 0 {
		rawPortals = parseRawPortals(c.Portals)
	}
	portSet := make(map[string]bool)
	volSet := make(map[string]bool)
	for svcName, svc := range c.Services {
		services = append(services, svcName)
		r := strings.ToLower(strings.TrimSpace(svc.Restart))
		if r == "always" || r == "unless-stopped" {
			autoStart = true
		}
		if mgmtPort == "" && svc.Labels != nil {
			switch lv := svc.Labels.(type) {
			case []any:
				for _, l := range lv {
					ls := fmt.Sprintf("%v", l)
					if strings.HasPrefix(ls, "nas.management.port=") || strings.HasPrefix(ls, "nas.webui.port=") {
						parts := strings.SplitN(ls, "=", 2)
						if len(parts) == 2 {
							mgmtPort = strings.TrimSpace(parts[1])
						}
					}
				}
			case map[string]any:
				for k, v := range lv {
					if k == "nas.management.port" || k == "nas.webui.port" {
						mgmtPort = strings.TrimSpace(fmt.Sprintf("%v", v))
					}
				}
			}
		}
		for _, p := range svc.Ports {
			ps := normalizePortSpec(fmt.Sprintf("%v", p))
			if ps != "" && !portSet[ps] {
				portSet[ps] = true
				ports = append(ports, ps)
			}
		}
		for _, v := range svc.Volumes {
			vs := fmt.Sprintf("%v", v)
			if !volSet[vs] {
				volSet[vs] = true
				volumes = append(volumes, vs)
			}
		}
	}
	sort.Strings(services)
	sort.Strings(ports)
	sort.Strings(volumes)
	return services, ports, volumes, mgmtPort, rawPortals, autoStart, catalogID, catalogSource, catalogVersion, nil
}

func deriveWebUIURL(ports []string, preferredContainerPort string) string {
	if preferredContainerPort != "" {
		for _, p := range ports {
			hPort, cPort := parsePortMapping(p)
			if cPort == preferredContainerPort && hPort != "" && hPort != "53" {
				return fmt.Sprintf("http://localhost:%s", hPort)
			}
		}
	}

	// Fallback 1: Prefer well-known HTTP/Web ports
	wellKnownWebPorts := []string{"80", "81", "8080", "3000", "8000", "8088", "8384", "9000", "5000", "5001", "8443", "443"}
	for _, targetPort := range wellKnownWebPorts {
		for _, p := range ports {
			hPort, cPort := parsePortMapping(p)
			if cPort == targetPort && hPort != "" && hPort != "53" {
				return fmt.Sprintf("http://localhost:%s", hPort)
			}
		}
	}

	return ""
}

func (a *Adapter) buildApp(ctx context.Context, appID string, liveContainers []dockerContainerSummary) (*domain.DockerApp, error) {
	composePath := filepath.Join(a.unitsDir, appID, "compose.yaml")
	var composeYAML string
	if data, err := os.ReadFile(composePath); err == nil {
		composeYAML = prettifyComposeYAML(string(data))
	}

	services, parsedPorts, parsedVols, mgmtPort, rawPortals, autoStart, catID, catSrc, catVer, _ := parseComposeYAML(composeYAML)

	var appContainers []domain.DockerAppContainer
	var totalCPU float64
	var totalMem int64
	var totalNetRx, totalNetTx int64
	var totalNetRxRate, totalNetTxRate int64
	runningCount := 0
	completedCount := 0
	failedCount := 0

	for _, c := range liveContainers {
		proj := c.Labels["com.docker.compose.project"]
		isMatch := (proj == appID)
		if !isMatch {
			for _, n := range c.Names {
				cleanName := strings.TrimPrefix(n, "/")
				if cleanName == appID || strings.HasPrefix(cleanName, appID+"-") || strings.HasPrefix(cleanName, appID+"_") {
					isMatch = true
					break
				}
			}
		}
		if isMatch {
			cleanName := ""
			if len(c.Names) > 0 {
				cleanName = strings.TrimPrefix(c.Names[0], "/")
			}
			svcName := c.Labels["com.docker.compose.service"]
			if svcName == "" {
				svcName = cleanName
			}
			stateUpper := strings.ToUpper(c.State)
			statusClean := strings.TrimSpace(c.Status)
			isCompleted := strings.HasPrefix(statusClean, "Exited (0)") || strings.Contains(statusClean, "Exited (0)")

			if stateUpper == "RUNNING" {
				runningCount++
			} else if isCompleted {
				completedCount++
				stateUpper = "COMPLETED"
			} else if stateUpper == "EXITED" || stateUpper == "DEAD" {
				failedCount++
			}
			var ports []string
			for _, p := range c.Ports {
				var portStr string
				if p.PublicPort > 0 {
					if p.Type == "" || strings.EqualFold(p.Type, "tcp") {
						portStr = fmt.Sprintf("%d:%d", p.PublicPort, p.PrivatePort)
					} else {
						portStr = fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, strings.ToLower(p.Type))
					}
				} else if p.PrivatePort > 0 {
					if p.Type == "" || strings.EqualFold(p.Type, "tcp") {
						portStr = fmt.Sprintf("%d", p.PrivatePort)
					} else {
						portStr = fmt.Sprintf("%d/%s", p.PrivatePort, strings.ToLower(p.Type))
					}
				}
				portStr = normalizePortSpec(portStr)
				if portStr != "" && !slices.Contains(ports, portStr) {
					ports = append(ports, portStr)
				}
			}

			var cpuPct float64
			var memMiB int64
			var netRx, netTx int64
			var rxRate, txRate int64
			if stateUpper == "RUNNING" && a.hasSocket {
				cpuPct, memMiB, netRx, netTx, rxRate, txRate = a.getLiveStats(ctx, c.ID, false)
			}

			totalCPU += cpuPct
			totalMem += memMiB
			totalNetRx += netRx
			totalNetTx += netTx
			totalNetRxRate += rxRate
			totalNetTxRate += txRate

			idLen := 12
			if len(c.ID) < 12 {
				idLen = len(c.ID)
			}

			appContainers = append(appContainers, domain.DockerAppContainer{
				ID:          c.ID[:idLen],
				Name:        cleanName,
				ServiceName: svcName,
				Image:       c.Image,
				State:       stateUpper,
				Status:      c.Status,
				CPUPercent:  cpuPct,
				MemoryMiB:   memMiB,
				Ports:       ports,
				CreatedAt:   time.Unix(c.Created, 0).UTC(),
				NetRxBytes:  netRx,
				NetTxBytes:  netTx,
				NetRxRate:   rxRate,
				NetTxRate:   txRate,
			})
		}
	}

	if len(appContainers) == 0 {
		for _, svc := range services {
			appContainers = append(appContainers, domain.DockerAppContainer{
				ID:          "app-" + appID + "-" + svc,
				Name:        fmt.Sprintf("%s-%s", appID, svc),
				ServiceName: svc,
				Image:       "compose:" + svc,
				State:       "STOPPED",
				Status:      "Defined in compose",
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	state := "STOPPED"
	total := len(appContainers)
	if total > 0 && runningCount == total {
		state = "RUNNING"
	} else if runningCount > 0 && failedCount == 0 {
		// All non-running containers are completed one-shot / init tasks (Exited 0)
		state = "RUNNING"
	} else if runningCount > 0 {
		state = "PARTIALLY_RUNNING"
	}

	statusText := fmt.Sprintf("%d/%d running", runningCount, total)
	if completedCount > 0 && runningCount > 0 && failedCount == 0 {
		statusText = fmt.Sprintf("%d/%d running (%d completed)", runningCount, total, completedCount)
	} else if total == 0 {
		statusText = "No containers deployed"
	} else if runningCount == 0 {
		statusText = "Stopped"
	}

	if a.deployingApps != nil {
		if msg, isDeploying := a.deployingApps[appID]; isDeploying {
			state = "DEPLOYING"
			statusText = msg
		} else if errMsg, hasErr := a.deployingErrors[appID]; hasErr {
			if runningCount == 0 {
				state = "ERROR"
				statusText = errMsg
			} else {
				state = "ERROR"
				statusText = fmt.Sprintf("%d/%d running (%s)", runningCount, total, errMsg)
			}
		}
	}

	portSet := make(map[string]bool)
	var allPorts []string
	for _, p := range parsedPorts {
		np := normalizePortSpec(p)
		if np != "" && !portSet[np] {
			portSet[np] = true
			allPorts = append(allPorts, np)
		}
	}
	for _, ac := range appContainers {
		for _, ap := range ac.Ports {
			np := normalizePortSpec(ap)
			if np != "" && !portSet[np] {
				portSet[np] = true
				allPorts = append(allPorts, np)
			}
		}
	}
	sort.Strings(allPorts)

	webUI := deriveWebUIURL(allPorts, mgmtPort)

	var appPortals []domain.DockerAppPortal
	for _, rp := range rawPortals {
		portVal := 0
		switch pv := rp.Port.(type) {
		case int:
			portVal = pv
		case float64:
			portVal = int(pv)
		case string:
			portVal, _ = strconv.Atoi(strings.TrimSpace(pv))
		}

		scheme := rp.Scheme
		if scheme == "" || scheme == "<nil>" {
			scheme = "http"
		}
		path := rp.Path
		if path == "" || path == "<nil>" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		// Find mapped host port for container port
		resolvedHostPort := 0
		if portVal > 0 {
			portStr := strconv.Itoa(portVal)
			for _, p := range allPorts {
				hPort, cPort := parsePortMapping(p)
				if cPort == portStr && hPort != "" {
					if hp, err := strconv.Atoi(hPort); err == nil && hp > 0 {
						resolvedHostPort = hp
						break
					}
				}
				if hPort == portStr {
					if hp, err := strconv.Atoi(hPort); err == nil && hp > 0 {
						resolvedHostPort = hp
						break
					}
				}
			}
		}

		if resolvedHostPort > 0 {
			portalURL := fmt.Sprintf("%s://localhost:%d%s", scheme, resolvedHostPort, path)
			appPortals = append(appPortals, domain.DockerAppPortal{
				Name:     rp.Name,
				URL:      portalURL,
				Port:     resolvedHostPort,
				Protocol: scheme,
				Path:     path,
			})
		}
	}

	if len(appPortals) > 0 {
		webUI = appPortals[0].URL
	} else if webUI != "" {
		portVal := 80
		if u, err := url.Parse(webUI); err == nil {
			if p, err := strconv.Atoi(u.Port()); err == nil {
				portVal = p
			}
		}
		primaryPortal := domain.DockerAppPortal{
			Name:     "Web UI",
			URL:      webUI,
			Port:     portVal,
			Protocol: "http",
			Path:     "/",
		}
		appPortals = append(appPortals, primaryPortal)
	}

	title := strings.ReplaceAll(appID, "-", " ")
	if len(title) > 0 {
		title = strings.ToUpper(title[:1]) + title[1:]
	}

	return &domain.DockerApp{
		ID:              appID,
		Name:            domain.DockerAppName(appID),
		Title:           title,
		State:           state,
		StatusText:      statusText,
		AutoStart:       autoStart,
		CatalogAppID:    catID,
		CatalogSource:   catSrc,
		CatalogVersion:  catVer,
		ComposeYAML:     composeYAML,
		ComposeFilePath: composePath,
		Services:        services,
		Containers:      appContainers,
		PortMappings:    allPorts,
		Volumes:         parsedVols,
		WebUIURL:        webUI,
		Portals:         appPortals,
		TotalCPUPercent: math.Round(totalCPU*100) / 100,
		TotalMemoryMiB:  totalMem,
		TotalNetRxBytes: totalNetRx,
		TotalNetTxBytes: totalNetTx,
		TotalNetRxRate:  totalNetRxRate,
		TotalNetTxRate:  totalNetTxRate,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}, nil
}

type dockerEventMessage struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time int64 `json:"time"`
}

// StartEventWatcher launches a background worker that streams real-time container lifecycle events
// directly from the Docker Engine Unix socket and invokes onEvent for every state change.
func (a *Adapter) StartEventWatcher(ctx context.Context, onEvent func(domain.Event)) {
	if !a.hasSocket {
		return
	}
	go a.watchDockerEvents(ctx, onEvent)
}

func (a *Adapter) watchDockerEvents(ctx context.Context, onEvent func(domain.Event)) {
	streamClient := &http.Client{
		Transport: a.client.Transport,
		Timeout:   0, // no timeout for long-lived event stream
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		endpoint := a.baseURL() + "/events?filters=%7B%22type%22%3A%5B%22container%22%5D%7D"
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}

		resp, err := streamClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}

		bodyDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = resp.Body.Close()
			case <-bodyDone:
			}
		}()

		dec := json.NewDecoder(resp.Body)
		for {
			var msg dockerEventMessage
			if err := dec.Decode(&msg); err != nil {
				close(bodyDone)
				_ = resp.Body.Close()
				break
			}

			if msg.Type != "container" {
				continue
			}

			action := strings.ToLower(msg.Action)
			switch action {
			case "start", "stop", "die", "kill", "restart", "destroy", "create", "pause", "unpause":
				name := msg.Actor.Attributes["name"]
				if name == "" {
					name = msg.Actor.ID
					if len(name) > 12 {
						name = name[:12]
					}
				}
				project := msg.Actor.Attributes["com.docker.compose.project"]
				service := msg.Actor.Attributes["com.docker.compose.service"]

				var resource string
				if project != "" {
					resource = "dockerApps/" + project
				} else {
					resource = "dockerContainers/" + name
				}

				payload := map[string]any{
					"action":      action,
					"container":   name,
					"containerId": msg.Actor.ID,
				}
				if project != "" {
					payload["appId"] = project
					payload["service"] = service
				}

				evt := domain.NewEvent(
					"docker",
					strings.ToUpper(action),
					resource,
					fmt.Sprintf("Container %s %s", name, action),
				)
				evt.Payload = payload
				onEvent(evt)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// SetZfsPort sets the ZFS port used to discover available storage pools.
func (a *Adapter) SetZfsPort(p ports.ZfsEnginePort) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.zfsPort = p
}

// SetMainPool enforces that the main storage pool is the only choice for apps roots.
func (a *Adapter) SetMainPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mainPool = strings.TrimPrefix(strings.TrimSpace(pool), "zfsPools/")
}

// GetConfig returns the Docker storage & pool configuration.
// The main storage pool is the only permitted choice for apps roots.
func (a *Adapter) GetConfig(ctx context.Context) (*domain.DockerConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	mainPool := a.mainPool
	if mainPool == "" {
		mainPool = config.GetGlobal().System.MainPool
	}

	var availablePools []string
	var availableDatasets []string

	if mainPool != "" {
		// Main pool is the ONLY choice for roots for apps
		availablePools = []string{mainPool}
		if a.zfsPort != nil {
			availableDatasets = a.listDatasetPaths(ctx, mainPool)
		}
	} else if a.zfsPort != nil {
		if pools, err := a.zfsPort.ListPools(ctx); err == nil && len(pools) > 0 {
			for _, p := range pools {
				cleanID := strings.TrimPrefix(p.ID, "zfsPools/")
				availablePools = append(availablePools, cleanID)
				availableDatasets = append(availableDatasets, a.listDatasetPaths(ctx, p.ID)...)
			}
		}
	}
	if len(availablePools) == 0 {
		availablePools = []string{"tank"}
	}
	if len(availableDatasets) == 0 {
		availableDatasets = []string{fmt.Sprintf("%s/apps", availablePools[0])}
	}

	hasMainPool := (mainPool != "")

	configFile := filepath.Join(a.unitsDir, "docker_config.json")
	if data, err := os.ReadFile(configFile); err == nil {
		var cfg domain.DockerConfig
		if json.Unmarshal(data, &cfg) == nil && (cfg.Dataset != "" || cfg.Pool != "") {
			// If a main pool is configured, enforce that docker config belongs to main pool
			if mainPool != "" {
				cfg.Pool = mainPool
				if !strings.HasPrefix(cfg.Dataset, mainPool+"/") && cfg.Dataset != mainPool {
					cfg.Dataset = fmt.Sprintf("%s/apps", mainPool)
					if slices.Contains(availableDatasets, fmt.Sprintf("%s/ix-apps", mainPool)) && !slices.Contains(availableDatasets, cfg.Dataset) {
						cfg.Dataset = fmt.Sprintf("%s/ix-apps", mainPool)
					}
				}
				cfg.DatasetSelected = true
				cfg.PoolSelected = true
			}
			cfg.AvailablePools = availablePools
			cfg.AvailableDatasets = availableDatasets
			if cfg.Dataset == "" && cfg.Pool != "" {
				cfg.Dataset = fmt.Sprintf("%s/apps", cfg.Pool)
			}
			if cfg.Pool == "" && cfg.Dataset != "" {
				parts := strings.Split(cfg.Dataset, "/")
				cfg.Pool = parts[0]
			}
			if cfg.Mountpoint == "" || strings.HasPrefix(cfg.Mountpoint, "/tank") || (!strings.HasPrefix(cfg.Mountpoint, "/mnt/") && !strings.HasPrefix(cfg.Mountpoint, "/nas/") && cfg.Pool != "") {
				poolMount := fmt.Sprintf("/mnt/%s", cfg.Pool)
				if a.zfsPort != nil {
					if p, err := a.zfsPort.GetPool(ctx, cfg.Pool); err == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
						poolMount = p.Mountpoint
					}
				}
				datasetSuffix := strings.TrimPrefix(cfg.Dataset, cfg.Pool)
				datasetSuffix = strings.TrimPrefix(datasetSuffix, "/")
				if datasetSuffix == "" {
					datasetSuffix = "apps"
				}
				cfg.Mountpoint = fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), datasetSuffix)
			}
			if cfg.AppdataPath == "" || strings.HasPrefix(cfg.AppdataPath, "/tank") || (!strings.HasPrefix(cfg.AppdataPath, "/mnt/") && !strings.HasPrefix(cfg.AppdataPath, "/nas/") && cfg.Pool != "") {
				cfg.AppdataPath = cfg.Mountpoint
			}
			if cfg.DockerDataRoot == "" || strings.HasPrefix(cfg.DockerDataRoot, "/tank") || (!strings.HasPrefix(cfg.DockerDataRoot, "/mnt/") && !strings.HasPrefix(cfg.DockerDataRoot, "/nas/") && cfg.Pool != "") {
				cfg.DockerDataRoot = filepath.Join(cfg.Mountpoint, "docker")
			}
			cfg.AppdataDataset = cfg.AppdataPath
			if hasMainPool {
				cfg.DatasetSelected = true
				cfg.PoolSelected = true
			} else {
				cfg.PoolSelected = cfg.DatasetSelected
			}
			return &cfg, nil
		}
	}

	// Default to main pool (or first pool if unconfigured)
	defaultPool := availablePools[0]
	defaultDataset := fmt.Sprintf("%s/apps", defaultPool)
	if slices.Contains(availableDatasets, fmt.Sprintf("%s/ix-apps", defaultPool)) && !slices.Contains(availableDatasets, defaultDataset) {
		defaultDataset = fmt.Sprintf("%s/ix-apps", defaultPool)
	}
	poolMount := fmt.Sprintf("/mnt/%s", defaultPool)
	if a.zfsPort != nil {
		if p, err := a.zfsPort.GetPool(ctx, defaultPool); err == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
			poolMount = p.Mountpoint
		}
	}
	dsSuffix := strings.TrimPrefix(defaultDataset, defaultPool)
	dsSuffix = strings.TrimPrefix(dsSuffix, "/")
	defaultMountpoint := fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), dsSuffix)
	return &domain.DockerConfig{
		Dataset:           defaultDataset,
		Pool:              defaultPool,
		Mountpoint:        defaultMountpoint,
		AppdataPath:       defaultMountpoint,
		DockerDataRoot:    filepath.Join(defaultMountpoint, "docker"),
		AvailableDatasets: availableDatasets,
		AvailablePools:    availablePools,
		DatasetSelected:   hasMainPool,
		AppdataDataset:    defaultMountpoint,
		PoolSelected:      hasMainPool,
	}, nil
}

// UpdateConfig updates the Docker owned ZFS dataset and ensures required datasets exist.
func (a *Adapter) UpdateConfig(ctx context.Context, cfg domain.DockerConfig) (*domain.DockerConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	mainPool := a.mainPool
	if mainPool == "" {
		mainPool = config.GetGlobal().System.MainPool
	}

	rawDataset := strings.TrimSpace(cfg.Dataset)
	if rawDataset == "" && cfg.Pool != "" {
		rawDataset = fmt.Sprintf("%s/apps", strings.TrimPrefix(cfg.Pool, "zfsPools/"))
	}
	cleanDataset := strings.Trim(rawDataset, "/")
	cleanDataset = strings.TrimPrefix(cleanDataset, "zfsPools/")
	cleanDataset = strings.ReplaceAll(cleanDataset, "/datasets/", "/")
	if cleanDataset == "" {
		return nil, fmt.Errorf("ZFS dataset cannot be empty")
	}

	parts := strings.Split(cleanDataset, "/")
	pool := parts[0]
	if mainPool != "" && pool != mainPool {
		return nil, fmt.Errorf("%w: apps root must reside on the main pool (%s); pool %q is not permitted", domain.ErrInvalidArgument, mainPool, pool)
	}
	datasetID := "apps"
	if len(parts) > 1 {
		datasetID = strings.Join(parts[1:], "/")
	}

	poolMount := fmt.Sprintf("/mnt/%s", pool)
	if a.zfsPort != nil {
		if p, err := a.zfsPort.GetPool(ctx, pool); err == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
			poolMount = p.Mountpoint
		}
	}
	mountpoint := fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), datasetID)
	appdataDir := mountpoint
	dockerDataDir := filepath.Join(mountpoint, "docker")

	// Create dataset in ZFS engine if possible
	if a.zfsPort != nil {
		_, _ = a.zfsPort.CreateDataset(ctx, domain.ZfsDataset{
			PoolID:      pool,
			ID:          datasetID,
			DatasetPath: cleanDataset,
			Mountpoint:  mountpoint,
			Compression: "lz4",
		})
	}

	// Ensure directories exist on host filesystem (ignoring errors if running in restricted test environments)
	_ = os.MkdirAll(appdataDir, 0755)
	_ = os.MkdirAll(dockerDataDir, 0755)

	savedCfg := domain.DockerConfig{
		Dataset:         cleanDataset,
		Pool:            pool,
		Mountpoint:      mountpoint,
		AppdataPath:     appdataDir,
		DockerDataRoot:  dockerDataDir,
		DatasetSelected: true,
		AppdataDataset:  appdataDir,
		PoolSelected:    true,
	}

	data, err := json.MarshalIndent(savedCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal docker config: %w", err)
	}

	configFile := filepath.Join(a.unitsDir, "docker_config.json")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write docker config file: %w", err)
	}

	// Populate available options for return
	var availablePools []string
	var availableDatasets []string
	if a.zfsPort != nil {
		if pools, err := a.zfsPort.ListPools(ctx); err == nil && len(pools) > 0 {
			for _, p := range pools {
				availablePools = append(availablePools, p.ID)
				availableDatasets = append(availableDatasets, a.listDatasetPaths(ctx, p.ID)...)
			}
		}
	}
	if len(availablePools) == 0 {
		availablePools = []string{pool}
	}
	if len(availableDatasets) == 0 {
		availableDatasets = []string{cleanDataset}
	}
	savedCfg.AvailablePools = availablePools
	savedCfg.AvailableDatasets = availableDatasets

	return &savedCfg, nil
}

// ListDirectories lists subdirectories for host path auto-completion and directory browsing.
func (a *Adapter) ListDirectories(ctx context.Context, reqPath string) (*domain.DirectoryList, error) {
	if reqPath == "" {
		cfg, _ := a.GetConfig(ctx)
		if cfg != nil && cfg.Mountpoint != "" {
			reqPath = cfg.Mountpoint
		} else if cfg != nil && cfg.Pool != "" {
			reqPath = fmt.Sprintf("/mnt/%s", cfg.Pool)
		} else {
			reqPath = "/mnt"
		}
	}

	cleanPath := filepath.Clean(reqPath)
	if strings.HasPrefix(cleanPath, "/tank") {
		cleanPath = "/mnt" + cleanPath
	}
	if !filepath.IsAbs(cleanPath) {
		cleanPath = "/" + cleanPath
	}

	parentPath := filepath.Dir(cleanPath)
	if cleanPath == "/" {
		parentPath = "/"
	}

	res := &domain.DirectoryList{
		CurrentPath: cleanPath,
		ParentPath:  parentPath,
		Entries:     make([]domain.DirectoryEntry, 0),
	}

	entries, err := os.ReadDir(cleanPath)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				res.Entries = append(res.Entries, domain.DirectoryEntry{
					Name:  e.Name(),
					Path:  filepath.Join(cleanPath, e.Name()),
					IsDir: true,
				})
			}
		}
	}

	sort.Slice(res.Entries, func(i, j int) bool {
		return strings.ToLower(res.Entries[i].Name) < strings.ToLower(res.Entries[j].Name)
	})

	return res, nil
}

func (a *Adapter) isContainerOfApp(c dockerContainerSummary, appID string) bool {
	if appID == "" {
		return false
	}
	cleanAppID := strings.ToLower(strings.TrimSpace(appID))
	if c.Labels != nil {
		proj := strings.ToLower(c.Labels["com.docker.compose.project"])
		nasID := strings.ToLower(c.Labels["nas.app.id"])
		if proj == cleanAppID || nasID == cleanAppID {
			return true
		}
		cleanProj := strings.ReplaceAll(proj, "-", "")
		cleanTarget := strings.ReplaceAll(cleanAppID, "-", "")
		if cleanProj == cleanTarget && cleanProj != "" {
			return true
		}
	}
	for _, n := range c.Names {
		cleanName := strings.ToLower(strings.TrimPrefix(n, "/"))
		if cleanName == cleanAppID ||
			strings.HasPrefix(cleanName, cleanAppID+"-") ||
			strings.HasPrefix(cleanName, cleanAppID+"_") {
			return true
		}
	}
	return false
}

func (a *Adapter) isPortOwnedByApp(ctx context.Context, port int, proto string, appID string, liveContainers []dockerContainerSummary) bool {
	if appID == "" {
		return false
	}

	// 1. Check live containers belonging to this app
	for _, c := range liveContainers {
		if a.isContainerOfApp(c, appID) {
			for _, p := range c.Ports {
				if int(p.PublicPort) == port && (proto == "" || strings.EqualFold(p.Type, proto)) {
					return true
				}
			}
		}
	}

	// 2. Check the existing compose.yaml for this app if it exists on disk
	appDir := filepath.Join(a.unitsDir, appID)
	existingCompose := filepath.Join(appDir, "compose.yaml")
	if data, err := os.ReadFile(existingCompose); err == nil {
		var c composeDescriptor
		if err := yaml.Unmarshal(data, &c); err == nil {
			for _, svc := range c.Services {
				for _, portRaw := range svc.Ports {
					var ps string
					switch pv := portRaw.(type) {
					case string:
						ps = pv
					case int:
						ps = strconv.Itoa(pv)
					case map[string]any:
						pub := fmt.Sprintf("%v", pv["published"])
						tgt := fmt.Sprintf("%v", pv["target"])
						prt := fmt.Sprintf("%v", pv["protocol"])
						if prt == "<nil>" || prt == "" {
							prt = "tcp"
						}
						if pub != "<nil>" && pub != "" {
							ps = fmt.Sprintf("%s:%s/%s", pub, tgt, prt)
						} else {
							ps = fmt.Sprintf("%s/%s", tgt, prt)
						}
					}
					hPort, _ := parsePortMapping(ps)
					if hPort != "" {
						cleanHPort := hPort
						if idx := strings.LastIndex(cleanHPort, ":"); idx != -1 {
							cleanHPort = cleanHPort[idx+1:]
						}
						// parsePortMapping strips the "/proto" suffix, so derive the protocol from the raw spec.
						pProto := "tcp"
						if strings.HasSuffix(strings.ToLower(strings.TrimSpace(ps)), "/udp") {
							pProto = "udp"
						}
						if num, err := strconv.Atoi(cleanHPort); err == nil && num == port {
							if proto == "" || strings.EqualFold(proto, pProto) {
								return true
							}
						}
					}
				}
			}
		}
	}

	return false
}

// CheckHostPort checks preemptively if a specific host port is free or already allocated.
func (a *Adapter) CheckHostPort(ctx context.Context, port int, protocol string, excludeAppID string) (domain.HostPortStatus, error) {
	if port <= 0 || port > 65535 {
		return domain.HostPortStatus{
			Port:      port,
			Protocol:  protocol,
			Available: false,
			InUseBy:   "invalid port number (must be 1-65535)",
		}, nil
	}

	proto := strings.ToLower(protocol)
	if proto == "" {
		proto = "tcp"
	}

	liveContainers := a.fetchLiveContainers(ctx)

	// If the port is already owned/published by the excluded app itself, it is available for reuse (e.g. during redeploy)
	if excludeAppID != "" && a.isPortOwnedByApp(ctx, port, proto, excludeAppID, liveContainers) {
		return domain.HostPortStatus{
			Port:      port,
			Protocol:  proto,
			Available: true,
			InUseBy:   "",
		}, nil
	}

	// 1. Check against active Docker container port bindings
	for _, c := range liveContainers {
		if excludeAppID != "" && a.isContainerOfApp(c, excludeAppID) {
			continue
		}

		for _, p := range c.Ports {
			if int(p.PublicPort) == port && (proto == "" || strings.EqualFold(p.Type, proto)) {
				cName := "unknown"
				if len(c.Names) > 0 {
					cName = strings.TrimPrefix(c.Names[0], "/")
				}
				return domain.HostPortStatus{
					Port:      port,
					Protocol:  proto,
					Available: false,
					InUseBy:   fmt.Sprintf("docker container %q (%s)", cName, c.Image),
				}, nil
			}
		}
	}

	// 2. Check host socket binding via OS network stack
	if proto == "tcp" {
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			inUseBy := a.identifyProcessUsingPort(port, "tcp")
			return domain.HostPortStatus{
				Port:      port,
				Protocol:  "tcp",
				Available: false,
				InUseBy:   inUseBy,
			}, nil
		}
		_ = ln.Close()

		if ln6, err6 := net.Listen("tcp6", fmt.Sprintf("[::]:%d", port)); err6 == nil {
			_ = ln6.Close()
		}
	} else if proto == "udp" {
		pc, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			inUseBy := a.identifyProcessUsingPort(port, "udp")
			return domain.HostPortStatus{
				Port:      port,
				Protocol:  "udp",
				Available: false,
				InUseBy:   inUseBy,
			}, nil
		}
		_ = pc.Close()
	}

	return domain.HostPortStatus{
		Port:      port,
		Protocol:  proto,
		Available: true,
		InUseBy:   "",
	}, nil
}

// CheckHostPorts checks a slice of host ports for availability.
func (a *Adapter) CheckHostPorts(ctx context.Context, ports []int, protocol string, excludeAppID string) ([]domain.HostPortStatus, error) {
	results := make([]domain.HostPortStatus, 0, len(ports))
	for _, p := range ports {
		st, err := a.CheckHostPort(ctx, p, protocol, excludeAppID)
		if err != nil {
			results = append(results, domain.HostPortStatus{
				Port:      p,
				Protocol:  protocol,
				Available: false,
				InUseBy:   err.Error(),
			})
		} else {
			results = append(results, st)
		}
	}
	return results, nil
}

func (a *Adapter) identifyProcessUsingPort(port int, proto string) string {
	if ssPath, err := exec.LookPath("ss"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, ssPath, "-H", "-t", "-u", "-l", "-p", "-n", fmt.Sprintf("sport = :%d", port))
		if out, err := cmd.Output(); err == nil && len(out) > 0 {
			outStr := string(out)
			if idx := strings.Index(outStr, `users:(("`); idx != -1 {
				sub := outStr[idx+len(`users:(("`):]
				if endIdx := strings.Index(sub, `"`); endIdx != -1 {
					procName := sub[:endIdx]
					return fmt.Sprintf("host process %q", procName)
				}
			}
		}
	}

	switch port {
	case 22:
		return "host service (SSH)"
	case 53:
		return "host service (DNS/systemd-resolved)"
	case 80:
		return "host service (HTTP/Web)"
	case 137, 138, 139, 445:
		return "host service (Samba/SMB)"
	case 443:
		return "host service (HTTPS)"
	case 631:
		return "host service (CUPS)"
	case 3306:
		return "host service (MySQL/MariaDB)"
	case 5432:
		return "host service (PostgreSQL)"
	case 5900, 5901, 5902, 5903:
		return "host service (VNC)"
	case 6379:
		return "host service (Redis)"
	default:
		return "host system process"
	}
}

func (a *Adapter) checkComposeHostPortsAvailable(ctx context.Context, composeYAML string, appID string) error {
	if strings.TrimSpace(composeYAML) == "" {
		return nil
	}

	var c composeDescriptor
	if err := yaml.Unmarshal([]byte(composeYAML), &c); err != nil {
		return nil
	}

	seen := make(map[string]string) // key -> serviceName
	for svcName, svc := range c.Services {
		for _, portRaw := range svc.Ports {
			var ps string
			switch pv := portRaw.(type) {
			case string:
				ps = pv
			case int:
				ps = strconv.Itoa(pv)
			case map[string]any:
				published := fmt.Sprintf("%v", pv["published"])
				target := fmt.Sprintf("%v", pv["target"])
				protocol := fmt.Sprintf("%v", pv["protocol"])
				if protocol == "<nil>" || protocol == "" {
					protocol = "tcp"
				}
				if published != "<nil>" && published != "" {
					ps = fmt.Sprintf("%s:%s/%s", published, target, protocol)
				} else {
					ps = fmt.Sprintf("%s/%s", target, protocol)
				}
			default:
				continue
			}

			hostP, _ := parsePortMapping(ps)
			if hostP == "" {
				continue // Internal container port or not published to host
			}

			// parsePortMapping strips the "/proto" suffix, so derive the protocol from the raw spec.
			proto := "tcp"
			if strings.HasSuffix(strings.ToLower(strings.TrimSpace(ps)), "/udp") {
				proto = "udp"
			}
			cleanHostP := hostP
			if idx := strings.LastIndex(cleanHostP, ":"); idx != -1 {
				cleanHostP = cleanHostP[idx+1:]
			}

			portNum, err := strconv.Atoi(cleanHostP)
			if err != nil || portNum <= 0 {
				continue
			}

			key := fmt.Sprintf("%d/%s", portNum, proto)
			if priorSvc, ok := seen[key]; ok {
				return fmt.Errorf("port conflict: host port %d/%s is mapped multiple times in compose file (service %s and %s)", portNum, proto, priorSvc, svcName)
			}
			seen[key] = svcName

			st, err := a.CheckHostPort(ctx, portNum, proto, appID)
			if err == nil && !st.Available {
				return fmt.Errorf("port conflict: host port %d/%s is already in use by %s; cannot bind port", portNum, proto, st.InUseBy)
			}
		}
	}

	return nil
}

func (a *Adapter) pickRandomHighPort(ctx context.Context, proto string, usedPorts map[int]bool) int {
	const minPort = 32000
	const maxPort = 65000
	for attempt := 0; attempt < 500; attempt++ {
		cand := rand.IntN(maxPort-minPort+1) + minPort
		if usedPorts != nil && usedPorts[cand] {
			continue
		}
		st, err := a.CheckHostPort(ctx, cand, proto, "")
		if err == nil && st.Available {
			if usedPorts != nil {
				usedPorts[cand] = true
			}
			return cand
		}
	}
	cand := rand.IntN(maxPort-minPort+1) + minPort
	if usedPorts != nil {
		usedPorts[cand] = true
	}
	return cand
}

// NormalizeComposeVolumes rewrites TrueNAS volume descriptors, filters out settings,
// and normalizes volume mounts to the NAS apps pool.
func (a *Adapter) NormalizeComposeVolumes(appID, composeYAML string) string {
	pool := a.mainPool
	if pool == "" {
		pool = config.GetGlobal().System.MainPool
	}
	if pool == "" {
		pool = "tank"
	}
	return NormalizeComposeVolumes(appID, pool, composeYAML)
}

// NormalizeComposePorts inspects the Compose YAML and assigns a random high host port (32000-65000)
// to any published port mapping that omits an explicit host port.
func (a *Adapter) NormalizeComposePorts(ctx context.Context, composeYAML string) string {
	if strings.TrimSpace(composeYAML) == "" {
		return composeYAML
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeYAML), &root); err != nil {
		return composeYAML
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return composeYAML
	}

	docMap := root.Content[0]
	usedPorts := make(map[int]bool)
	var servicesNode *yaml.Node

	for i := 0; i < len(docMap.Content); i += 2 {
		if docMap.Content[i].Value == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
			servicesNode = docMap.Content[i+1]
			break
		}
	}

	if servicesNode == nil {
		return composeYAML
	}

	extractProto := func(pStr string) string {
		if strings.HasSuffix(strings.ToLower(pStr), "/udp") {
			return "udp"
		}
		return "tcp"
	}

	// 1. Scan existing host ports
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcMap := servicesNode.Content[i]
		if svcMap.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(svcMap.Content); j += 2 {
			if svcMap.Content[j].Value == "ports" && svcMap.Content[j+1].Kind == yaml.SequenceNode {
				portsSeq := svcMap.Content[j+1]
				for _, pNode := range portsSeq.Content {
					if pNode.Kind == yaml.ScalarNode {
						hPort, _ := parsePortMapping(pNode.Value)
						if hPort != "" {
							if hp, err := strconv.Atoi(hPort); err == nil && hp > 0 {
								usedPorts[hp] = true
							}
						}
					} else if pNode.Kind == yaml.MappingNode {
						for k := 0; k < len(pNode.Content); k += 2 {
							if pNode.Content[k].Value == "published" {
								if hp, err := strconv.Atoi(pNode.Content[k+1].Value); err == nil && hp > 0 {
									usedPorts[hp] = true
								}
							}
						}
					}
				}
			}
		}
	}

	// 2. Normalize ports without host mapping
	modified := false
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcMap := servicesNode.Content[i]
		if svcMap.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(svcMap.Content); j += 2 {
			if svcMap.Content[j].Value == "ports" && svcMap.Content[j+1].Kind == yaml.SequenceNode {
				portsSeq := svcMap.Content[j+1]
				for _, pNode := range portsSeq.Content {
					if pNode.Kind == yaml.ScalarNode {
						val := strings.TrimSpace(pNode.Value)
						hPort, cPort := parsePortMapping(val)
						if hPort == "" && cPort != "" {
							proto := extractProto(val)
							cleanCPort := strings.TrimSuffix(strings.TrimSuffix(cPort, "/tcp"), "/udp")
							randPort := a.pickRandomHighPort(ctx, proto, usedPorts)
							if proto == "udp" {
								pNode.Value = fmt.Sprintf("%d:%s/udp", randPort, cleanCPort)
							} else {
								pNode.Value = fmt.Sprintf("%d:%s", randPort, cleanCPort)
							}
							pNode.Tag = "!!str"
							pNode.Style = yaml.DoubleQuotedStyle
							modified = true
						}
					} else if pNode.Kind == yaml.MappingNode {
						var pubNode *yaml.Node
						var targetNode *yaml.Node
						var protoNode *yaml.Node
						for k := 0; k < len(pNode.Content); k += 2 {
							key := pNode.Content[k].Value
							val := pNode.Content[k+1]
							switch key {
							case "published":
								pubNode = val
							case "target":
								targetNode = val
							case "protocol":
								protoNode = val
							}
						}
						proto := "tcp"
						if protoNode != nil && strings.ToLower(protoNode.Value) == "udp" {
							proto = "udp"
						}
						needsHost := false
						if pubNode == nil {
							needsHost = true
						} else {
							hp, err := strconv.Atoi(pubNode.Value)
							if err != nil || hp <= 0 {
								needsHost = true
							}
						}
						if needsHost && targetNode != nil && targetNode.Value != "" {
							randPort := a.pickRandomHighPort(ctx, proto, usedPorts)
							if pubNode != nil {
								pubNode.Value = strconv.Itoa(randPort)
								pubNode.Tag = "!!int"
								pubNode.Style = 0
							} else {
								keyNode := &yaml.Node{
									Kind:  yaml.ScalarNode,
									Tag:   "!!str",
									Value: "published",
								}
								valNode := &yaml.Node{
									Kind:  yaml.ScalarNode,
									Tag:   "!!int",
									Value: strconv.Itoa(randPort),
								}
								pNode.Content = append(pNode.Content, keyNode, valNode)
							}
							modified = true
						}
					}
				}
			}
		}
	}

	if !modified {
		return composeYAML
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return composeYAML
	}
	return buf.String()
}

// listDatasetPaths returns dataset names for a pool, preferring a cheap name-only listing when the
// ZFS port supports it (avoids per-dataset property and snapshot statistics collection).
func (a *Adapter) listDatasetPaths(ctx context.Context, poolID string) []string {
	if a.zfsPort == nil {
		return nil
	}
	if lister, ok := a.zfsPort.(interface {
		ListDatasetPaths(ctx context.Context, poolID string) ([]string, error)
	}); ok {
		if names, err := lister.ListDatasetPaths(ctx, poolID); err == nil {
			return names
		}
	}
	var names []string
	if datasets, err := a.zfsPort.ListDatasets(ctx, poolID); err == nil {
		for _, d := range datasets {
			names = append(names, d.DatasetPath)
		}
	}
	return names
}

// startDeploymentLocked marks the app as deploying and runs `docker compose up -d` in the background
// with its own long-lived context, so client request deadlines cannot cancel image pulls.
// Caller must hold a.mu. A second call while a deployment is in flight is a no-op.
func (a *Adapter) startDeploymentLocked(dir, id, msg string) {
	if _, busy := a.deployingApps[id]; busy {
		return
	}
	if a.deployCancel == nil {
		a.deployCancel = make(map[string]context.CancelFunc)
	}
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	a.deployingApps[id] = msg
	a.deployCancel[id] = cancel
	delete(a.deployingErrors, id)

	go func() {
		defer cancel()
		runErr := a.runCompose(bgCtx, dir, id, "up", "-d", "--remove-orphans")
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.deployCancel, id)
		if _, stillDeploying := a.deployingApps[id]; !stillDeploying {
			return
		}
		delete(a.deployingApps, id)
		if runErr != nil && !isComposeCanceled(runErr, "") {
			a.deployingErrors[id] = fmt.Sprintf("Deployment failed: %v", runErr)
		}
	}()
}

// startUpgradeLocked marks the app as deploying, pulls latest images with `docker compose pull`,
// and runs `docker compose up -d --remove-orphans` in the background.
// Caller must hold a.mu.
func (a *Adapter) startUpgradeLocked(dir, id, msg string) {
	if _, busy := a.deployingApps[id]; busy {
		return
	}
	if a.deployCancel == nil {
		a.deployCancel = make(map[string]context.CancelFunc)
	}
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	a.deployingApps[id] = msg
	a.deployCancel[id] = cancel
	delete(a.deployingErrors, id)

	go func() {
		defer cancel()
		_ = a.runCompose(bgCtx, dir, id, "pull")
		runErr := a.runCompose(bgCtx, dir, id, "up", "-d", "--remove-orphans")
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.deployCancel, id)
		if _, stillDeploying := a.deployingApps[id]; !stillDeploying {
			return
		}
		delete(a.deployingApps, id)
		if runErr != nil && !isComposeCanceled(runErr, "") {
			a.deployingErrors[id] = fmt.Sprintf("Upgrade failed: %v", runErr)
		}
	}()
}

func (a *Adapter) snapshotAppVolumes(ctx context.Context, id string, volumes []string) {
	if a.zfsPort == nil {
		return
	}
	snapSuffix := fmt.Sprintf("pre-upgrade-%s", time.Now().UTC().Format("20060102-150405"))
	snapshotted := make(map[string]bool)

	pools, err := a.zfsPort.ListPools(ctx)
	if err != nil || len(pools) == 0 {
		return
	}

	for _, pool := range pools {
		poolName := pool.PoolName
		datasets, err := a.zfsPort.ListDatasets(ctx, poolName)
		if err != nil {
			continue
		}
		for _, d := range datasets {
			dsPath := d.DatasetPath
			cleanID := strings.TrimSpace(id)
			if cleanID != "" && (strings.HasSuffix(dsPath, "/"+cleanID) || strings.HasSuffix(dsPath, "/apps/"+cleanID) || strings.HasSuffix(dsPath, "/appdata/"+cleanID)) {
				if !snapshotted[dsPath] {
					if _, err := a.zfsPort.SnapshotDataset(ctx, poolName, dsPath, snapSuffix); err == nil {
						slog.Info("[docker] created pre-upgrade zfs snapshot for app dataset", "app", cleanID, "dataset", dsPath, "snapshot", snapSuffix)
						snapshotted[dsPath] = true
					}
				}
			}
			for _, v := range volumes {
				hostPath, _ := parsePortMapping(v)
				if hostPath == "" {
					parts := strings.Split(v, ":")
					hostPath = parts[0]
				}
				cleanHost := filepath.Clean(hostPath)
				if d.Mountpoint != "" && (cleanHost == d.Mountpoint || strings.HasPrefix(cleanHost, d.Mountpoint+"/")) {
					if !snapshotted[dsPath] {
						if _, err := a.zfsPort.SnapshotDataset(ctx, poolName, dsPath, snapSuffix); err == nil {
							slog.Info("[docker] created pre-upgrade zfs snapshot for mounted dataset", "app", cleanID, "dataset", dsPath, "snapshot", snapSuffix)
							snapshotted[dsPath] = true
						}
					}
				}
			}
		}
	}
}

// cancelDeployment aborts an in-flight background deployment (if any) and waits for it to exit.
func (a *Adapter) cancelDeployment(id string, maxWait time.Duration) {
	a.mu.Lock()
	cancel := a.deployCancel[id]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.waitForDeployment(id, maxWait)
}

// ensureExternalNetworks creates any networks a compose file declares as `external: true` that do not
// exist yet. Compose refuses to start ("declared as external, but could not be found") otherwise,
// which happens for imported TrueNAS apps (e.g. proxy_net) or after the Docker data root is reset.
func (a *Adapter) ensureExternalNetworks(ctx context.Context, composeYAML string) {
	var doc struct {
		Networks map[string]struct {
			External any    `yaml:"external"`
			Name     string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(composeYAML), &doc); err != nil {
		return
	}
	for key, n := range doc.Networks {
		name := key
		switch ext := n.External.(type) {
		case bool:
			if !ext {
				continue
			}
		case map[string]any: // legacy form: external: {name: foo}
			if v, ok := ext["name"].(string); ok && v != "" {
				name = v
			}
		default:
			continue
		}
		if n.Name != "" {
			name = n.Name
		}
		inspect := exec.CommandContext(ctx, "docker", "network", "inspect", name)
		inspect.Env = a.dockerCLIEnv()
		if inspect.Run() == nil {
			continue
		}
		create := exec.CommandContext(ctx, "docker", "network", "create", name)
		create.Env = a.dockerCLIEnv()
		if out, err := create.CombinedOutput(); err != nil {
			a.logError("network_create", name, err, string(out))
		} else {
			a.logger.Info("created missing external docker network", "subsystem", "docker", "network", name)
		}
	}
}

type dockerNetworkRaw struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Internal   bool   `json:"Internal"`
	Attachable bool   `json:"Attachable"`
	IPAM       struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
}

// ListNetworks retrieves all Docker networks on the host daemon (e.g. bridge, host, custom bridge, proxy_net).
func (a *Adapter) ListNetworks(ctx context.Context) ([]domain.DockerNetwork, error) {
	if a.hasSocket {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL()+"/networks", nil)
		if err == nil {
			resp, err := a.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var rawList []dockerNetworkRaw
					if err := json.NewDecoder(resp.Body).Decode(&rawList); err == nil {
						var result []domain.DockerNetwork
						for _, r := range rawList {
							var subnets []string
							for _, c := range r.IPAM.Config {
								if c.Subnet != "" {
									subnets = append(subnets, c.Subnet)
								}
							}
							result = append(result, domain.DockerNetwork{
								ID:          r.ID,
								Name:        r.Name,
								Driver:      r.Driver,
								Scope:       r.Scope,
								Internal:    r.Internal,
								Attachable:  r.Attachable,
								IPAMSubnets: subnets,
							})
						}
						sort.Slice(result, func(i, j int) bool {
							return result[i].Name < result[j].Name
						})
						return result, nil
					}
				}
			}
		}
	}

	// Fallback to docker CLI if socket is unavailable
	if a.hasCompose || a.hasSocket {
		cmd := exec.CommandContext(ctx, "docker", "network", "ls", "--format", "{{.ID}}\t{{.Name}}\t{{.Driver}}\t{{.Scope}}")
		cmd.Env = a.dockerCLIEnv()
		if out, err := cmd.Output(); err == nil {
			var result []domain.DockerNetwork
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, l := range lines {
				parts := strings.Split(l, "\t")
				if len(parts) >= 3 {
					scope := "local"
					if len(parts) >= 4 {
						scope = parts[3]
					}
					result = append(result, domain.DockerNetwork{
						ID:     parts[0],
						Name:   parts[1],
						Driver: parts[2],
						Scope:  scope,
					})
				}
			}
			if len(result) > 0 {
				sort.Slice(result, func(i, j int) bool {
					return result[i].Name < result[j].Name
				})
				return result, nil
			}
		}
	}

	// Standard default docker fallback
	return []domain.DockerNetwork{
		{ID: "bridge", Name: "bridge", Driver: "bridge", Scope: "local"},
		{ID: "host", Name: "host", Driver: "host", Scope: "local"},
		{ID: "none", Name: "none", Driver: "null", Scope: "local"},
	}, nil
}
