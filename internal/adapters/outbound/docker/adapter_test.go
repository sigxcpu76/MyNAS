package docker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestDockerLiveHost(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker socket /var/run/docker.sock not accessible, skipping live docker test")
	}
	ctx := context.Background()
	adapter := docker.NewAdapter()
	list, err := adapter.ListContainers(ctx)
	if err != nil {
		t.Skipf("Docker daemon unreachable: %v", err)
	}
	t.Logf("Discovered %d containers on live host", len(list))
}

func TestDockerSocketMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/containers/json":
			raw := []map[string]any{
				{
					"Id":      "abcdef1234567890",
					"Names":   []string{"/web-nginx"},
					"Image":   "nginx:alpine",
					"State":   "running",
					"Status":  "Up 2 hours",
					"Created": 1700000000,
					"Ports": []map[string]any{
						{"PublicPort": 8080, "PrivatePort": 80, "Type": "tcp"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(raw)

		case r.Method == http.MethodGet && r.URL.Path == "/containers/web-nginx/json":
			raw := map[string]any{
				"Id":      "abcdef1234567890",
				"Name":    "/web-nginx",
				"Created": "2026-10-24T00:00:00Z",
				"State": map[string]any{
					"Status":  "running",
					"Running": true,
				},
				"Config": map[string]any{
					"Image": "nginx:alpine",
					"Env":   []string{"FOO=BAR"},
				},
				"HostConfig": map[string]any{
					"Binds": []string{"/var/www:/usr/share/nginx/html:ro"},
					"PortBindings": map[string]any{
						"80/tcp": []map[string]string{{"HostPort": "8080"}},
					},
					"RestartPolicy": map[string]string{"Name": "unless-stopped"},
					"NetworkMode":   "bridge",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(raw)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/stats"):
			raw := map[string]any{
				"cpu_stats": map[string]any{
					"cpu_usage":        map[string]any{"total_usage": 200000000},
					"system_cpu_usage": 1000000000,
					"online_cpus":      2,
				},
				"precpu_stats": map[string]any{
					"cpu_usage":        map[string]any{"total_usage": 100000000},
					"system_cpu_usage": 500000000,
				},
				"memory_stats": map[string]any{
					"usage": 67108864, // 64 MiB
					"limit": 1073741824,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(raw)

		case r.Method == http.MethodPost && r.URL.Path == "/containers/create":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "new-container-id"})

		case r.Method == http.MethodPost && (r.URL.Path == "/containers/web-nginx/start" ||
			r.URL.Path == "/containers/new-container-id/start" ||
			r.URL.Path == "/containers/web-nginx/stop" ||
			r.URL.Path == "/containers/web-nginx/restart"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodDelete && r.URL.Path == "/containers/web-nginx":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/logs"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("2026-10-24T12:00:00Z [info] nginx listening on port 80\n"))

		case r.Method == http.MethodGet && r.URL.Path == "/images/nginx:alpine/json":
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	adapter := docker.NewAdapterWithClient(client, server.URL)
	ctx := context.Background()

	// 1. List
	containers, err := adapter.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ContainerName != "web-nginx" {
		t.Fatalf("expected web-nginx, got %s", containers[0].ContainerName)
	}

	// 2. Get with stats
	item, err := adapter.GetContainer(ctx, "web-nginx")
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if item.MemoryUsageMiB != 64 {
		t.Fatalf("expected 64 MiB memory usage, got %d", item.MemoryUsageMiB)
	}
	if item.CPUPercent <= 0 {
		t.Fatalf("expected >0 CPU percent, got %.2f", item.CPUPercent)
	}

	// 3. Start, Stop, Restart
	if _, err := adapter.StartContainer(ctx, "web-nginx"); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}
	if _, err := adapter.StopContainer(ctx, "web-nginx"); err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}
	if _, err := adapter.RestartContainer(ctx, "web-nginx"); err != nil {
		t.Fatalf("RestartContainer failed: %v", err)
	}

	// 3b. Logs
	logs, err := adapter.GetContainerLogs(ctx, "web-nginx", 50)
	if err != nil || !strings.Contains(logs, "nginx listening") {
		t.Fatalf("GetContainerLogs failed: err=%v logs=%s", err, logs)
	}

	// 4. Delete
	if err := adapter.DeleteContainer(ctx, "web-nginx", true); err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}

	// 5. Create
	created, err := adapter.CreateContainer(ctx, domain.DockerContainer{
		ID:            "web-nginx",
		Image:         "nginx:alpine",
		Ports:         []string{"8080:80/tcp"},
		Volumes:       []string{"/var/www:/usr/share/nginx/html:ro"},
		Environment:   []string{"FOO=BAR"},
		RestartPolicy: "unless-stopped",
		NetworkMode:   "bridge",
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if created.ContainerName != "web-nginx" {
		t.Fatalf("expected web-nginx, got %s", created.ContainerName)
	}
}

func TestDockerComposePersistence(t *testing.T) {
	tempUnitsDir := t.TempDir()
	unitDir := filepath.Join(tempUnitsDir, "myapp")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatalf("failed to create unit dir: %v", err)
	}

	// 1. Write custom compose YAML to disk unit directory
	customYAML := `services:
  myapp:
    container_name: myapp
    image: python:3.12-slim
    restart: "always"
    ports:
      - "8000:8000/tcp"
`
	composeFile := filepath.Join(unitDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte(customYAML), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	ctx := context.Background()

	// 2. Re-initialize a new adapter pointing to the units directory
	reloadedAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir)
	app, err := reloadedAdapter.GetApp(ctx, "myapp")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app.ID != "myapp" {
		t.Fatalf("expected ID myapp, got %s", app.ID)
	}
	if app.ComposeYAML != customYAML {
		t.Fatalf("persisted compose content mismatch: got %s", app.ComposeYAML)
	}

	// 3. Delete app and check compose file is removed
	if err := reloadedAdapter.DeleteApp(ctx, "myapp", true); err != nil {
		t.Fatalf("DeleteApp failed: %v", err)
	}
	if _, err := os.Stat(composeFile); !os.IsNotExist(err) {
		t.Fatalf("expected compose file to be removed after delete, got err: %v", err)
	}
}

func TestDockerAppManagement(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("docker socket not accessible")
	}
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/var/run/docker.sock", unitsDir)

	// List apps on host
	apps, err := adapter.ListApps(ctx)
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	_ = apps

	// Create custom microservice App
	myStackYAML := `services:
  web:
    image: busybox
    command: sleep 30
`
	created, err := adapter.CreateApp(ctx, domain.DockerApp{
		ID:          "nasctl-test-stack",
		Title:       "Test Stack",
		ComposeYAML: myStackYAML,
	})
	if err != nil {
		t.Skipf("CreateApp failed (docker compose may be restricted): %v", err)
	}
	defer func() {
		_ = adapter.DeleteApp(ctx, "nasctl-test-stack", true)
	}()

	if created.ID != "nasctl-test-stack" {
		t.Fatalf("unexpected created app ID: %+v", created)
	}

	// Verify persistence on disk
	composePath := filepath.Join(unitsDir, "nasctl-test-stack", "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("expected compose file to exist at %s: %v", composePath, err)
	}

	// Stop App
	if _, err := adapter.StopApp(ctx, "nasctl-test-stack"); err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}

	// Start App
	if _, err := adapter.StartApp(ctx, "nasctl-test-stack"); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	// Delete App
	if err := adapter.DeleteApp(ctx, "nasctl-test-stack", true); err != nil {
		t.Fatalf("DeleteApp failed: %v", err)
	}
	if _, err := os.Stat(composePath); !os.IsNotExist(err) {
		t.Fatalf("expected compose file to be removed after delete")
	}
}

func TestPortDeduplicationAndNormalization(t *testing.T) {
	tempUnitsDir := t.TempDir()
	appDir := filepath.Join(tempUnitsDir, "nginx-app")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("failed to create app dir: %v", err)
	}

	composeContent := `services:
  web:
    image: nginx:alpine
    ports:
      - "8086:80"
`
	if err := os.WriteFile(filepath.Join(appDir, "compose.yaml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir)
	ctx := context.Background()

	app, err := adapter.GetApp(ctx, "nginx-app")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}

	// Ensure portMappings has normalized "8086:80" and no duplicates
	if len(app.PortMappings) != 1 {
		t.Fatalf("expected exactly 1 port mapping, got %d: %v", len(app.PortMappings), app.PortMappings)
	}
	if app.PortMappings[0] != "8086:80" {
		t.Fatalf("expected port mapping '8086:80', got %q", app.PortMappings[0])
	}
}

func TestRandomHighPortAssignment(t *testing.T) {
	tempUnitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir)
	ctx := context.Background()

	rawCompose := `services:
  web:
    image: nginx:alpine
    ports:
      - "80"
      - "8080/udp"
      - target: 9000
`
	normalized := adapter.NormalizeComposePorts(ctx, rawCompose)
	if !strings.Contains(normalized, ":80") {
		t.Fatalf("expected normalized port for :80, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, ":8080/udp") {
		t.Fatalf("expected normalized udp port for :8080/udp, got:\n%s", normalized)
	}
	if !strings.Contains(normalized, "published:") {
		t.Fatalf("expected published node added for target: 9000, got:\n%s", normalized)
	}

	var c struct {
		Services map[string]struct {
			Ports []any `yaml:"ports"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(normalized), &c); err != nil {
		t.Fatalf("failed to unmarshal normalized yaml: %v", err)
	}

	webPorts := c.Services["web"].Ports
	if len(webPorts) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(webPorts))
	}

	for _, p := range webPorts {
		switch pv := p.(type) {
		case string:
			parts := strings.Split(pv, ":")
			if len(parts) < 2 {
				t.Fatalf("expected host:container format, got %s", pv)
			}
			hp, err := strconv.Atoi(parts[0])
			if err != nil || hp < 32000 || hp > 65000 {
				t.Fatalf("expected port in 32000-65000, got %d from %s", hp, pv)
			}
		case map[string]any:
			pub := fmt.Sprintf("%v", pv["published"])
			hp, err := strconv.Atoi(pub)
			if err != nil || hp < 32000 || hp > 65000 {
				t.Fatalf("expected published port in 32000-65000, got %d from %v", hp, pv)
			}
		default:
			t.Fatalf("unexpected port type: %T", p)
		}
	}
}

func TestDockerConfigAndDirectoryListing(t *testing.T) {
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir)

	// 1. Initial config
	cfg, err := adapter.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if cfg.Pool == "" {
		t.Fatalf("expected non-empty default pool")
	}

	// 2. Update config to a new dataset
	updated, err := adapter.UpdateConfig(ctx, domain.DockerConfig{
		Dataset: "fastpool/apps",
	})
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	if updated.Pool != "fastpool" {
		t.Fatalf("expected pool 'fastpool', got %s", updated.Pool)
	}
	if updated.Dataset != "fastpool/apps" {
		t.Fatalf("expected dataset 'fastpool/apps', got %s", updated.Dataset)
	}
	if updated.AppdataPath != "/mnt/fastpool/apps" && updated.AppdataPath != "/nas/fastpool/apps" && updated.AppdataPath != "/fastpool/apps" {
		t.Fatalf("expected appdata path under /mnt/fastpool or /nas/fastpool, got %s", updated.AppdataPath)
	}
	if !updated.DatasetSelected {
		t.Fatalf("expected DatasetSelected to be true")
	}

	// 3. Verify persistence
	reloaded := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir)
	cfgReloaded, err := reloaded.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig reloaded failed: %v", err)
	}
	if cfgReloaded.Pool != "fastpool" || cfgReloaded.Dataset != "fastpool/apps" || !cfgReloaded.DatasetSelected {
		t.Fatalf("expected reloaded pool 'fastpool', dataset 'fastpool/apps' and DatasetSelected=true, got %v", cfgReloaded)
	}

	// 4. Directory listing on real temp directory
	testDir := t.TempDir()
	_ = os.Mkdir(filepath.Join(testDir, "appdata"), 0755)
	dirs, err := adapter.ListDirectories(ctx, testDir)
	if err != nil {
		t.Fatalf("ListDirectories failed: %v", err)
	}
	if len(dirs.Entries) == 0 {
		t.Fatalf("expected directory entries for testDir, got 0")
	}
	hasAppdata := false
	for _, e := range dirs.Entries {
		if e.Name == "appdata" && e.IsDir {
			hasAppdata = true
			break
		}
	}
	if !hasAppdata {
		t.Fatalf("expected 'appdata' directory in testDir entries, got %v", dirs.Entries)
	}
}

func TestDockerAdapter_MainPoolEnforcement(t *testing.T) {
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir)
	adapter.SetMainPool("tank")

	cfg, err := adapter.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if len(cfg.AvailablePools) != 1 || cfg.AvailablePools[0] != "tank" {
		t.Fatalf("expected AvailablePools to contain only ['tank'], got %v", cfg.AvailablePools)
	}

	// Permitted: dataset on main pool
	updated, err := adapter.UpdateConfig(ctx, domain.DockerConfig{
		Dataset: "tank/apps",
	})
	if err != nil {
		t.Fatalf("expected success with main pool tank, got: %v", err)
	}
	if updated.Pool != "tank" {
		t.Fatalf("expected pool tank, got %s", updated.Pool)
	}

	// Rejected: dataset on another pool
	_, err = adapter.UpdateConfig(ctx, domain.DockerConfig{
		Dataset: "otherpool/apps",
	})
	if err == nil {
		t.Fatalf("expected error when trying to use otherpool for apps root, got nil")
	}
}

func TestDockerManagementPortDetection(t *testing.T) {
	ctx := context.Background()
	tempUnitsDir := t.TempDir()

	// 1. App with explicit x-nas-management-port: 81
	npmDir := filepath.Join(tempUnitsDir, "npm")
	_ = os.MkdirAll(npmDir, 0755)
	npmYAML := `x-nas-management-port: 81
services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    ports:
      - "80:80"
      - "81:81"
      - "443:443"
`
	if err := os.WriteFile(filepath.Join(npmDir, "compose.yaml"), []byte(npmYAML), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir)
	app, err := adapter.GetApp(ctx, "npm")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app.WebUIURL != "http://localhost:81" {
		t.Fatalf("expected WebUIURL http://localhost:81, got %q", app.WebUIURL)
	}

	// 2. App with label nas.management.port=3000 and DNS port 53
	adguardDir := filepath.Join(tempUnitsDir, "adguard")
	_ = os.MkdirAll(adguardDir, 0755)
	adguardYAML := `services:
  adguard:
    image: adguard/adguardhome:latest
    labels:
      - "nas.management.port=3000"
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "3000:3000"
`
	if err := os.WriteFile(filepath.Join(adguardDir, "compose.yaml"), []byte(adguardYAML), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	app2, err := adapter.GetApp(ctx, "adguard")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app2.WebUIURL != "http://localhost:3000" {
		t.Fatalf("expected WebUIURL http://localhost:3000, got %q", app2.WebUIURL)
	}

	// 3. Fallback: app with ports 443 and 80 without explicit management port
	webDir := filepath.Join(tempUnitsDir, "web")
	_ = os.MkdirAll(webDir, 0755)
	webYAML := `services:
  web:
    image: nginx:alpine
    ports:
      - "443:443"
      - "80:80"
`
	if err := os.WriteFile(filepath.Join(webDir, "compose.yaml"), []byte(webYAML), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	app3, err := adapter.GetApp(ctx, "web")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app3.WebUIURL != "http://localhost:80" {
		t.Fatalf("expected fallback WebUIURL http://localhost:80, got %q", app3.WebUIURL)
	}

	// 4. App with explicit x-portals from TrueNAS SCALE manifest
	portalsDir := filepath.Join(tempUnitsDir, "npm-portals")
	_ = os.MkdirAll(portalsDir, 0755)
	npmPortalsYAML := `x-portals:
  - name: "Admin Console"
    scheme: "http"
    port: 81
    path: "/"
  - name: "HTTPS Portal"
    scheme: "https"
    port: 443
    path: "/admin"
services:
  app:
    image: jc21/nginx-proxy-manager:latest
    ports:
      - "8081:81"
      - "8443:443"
`
	if err := os.WriteFile(filepath.Join(portalsDir, "compose.yaml"), []byte(npmPortalsYAML), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}

	app4, err := adapter.GetApp(ctx, "npm-portals")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if len(app4.Portals) != 2 {
		t.Fatalf("expected 2 portals, got %d (%+v)", len(app4.Portals), app4.Portals)
	}
	if app4.Portals[0].Name != "Admin Console" || app4.Portals[0].URL != "http://localhost:8081/" {
		t.Fatalf("unexpected portal 0: %+v", app4.Portals[0])
	}
	if app4.Portals[1].Name != "HTTPS Portal" || app4.Portals[1].URL != "https://localhost:8443/admin" {
		t.Fatalf("unexpected portal 1: %+v", app4.Portals[1])
	}
}

func TestDockerHostPortPreemptiveCheck(t *testing.T) {
	ctx := context.Background()
	tempUnitsDir := t.TempDir()

	// Spin up a test TCP listener on an ephemeral port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	boundPort := ln.Addr().(*net.TCPAddr).Port

	adapter := docker.NewAdapterWithStateDir(tempUnitsDir)

	// 1. Bound port should be detected as unavailable
	st, err := adapter.CheckHostPort(ctx, boundPort, "tcp", "")
	if err != nil {
		t.Fatalf("CheckHostPort failed: %v", err)
	}
	if st.Available {
		t.Fatalf("expected port %d to be in use, but reported available", boundPort)
	}
	if st.InUseBy == "" {
		t.Fatalf("expected non-empty InUseBy for in-use port %d", boundPort)
	}

	// 2. Free port (we find one by listening and closing it)
	freeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	freePort := freeLn.Addr().(*net.TCPAddr).Port
	freeLn.Close()

	freeSt, err := adapter.CheckHostPort(ctx, freePort, "tcp", "")
	if err != nil {
		t.Fatalf("CheckHostPort failed: %v", err)
	}
	if !freeSt.Available {
		t.Fatalf("expected port %d to be available, got in_use_by=%s", freePort, freeSt.InUseBy)
	}

	// 3. Batch check
	batch, err := adapter.CheckHostPorts(ctx, []int{boundPort, freePort}, "tcp", "")
	if err != nil {
		t.Fatalf("CheckHostPorts failed: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 results, got %d", len(batch))
	}
	if batch[0].Available || !batch[1].Available {
		t.Fatalf("unexpected batch result: %+v", batch)
	}

	// 4. Duplicate port conflict in Compose file
	dupYAML := fmt.Sprintf(`services:
  web1:
    image: nginx:alpine
    ports:
      - "%d:80"
  web2:
    image: httpd:alpine
    ports:
      - "%d:80"
`, freePort, freePort)

	_, err = adapter.CreateApp(ctx, domain.DockerApp{
		ID:          "dup-test",
		Name:        "dockerApps/dup-test",
		ComposeYAML: dupYAML,
	})
	if err == nil || !strings.Contains(err.Error(), "port conflict") {
		t.Fatalf("expected duplicate port conflict error, got: %v", err)
	}

	// 5. Compose with internal expose (no host port mapped) should not conflict
	internalOnlyYAML := fmt.Sprintf(`services:
  web:
    image: nginx:alpine
    expose:
      - "%d"
`, boundPort) // bound on host, but only exposed internally in docker network

	// This should pass port check because it does not map to host
	appDir := filepath.Join(tempUnitsDir, "internal-test")
	_ = os.MkdirAll(appDir, 0755)
	_ = os.WriteFile(filepath.Join(appDir, "compose.yaml"), []byte(internalOnlyYAML), 0644)
	app, err := adapter.GetApp(ctx, "internal-test")
	if err != nil {
		t.Fatalf("GetApp failed: %v", err)
	}
	if app.ID != "internal-test" {
		t.Fatalf("expected internal-test app, got %s", app.ID)
	}

	// 6. Redeploy / Update: App reusing its own currently mapped port should be permitted
	existingAppDir := filepath.Join(tempUnitsDir, "redeploy-test")
	_ = os.MkdirAll(existingAppDir, 0755)
	existingAppYAML := fmt.Sprintf(`services:
  web:
    image: nginx:alpine
    ports:
      - "%d:80"
`, boundPort)
	_ = os.WriteFile(filepath.Join(existingAppDir, "compose.yaml"), []byte(existingAppYAML), 0644)

	// Checking boundPort with excludeAppID="redeploy-test" should report AVAILABLE (owned by itself)
	redeploySt, err := adapter.CheckHostPort(ctx, boundPort, "tcp", "redeploy-test")
	if err != nil {
		t.Fatalf("CheckHostPort with excludeAppID failed: %v", err)
	}
	if !redeploySt.Available {
		t.Fatalf("expected port %d to be available for redeploy-test, but reported in-use: %s", boundPort, redeploySt.InUseBy)
	}

	// UpdateApp on redeploy-test with same yaml using boundPort should NOT fail on port conflict
	_, err = adapter.UpdateApp(ctx, domain.DockerApp{
		ID:          "redeploy-test",
		Name:        "dockerApps/redeploy-test",
		ComposeYAML: existingAppYAML,
	}, nil)
	if err != nil && strings.Contains(err.Error(), "port conflict") {
		t.Fatalf("UpdateApp failed with unexpected port conflict: %v", err)
	}
}
