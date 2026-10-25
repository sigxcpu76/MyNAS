package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

const (
	defaultCatalogURL = "https://raw.githubusercontent.com/truenas/apps/master/catalog.json"
	defaultSyncPeriod = 24 * time.Hour
)

type rawCatalogEntry struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	IconURL     string   `json:"icon_url"`
	Home        string   `json:"home"`
	LatestVer   string   `json:"latest_version"`
	LatestAppV  string   `json:"latest_app_version"`
	Tags        []string `json:"tags"`
	Screenshots []string `json:"screenshots"`
	Sources     []string `json:"sources"`
}

type Adapter struct {
	mu           sync.RWMutex
	cacheDir     string
	apps         map[string]domain.CatalogApp // keyed by "train/app_id"
	lastSync     time.Time
	httpClient   *http.Client
	catalogURL   string
	appsPool     string
	appdataPath  string
}

func NewAdapter(cacheDir string) *Adapter {
	if cacheDir == "" {
		cacheDir = "/tmp/nas-catalog"
	}
	_ = os.MkdirAll(cacheDir, 0755)

	a := &Adapter{
		cacheDir:    cacheDir,
		apps:        make(map[string]domain.CatalogApp),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		catalogURL:  defaultCatalogURL,
		appsPool:    "tank",
		appdataPath: "/tank/appdata",
	}

	// 1. Seed high-quality offline apps
	a.seedDefaultApps()

	// 2. Load cached catalog from disk if present
	a.loadCacheFromDisk()

	return a
}

func (a *Adapter) ListApps(ctx context.Context, train, category, query string) ([]domain.CatalogApp, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	q := strings.TrimSpace(strings.ToLower(query))
	cat := strings.TrimSpace(strings.ToLower(category))
	tr := strings.TrimSpace(strings.ToLower(train))

	var results []domain.CatalogApp
	for _, app := range a.apps {
		if tr != "" && tr != "all" && !strings.EqualFold(app.Train, tr) {
			continue
		}
		if cat != "" && cat != "all" {
			hasCat := false
			for _, c := range app.Categories {
				if strings.EqualFold(c, cat) {
					hasCat = true
					break
				}
			}
			if !hasCat && !strings.EqualFold(app.Category, cat) {
				continue
			}
		}
		if q != "" {
			matches := strings.Contains(strings.ToLower(app.Title), q) ||
				strings.Contains(strings.ToLower(app.AppID), q) ||
				strings.Contains(strings.ToLower(app.Description), q)
			if !matches {
				for _, t := range app.Tags {
					if strings.Contains(strings.ToLower(t), q) {
						matches = true
						break
					}
				}
			}
			if !matches {
				continue
			}
		}
		results = append(results, app)
	}

	sort.Slice(results, func(i, j int) bool {
		// Stable train first, then alphabetical by Title
		if results[i].Train != results[j].Train {
			return results[i].Train == "stable"
		}
		return results[i].Title < results[j].Title
	})

	return results, nil
}

func (a *Adapter) GetApp(ctx context.Context, train, appID string) (*domain.CatalogApp, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Try exact train/app_id
	if train != "" {
		key := fmt.Sprintf("%s/%s", train, appID)
		if app, ok := a.apps[key]; ok {
			res := a.ensureComposeYAML(app)
			return &res, nil
		}
	}

	// Try stable first, then any
	if app, ok := a.apps["stable/"+appID]; ok {
		res := a.ensureComposeYAML(app)
		return &res, nil
	}
	if app, ok := a.apps["community/"+appID]; ok {
		res := a.ensureComposeYAML(app)
		return &res, nil
	}

	// Scan by AppID
	for _, app := range a.apps {
		if app.AppID == appID {
			res := a.ensureComposeYAML(app)
			return &res, nil
		}
	}

	return nil, fmt.Errorf("app %q not found in catalog", appID)
}

