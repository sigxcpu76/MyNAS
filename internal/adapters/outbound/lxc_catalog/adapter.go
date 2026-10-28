package lxc_catalog

import (
	"bufio"
	"context"
	_ "embed"
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

//go:embed catalog_seeds.json
var embeddedCatalogSeeds []byte

const (
	communityScriptsPBURL = "https://db.community-scripts.org"
	turnkeyAplInfoURL     = "https://releases.turnkeylinux.org/pve/aplinfo.dat"
	linuxContainersURL    = "https://images.linuxcontainers.org/meta/1.0/index-system"
)

type CatalogMeta struct {
	LastSync   time.Time `json:"last_sync"`
	TotalItems int       `json:"total_items"`
	Version    int       `json:"version"`
}

type Adapter struct {
	mu                    sync.RWMutex
	cacheDir              string
	items                 map[string]domain.LxcCatalogItem
	lastSync              time.Time
	httpClient            *http.Client
	communityScriptsPBURL string
	turnkeyAplInfoURL     string
	linuxContainersURL    string
}

func NewAdapter(cacheDir string) *Adapter {
	if cacheDir == "" {
		cacheDir = "/tmp/nas-lxc-catalog"
	}
	_ = os.MkdirAll(cacheDir, 0755)

	a := &Adapter{
		cacheDir:              cacheDir,
		items:                 make(map[string]domain.LxcCatalogItem),
		httpClient:            &http.Client{Timeout: 45 * time.Second},
		communityScriptsPBURL: communityScriptsPBURL,
		turnkeyAplInfoURL:     turnkeyAplInfoURL,
		linuxContainersURL:    linuxContainersURL,
	}

	// 1. Seed pre-defined embedded catalog items (900+ community scripts, turnkey, and linuxcontainers)
	a.seedDefaultItems()

	// 2. Load cached catalog from disk if present (which may be newer than seeds)
	a.loadCacheFromDisk()

	return a
}

func (a *Adapter) SetURLsForTest(communityScripts, turnkey, linuxContainers string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if communityScripts != "" {
		a.communityScriptsPBURL = communityScripts
	}
	if turnkey != "" {
		a.turnkeyAplInfoURL = turnkey
	}
	if linuxContainers != "" {
		a.linuxContainersURL = linuxContainers
	}
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

	if !force && !a.lastSync.IsZero() && time.Since(a.lastSync) < 1*time.Hour && len(a.items) > 0 {
		return len(a.items), a.lastSync, nil
	}

	// 1. Sync Community Scripts from PocketBase
	a.syncCommunityScripts(ctx)

	// 2. Sync TurnKey Linux appliances from aplinfo.dat
	a.syncTurnKey(ctx)

	// 3. Sync LinuxContainers official OS minimal templates from index-system
	a.syncLinuxContainers(ctx)

	a.lastSync = time.Now().UTC()
	_ = a.saveCacheToDiskLocked()

	return len(a.items), a.lastSync, nil
}

type pbScriptItem struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Description    string   `json:"description"`
	Logo           string   `json:"logo"`
	Categories     []string `json:"categories"`
	Port           int      `json:"port"`
	Privileged     bool     `json:"privileged"`
	Website        string   `json:"website"`
	Documentation  string   `json:"documentation"`
	InstallMethods []struct {
		Type      string `json:"type"`
		Resources struct {
			CPU     any    `json:"cpu"`
			RAM     any    `json:"ram"`
			HDD     any    `json:"hdd"`
			OS      string `json:"os"`
			Version any    `json:"version"`
		} `json:"resources"`
	} `json:"install_methods"`
}

type pbScriptResponse struct {
	Items      []pbScriptItem `json:"items"`
	TotalItems int            `json:"totalItems"`
	TotalPages int            `json:"totalPages"`
}

