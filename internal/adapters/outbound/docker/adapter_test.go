package docker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/domain"
)

func TestDockerHybridMode(t *testing.T) {
	// Adapter in hybrid mode with a nonexistent socket uses in-memory simulation
	adapter := docker.NewAdapterWithSocketPath("/nonexistent/docker.sock", true)
	ctx := context.Background()

	// 1. ListContainers returns default seeded container
	list, err := adapter.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected seeded containers in hybrid mode, got 0")
	}

	// 2. GetContainer
	item, err := adapter.GetContainer(ctx, list[0].ID)
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if item.ID != list[0].ID {
		t.Fatalf("expected ID %s, got %s", list[0].ID, item.ID)
	}

	// 3. CreateContainer
	created, err := adapter.CreateContainer(ctx, domain.DockerContainer{
		ID:            "my-test-container",
		Image:         "redis:alpine",
		Ports:         []string{"6379:6379/tcp"},
		Volumes:       []string{"/tmp/data:/data"},
		Environment:   []string{"ALLOW_EMPTY_PASSWORD=yes"},
		RestartPolicy: "unless-stopped",
		NetworkMode:   "bridge",
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if created.State != "RUNNING" {
		t.Fatalf("expected RUNNING, got %s", created.State)
	}

	// 4. Duplicate CreateContainer should fail
	_, err = adapter.CreateContainer(ctx, domain.DockerContainer{
		ID:    "my-test-container",
		Image: "redis:alpine",
	})
	if err == nil {
		t.Fatalf("expected error on duplicate create, got nil")
	}

	// 5. StopContainer
	stopped, err := adapter.StopContainer(ctx, "my-test-container")
	if err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}
	if stopped.State != "EXITED" {
		t.Fatalf("expected EXITED, got %s", stopped.State)
	}

	// 6. RestartContainer
	restarted, err := adapter.RestartContainer(ctx, "my-test-container")
	if err != nil {
		t.Fatalf("RestartContainer failed: %v", err)
	}
	if restarted.State != "RUNNING" {
		t.Fatalf("expected RUNNING, got %s", restarted.State)
	}

	// 7. UpdateContainer
	updated, err := adapter.UpdateContainer(ctx, domain.DockerContainer{
		ID:            "my-test-container",
		Image:         "redis:7.2-alpine",
		RestartPolicy: "always",
	}, []string{"image", "restart_policy"})
	if err != nil {
		t.Fatalf("UpdateContainer failed: %v", err)
	}
	if updated.Image != "redis:7.2-alpine" || updated.RestartPolicy != "always" {
		t.Fatalf("UpdateContainer did not apply changes: image=%s policy=%s", updated.Image, updated.RestartPolicy)
	}

	// 8. GetContainerLogs
	logs, err := adapter.GetContainerLogs(ctx, "my-test-container", 50)
	if err != nil || logs == "" {
		t.Fatalf("GetContainerLogs failed: %v", err)
	}

	// 9. DeleteContainer
	err = adapter.DeleteContainer(ctx, "my-test-container", true)
	if err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}

	// 10. Get deleted container should return ErrNotFound
	_, err = adapter.GetContainer(ctx, "my-test-container")
	if err == nil {
		t.Fatalf("expected not found error, got nil")
	}
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
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir, true)
	ctx := context.Background()

	// 1. Create a container with custom compose YAML
	customYAML := `services:
  myapp:
    container_name: myapp
    image: python:3.12-slim
    restart: "always"
    ports:
      - "8000:8000/tcp"
`
	created, err := adapter.CreateContainer(ctx, domain.DockerContainer{
		ID:          "myapp",
		ComposeYAML: customYAML,
	})
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if created.ID != "myapp" {
		t.Fatalf("expected ID myapp, got %s", created.ID)
	}

	// 2. Verify compose.yaml was written to units directory
	composeFile := filepath.Join(tempUnitsDir, "myapp", "compose.yaml")
	data, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatalf("expected compose.yaml to exist on disk: %v", err)
	}
	if string(data) != customYAML {
		t.Fatalf("persisted compose content mismatch: got %s", string(data))
	}

	// 3. Re-initialize a new adapter pointing to the same units directory
	reloadedAdapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir, true)
	containers, err := reloadedAdapter.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ID == "myapp" {
			found = true
			if c.ComposeYAML != customYAML {
				t.Fatalf("reloaded container compose yaml mismatch: %s", c.ComposeYAML)
			}
		}
	}
	if !found {
		t.Fatalf("expected persisted unit 'myapp' to be loaded into new adapter")
	}

	// 4. Delete container and check compose file is removed
	if err := reloadedAdapter.DeleteContainer(ctx, "myapp", true); err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}
	if _, err := os.Stat(composeFile); !os.IsNotExist(err) {
		t.Fatalf("expected compose file to be removed after delete, got err: %v", err)
	}
}