func (a *Adapter) SyncCatalog(ctx context.Context, force bool) (int, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.catalogURL, nil)
	if err != nil {
		return 0, a.lastSync, fmt.Errorf("failed to create sync request: %w", err)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, a.lastSync, fmt.Errorf("failed to fetch TrueNAS catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, a.lastSync, fmt.Errorf("truenas catalog returned HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, a.lastSync, fmt.Errorf("failed to read catalog body: %w", err)
	}

	var rawCatalog map[string]map[string]rawCatalogEntry
	if err := json.Unmarshal(body, &rawCatalog); err != nil {
		return 0, a.lastSync, fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	for trainName, trainApps := range rawCatalog {
		if trainName != "stable" && trainName != "community" {
			continue
		}
		for appKey, item := range trainApps {
			cat := ""
			if len(item.Categories) > 0 {
				cat = item.Categories[0]
			}
			appID := item.Name
			if appID == "" {
				appID = appKey
			}
			title := item.Title
			if title == "" {
				title = appID
			}

			key := fmt.Sprintf("%s/%s", trainName, appID)
			existing, exists := a.apps[key]

			compose := ""
			defaultPort := 0
			if exists {
				compose = existing.ComposeYAML
				defaultPort = existing.DefaultPort
			}

			a.apps[key] = domain.CatalogApp{
				AppID:       appID,
				Title:       title,
				Description: cleanDescription(item.Description),
				Category:    cat,
				Categories:  item.Categories,
				IconURL:     item.IconURL,
				Home:        item.Home,
				Train:       trainName,
				Version:     item.LatestVer,
				AppVersion:  item.LatestAppV,
				Tags:        item.Tags,
				Screenshots: item.Screenshots,
				ComposeYAML: compose,
				DefaultPort: defaultPort,
			}
			count++
		}
	}

	a.lastSync = time.Now().UTC()
	_ = a.saveCacheToDisk(body)

	return count, a.lastSync, nil
}

func (a *Adapter) LastSyncTime() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastSync
}

func (a *Adapter) SetCatalogURLForTest(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.catalogURL = url
}

func (a *Adapter) SetAppsPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pool = strings.TrimSpace(strings.TrimPrefix(pool, "zfsPools/"))
	if pool != "" {
		a.appsPool = pool
		a.appdataPath = fmt.Sprintf("/nas/%s/appdata", pool)
	}
}

func (a *Adapter) SetAppdataPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanPath := strings.TrimSpace(path)
	if cleanPath != "" {
		a.appdataPath = cleanPath
	}
}

func (a *Adapter) getAppsPool() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.appsPool != "" {
		return a.appsPool
	}
	return "tank"
}

func (a *Adapter) getAppdataPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.appdataPath != "" {
		return a.appdataPath
	}
	if a.appsPool != "" {
		return fmt.Sprintf("/nas/%s/appdata", a.appsPool)
	}
	return "/nas/tank/appdata"
}

func (a *Adapter) ensureComposeYAML(app domain.CatalogApp) domain.CatalogApp {
	if app.ComposeYAML == "" {
		app.ComposeYAML, app.DefaultPort = generateResolvedCompose(app)
	}
	appdataPath := a.getAppdataPath()
	if appdataPath != "/nas/tank/appdata" && appdataPath != "/tank/appdata" {
		app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/tank/appdata", appdataPath)
		app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/nas/tank/appdata", appdataPath)
	}
	pool := a.getAppsPool()
	if pool != "tank" {
		app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/tank/media", fmt.Sprintf("/nas/%s/media", pool))
		app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/nas/tank/media", fmt.Sprintf("/nas/%s/media", pool))
	}
	return app
}

func (a *Adapter) saveCacheToDisk(data []byte) error {
	cacheFile := filepath.Join(a.cacheDir, "catalog.json")
	return os.WriteFile(cacheFile, data, 0644)
}

