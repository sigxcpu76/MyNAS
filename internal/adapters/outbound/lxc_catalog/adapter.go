package lxc_catalog

import (
	"context"
	"encoding/json"
	"fmt"
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
	communityScriptsAPI = "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/json/proxmox.json"
	turnkeyManifestURL  = "https://releases.turnkeylinux.org/turnkey.json"
	linuxContainersURL  = "https://images.linuxcontainers.org/meta/1.0/index-system"
)

type Adapter struct {
	mu         sync.RWMutex
	cacheDir   string
	items      map[string]domain.LxcCatalogItem
	lastSync   time.Time
	httpClient *http.Client
}

func NewAdapter(cacheDir string) *Adapter {
	if cacheDir == "" {
		cacheDir = "/tmp/nas-lxc-catalog"
	}
	_ = os.MkdirAll(cacheDir, 0755)

	a := &Adapter{
		cacheDir:   cacheDir,
		items:      make(map[string]domain.LxcCatalogItem),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Seed pre-defined catalog items
	a.seedDefaultItems()

	// Load cached catalog from disk if present
	a.loadCacheFromDisk()

	return a
}

func (a *Adapter) LastSyncTime() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastSync
}

func (a *Adapter) GetItem(ctx context.Context, id string) (*domain.LxcCatalogItem, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	item, exists := a.items[id]
	if !exists {
		return nil, fmt.Errorf("%w: lxc catalog item %q", domain.ErrNotFound, id)
	}
	return &item, nil
}

func (a *Adapter) ListItems(ctx context.Context, source, category, query string) ([]domain.LxcCatalogItem, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(query))
	srcFilter := strings.ToLower(strings.TrimSpace(source))
	catFilter := strings.ToLower(strings.TrimSpace(category))

	var results []domain.LxcCatalogItem
	for _, item := range a.items {
		// Filter by source
		if srcFilter != "" && srcFilter != "all" && strings.ToLower(item.Source) != srcFilter {
			continue
		}

		// Filter by category
		if catFilter != "" && catFilter != "all" {
			matchCat := strings.EqualFold(item.Category, catFilter)
			if !matchCat {
				for _, c := range item.Categories {
					if strings.EqualFold(c, catFilter) {
						matchCat = true
						break
					}
				}
			}
			if !matchCat {
				continue
			}
		}

		// Filter by search query
		if q != "" {
			matchTitle := strings.Contains(strings.ToLower(item.Title), q)
			matchDesc := strings.Contains(strings.ToLower(item.Description), q)
			matchID := strings.Contains(strings.ToLower(item.ID), q)
			matchBase := strings.Contains(strings.ToLower(item.BaseDistribution), q)
			if !matchTitle && !matchDesc && !matchID && !matchBase {
				continue
			}
		}

		results = append(results, item)
	}

	sort.Slice(results, func(i, j int) bool {
		// Order: Community-Scripts, TurnKey, LinuxContainers
		if results[i].Source != results[j].Source {
			return results[i].Source < results[j].Source
		}
		return results[i].Title < results[j].Title
	})

	return results, nil
}