func TestDockerAppManagement(t *testing.T) {
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir, true)

	// 1. List seeded apps in hybrid mode
	apps, err := adapter.ListApps(ctx)
	if err != nil {
		t.Fatalf("ListApps failed: %v", err)
	}
	if len(apps) == 0 {
		t.Fatalf("expected seeded apps in hybrid mode, got 0")
	}

	var ncApp *domain.DockerApp
	for i := range apps {
		if apps[i].ID == "nextcloud" {
			ncApp = &apps[i]
			break
		}
	}
	if ncApp == nil {
		t.Fatalf("expected seeded 'nextcloud' app in ListApps")
	}
	if len(ncApp.Containers) != 3 {
		t.Fatalf("expected nextcloud to have 3 containers (app, db, redis), got %d", len(ncApp.Containers))
	}
	if ncApp.WebUIURL != "http://localhost:8080" {
		t.Fatalf("expected WebUIURL 'http://localhost:8080', got %q", ncApp.WebUIURL)
	}

	// 2. Create custom multi-service App
	myStackYAML := `services:
  web:
    image: nginx:alpine
    ports:
      - "9000:80"
  api:
    image: golang:alpine
`
	created, err := adapter.CreateApp(ctx, domain.DockerApp{
		ID:          "mystack",
		Title:       "My Microservice Stack",
		ComposeYAML: myStackYAML,
	})
	if err != nil {
		t.Fatalf("CreateApp failed: %v", err)
	}
	if created.ID != "mystack" || created.State != "RUNNING" {
		t.Fatalf("unexpected created app state: %+v", created)
	}
	if len(created.Containers) != 2 {
		t.Fatalf("expected 2 containers for mystack, got %d", len(created.Containers))
	}

	// Verify persistence on disk
	composePath := filepath.Join(unitsDir, "mystack", "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("expected compose file to exist at %s: %v", composePath, err)
	}

	// 3. Stop App
	stopped, err := adapter.StopApp(ctx, "mystack")
	if err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}
	if stopped.State != "STOPPED" {
		t.Fatalf("expected app state STOPPED, got %s", stopped.State)
	}

	// 4. Start App
	started, err := adapter.StartApp(ctx, "mystack")
	if err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}
	if started.State != "RUNNING" {
		t.Fatalf("expected app state RUNNING, got %s", started.State)
	}

	// 5. App Logs
	logs, err := adapter.GetAppLogs(ctx, "mystack", 50, "")
	if err != nil {
		t.Fatalf("GetAppLogs failed: %v", err)
	}
	if !strings.Contains(logs, "web") {
		t.Fatalf("expected logs to contain service name, got: %s", logs)
	}

	// 6. Delete App
	if err := adapter.DeleteApp(ctx, "mystack", true); err != nil {
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

	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", tempUnitsDir, true)
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

func TestDockerConfigAndDirectoryListing(t *testing.T) {
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir, true)

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
	if updated.AppdataPath != "/fastpool/apps/appdata" {
		t.Fatalf("expected appdata path '/fastpool/apps/appdata', got %s", updated.AppdataPath)
	}
	if !updated.DatasetSelected {
		t.Fatalf("expected DatasetSelected to be true")
	}

	// 3. Verify persistence
	reloaded := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir, true)
	cfgReloaded, err := reloaded.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig reloaded failed: %v", err)
	}
	if cfgReloaded.Pool != "fastpool" || cfgReloaded.Dataset != "fastpool/apps" || !cfgReloaded.DatasetSelected {
		t.Fatalf("expected reloaded pool 'fastpool', dataset 'fastpool/apps' and DatasetSelected=true, got %v", cfgReloaded)
	}

	// 4. Directory listing
	dirs, err := adapter.ListDirectories(ctx, "/tank")
	if err != nil {
		t.Fatalf("ListDirectories failed: %v", err)
	}
	if len(dirs.Entries) == 0 {
		t.Fatalf("expected directory entries for /tank, got 0")
	}
	hasAppdata := false
	for _, e := range dirs.Entries {
		if e.Name == "appdata" && e.IsDir {
			hasAppdata = true
			break
		}
	}
	if !hasAppdata {
		t.Fatalf("expected 'appdata' directory in /tank entries, got %v", dirs.Entries)
	}
}

func TestDockerAdapter_MainPoolEnforcement(t *testing.T) {
	ctx := context.Background()
	unitsDir := t.TempDir()
	adapter := docker.NewAdapterFull("/nonexistent/docker.sock", unitsDir, true)
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