func (a *Adapter) loadCacheFromDisk() {
	cacheFile := filepath.Join(a.cacheDir, "catalog.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return
	}

	var rawCatalog map[string]map[string]rawCatalogEntry
	if err := json.Unmarshal(data, &rawCatalog); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for trainName, trainApps := range rawCatalog {
		if trainName != "stable" && trainName != "community" {
			continue
		}
		for appKey, item := range trainApps {
			cat := ""
			if len(item.Categories) > 0 {
				cat = item.Categories[0]
			}
			appID := item.Name
			if appID == "" {
				appID = appKey
			}
			title := item.Title
			if title == "" {
				title = appID
			}

			key := fmt.Sprintf("%s/%s", trainName, appID)
			existing, exists := a.apps[key]
			compose := ""
			defaultPort := 0
			if exists {
				compose = existing.ComposeYAML
				defaultPort = existing.DefaultPort
			}

			a.apps[key] = domain.CatalogApp{
				AppID:       appID,
				Title:       title,
				Description: cleanDescription(item.Description),
				Category:    cat,
				Categories:  item.Categories,
				IconURL:     item.IconURL,
				Home:        item.Home,
				Train:       trainName,
				Version:     item.LatestVer,
				AppVersion:  item.LatestAppV,
				Tags:        item.Tags,
				Screenshots: item.Screenshots,
				ComposeYAML: compose,
				DefaultPort: defaultPort,
			}
		}
	}

	if fi, err := os.Stat(cacheFile); err == nil {
		a.lastSync = fi.ModTime().UTC()
	}
}

func cleanDescription(s string) string {
	s = strings.TrimSpace(s)
	// Remove basic HTML tags if present
	s = strings.ReplaceAll(s, "<h1>", "")
	s = strings.ReplaceAll(s, "</h1>", " ")
	s = strings.ReplaceAll(s, "<p>", "")
	s = strings.ReplaceAll(s, "</p>", " ")
	return strings.TrimSpace(s)
}

// generateResolvedCompose generates a clean, production-ready Docker Compose YAML
// mapped directly to the NAS appliance's ZFS storage paths.
func generateResolvedCompose(app domain.CatalogApp) (string, int) {
	appID := app.AppID
	port := 8080
	if app.DefaultPort > 0 {
		port = app.DefaultPort
	}

	// Curated ports and images for well-known TrueNAS apps
	image := fmt.Sprintf("%s:latest", appID)
	switch appID {
	case "syncthing":
		image = "ghcr.io/syncthing/syncthing:latest"
		port = 8384
		return fmt.Sprintf(`services:
  syncthing:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:8384"
      - "22000:22000/tcp"
      - "22000:22000/udp"
      - "21027:21027/udp"
    environment:
      - PUID=1000
      - PGID=1000
    volumes:
      - /tank/appdata/%s/config:/var/syncthing
      - /tank/media:/var/syncthing/Sync
`, image, appID, port, appID), port

	case "wg-easy":
		image = "ghcr.io/wg-easy/wg-easy:latest"
		port = 51821
		return fmt.Sprintf(`services:
  wg-easy:
    image: %s
    container_name: %s
    restart: unless-stopped
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    ports:
      - "51820:51820/udp"
      - "%d:51821/tcp"
    environment:
      - WG_HOST=your-server-ip
      - PASSWORD=admin
    volumes:
      - /tank/appdata/%s:/etc/wireguard
`, image, appID, port, appID), port

	case "ollama":
		image = "ollama/ollama:latest"
		port = 11434
		return fmt.Sprintf(`services:
  ollama:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:11434"
    volumes:
      - /tank/appdata/%s:/root/.ollama
`, image, appID, port, appID), port

	case "uptime-kuma":
		image = "louislam/uptime-kuma:latest"
		port = 3001
		return fmt.Sprintf(`services:
  uptime-kuma:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:3001"
    volumes:
      - /tank/appdata/%s:/app/data
`, image, appID, port, appID), port

	case "vaultwarden":
		image = "vaultwarden/server:latest"
		port = 8088
		return fmt.Sprintf(`services:
  vaultwarden:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:80"
    environment:
      - WEBSOCKET_ENABLED=true
    volumes:
      - /tank/appdata/%s:/data
`, image, appID, port, appID), port

	case "pihole":
		image = "pihole/pihole:latest"
		port = 8085
		return fmt.Sprintf(`services:
  pihole:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "%d:80/tcp"
    environment:
      - TZ=Etc/UTC
      - WEBPASSWORD=admin
    volumes:
      - /tank/appdata/%s/etc-pihole:/etc/pihole
      - /tank/appdata/%s/etc-dnsmasq.d:/etc/dnsmasq.d
`, image, appID, port, appID, appID), port

	case "plex":
		image = "lscr.io/linuxserver/plex:latest"
		port = 32400
		return fmt.Sprintf(`services:
  plex:
    image: %s
    container_name: %s
    restart: unless-stopped
    network_mode: host
    environment:
      - PUID=1000
      - PGID=1000
      - VERSION=docker
    volumes:
      - /tank/appdata/%s:/config
      - /tank/media:/media
`, image, appID, appID), port

	case "home-assistant":
		image = "ghcr.io/home-assistant/home-assistant:stable"
		port = 8123
		return fmt.Sprintf(`services:
  home-assistant:
    image: %s
    container_name: %s
    restart: unless-stopped
    network_mode: host
    volumes:
      - /tank/appdata/%s:/config
      - /etc/localtime:/etc/localtime:ro
`, image, appID, appID), port

	case "adguard-home":
		image = "adguard/adguardhome:latest"
		port = 3000
		return fmt.Sprintf(`services:
  adguard-home:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "%d:3000/tcp"
    volumes:
      - /tank/appdata/%s/work:/opt/adguardhome/work
      - /tank/appdata/%s/conf:/opt/adguardhome/conf
`, image, appID, port, appID, appID), port

	case "actual-budget":
		image = "ghcr.io/actualbudget/actual-server:latest"
		port = 5006
		return fmt.Sprintf(`services:
  actual-budget:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:5006"
    volumes:
      - /tank/appdata/%s:/data
`, image, appID, port, appID), port
	}

	// Default template generator for any other TrueNAS app
	return fmt.Sprintf(`services:
  %s:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:80"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /tank/appdata/%s:/data
`, appID, image, appID, port, appID), port
}