func (a *Adapter) SyncCatalog(ctx context.Context, force bool) (int, time.Time, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !force && time.Since(a.lastSync) < 12*time.Hour && len(a.items) > 0 {
		return len(a.items), a.lastSync, nil
	}

	// Try fetching Community-Scripts dynamic JSON
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, communityScriptsAPI, nil)
	if err == nil {
		resp, err := a.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var remoteEntries []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Slug        string `json:"slug"`
				Logo        string `json:"logo"`
				Category    string `json:"category"`
				Type        string `json:"type"`
				Install     string `json:"install_methods"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&remoteEntries); err == nil && len(remoteEntries) > 0 {
				for _, entry := range remoteEntries {
					if entry.Slug == "" {
						continue
					}
					id := "community-" + entry.Slug
					if existing, ok := a.items[id]; ok {
						if entry.Description != "" {
							existing.Description = entry.Description
						}
						if entry.Logo != "" {
							existing.IconURL = entry.Logo
						}
						existing.UpdatedAt = time.Now()
						a.items[id] = existing
					} else {
						a.items[id] = domain.LxcCatalogItem{
							ID:                    id,
							Title:                 entry.Name,
							Description:           entry.Description,
							Source:                "community-scripts",
							Category:              entry.Category,
							Categories:            []string{entry.Category, "LXC Apps"},
							IconURL:               entry.Logo,
							BaseDistribution:      "debian",
							BaseRelease:           "bookworm",
							Architecture:          "amd64",
							DefaultCPUCores:       2,
							DefaultMemoryLimitMiB: 1024,
							InstallScriptURL:      fmt.Sprintf("https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/%s-install.sh", entry.Slug),
							UpdatedAt:             time.Now(),
						}
					}
				}
			}
		}
	}

	a.lastSync = time.Now()
	a.saveCacheToDisk()

	return len(a.items), a.lastSync, nil
}

func (a *Adapter) loadCacheFromDisk() {
	cacheFile := filepath.Join(a.cacheDir, "lxc_catalog.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return
	}

	var cachedItems []domain.LxcCatalogItem
	if err := json.Unmarshal(data, &cachedItems); err != nil {
		return
	}

	for _, item := range cachedItems {
		a.items[item.ID] = item
	}
	a.lastSync = time.Now()
}

func (a *Adapter) saveCacheToDisk() {
	cacheFile := filepath.Join(a.cacheDir, "lxc_catalog.json")
	var items []domain.LxcCatalogItem
	for _, item := range a.items {
		items = append(items, item)
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(cacheFile, data, 0644)
}

func (a *Adapter) seedDefaultItems() {
	now := time.Now()

	// 1. Community-Scripts (App-centric LXC templates with setup scripts)
	communityApps := []domain.LxcCatalogItem{
		{
			ID:                    "community-pihole",
			Title:                 "Pi-hole",
			Description:           "Network-wide ad and tracker blocking via DNS sinkhole with a web management interface.",
			Source:                "community-scripts",
			Category:              "DNS & Networking",
			Categories:            []string{"DNS & Networking", "Security", "Recommended"},
			IconURL:               "https://pi-hole.net/wp-content/uploads/2016/12/Vortex-R.png",
			Website:               "https://pi-hole.net",
			DocumentationURL:      "https://docs.pi-hole.net",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 512,
			DefaultPort:           80,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/pihole-install.sh",
			InstallRecipe:         "curl -sSL https://install.pi-hole.net | bash --unattended",
			RecommendedMounts:     []string{"/etc/pihole", "/etc/dnsmasq.d"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-plex",
			Title:                 "Plex Media Server",
			Description:           "Organizes personal video, music, and photo collections and streams them to any smart screen or device with hardware transcoding.",
			Source:                "community-scripts",
			Category:              "Media & Streaming",
			Categories:            []string{"Media & Streaming", "Video", "Recommended"},
			IconURL:               "https://www.plex.tv/wp-content/themes/plex/assets/img/plex-logo.svg",
			Website:               "https://www.plex.tv",
			DocumentationURL:      "https://support.plex.tv",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       4,
			DefaultMemoryLimitMiB: 2048,
			DefaultPort:           32400,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/plex-install.sh",
			InstallRecipe:         "echo 'deb https://downloads.plex.tv/repo/deb public main' > /etc/apt/sources.list.d/plexmediaserver.list && curl -fsSL https://downloads.plex.tv/plex-keys/PlexSign.key | gpg --dearmor -o /etc/apt/trusted.gpg.d/plex.gpg && apt-get update && apt-get install -y plexmediaserver",
			RecommendedMounts:     []string{"/media/movies", "/media/tv", "/media/music"},
			GpuRecommended:        true,
			UpdatedAt:             now,
		},
		{
			ID:                    "community-jellyfin",
			Title:                 "Jellyfin",
			Description:           "The Free Software Media System that puts you in control of managing and streaming your media.",
			Source:                "community-scripts",
			Category:              "Media & Streaming",
			Categories:            []string{"Media & Streaming", "Video", "Open Source"},
			IconURL:               "https://jellyfin.org/images/logo.svg",
			Website:               "https://jellyfin.org",
			DocumentationURL:      "https://jellyfin.org/docs/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       4,
			DefaultMemoryLimitMiB: 2048,
			DefaultPort:           8096,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/jellyfin-install.sh",
			InstallRecipe:         "curl -fsSL https://repo.jellyfin.org/install-debuntu.sh | bash",
			RecommendedMounts:     []string{"/media"},
			GpuRecommended:        true,
			UpdatedAt:             now,
		},
		{
			ID:                    "community-homeassistant",
			Title:                 "Home Assistant",
			Description:           "Open source home automation that puts local control and privacy first. Powered by a worldwide community.",
			Source:                "community-scripts",
			Category:              "Home Automation",
			Categories:            []string{"Home Automation", "IoT", "Recommended"},
			IconURL:               "https://brands.home-assistant.io/_/homeassistant/logo.png",
			Website:               "https://www.home-assistant.io",
			DocumentationURL:      "https://www.home-assistant.io/docs/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 2048,
			DefaultPort:           8123,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/homeassistant-install.sh",
			InstallRecipe:         "apt-get update && apt-get install -y python3 python3-pip python3-venv && python3 -m venv /opt/homeassistant && /opt/homeassistant/bin/pip install homeassistant",
			RecommendedMounts:     []string{"/root/.homeassistant"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-adguard-home",
			Title:                 "AdGuard Home",
			Description:           "Network-wide software for blocking ads, tracking, and telemetry with encrypted DNS upstream (DoH/DoT/DoQ).",
			Source:                "community-scripts",
			Category:              "DNS & Networking",
			Categories:            []string{"DNS & Networking", "Security"},
			IconURL:               "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/adguard-home.png",
			Website:               "https://adguard.com/en/adguard-home.html",
			DocumentationURL:      "https://github.com/AdguardTeam/AdGuardHome/wiki",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 512,
			DefaultPort:           3000,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/adguard-home-install.sh",
			InstallRecipe:         "curl -s -S -L https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/scripts/install.sh | sh -s -- -v",
			UpdatedAt:             now,
		},
		{
			ID:                    "community-vaultwarden",
			Title:                 "Vaultwarden",
			Description:           "Lightweight Bitwarden-compatible server written in Rust, perfect for self-hosted password management.",
			Source:                "community-scripts",
			Category:              "Security & Passwords",
			Categories:            []string{"Security & Passwords", "Cloud"},
			IconURL:               "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/vaultwarden.png",
			Website:               "https://github.com/dani-garcia/vaultwarden",
			DocumentationURL:      "https://github.com/dani-garcia/vaultwarden/wiki",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 512,
			DefaultPort:           8000,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/vaultwarden-install.sh",
			RecommendedMounts:     []string{"/var/lib/vaultwarden/data"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-nextcloud",
			Title:                 "Nextcloud Hub",
			Description:           "Self-hosted productivity platform that keeps you in control. Store documents, photos, calendars, and collaborate privately.",
			Source:                "community-scripts",
			Category:              "Cloud & Storage",
			Categories:            []string{"Cloud & Storage", "File Sharing", "Productivity"},
			IconURL:               "https://nextcloud.com/media/nextcloud-logo-sq-blue.svg",
			Website:               "https://nextcloud.com",
			DocumentationURL:      "https://docs.nextcloud.com",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       4,
			DefaultMemoryLimitMiB: 2048,
			DefaultPort:           80,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/nextcloud-install.sh",
			RecommendedMounts:     []string{"/var/www/nextcloud/data"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-npm",
			Title:                 "Nginx Proxy Manager",
			Description:           "Expose your services easily and securely with automatic Let's Encrypt SSL certificates and a clean web UI.",
			Source:                "community-scripts",
			Category:              "DNS & Networking",
			Categories:            []string{"DNS & Networking", "Reverse Proxy"},
			IconURL:               "https://nginxproxymanager.com/icon.png",
			Website:               "https://nginxproxymanager.com",
			DocumentationURL:      "https://nginxproxymanager.com/guide/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           81,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/npm-install.sh",
			RecommendedMounts:     []string{"/data", "/etc/letsencrypt"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-tailscale",
			Title:                 "Tailscale Mesh VPN",
			Description:           "Zero-config mesh VPN based on WireGuard that makes your devices and servers accessible securely anywhere.",
			Source:                "community-scripts",
			Category:              "DNS & Networking",
			Categories:            []string{"DNS & Networking", "VPN"},
			IconURL:               "https://tailscale.com/brand/logo/tailscale-logo-circle.png",
			Website:               "https://tailscale.com",
			DocumentationURL:      "https://tailscale.com/kb/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 512,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/tailscale-install.sh",
			InstallRecipe:         "curl -fsSL https://tailscale.com/install.sh | sh",
			UpdatedAt:             now,
		},
		{
			ID:                    "community-wireguard",
			Title:                 "WireGuard VPN Server",
			Description:           "Extremely simple yet fast and modern VPN that utilizes state-of-the-art cryptography.",
			Source:                "community-scripts",
			Category:              "DNS & Networking",
			Categories:            []string{"DNS & Networking", "VPN", "Security"},
			IconURL:               "https://www.wireguard.com/img/wireguard.svg",
			Website:               "https://www.wireguard.com",
			DocumentationURL:      "https://www.wireguard.com/quickstart/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 512,
			DefaultPort:           51820,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/wireguard-install.sh",
			RecommendedMounts:     []string{"/etc/wireguard"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-uptime-kuma",
			Title:                 "Uptime Kuma",
			Description:           "Self-hosted monitoring tool with clean status pages, notifications, and uptime charts.",
			Source:                "community-scripts",
			Category:              "Monitoring",
			Categories:            []string{"Monitoring", "Analytics"},
			IconURL:               "https://uptime.kuma.pet/img/icon.svg",
			Website:               "https://uptime.kuma.pet",
			DocumentationURL:      "https://github.com/louislam/uptime-kuma/wiki",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           3001,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/uptime-kuma-install.sh",
			RecommendedMounts:     []string{"/opt/uptime-kuma/data"},
			UpdatedAt:             now,
		},
		{
			ID:                    "community-qbittorrent",
			Title:                 "qBittorrent Web UI",
			Description:           "Fast, stable, and ad-free BitTorrent client with feature-rich web management interface.",
			Source:                "community-scripts",
			Category:              "Torrents & Downloads",
			Categories:            []string{"Torrents & Downloads", "Media"},
			IconURL:               "https://www.qbittorrent.org/images/qbittorrent16.png",
			Website:               "https://www.qbittorrent.org",
			DocumentationURL:      "https://github.com/qbittorrent/qBittorrent/wiki",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           8080,
			InstallScriptURL:      "https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/install/qbittorrent-install.sh",
			InstallRecipe:         "apt-get update && apt-get install -y qbittorrent-nox",
			RecommendedMounts:     []string{"/downloads"},
			UpdatedAt:             now,
		},
	}

	// 2. TurnKey Linux (Pre-packaged Debian-based appliance rootfs archives)
	turnkeyAppliances := []domain.LxcCatalogItem{
		{
			ID:                    "turnkey-lamp",
			Title:                 "TurnKey LAMP Stack",
			Description:           "Apache, MariaDB/MySQL, and PHP web stack appliance with phpMyAdmin, ready for web application hosting.",
			Source:                "turnkey",
			Category:              "Web & Databases",
			Categories:            []string{"Web & Databases", "Development", "Appliances"},
			IconURL:               "https://www.turnkeylinux.org/files/images/lamp_0.png",
			Website:               "https://www.turnkeylinux.org/lamp",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/lamp",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           80,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-lamp/turnkey-lamp-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/var/www"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-wordpress",
			Title:                 "TurnKey WordPress",
			Description:           "Production-grade WordPress blogging and CMS appliance preconfigured with Apache, MySQL, and automatic updates.",
			Source:                "turnkey",
			Category:              "Web & CMS",
			Categories:            []string{"Web & CMS", "Appliances"},
			IconURL:               "https://s.w.org/style/images/about/WordPress-logotype-wmark.png",
			Website:               "https://www.turnkeylinux.org/wordpress",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/wordpress",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           80,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-wordpress/turnkey-wordpress-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/var/www/wordpress"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-postgresql",
			Title:                 "TurnKey PostgreSQL",
			Description:           "Dedicated relational database server appliance with phpPgAdmin, Webmin, and SSL encryption.",
			Source:                "turnkey",
			Category:              "Databases",
			Categories:            []string{"Databases", "Appliances"},
			IconURL:               "https://www.postgresql.org/media/img/about/press/elephant.png",
			Website:               "https://www.turnkeylinux.org/postgresql",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/postgresql",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           5432,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-postgresql/turnkey-postgresql-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/var/lib/postgresql"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-fileserver",
			Title:                 "TurnKey File Server",
			Description:           "Enterprise Samba (CIFS) and WebDAV network file server appliance with web-based user management.",
			Source:                "turnkey",
			Category:              "Storage & Sharing",
			Categories:            []string{"Storage & Sharing", "Samba", "Appliances"},
			IconURL:               "https://www.turnkeylinux.org/files/images/fileserver.png",
			Website:               "https://www.turnkeylinux.org/fileserver",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/fileserver",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           445,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-fileserver/turnkey-fileserver-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/srv/storage"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-torrentserver",
			Title:                 "TurnKey Torrent Server",
			Description:           "Full-featured Deluge BitTorrent download server with integrated Web UI and Samba file share for completed downloads.",
			Source:                "turnkey",
			Category:              "Torrents & Downloads",
			Categories:            []string{"Torrents & Downloads", "Appliances"},
			IconURL:               "https://www.turnkeylinux.org/files/images/torrentserver.png",
			Website:               "https://www.turnkeylinux.org/torrentserver",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/torrentserver",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           8112,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-torrentserver/turnkey-torrentserver-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/srv/torrents"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-gitlab",
			Title:                 "TurnKey GitLab",
			Description:           "Self-hosted Git repository management, code reviews, issue tracking, activity feeds, and wikis.",
			Source:                "turnkey",
			Category:              "Development",
			Categories:            []string{"Development", "DevOps", "Appliances"},
			IconURL:               "https://about.gitlab.com/images/press/logo/png/gitlab-icon-rgb.png",
			Website:               "https://www.turnkeylinux.org/gitlab",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/gitlab",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       4,
			DefaultMemoryLimitMiB: 4096,
			DefaultPort:           80,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-gitlab/turnkey-gitlab-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/var/opt/gitlab"},
			UpdatedAt:             now,
		},
		{
			ID:                    "turnkey-nodejs",
			Title:                 "TurnKey Node.js Stack",
			Description:           "Full-stack JavaScript development platform with Node.js, npm, PM2 process manager, and Nginx reverse proxy.",
			Source:                "turnkey",
			Category:              "Development",
			Categories:            []string{"Development", "JavaScript", "Appliances"},
			IconURL:               "https://nodejs.org/static/images/logo.svg",
			Website:               "https://www.turnkeylinux.org/nodejs",
			DocumentationURL:      "https://www.turnkeylinux.org/docs/nodejs",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           80,
			DownloadURL:           "https://releases.turnkeylinux.org/turnkey-nodejs/turnkey-nodejs-18.0-bookworm-amd64.tar.gz",
			RecommendedMounts:     []string{"/var/www"},
			UpdatedAt:             now,
		},
	}

	// 3. LinuxContainers (Canonical / linuxcontainers.org official minimal base images)
	linuxContainers := []domain.LxcCatalogItem{
		{
			ID:                    "lxc-debian-12",
			Title:                 "Debian 12 (Bookworm)",
			Description:           "The Universal Operating System. Official minimal Debian 12 Bookworm LXC base rootfs template.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Debian", "Recommended"},
			IconURL:               "https://www.debian.org/logos/openlogo-nd-100.png",
			Website:               "https://www.debian.org",
			DocumentationURL:      "https://www.debian.org/doc/",
			BaseDistribution:      "debian",
			BaseRelease:           "bookworm",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 512,
			DownloadURL:           "https://images.linuxcontainers.org/images/debian/bookworm/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-ubuntu-2404",
			Title:                 "Ubuntu 24.04 LTS (Noble Numbat)",
			Description:           "Canonical Ubuntu 24.04 LTS minimal cloud and container rootfs image with 5-year support window.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Ubuntu", "Recommended"},
			IconURL:               "https://assets.ubuntu.com/v1/29985a98-ubuntu-logo112.png",
			Website:               "https://ubuntu.com",
			DocumentationURL:      "https://help.ubuntu.com",
			BaseDistribution:      "ubuntu",
			BaseRelease:           "noble",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/ubuntu/noble/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-ubuntu-2204",
			Title:                 "Ubuntu 22.04 LTS (Jammy Jellyfish)",
			Description:           "Canonical Ubuntu 22.04 LTS minimal rootfs image with long-term kernel and package support.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Ubuntu"},
			IconURL:               "https://assets.ubuntu.com/v1/29985a98-ubuntu-logo112.png",
			Website:               "https://ubuntu.com",
			DocumentationURL:      "https://help.ubuntu.com",
			BaseDistribution:      "ubuntu",
			BaseRelease:           "jammy",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/ubuntu/jammy/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-alpine-320",
			Title:                 "Alpine Linux 3.20",
			Description:           "Security-oriented, lightweight Linux distribution based on musl libc and busybox (~5MB compressed).",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Alpine", "Lightweight"},
			IconURL:               "https://alpinelinux.org/alpinelinux-logo.svg",
			Website:               "https://alpinelinux.org",
			DocumentationURL:      "https://wiki.alpinelinux.org",
			BaseDistribution:      "alpine",
			BaseRelease:           "3.20",
			Architecture:          "amd64",
			DefaultCPUCores:       1,
			DefaultMemoryLimitMiB: 256,
			DownloadURL:           "https://images.linuxcontainers.org/images/alpine/3.20/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-archlinux",
			Title:                 "Arch Linux",
			Description:           "A lightweight and flexible Linux distribution that tries to Keep It Simple. Rolling release rootfs.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Arch"},
			IconURL:               "https://archlinux.org/static/logos/archlinux-logo-dark-scalable.518507f0aa9f.svg",
			Website:               "https://archlinux.org",
			DocumentationURL:      "https://wiki.archlinux.org",
			BaseDistribution:      "archlinux",
			BaseRelease:           "current",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/archlinux/current/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-fedora-40",
			Title:                 "Fedora 40",
			Description:           "Innovative, free, and open source platform built on the cutting edge of the Linux ecosystem.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Fedora"},
			IconURL:               "https://fedoraproject.org/assets/images/fedora-logo.svg",
			Website:               "https://fedoraproject.org",
			DocumentationURL:      "https://docs.fedoraproject.org",
			BaseDistribution:      "fedora",
			BaseRelease:           "40",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/fedora/40/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-rocky-9",
			Title:                 "Rocky Linux 9",
			Description:           "Enterprise Linux compatible downstream release based on RHEL 9 source code.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Enterprise"},
			IconURL:               "https://rockylinux.org/images/logo.svg",
			Website:               "https://rockylinux.org",
			DocumentationURL:      "https://docs.rockylinux.org",
			BaseDistribution:      "rocky",
			BaseRelease:           "9",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/rockylinux/9/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
		{
			ID:                    "lxc-almalinux-9",
			Title:                 "AlmaLinux 9",
			Description:           "Open source, community-governed and forever-free enterprise Linux distribution.",
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", "Enterprise"},
			IconURL:               "https://almalinux.org/images/almalinux-logo.svg",
			Website:               "https://almalinux.org",
			DocumentationURL:      "https://wiki.almalinux.org",
			BaseDistribution:      "almalinux",
			BaseRelease:           "9",
			Architecture:          "amd64",
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           "https://images.linuxcontainers.org/images/almalinux/9/amd64/default/rootfs.tar.xz",
			UpdatedAt:             now,
		},
	}

	for _, app := range communityApps {
		a.items[app.ID] = app
	}
	for _, app := range turnkeyAppliances {
		a.items[app.ID] = app
	}
	for _, app := range linuxContainers {
		a.items[app.ID] = app
	}

	a.lastSync = now
}