type pbCategoryResponse struct {
	Items []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"items"`
}

func (a *Adapter) syncCommunityScripts(ctx context.Context) {
	pbURL := a.communityScriptsPBURL
	if pbURL == "" {
		pbURL = communityScriptsPBURL
	}

	// Categories lookup
	catMap := make(map[string]string)
	catReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pbURL+"/api/collections/script_categories/records?perPage=100", nil)
	if err == nil {
		resp, err := a.httpClient.Do(catReq)
		if err == nil {
			defer resp.Body.Close()
			var catResp pbCategoryResponse
			if err := json.NewDecoder(resp.Body).Decode(&catResp); err == nil {
				for _, c := range catResp.Items {
					catMap[c.ID] = c.Name
				}
			}
		}
	}

	now := time.Now()
	for page := 1; page <= 5; page++ {
		url := fmt.Sprintf("%s/api/collections/script_scripts/records?page=%d&perPage=500", pbURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			break
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			break
		}

		var pResp pbScriptResponse
		if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
			resp.Body.Close()
			break
		}
		resp.Body.Close()

		for _, it := range pResp.Items {
			if it.Slug == "" {
				continue
			}
			category := "General"
			var categories []string
			for _, cID := range it.Categories {
				if cName, ok := catMap[cID]; ok {
					if category == "General" {
						category = cName
					}
					categories = append(categories, cName)
				}
			}
			if len(categories) == 0 {
				categories = []string{category}
			}

			cpu := 1
			ram := int64(1024)
			distro := "debian"
			release := "bookworm"
			if len(it.InstallMethods) > 0 {
				res := it.InstallMethods[0].Resources
				if v, ok := res.CPU.(float64); ok && v > 0 {
					cpu = int(v)
				}
				if v, ok := res.RAM.(float64); ok && v > 0 {
					ram = int64(v)
				}
				if res.OS != "" {
					distro = strings.ToLower(res.OS)
				}
				if res.Version != nil {
					release = strings.ToLower(fmt.Sprintf("%v", res.Version))
				}
			}

			titleLower := strings.ToLower(it.Name)
			descLower := strings.ToLower(it.Description)
			gpu := strings.Contains(titleLower, "plex") ||
				strings.Contains(titleLower, "jellyfin") ||
				strings.Contains(titleLower, "emby") ||
				strings.Contains(titleLower, "frigate") ||
				strings.Contains(titleLower, "ollama") ||
				strings.Contains(descLower, "hardware acceleration") ||
				strings.Contains(descLower, "transcoding") ||
				strings.Contains(descLower, "cuda")

			id := "community-" + it.Slug
			a.items[id] = domain.LxcCatalogItem{
				ID:                    id,
				Title:                 it.Name,
				Description:           it.Description,
				Source:                "community-scripts",
				Category:              category,
				Categories:            categories,
				IconURL:               it.Logo,
				Website:               it.Website,
				DocumentationURL:      it.Documentation,
				BaseDistribution:      distro,
				BaseRelease:           release,
				Architecture:          "amd64",
				DefaultCPUCores:       int32(cpu),
				DefaultMemoryLimitMiB: ram,
				DefaultPort:           it.Port,
				InstallScriptURL:      fmt.Sprintf("https://raw.githubusercontent.com/community-scripts/ProxmoxVE/main/ct/%s.sh", it.Slug),
				PrivilegedRequired:    it.Privileged,
				GpuRecommended:        gpu,
				UpdatedAt:             now,
			}
		}

		if page >= pResp.TotalPages || len(pResp.Items) == 0 {
			break
		}
	}
}

func (a *Adapter) syncTurnKey(ctx context.Context) {
	url := a.turnkeyAplInfoURL
	if url == "" {
		url = turnkeyAplInfoURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	now := time.Now()
	scanner := bufio.NewScanner(resp.Body)
	var curPkg, curVer, curOS, curArch, curLoc, curInfo, curDesc string
	_ = curVer

	parseCurrent := func() {
		if curPkg == "" || curLoc == "" {
			return
		}
		slug := strings.TrimPrefix(curPkg, "turnkey-")
		title := strings.Title(strings.ReplaceAll(slug, "-", " "))
		if strings.HasPrefix(curDesc, "TurnKey ") {
			parts := strings.SplitN(curDesc, " - ", 2)
			if len(parts) > 0 {
				title = strings.TrimSpace(parts[0])
			}
		}

		desc := curDesc
		if idx := strings.Index(desc, " - "); idx != -1 {
			desc = strings.TrimSpace(desc[idx+3:])
		}
		if desc == "" {
			desc = fmt.Sprintf("TurnKey %s production-ready appliance pre-configured on Debian.", title)
		}

		distro := "debian"
		release := "bookworm"
		if strings.Contains(curOS, "13") {
			release = "trixie"
		} else if strings.Contains(curOS, "12") {
			release = "bookworm"
		} else if strings.Contains(curOS, "11") {
			release = "bullseye"
		}

		cat := "Appliance"
		if strings.Contains(slug, "sql") || strings.Contains(slug, "mongo") || strings.Contains(slug, "redis") || strings.Contains(slug, "database") {
			cat = "Databases"
		} else if strings.Contains(slug, "lamp") || strings.Contains(slug, "nginx") || strings.Contains(slug, "web") || strings.Contains(slug, "wordpress") || strings.Contains(slug, "drupal") || strings.Contains(slug, "joomla") {
			cat = "Web & CMS"
		} else if strings.Contains(slug, "file") || strings.Contains(slug, "nextcloud") || strings.Contains(slug, "owncloud") || strings.Contains(slug, "sync") || strings.Contains(slug, "storage") {
			cat = "Storage & Files"
		} else if strings.Contains(slug, "git") || strings.Contains(slug, "dev") || strings.Contains(slug, "node") || strings.Contains(slug, "python") || strings.Contains(slug, "rails") || strings.Contains(slug, "php") {
			cat = "Development"
		} else if strings.Contains(slug, "mail") || strings.Contains(slug, "dns") || strings.Contains(slug, "vpn") || strings.Contains(slug, "wireguard") || strings.Contains(slug, "openvpn") || strings.Contains(slug, "firewall") {
			cat = "Networking & Security"
		}

		port := 80
		if strings.Contains(slug, "nextcloud") || strings.Contains(slug, "owncloud") {
			port = 443
		} else if strings.Contains(slug, "postgres") {
			port = 5432
		} else if strings.Contains(slug, "mysql") || strings.Contains(slug, "mariadb") {
			port = 3306
		}

		a.items[curPkg] = domain.LxcCatalogItem{
			ID:                    curPkg,
			Title:                 title,
			Description:           desc,
			Source:                "turnkey",
			Category:              cat,
			Categories:            []string{cat, "TurnKey Appliance"},
			Website:               curInfo,
			DocumentationURL:      curInfo,
			BaseDistribution:      distro,
			BaseRelease:           release,
			Architecture:          curArch,
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DefaultPort:           port,
			DownloadURL:           curLoc,
			PrivilegedRequired:    false,
			UpdatedAt:             now,
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			parseCurrent()
			curPkg, curVer, curOS, curArch, curLoc, curInfo, curDesc = "", "", "", "", "", "", ""
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			switch key {
			case "Package":
				curPkg = val
			case "Version":
				curVer = val
			case "OS":
				curOS = val
			case "Architecture":
				curArch = val
			case "Location":
				curLoc = val
			case "Infopage":
				curInfo = val
			case "Description":
				curDesc = val
			}
		} else if strings.HasPrefix(line, " ") && curDesc != "" {
			curDesc += " " + strings.TrimSpace(line)
		}
	}
	parseCurrent()
}

func (a *Adapter) syncLinuxContainers(ctx context.Context) {
	url := a.linuxContainersURL
	if url == "" {
		url = linuxContainersURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	now := time.Now()
	scanner := bufio.NewScanner(resp.Body)
	seen := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ";")
		if len(parts) < 6 {
			continue
		}
		distro := parts[0]
		release := parts[1]
		arch := parts[2]
		variant := parts[3]
		path := parts[5]

		if arch != "amd64" || variant != "default" {
			continue
		}
		key := fmt.Sprintf("%s-%s", distro, release)
		if seen[key] {
			continue
		}
		seen[key] = true

		distroTitle := strings.Title(distro)
		title := fmt.Sprintf("%s %s (Official Minimal)", distroTitle, strings.Title(release))
		desc := fmt.Sprintf("Official upstream %s %s minimal system container rootfs from LinuxContainers (Canonical).", distroTitle, release)
		rootfsURL := fmt.Sprintf("https://images.linuxcontainers.org%srootfs.tar.xz", path)
		id := fmt.Sprintf("lxc-%s-%s", distro, release)

		a.items[id] = domain.LxcCatalogItem{
			ID:                    id,
			Title:                 title,
			Description:           desc,
			Source:                "linuxcontainers",
			Category:              "Base OS",
			Categories:            []string{"Base OS", distroTitle},
			Website:               "https://linuxcontainers.org",
			DocumentationURL:      "https://images.linuxcontainers.org",
			BaseDistribution:      distro,
			BaseRelease:           release,
			Architecture:          arch,
			DefaultCPUCores:       2,
			DefaultMemoryLimitMiB: 1024,
			DownloadURL:           rootfsURL,
			PrivilegedRequired:    false,
			UpdatedAt:             now,
		}
	}
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(filename)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, filename)
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

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, item := range cachedItems {
		a.items[item.ID] = item
	}

	metaFile := filepath.Join(a.cacheDir, "lxc_catalog_meta.json")
	if metaData, err := os.ReadFile(metaFile); err == nil {
		var meta CatalogMeta
		if err := json.Unmarshal(metaData, &meta); err == nil && !meta.LastSync.IsZero() {
			a.lastSync = meta.LastSync.UTC()
			return
		}
	}

	if fi, err := os.Stat(cacheFile); err == nil {
		a.lastSync = fi.ModTime().UTC()
	}
}

func (a *Adapter) saveCacheToDiskLocked() error {
	var items []domain.LxcCatalogItem
	for _, item := range a.items {
		items = append(items, item)
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	cacheFile := filepath.Join(a.cacheDir, "lxc_catalog.json")
	if err := atomicWriteFile(cacheFile, data, 0644); err != nil {
		return err
	}

	meta := CatalogMeta{
		LastSync:   a.lastSync.UTC(),
		TotalItems: len(items),
		Version:    1,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	metaFile := filepath.Join(a.cacheDir, "lxc_catalog_meta.json")
	return atomicWriteFile(metaFile, metaData, 0644)
}

func (a *Adapter) saveCacheToDisk() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.saveCacheToDiskLocked()
}

func (a *Adapter) seedDefaultItems() {
	if len(embeddedCatalogSeeds) > 0 {
		var seeds []domain.LxcCatalogItem
		if err := json.Unmarshal(embeddedCatalogSeeds, &seeds); err == nil && len(seeds) > 0 {
			for _, item := range seeds {
				a.items[item.ID] = item
			}
			return
		}
	}
}