func (a *Adapter) seedDefaultApps() {
	defaults := []domain.CatalogApp{
		{
			AppID:       "nextcloud",
			Title:       "Nextcloud Hub",
			Description: "Enterprise-grade private cloud storage, file synchronization, contacts, calendar and office collaboration suite.",
			Category:    "productivity",
			Categories:  []string{"productivity", "storage"},
			IconURL:     "https://media.sys.truenas.net/apps/nextcloud/icons/icon.png",
			Train:       "stable",
			Version:     "2.0.1",
			DefaultPort: 8080,
			ComposeYAML: `services:
  app:
    image: nextcloud:apache
    container_name: nextcloud-app
    restart: always
    ports:
      - "8080:80"
    environment:
      - POSTGRES_HOST=db
      - POSTGRES_DB=nextcloud
      - POSTGRES_USER=nextcloud
      - POSTGRES_PASSWORD=nextcloud
      - REDIS_HOST=redis
    volumes:
      - /tank/appdata/nextcloud:/var/www/html
    depends_on:
      - db
      - redis
  db:
    image: postgres:16-alpine
    container_name: nextcloud-db
    restart: always
    environment:
      - POSTGRES_DB=nextcloud
      - POSTGRES_USER=nextcloud
      - POSTGRES_PASSWORD=nextcloud
    volumes:
      - /tank/appdata/nextcloud-db:/var/lib/postgresql/data
  redis:
    image: redis:alpine
    container_name: nextcloud-redis
    restart: always
`,
		},
		{
			AppID:       "jellyfin",
			Title:       "Jellyfin Media Server",
			Description: "The Free Software Media System that puts you in control of managing and streaming your media files.",
			Category:    "media",
			Categories:  []string{"media", "video"},
			IconURL:     "https://media.sys.truenas.net/apps/jellyfin/icons/icon.png",
			Train:       "community",
			Version:     "10.9.8",
			DefaultPort: 8096,
			ComposeYAML: `services:
  jellyfin:
    image: jellyfin/jellyfin:latest
    container_name: jellyfin
    restart: unless-stopped
    ports:
      - "8096:8096"
    volumes:
      - /tank/appdata/jellyfin/config:/config
      - /tank/appdata/jellyfin/cache:/cache
      - /tank/media:/media:ro
`,
		},
		{
			AppID:       "syncthing",
			Title:       "Syncthing",
			Description: "Continuous decentralized file synchronization program with end-to-end TLS encryption.",
			Category:    "storage",
			Categories:  []string{"storage", "sync"},
			IconURL:     "https://media.sys.truenas.net/apps/syncthing/icons/icon.svg",
			Train:       "stable",
			Version:     "1.3.21",
			DefaultPort: 8384,
		},
		{
			AppID:       "wg-easy",
			Title:       "WG Easy (WireGuard VPN)",
			Description: "The easiest, web-managed way to run WireGuard VPN server with QR code client provisioning.",
			Category:    "networking",
			Categories:  []string{"networking", "vpn"},
			IconURL:     "https://media.sys.truenas.net/apps/wg-easy/icons/icon.png",
			Train:       "stable",
			Version:     "2.1.12",
			DefaultPort: 51821,
		},
		{
			AppID:       "ollama",
			Title:       "Ollama",
			Description: "Run large language models (Llama 3.2, Mistral, Gemma 2, DeepSeek) locally on CPU or GPU.",
			Category:    "ai",
			Categories:  []string{"ai", "llm"},
			IconURL:     "https://media.sys.truenas.net/apps/ollama/icons/icon.png",
			Train:       "community",
			Version:     "1.2.85",
			DefaultPort: 11434,
		},
		{
			AppID:       "vaultwarden",
			Title:       "Vaultwarden",
			Description: "Lightweight, self-hosted Bitwarden-compatible password manager written in Rust.",
			Category:    "security",
			Categories:  []string{"security", "productivity"},
			IconURL:     "https://media.sys.truenas.net/apps/vaultwarden/icons/icon.png",
			Train:       "community",
			Version:     "1.32.0",
			DefaultPort: 8088,
		},
		{
			AppID:       "uptime-kuma",
			Title:       "Uptime Kuma",
			Description: "Self-hosted monitoring tool like Uptime Robot with beautiful reactive status pages and alert channels.",
			Category:    "monitoring",
			Categories:  []string{"monitoring", "networking"},
			IconURL:     "https://media.sys.truenas.net/apps/uptime-kuma/icons/icon.png",
			Train:       "community",
			Version:     "1.23.16",
			DefaultPort: 3001,
		},
		{
			AppID:       "pihole",
			Title:       "Pi-hole",
			Description: "Network-wide ad blocking via DNS sinkhole with DNSSEC, DHCP and comprehensive query logging.",
			Category:    "networking",
			Categories:  []string{"networking", "security"},
			IconURL:     "https://media.sys.truenas.net/apps/pihole/icons/icon.png",
			Train:       "stable",
			Version:     "2024.07.0",
			DefaultPort: 8085,
		},
		{
			AppID:       "plex",
			Title:       "Plex Media Server",
			Description: "Stream movies, music, TV shows, and photos to any device with hardware transcoding.",
			Category:    "media",
			Categories:  []string{"media", "video"},
			IconURL:     "https://media.sys.truenas.net/apps/plex/icons/icon.png",
			Train:       "stable",
			Version:     "1.41.0",
			DefaultPort: 32400,
		},
		{
			AppID:       "home-assistant",
			Title:       "Home Assistant",
			Description: "Open source home automation that puts local control and privacy first.",
			Category:    "productivity",
			Categories:  []string{"productivity", "iot"},
			IconURL:     "https://media.sys.truenas.net/apps/home-assistant/icons/icon.png",
			Train:       "stable",
			Version:     "2024.10.2",
			DefaultPort: 8123,
		},
	}

	for _, app := range defaults {
		key := fmt.Sprintf("%s/%s", app.Train, app.AppID)
		a.apps[key] = app
	}
}
