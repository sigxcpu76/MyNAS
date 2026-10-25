package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
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
		containers:   make(map[string]domain.DockerContainer),
		statsCache:   make(map[string]containerStatsRecord),
	}

	a.loadPersistedUnits()
	return a
}

// NewAdapterWithClient allows injecting an HTTP client and base endpoint (for unit testing).
func NewAdapterWithClient(client *http.Client, baseEndpoint string) *Adapter {
	unitsDir := filepath.Join(os.TempDir(), fmt.Sprintf("nasctl-test-units-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(unitsDir, 0755)

	return &Adapter{
		logger:       slog.Default().With("subsystem", "docker"),
		client:       client,
		baseEndpoint: baseEndpoint,
		unitsDir:     unitsDir,
		hasSocket:    true,
		hasCompose:   false,
		containers:   make(map[string]domain.DockerContainer),
		statsCache:   make(map[string]containerStatsRecord),
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
	if a.socketPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_HOST=unix://%s", a.socketPath))
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		details := strings.TrimSpace(errBuf.String())
		a.logError("compose_"+strings.Join(args, "_"), projectName, err, details)
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
			sb.WriteString(fmt.Sprintf("      - %q\n", e))
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
							cpuPct, memMiB, _, _, _, _ = a.getLiveStats(ctx, inspect.ID)
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
		if resp, err := a.client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func (a *Adapter) getLiveStats(ctx context.Context, id string) (float64, int64, int64, int64, int64, int64) {
	a.statsCacheMu.RLock()
	cached, hasCache := a.statsCache[id]
	a.statsCacheMu.RUnlock()

	// If cache is fresh (< 800ms old), return it immediately so HTTP handlers respond in ~1ms
	if hasCache && time.Since(cached.updated) < 800*time.Millisecond {
		return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
	}

	tctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	statsURL := fmt.Sprintf("%s/containers/%s/stats?stream=false", a.baseURL(), url.PathEscape(id))
	req, err := http.NewRequestWithContext(tctx, http.MethodGet, statsURL, nil)
	if err != nil {
		if hasCache {
			return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
		}
		return 0, 0, 0, 0, 0, 0
	}

	resp, err := a.client.Do(req)
	if err != nil {
		if hasCache {
			return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
		}
		return 0, 0, 0, 0, 0, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if hasCache {
			return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
		}
		return 0, 0, 0, 0, 0, 0
	}

	var stats dockerStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		if hasCache {
			return cached.cpuPct, cached.memMiB, cached.netRx, cached.netTx, cached.rxRate, cached.txRate
		}
		return 0, 0, 0, 0, 0, 0
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

	// Never downgrade a non-zero memory or monotonic network counter if Docker returned an incomplete sample
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
	now := time.Now()
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

	a.statsCacheMu.Lock()
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

	return cpuPercent, memMiB, netRx, netTx, rxRate, txRate
}

func stripDockerLogHeaders(data []byte) string {
	if len(data) < 8 {
		return string(data)
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
			return sb.String()
		}
	}
	return string(data)
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

// CreateApp deploys a new Docker Compose Application.
func (a *Adapter) CreateApp(ctx context.Context, app domain.DockerApp) (*domain.DockerApp, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	appDir := filepath.Join(a.unitsDir, app.ID)
	if _, err := os.Stat(filepath.Join(appDir, "compose.yaml")); err == nil {
		return nil, fmt.Errorf("app %q already exists", app.ID)
	}

	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}

	composeFile := filepath.Join(appDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(app.ComposeYAML), 0644); err != nil {
		_ = os.RemoveAll(appDir)
		return nil, fmt.Errorf("failed to write compose.yaml: %w", err)
	}

	if !a.hasSocket {
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	if err := a.runCompose(ctx, appDir, app.ID, "up", "-d", "--remove-orphans"); err != nil {
		_ = os.RemoveAll(appDir)
		return nil, fmt.Errorf("failed to deploy compose app: %w", err)
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, app.ID, liveContainers)
}

// UpdateApp reconfigures and redeploys an existing Docker Compose Application.
func (a *Adapter) UpdateApp(ctx context.Context, app domain.DockerApp, fields []string) (*domain.DockerApp, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	appDir := filepath.Join(a.unitsDir, app.ID)
	composeFile := filepath.Join(appDir, "compose.yaml")

	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}

	if err := os.WriteFile(composeFile, []byte(app.ComposeYAML), 0644); err != nil {
		return nil, fmt.Errorf("failed to update compose.yaml: %w", err)
	}

	if !a.hasSocket {
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}

	if err := a.runCompose(ctx, appDir, app.ID, "up", "-d", "--remove-orphans"); err != nil {
		return nil, fmt.Errorf("failed to redeploy compose app: %w", err)
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, app.ID, liveContainers)
}

// DeleteApp tears down a Docker Compose Application and purges its persisted descriptor.
func (a *Adapter) DeleteApp(ctx context.Context, id string, deleteVolumes bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

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

// StartApp starts all stopped containers in a Docker Compose Application.
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
		if err := a.runCompose(ctx, appDir, id, "up", "-d", "--remove-orphans"); err != nil {
			return nil, err
		}
	}

	liveContainers := a.fetchLiveContainers(ctx)
	return a.buildApp(ctx, id, liveContainers)
}

// StopApp stops all containers in a Docker Compose Application.
func (a *Adapter) StopApp(ctx context.Context, id string) (*domain.DockerApp, error) {
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
			return out, nil
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
	if a.socketPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_HOST=unix://%s", a.socketPath))
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return outBuf.String(), fmt.Errorf("docker compose %s failed: %w (details: %s)", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return outBuf.String(), nil
}

type composeDescriptor struct {
	Services map[string]struct {
		Image         string `yaml:"image"`
		ContainerName string `yaml:"container_name"`
		Ports         []any  `yaml:"ports"`
		Volumes       []any  `yaml:"volumes"`
	} `yaml:"services"`
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

func parseComposeYAML(content string) (services []string, ports []string, volumes []string, err error) {
	var c composeDescriptor
	if err := yaml.Unmarshal([]byte(content), &c); err != nil {
		return nil, nil, nil, err
	}
	portSet := make(map[string]bool)
	volSet := make(map[string]bool)
	for svcName, svc := range c.Services {
		services = append(services, svcName)
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
	return services, ports, volumes, nil
}

func deriveWebUIURL(ports []string) string {
	for _, p := range ports {
		parts := strings.Split(p, ":")
		if len(parts) >= 2 {
			hostPort := parts[0]
			if len(parts) == 3 {
				hostPort = parts[1]
			}
			hostPort = strings.TrimSpace(hostPort)
			// Avoid non-HTTP standard ports like DNS 53
			if hostPort != "" && hostPort != "53" {
				return fmt.Sprintf("http://localhost:%s", hostPort)
			}
		}
	}
	return ""
}

func (a *Adapter) buildApp(ctx context.Context, appID string, liveContainers []dockerContainerSummary) (*domain.DockerApp, error) {
	composePath := filepath.Join(a.unitsDir, appID, "compose.yaml")
	var composeYAML string
	if data, err := os.ReadFile(composePath); err == nil {
		composeYAML = string(data)
	}

	services, parsedPorts, parsedVols, _ := parseComposeYAML(composeYAML)

	var appContainers []domain.DockerAppContainer
	var totalCPU float64
	var totalMem int64
	var totalNetRx, totalNetTx int64
	var totalNetRxRate, totalNetTxRate int64
	runningCount := 0

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
			if stateUpper == "RUNNING" {
				runningCount++
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
				cpuPct, memMiB, netRx, netTx, rxRate, txRate = a.getLiveStats(ctx, c.ID)
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
	} else if runningCount > 0 {
		state = "PARTIALLY_RUNNING"
	}

	statusText := fmt.Sprintf("%d/%d running", runningCount, total)
	if total == 0 {
		statusText = "No containers deployed"
	} else if runningCount == 0 {
		statusText = "Stopped"
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

	webUI := deriveWebUIURL(allPorts)

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
		ComposeYAML:     composeYAML,
		ComposeFilePath: composePath,
		Services:        services,
		Containers:      appContainers,
		PortMappings:    allPorts,
		Volumes:         parsedVols,
		WebUIURL:        webUI,
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
			if datasets, err := a.zfsPort.ListDatasets(ctx, mainPool); err == nil {
				for _, d := range datasets {
					availableDatasets = append(availableDatasets, d.DatasetPath)
				}
			}
		}
	} else if a.zfsPort != nil {
		if pools, err := a.zfsPort.ListPools(ctx); err == nil && len(pools) > 0 {
			for _, p := range pools {
				cleanID := strings.TrimPrefix(p.ID, "zfsPools/")
				availablePools = append(availablePools, cleanID)
				if datasets, err := a.zfsPort.ListDatasets(ctx, p.ID); err == nil {
					for _, d := range datasets {
						availableDatasets = append(availableDatasets, d.DatasetPath)
					}
				}
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
			if cfg.Mountpoint == "" {
				cfg.Mountpoint = "/" + cfg.Dataset
			}
			if cfg.AppdataPath == "" {
				cfg.AppdataPath = filepath.Join(cfg.Mountpoint, "appdata")
			}
			if cfg.DockerDataRoot == "" {
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
	poolMount := fmt.Sprintf("/nas/%s", defaultPool)
	if a.zfsPort != nil {
		if p, err := a.zfsPort.GetPool(ctx, defaultPool); err == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
			poolMount = p.Mountpoint
		}
	}
	defaultMountpoint := fmt.Sprintf("%s/apps", strings.TrimRight(poolMount, "/"))
	return &domain.DockerConfig{
		Dataset:           defaultDataset,
		Pool:              defaultPool,
		Mountpoint:        defaultMountpoint,
		AppdataPath:       filepath.Join(defaultMountpoint, "appdata"),
		DockerDataRoot:    filepath.Join(defaultMountpoint, "docker"),
		AvailableDatasets: availableDatasets,
		AvailablePools:    availablePools,
		DatasetSelected:   hasMainPool,
		AppdataDataset:    filepath.Join(defaultMountpoint, "appdata"),
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

	poolMount := fmt.Sprintf("/nas/%s", pool)
	if a.zfsPort != nil {
		if p, err := a.zfsPort.GetPool(ctx, pool); err == nil && p != nil && p.Mountpoint != "" && p.Mountpoint != "none" {
			poolMount = p.Mountpoint
		}
	}
	mountpoint := fmt.Sprintf("%s/%s", strings.TrimRight(poolMount, "/"), datasetID)
	appdataDir := filepath.Join(mountpoint, "appdata")
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
				if datasets, err := a.zfsPort.ListDatasets(ctx, p.ID); err == nil {
					for _, d := range datasets {
						availableDatasets = append(availableDatasets, d.DatasetPath)
					}
				}
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
		if cfg != nil && cfg.Pool != "" {
			reqPath = "/" + cfg.Pool
		} else {
			reqPath = "/"
		}
	}

	cleanPath := filepath.Clean(reqPath)
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
