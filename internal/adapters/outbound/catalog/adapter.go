package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	defaultCatalogURL = "https://raw.githubusercontent.com/truenas/apps/master/catalog.json"
	defaultSyncPeriod = 24 * time.Hour
)

type CatalogMeta struct {
	LastSync  time.Time `json:"last_sync"`
	TotalApps int       `json:"total_apps"`
	Version   int       `json:"version"`
}

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
	mu          sync.RWMutex
	cacheDir    string
	apps        map[string]domain.CatalogApp // keyed by "train/app_id"
	lastSync    time.Time
	httpClient  *http.Client
	catalogURL  string
	casaosURL   string
	dokployURL  string
	appsPool    string
	appdataPath string
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
		casaosURL:   defaultCasaOSIndexURL,
		dokployURL:  defaultDokployMetaURL,
		appsPool:    "tank",
		appdataPath: "/mnt/tank/apps",
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
		if tr != "" && tr != "all" {
			if tr == "truenas" {
				if app.Train != "stable" && app.Train != "community" {
					continue
				}
			} else if !strings.EqualFold(app.Train, tr) {
				continue
			}
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
		rank := func(tr string) int {
			switch strings.ToLower(tr) {
			case "stable":
				return 0
			case "casaos":
				return 1
			case "dokploy":
				return 2
			case "community":
				return 3
			default:
				return 4
			}
		}
		rI := rank(results[i].Train)
		rJ := rank(results[j].Train)
		if rI != rJ {
			return rI < rJ
		}
		return results[i].Title < results[j].Title
	})

	return results, nil
}

func (a *Adapter) GetApp(ctx context.Context, train, appID string) (*domain.CatalogApp, error) {
	a.mu.RLock()
	var foundApp *domain.CatalogApp

	// Try exact train/app_id
	if train != "" {
		key := fmt.Sprintf("%s/%s", train, appID)
		if app, ok := a.apps[key]; ok {
			cp := app
			foundApp = &cp
		}
	}

	// Try known catalogs in order: stable, casaos, dokploy, community
	if foundApp == nil {
		for _, tr := range []string{"stable", "casaos", "dokploy", "community"} {
			if app, ok := a.apps[fmt.Sprintf("%s/%s", tr, appID)]; ok {
				cp := app
				foundApp = &cp
				break
			}
		}
	}

	// Scan by AppID case-insensitively
	if foundApp == nil {
		for _, app := range a.apps {
			if strings.EqualFold(app.AppID, appID) {
				cp := app
				foundApp = &cp
				break
			}
		}
	}
	a.mu.RUnlock()

	if foundApp == nil {
		return nil, fmt.Errorf("app %q not found in catalog", appID)
	}

	res := *foundApp
	if res.Train == "casaos" {
		enrichFromCasaOS(ctx, a.httpClient, &res, a.cacheDir, a.getAppsPool(), a.getAppdataPath())
	} else if res.Train == "dokploy" {
		enrichFromDokploy(ctx, a.httpClient, &res, a.cacheDir, a.getAppsPool(), a.getAppdataPath())
	} else {
		res = a.ensureComposeYAML(res)
		a.enrichFromIXValues(ctx, &res)
	}
	return &res, nil
}

func (a *Adapter) SyncCatalog(ctx context.Context, force bool) (int, time.Time, error) {
	a.mu.RLock()
	if !force && !a.lastSync.IsZero() && time.Since(a.lastSync) < 1*time.Hour && len(a.apps) > 0 {
		count := len(a.apps)
		lastSync := a.lastSync
		a.mu.RUnlock()
		return count, lastSync, nil
	}
	catalogURL := a.catalogURL
	casaosURL := a.casaosURL
	dokployURL := a.dokployURL
	a.mu.RUnlock()

	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		truenasBody []byte
		truenasErr  error
		rawCatalog  map[string]map[string]rawCatalogEntry

		casaosBody []byte
		casaosErr  error
		casaosIdx  *casaosIndex

		dokployBody    []byte
		dokployErr     error
		dokployEntries []dokployMetaEntry
	)

	// Fetch TrueNAS catalog
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
		if err != nil {
			mu.Lock()
			truenasErr = err
			mu.Unlock()
			return
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			mu.Lock()
			truenasErr = err
			mu.Unlock()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			mu.Lock()
			truenasErr = fmt.Errorf("truenas catalog returned HTTP status %d", resp.StatusCode)
			mu.Unlock()
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			mu.Lock()
			truenasErr = err
			mu.Unlock()
			return
		}
		var parsed map[string]map[string]rawCatalogEntry
		if err := json.Unmarshal(body, &parsed); err != nil {
			mu.Lock()
			truenasErr = err
			mu.Unlock()
			return
		}
		mu.Lock()
		truenasBody = body
		rawCatalog = parsed
		mu.Unlock()
	}()

	// Fetch CasaOS catalog
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx, body, err := fetchCasaOSIndex(ctx, a.httpClient, casaosURL)
		mu.Lock()
		if err != nil {
			casaosErr = err
		} else {
			casaosIdx = idx
			casaosBody = body
		}
		mu.Unlock()
	}()

	// Fetch Dokploy catalog
	wg.Add(1)
	go func() {
		defer wg.Done()
		entries, body, err := fetchDokployIndex(ctx, a.httpClient, dokployURL)
		mu.Lock()
		if err != nil {
			dokployErr = err
		} else {
			dokployEntries = entries
			dokployBody = body
		}
		mu.Unlock()
	}()

	wg.Wait()

	if truenasErr != nil && casaosErr != nil && dokployErr != nil {
		return 0, a.lastSync, fmt.Errorf("failed to fetch any catalog (truenas: %v, casaos: %v, dokploy: %v)", truenasErr, casaosErr, dokployErr)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Process TrueNAS entries
	if rawCatalog != nil {
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

				image := resolveContainerImage(appID, item.Sources, item.Home)
				if image == "" && existing.ContainerImage != "" {
					image = existing.ContainerImage
				}

				a.apps[key] = domain.CatalogApp{
					AppID:          appID,
					Title:          title,
					Description:    cleanDescription(item.Description),
					Category:       cat,
					Categories:     item.Categories,
					IconURL:        item.IconURL,
					Home:           item.Home,
					Train:          trainName,
					Version:        item.LatestVer,
					AppVersion:     item.LatestAppV,
					Tags:           item.Tags,
					Screenshots:    item.Screenshots,
					Sources:        item.Sources,
					ContainerImage: image,
					ComposeYAML:    compose,
					DefaultPort:    defaultPort,
				}
			}
		}
	}

	// Process CasaOS entries
	if casaosIdx != nil {
		base := casaosIdx.BaseURL
		if base == "" {
			base = defaultCasaOSBaseURL
		}
		for _, item := range casaosIdx.Apps {
			appID := item.ID
			if appID == "" {
				continue
			}
			title := item.Title
			if title == "" {
				title = appID
			}
			cat := item.Category
			if cat == "" && len(item.Categories) > 0 {
				cat = item.Categories[0]
			}
			iconURL := item.Icon
			if iconURL != "" && strings.HasPrefix(iconURL, "/") {
				iconURL = strings.TrimRight(base, "/") + iconURL
			}
			var sources []string
			if item.ComposeURL != "" {
				compURL := item.ComposeURL
				if strings.HasPrefix(compURL, "/") {
					compURL = strings.TrimRight(base, "/") + compURL
				}
				sources = append(sources, compURL)
			}
			if item.MetaURL != "" {
				metaURL := item.MetaURL
				if strings.HasPrefix(metaURL, "/") {
					metaURL = strings.TrimRight(base, "/") + metaURL
				}
				sources = append(sources, metaURL)
			}
			key := fmt.Sprintf("casaos/%s", appID)
			existing, exists := a.apps[key]
			compose := ""
			defaultPort := 0
			containerImage := ""
			if exists {
				compose = existing.ComposeYAML
				defaultPort = existing.DefaultPort
				containerImage = existing.ContainerImage
			}
			a.apps[key] = domain.CatalogApp{
				AppID:          appID,
				Title:          title,
				Description:    cleanDescription(item.Tagline),
				Category:       cat,
				Categories:     item.Categories,
				IconURL:        iconURL,
				Train:          "casaos",
				Version:        item.Version,
				AppVersion:     item.Version,
				Sources:        sources,
				ComposeYAML:    compose,
				ContainerImage: containerImage,
				DefaultPort:    defaultPort,
			}
		}
	}

	// Process Dokploy entries
	if dokployEntries != nil {
		for _, item := range dokployEntries {
			appID := item.ID
			if appID == "" {
				continue
			}
			title := item.Name
			if title == "" {
				title = appID
			}
			cat := ""
			if len(item.Tags) > 0 {
				cat = item.Tags[0]
			}
			home := ""
			if item.Links != nil {
				if h, ok := item.Links["website"]; ok {
					home = h
				} else if g, ok := item.Links["github"]; ok {
					home = g
				}
			}
			key := fmt.Sprintf("dokploy/%s", appID)
			existing, exists := a.apps[key]
			compose := ""
			defaultPort := 0
			containerImage := ""
			if exists {
				compose = existing.ComposeYAML
				defaultPort = existing.DefaultPort
				containerImage = existing.ContainerImage
			}
			a.apps[key] = domain.CatalogApp{
				AppID:          appID,
				Title:          title,
				Description:    cleanDescription(item.Description),
				Category:       cat,
				Categories:     item.Tags,
				IconURL:        item.Logo,
				Home:           home,
				Train:          "dokploy",
				Version:        item.Version,
				AppVersion:     item.Version,
				Tags:           item.Tags,
				ComposeYAML:    compose,
				ContainerImage: containerImage,
				DefaultPort:    defaultPort,
			}
		}
	}

	a.lastSync = time.Now().UTC()
	_ = a.saveCacheToDisk(truenasBody, casaosBody, dokployBody)

	return len(a.apps), a.lastSync, nil
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

func (a *Adapter) SetCasaOSURLForTest(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.casaosURL = url
}

func (a *Adapter) SetDokployURLForTest(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dokployURL = url
}

func (a *Adapter) SetAppsPool(pool string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pool = strings.TrimSpace(strings.TrimPrefix(pool, "zfsPools/"))
	if pool != "" {
		a.appsPool = pool
		a.appdataPath = fmt.Sprintf("/mnt/%s/apps", pool)
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
		if strings.HasPrefix(a.appdataPath, "/mnt/") || strings.HasPrefix(a.appdataPath, "/nas/") {
			return a.appdataPath
		}
		clean := strings.TrimPrefix(a.appdataPath, "/")
		return "/mnt/" + clean
	}
	if a.appsPool != "" {
		return fmt.Sprintf("/mnt/%s/apps", a.appsPool)
	}
	return "/mnt/tank/apps"
}

func (a *Adapter) ensureComposeYAML(app domain.CatalogApp) domain.CatalogApp {
	if app.ContainerImage == "" {
		app.ContainerImage = resolveContainerImage(app.AppID, app.Sources, app.Home)
	}
	if app.ComposeYAML == "" {
		app.ComposeYAML, app.DefaultPort = generateResolvedCompose(app)
	}
	appdataPath := a.getAppdataPath()
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/tank/appdata", appdataPath)
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/mnt/tank/apps", appdataPath)
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/mnt/tank/apps", appdataPath)
	pool := a.getAppsPool()
	if pool == "" {
		pool = "tank"
	}
	mediaPath := fmt.Sprintf("/mnt/%s/media", pool)
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/tank/media", mediaPath)
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/mnt/tank/media", mediaPath)
	app.ComposeYAML = strings.ReplaceAll(app.ComposeYAML, "/mnt/tank/media", mediaPath)
	app.ComposeYAML = docker.NormalizeComposeVolumes(app.AppID, pool, app.ComposeYAML)
	return app
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

func (a *Adapter) saveCacheToDisk(truenasData, casaosData, dokployData []byte) error {
	if len(truenasData) > 0 {
		cacheFile := filepath.Join(a.cacheDir, "catalog.json")
		_ = atomicWriteFile(cacheFile, truenasData, 0644)
	}
	if len(casaosData) > 0 {
		casaosFile := filepath.Join(a.cacheDir, "casaos_index.json")
		_ = atomicWriteFile(casaosFile, casaosData, 0644)
	}
	if len(dokployData) > 0 {
		dokployFile := filepath.Join(a.cacheDir, "dokploy_meta.json")
		_ = atomicWriteFile(dokployFile, dokployData, 0644)
	}

	meta := CatalogMeta{
		LastSync:  a.lastSync.UTC(),
		TotalApps: len(a.apps),
		Version:   1,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	metaFile := filepath.Join(a.cacheDir, "catalog_meta.json")
	return atomicWriteFile(metaFile, metaData, 0644)
}

func (a *Adapter) loadCacheFromDisk() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 1. TrueNAS catalog
	cacheFile := filepath.Join(a.cacheDir, "catalog.json")
	if data, err := os.ReadFile(cacheFile); err == nil {
		var rawCatalog map[string]map[string]rawCatalogEntry
		if err := json.Unmarshal(data, &rawCatalog); err == nil {
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

					image := resolveContainerImage(appID, item.Sources, item.Home)
					if image == "" && existing.ContainerImage != "" {
						image = existing.ContainerImage
					}

					a.apps[key] = domain.CatalogApp{
						AppID:          appID,
						Title:          title,
						Description:    cleanDescription(item.Description),
						Category:       cat,
						Categories:     item.Categories,
						IconURL:        item.IconURL,
						Home:           item.Home,
						Train:          trainName,
						Version:        item.LatestVer,
						AppVersion:     item.LatestAppV,
						Tags:           item.Tags,
						Screenshots:    item.Screenshots,
						Sources:        item.Sources,
						ContainerImage: image,
						ComposeYAML:    compose,
						DefaultPort:    defaultPort,
					}
				}
			}
		}
	}

	// 2. CasaOS catalog
	casaosFile := filepath.Join(a.cacheDir, "casaos_index.json")
	if data, err := os.ReadFile(casaosFile); err == nil {
		var idx casaosIndex
		if err := json.Unmarshal(data, &idx); err == nil {
			base := idx.BaseURL
			if base == "" {
				base = defaultCasaOSBaseURL
			}
			for _, item := range idx.Apps {
				appID := item.ID
				if appID == "" {
					continue
				}
				title := item.Title
				if title == "" {
					title = appID
				}
				cat := item.Category
				if cat == "" && len(item.Categories) > 0 {
					cat = item.Categories[0]
				}
				iconURL := item.Icon
				if iconURL != "" && strings.HasPrefix(iconURL, "/") {
					iconURL = strings.TrimRight(base, "/") + iconURL
				}
				var sources []string
				if item.ComposeURL != "" {
					compURL := item.ComposeURL
					if strings.HasPrefix(compURL, "/") {
						compURL = strings.TrimRight(base, "/") + compURL
					}
					sources = append(sources, compURL)
				}
				if item.MetaURL != "" {
					metaURL := item.MetaURL
					if strings.HasPrefix(metaURL, "/") {
						metaURL = strings.TrimRight(base, "/") + metaURL
					}
					sources = append(sources, metaURL)
				}
				key := fmt.Sprintf("casaos/%s", appID)
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
					Description: cleanDescription(item.Tagline),
					Category:    cat,
					Categories:  item.Categories,
					IconURL:     iconURL,
					Train:       "casaos",
					Version:     item.Version,
					AppVersion:  item.Version,
					Sources:     sources,
					ComposeYAML: compose,
					DefaultPort: defaultPort,
				}
			}
		}
	}

	// 3. Dokploy catalog
	dokployFile := filepath.Join(a.cacheDir, "dokploy_meta.json")
	if data, err := os.ReadFile(dokployFile); err == nil {
		var entries []dokployMetaEntry
		if err := json.Unmarshal(data, &entries); err == nil {
			for _, item := range entries {
				appID := item.ID
				if appID == "" {
					continue
				}
				title := item.Name
				if title == "" {
					title = appID
				}
				cat := ""
				if len(item.Tags) > 0 {
					cat = item.Tags[0]
				}
				home := ""
				if item.Links != nil {
					if h, ok := item.Links["website"]; ok {
						home = h
					} else if g, ok := item.Links["github"]; ok {
						home = g
					}
				}
				key := fmt.Sprintf("dokploy/%s", appID)
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
					Categories:  item.Tags,
					IconURL:     item.Logo,
					Home:        home,
					Train:       "dokploy",
					Version:     item.Version,
					AppVersion:  item.Version,
					Tags:        item.Tags,
					ComposeYAML: compose,
					DefaultPort: defaultPort,
				}
			}
		}
	}

	metaFile := filepath.Join(a.cacheDir, "catalog_meta.json")
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

var (
	reDockerHubRepo = regexp.MustCompile(`hub\.docker\.com/r/([^/\s\"'\?]+/[^/\s\"'\?]+)`)
	reDockerHubLib  = regexp.MustCompile(`hub\.docker\.com/_/([^/\s\"'\?]+)`)
	reGHCR          = regexp.MustCompile(`ghcr\.io/([^/\s\"'\?]+/[^/\s\"'\?]+)`)
	reQuayRepo      = regexp.MustCompile(`quay\.io/repository/([^/\s\"'\?]+/[^/\s\"'\?]+)`)
	reQuayDirect    = regexp.MustCompile(`quay\.io/([^/\s\"'\?]+/[^/\s\"'\?]+)`)
	reLscr          = regexp.MustCompile(`lscr\.io/([^/\s\"'\?]+/[^/\s\"'\?]+)`)
	reLscrGH        = regexp.MustCompile(`github\.com/linuxserver/docker-([^/\s\"'\?]+)`)
	reHomeOps       = regexp.MustCompile(`github\.com/home-operations/containers/tree/main/apps/([^/\s\"'\?]+)`)
	reTrueNASCont1  = regexp.MustCompile(`github\.com/truenas/containers/pkgs/container/containers%2F([^/\s\"'\?]+)`)
	reTrueNASCont2  = regexp.MustCompile(`github\.com/truenas/containers/tree/master/apps/([^/\s\"'\?]+)`)
	reGHPkgs        = regexp.MustCompile(`github\.com/([^/\s\"'\?]+)/([^/\s\"'\?]+)/pkgs/container/([^/\s\"'\?]+)`)
	reRepo          = regexp.MustCompile(`(?m)^\s*repository:\s*["']?([^"'\s#]+)["']?`)
	reTag           = regexp.MustCompile(`(?m)^\s*tag:\s*["']?([^"'\s#@]+)`)
)

func resolveContainerImage(appID string, sources []string, home string) string {
	// 1. Curated images
	switch appID {
	case "nginx-proxy-manager":
		return "jc21/nginx-proxy-manager:latest"
	case "syncthing":
		return "ghcr.io/syncthing/syncthing:latest"
	case "wg-easy":
		return "ghcr.io/wg-easy/wg-easy:latest"
	case "ollama":
		return "ollama/ollama:latest"
	case "uptime-kuma":
		return "louislam/uptime-kuma:latest"
	case "vaultwarden":
		return "vaultwarden/server:latest"
	case "pihole":
		return "pihole/pihole:latest"
	case "plex":
		return "lscr.io/linuxserver/plex:latest"
	case "home-assistant":
		return "ghcr.io/home-assistant/home-assistant:stable"
	case "adguard-home":
		return "adguard/adguardhome:latest"
	case "actual-budget":
		return "ghcr.io/actualbudget/actual-server:latest"
	case "qbittorrent":
		return "lscr.io/linuxserver/qbittorrent:latest"
	case "transmission":
		return "lscr.io/linuxserver/transmission:latest"
	case "sabnzbd":
		return "lscr.io/linuxserver/sabnzbd:latest"
	case "radarr":
		return "lscr.io/linuxserver/radarr:latest"
	case "sonarr":
		return "lscr.io/linuxserver/sonarr:latest"
	case "lidarr":
		return "lscr.io/linuxserver/lidarr:latest"
	case "readarr":
		return "lscr.io/linuxserver/readarr:latest"
	case "prowlarr":
		return "lscr.io/linuxserver/prowlarr:latest"
	case "bazarr":
		return "lscr.io/linuxserver/bazarr:latest"
	case "audiobookshelf":
		return "ghcr.io/advplyr/audiobookshelf:latest"
	case "navidrome":
		return "deluan/navidrome:latest"
	case "paperless-ngx":
		return "ghcr.io/paperless-ngx/paperless-ngx:latest"
	case "dockge":
		return "louislam/dockge:latest"
	case "portainer":
		return "portainer/portainer-ce:latest"
	case "tailscale":
		return "tailscale/tailscale:latest"
	case "photoprism":
		return "photoprism/photoprism:latest"
	case "immich":
		return "ghcr.io/immich-app/immich-server:release"
	case "calibre-web":
		return "lscr.io/linuxserver/calibre-web:latest"
	case "mealie":
		return "ghcr.io/mealie-recipes/mealie:latest"
	case "duplicati":
		return "duplicati/duplicati:latest"
	case "kavita":
		return "kareadita/kavita:latest"
	case "komga":
		return "gotson/komga:latest"
	case "flaresolverr":
		return "flaresolverr/flaresolverr:latest"
	case "grafana":
		return "grafana/grafana:latest"
	case "prometheus":
		return "prom/prometheus:latest"
	case "netdata":
		return "netdata/netdata:latest"
	case "redis":
		return "redis:alpine"
	case "postgres":
		return "postgres:16-alpine"
	case "mariadb":
		return "mariadb:11"
	}

	// 2. Extract from Sources or Home
	if img := extractImageFromSources(appID, sources, home); img != "" {
		return img
	}

	// 3. Fallback
	return fmt.Sprintf("%s:latest", appID)
}

func extractImageFromSources(appID string, sources []string, home string) string {
	candidates := make([]string, 0, len(sources)+1)
	candidates = append(candidates, sources...)
	if home != "" {
		candidates = append(candidates, home)
	}

	for _, s := range candidates {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if m := reDockerHubRepo.FindStringSubmatch(s); len(m) > 1 {
			return strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reDockerHubLib.FindStringSubmatch(s); len(m) > 1 {
			return strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reGHCR.FindStringSubmatch(s); len(m) > 1 {
			return "ghcr.io/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reQuayRepo.FindStringSubmatch(s); len(m) > 1 {
			return "quay.io/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reQuayDirect.FindStringSubmatch(s); len(m) > 1 {
			return "quay.io/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reLscr.FindStringSubmatch(s); len(m) > 1 {
			return "lscr.io/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reLscrGH.FindStringSubmatch(s); len(m) > 1 {
			return "lscr.io/linuxserver/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reHomeOps.FindStringSubmatch(s); len(m) > 1 {
			return "ghcr.io/home-operations/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reTrueNASCont1.FindStringSubmatch(s); len(m) > 1 {
			return "ghcr.io/truenas/containers/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reTrueNASCont2.FindStringSubmatch(s); len(m) > 1 {
			return "ghcr.io/truenas/containers/" + strings.TrimRight(m[1], "/") + ":latest"
		}
		if m := reGHPkgs.FindStringSubmatch(s); len(m) > 3 {
			return fmt.Sprintf("ghcr.io/%s/%s:latest", m[1], strings.TrimRight(m[3], "/"))
		}
	}
	return ""
}

var (
	reTplPortals   = regexp.MustCompile(`tpl\.portals\.add\(([^,\)]+)(?:,\s*(\{[^\}]+\}))?\)`)
	reAddPort      = regexp.MustCompile(`add_port\(\s*([^,\)]+)(?:,\s*\{[^}]*"container_port":\s*([^\s,}]+)[^}]*\})?`)
	reInternalPort = regexp.MustCompile(`(?i)(?:internal_web_port|web_port|port):\s*(\d+)`)
)

type ExtractedPortal struct {
	Name    string
	Port    int
	Scheme  string
	Path    string
	PortVar string
}

func ExtractPortalsFromTemplate(tplContent, ixValuesContent string, qData []byte) []ExtractedPortal {
	var portals []ExtractedPortal

	// 1. Check tpl.portals.add calls in template
	for _, m := range reTplPortals.FindAllStringSubmatch(tplContent, -1) {
		if len(m) < 2 {
			continue
		}
		portVar := strings.TrimSpace(m[1])
		name := "Web UI"
		scheme := "http"
		path := "/"
		port := 0

		if len(m) > 2 && m[2] != "" {
			opts := m[2]
			if nm := regexp.MustCompile(`"name":\s*"([^"]+)"`).FindStringSubmatch(opts); len(nm) > 1 {
				name = nm[1]
			}
			if pm := regexp.MustCompile(`"path":\s*"([^"]+)"`).FindStringSubmatch(opts); len(pm) > 1 {
				path = pm[1]
			}
			if sm := regexp.MustCompile(`"scheme":\s*"([^"]+)"`).FindStringSubmatch(opts); len(sm) > 1 {
				scheme = sm[1]
			}
			if pt := regexp.MustCompile(`"port":\s*(\d+)`).FindStringSubmatch(opts); len(pt) > 1 {
				port, _ = strconv.Atoi(pt[1])
			}
		}

		if port == 0 {
			if p, err := strconv.Atoi(portVar); err == nil && p > 0 {
				port = p
			}
		}

		if port == 0 {
			// Look for matching add_port call
			for _, am := range reAddPort.FindAllStringSubmatch(tplContent, -1) {
				if len(am) > 1 && strings.TrimSpace(am[1]) == portVar {
					if len(am) > 2 && am[2] != "" {
						target := strings.TrimSpace(am[2])
						if p, err := strconv.Atoi(target); err == nil && p > 0 {
							port = p
							break
						}
						if strings.Contains(target, "values.consts.") {
							constName := strings.TrimPrefix(target, "values.consts.")
							reNamed := regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%s:\s*(\d+)`, regexp.QuoteMeta(constName)))
							if nm := reNamed.FindStringSubmatch(ixValuesContent); len(nm) > 1 {
								if p, err := strconv.Atoi(nm[1]); err == nil && p > 0 {
									port = p
									break
								}
							}
						}
					}
				}
			}
		}

		if port == 0 && len(qData) > 0 {
			parts := strings.Split(portVar, ".")
			leaf := parts[len(parts)-1]
			if leaf == "port_number" && len(parts) >= 2 {
				leaf = parts[len(parts)-2]
			}
			reQ := regexp.MustCompile(fmt.Sprintf(`(?s)variable:\s*"?%s"?[^\n]*\n.*?default:\s*(\d+)`, regexp.QuoteMeta(leaf)))
			if qm := reQ.FindSubmatch(qData); len(qm) > 1 {
				if p, err := strconv.Atoi(string(qm[1])); err == nil && p > 0 {
					port = p
				}
			}
		}

		if port == 0 {
			if m := reInternalPort.FindStringSubmatch(ixValuesContent); len(m) > 1 {
				if p, err := strconv.Atoi(m[1]); err == nil && p > 0 {
					port = p
				}
			}
		}

		portals = append(portals, ExtractedPortal{
			Name:    name,
			Port:    port,
			Scheme:  scheme,
			Path:    path,
			PortVar: portVar,
		})
	}

	// 2. Check if template or compose content contains explicit x-portals:
	if len(portals) == 0 {
		reXPortals := regexp.MustCompile(`(?s)x-portals:\s*\n((?:\s*-\s*[^\n]+\n(?:(?:\s{4,}[^\n]+\n)*)?)+)`)
		if xm := reXPortals.FindStringSubmatch(tplContent); len(xm) > 1 {
			var parsed []map[string]any
			if err := yaml.Unmarshal([]byte(xm[1]), &parsed); err == nil {
				for _, item := range parsed {
					pName := "Web UI"
					if n, ok := item["name"].(string); ok && n != "" {
						pName = n
					}
					pScheme := "http"
					if s, ok := item["scheme"].(string); ok && s != "" {
						pScheme = s
					}
					pPath := "/"
					if pt, ok := item["path"].(string); ok && pt != "" {
						pPath = pt
					}
					pPort := 0
					switch pv := item["port"].(type) {
					case int:
						pPort = pv
					case float64:
						pPort = int(pv)
					case string:
						pPort, _ = strconv.Atoi(strings.TrimSpace(pv))
					}
					if pPort > 0 {
						portals = append(portals, ExtractedPortal{
							Name:   pName,
							Port:   pPort,
							Scheme: pScheme,
							Path:   pPath,
						})
					}
				}
			}
		}
	}

	// 3. Fallback: check questions for a web_port / webui_port question
	if len(portals) == 0 && len(qData) > 0 {
		reWebQ := regexp.MustCompile(`(?s)variable:\s*"?([^\s"]*(?:web_port|webui_port|portal_port)[^\s"]*)"?[^\n]*\n.*?default:\s*(\d+)`)
		if wqm := reWebQ.FindSubmatch(qData); len(wqm) > 2 {
			if p, err := strconv.Atoi(string(wqm[2])); err == nil && p > 0 {
				portals = append(portals, ExtractedPortal{
					Name:   "Web UI",
					Port:   p,
					Scheme: "http",
					Path:   "/",
				})
			}
		}
	}

	// 4. Fallback: check ixValuesContent
	if len(portals) == 0 {
		if m := reInternalPort.FindStringSubmatch(ixValuesContent); len(m) > 1 {
			if p, err := strconv.Atoi(m[1]); err == nil && p > 0 {
				portals = append(portals, ExtractedPortal{
					Name:   "Web UI",
					Port:   p,
					Scheme: "http",
					Path:   "/",
				})
			}
		}
	}

	return portals
}

func ExtractPortalFromTemplate(tplContent, ixValuesContent string) (port int, name, scheme, path string) {
	portals := ExtractPortalsFromTemplate(tplContent, ixValuesContent, nil)
	if len(portals) > 0 {
		return portals[0].Port, portals[0].Name, portals[0].Scheme, portals[0].Path
	}
	return 0, "Web UI", "http", "/"
}

func (a *Adapter) enrichFromIXValues(ctx context.Context, app *domain.CatalogApp) {
	if app.Version == "" || app.Train == "" {
		return
	}

	valuesCacheDir := filepath.Join(a.cacheDir, "values", app.Train)
	_ = os.MkdirAll(valuesCacheDir, 0755)
	cacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("%s-%s.yaml", app.AppID, app.Version))

	var data []byte
	if fileBytes, err := os.ReadFile(cacheFile); err == nil && len(fileBytes) > 0 {
		data = fileBytes
	} else {
		url := fmt.Sprintf("https://raw.githubusercontent.com/truenas/apps/master/trains/%s/%s/%s/ix_values.yaml",
			app.Train, app.AppID, app.Version)
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			return
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		_ = os.WriteFile(cacheFile, data, 0644)
	}

	repo, tag := extractIXImageAndTag(string(data))
	if repo != "" {
		if tag != "" {
			app.ContainerImage = fmt.Sprintf("%s:%s", repo, tag)
		} else {
			app.ContainerImage = fmt.Sprintf("%s:latest", repo)
		}
		// Regenerate compose if using fallback template
		app.ComposeYAML, app.DefaultPort = generateResolvedCompose(*app)
	}

	// Fetch docker-compose.yaml template if available to extract portals
	tplCacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("%s-%s-compose.yaml", app.AppID, app.Version))
	var tplData []byte
	if fileBytes, err := os.ReadFile(tplCacheFile); err == nil && len(fileBytes) > 0 {
		tplData = fileBytes
	} else {
		tplURL := fmt.Sprintf("https://raw.githubusercontent.com/truenas/apps/master/trains/%s/%s/%s/templates/docker-compose.yaml",
			app.Train, app.AppID, app.Version)
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, tplURL, nil); err == nil {
			if resp, err := a.httpClient.Do(req); err == nil {
				if resp.StatusCode == http.StatusOK {
					if bodyBytes, err := io.ReadAll(resp.Body); err == nil {
						tplData = bodyBytes
						_ = os.WriteFile(tplCacheFile, bodyBytes, 0644)
					}
				}
				resp.Body.Close()
			}
		}
	}

	// Fetch questions.yaml if available to extract dynamic configuration questions
	qCacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("%s-%s-questions.yaml", app.AppID, app.Version))
	var qData []byte
	if fileBytes, err := os.ReadFile(qCacheFile); err == nil && len(fileBytes) > 0 {
		qData = fileBytes
	} else {
		qURL := fmt.Sprintf("https://raw.githubusercontent.com/truenas/apps/master/trains/%s/%s/%s/questions.yaml",
			app.Train, app.AppID, app.Version)
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, qURL, nil); err == nil {
			if resp, err := a.httpClient.Do(req); err == nil {
				if resp.StatusCode == http.StatusOK {
					if bodyBytes, err := io.ReadAll(resp.Body); err == nil {
						qData = bodyBytes
						_ = os.WriteFile(qCacheFile, bodyBytes, 0644)
					}
				}
				resp.Body.Close()
			}
		}
	}

	if len(qData) > 0 {
		groups, flatQs := ParseQuestionsYAMLWithTemplate(qData, app.AppID, a.getAppsPool(), a.getAppdataPath(), string(tplData), string(data))
		app.QuestionGroups = groups
		app.Questions = flatQs
	} else {
		groups, flatQs := GetCuratedQuestions(*app, a.getAppsPool(), a.getAppdataPath())
		app.QuestionGroups = groups
		app.Questions = flatQs
	}

	portals := ExtractPortalsFromTemplate(string(tplData), string(data), qData)
	if len(portals) > 0 {
		if (app.DefaultPort <= 0 || app.DefaultPort == 8080) && portals[0].Port > 0 {
			app.DefaultPort = portals[0].Port
		}
		var sb strings.Builder
		if app.DefaultPort > 0 {
			sb.WriteString(fmt.Sprintf("x-nas-management-port: %d\n", app.DefaultPort))
		}
		sb.WriteString("x-portals:\n")
		hasValid := false
		for _, p := range portals {
			portNum := p.Port
			if portNum <= 0 && app.DefaultPort > 0 {
				portNum = app.DefaultPort
			}
			if portNum > 0 {
				hasValid = true
				sb.WriteString(fmt.Sprintf("  - name: %q\n    scheme: %q\n    port: %d\n    path: %q\n",
					p.Name, p.Scheme, portNum, p.Path))
			}
		}
		if hasValid && !strings.Contains(app.ComposeYAML, "x-portals:") {
			app.ComposeYAML = sb.String() + app.ComposeYAML
		}
	}
}

func extractIXImageAndTag(yamlContent string) (string, string) {
	var repo, tag string
	if m := reRepo.FindStringSubmatch(yamlContent); len(m) > 1 {
		repo = strings.TrimSpace(m[1])
	}
	if m := reTag.FindStringSubmatch(yamlContent); len(m) > 1 {
		tag = strings.TrimSpace(m[1])
		if idx := strings.Index(tag, "@"); idx != -1 {
			tag = tag[:idx]
		}
	}
	return repo, tag
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
	case "esphome":
		image = "ghcr.io/esphome/esphome:latest"
		if app.ContainerImage != "" {
			image = app.ContainerImage
		}
		port = 6052
		if app.DefaultPort > 0 {
			port = app.DefaultPort
		}
		return fmt.Sprintf(`services:
  esphome:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:6052"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /mnt/tank/apps/%s/config:/config
`, image, appID, port, appID), port

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
      - /mnt/tank/apps/%s/config:/var/syncthing
      - /mnt/tank/media:/var/syncthing/Sync
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
      - /mnt/tank/apps/%s:/etc/wireguard
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
      - /mnt/tank/apps/%s:/root/.ollama
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
      - /mnt/tank/apps/%s:/app/data
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
      - /mnt/tank/apps/%s:/data
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
      - /mnt/tank/apps/%s/etc-pihole:/etc/pihole
      - /mnt/tank/apps/%s/etc-dnsmasq.d:/etc/dnsmasq.d
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
      - /mnt/tank/apps/%s:/config
      - /mnt/tank/media:/media
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
      - /mnt/tank/apps/%s:/config
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
      - /mnt/tank/apps/%s/work:/opt/adguardhome/work
      - /mnt/tank/apps/%s/conf:/opt/adguardhome/conf
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
      - /mnt/tank/apps/%s:/data
`, image, appID, port, appID), port

	case "nginx-proxy-manager":
		image = "jc21/nginx-proxy-manager:latest"
		port = 81
		return fmt.Sprintf(`x-nas-management-port: 81
x-portals:
  - name: "Admin Web UI"
    scheme: "http"
    port: 81
    path: "/"
  - name: "HTTP Traffic"
    scheme: "http"
    port: 80
    path: "/"
  - name: "HTTPS Traffic"
    scheme: "https"
    port: 443
    path: "/"
services:
  nginx-proxy-manager:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "80:80"
      - "%d:81"
      - "443:443"
    environment:
      - DISABLE_IPV6=true
    volumes:
      - /mnt/tank/apps/%s/data:/data
      - /mnt/tank/apps/%s/letsencrypt:/etc/letsencrypt
`, image, appID, port, appID, appID), port

	case "qbittorrent":
		image = "lscr.io/linuxserver/qbittorrent:latest"
		port = 8080
		return fmt.Sprintf(`services:
  qbittorrent:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:8080"
      - "6881:6881/tcp"
      - "6881:6881/udp"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
      - WEBUI_PORT=8080
    volumes:
      - /mnt/tank/apps/%s/config:/config
      - /mnt/tank/media/downloads:/downloads
`, image, appID, port, appID), port

	case "elastic-search", "elasticsearch":
		image = "docker.elastic.co/elasticsearch/elasticsearch:9.5.5"
		port = 9200
		return fmt.Sprintf(`services:
  elastic-search:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:9200"
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m"
    volumes:
      - /mnt/tank/apps/%s/data:/usr/share/elasticsearch/data
`, image, appID, port, appID), port

	case "transmission":
		image = "lscr.io/linuxserver/transmission:latest"
		port = 9091
		return fmt.Sprintf(`services:
  transmission:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:9091"
      - "51413:51413/tcp"
      - "51413:51413/udp"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /mnt/tank/apps/%s/config:/config
      - /mnt/tank/media/downloads:/downloads
`, image, appID, port, appID), port

	case "radarr":
		image = "lscr.io/linuxserver/radarr:latest"
		port = 7878
		return fmt.Sprintf(`services:
  radarr:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:7878"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /mnt/tank/apps/%s/config:/config
      - /mnt/tank/media:/media
`, image, appID, port, appID), port

	case "sonarr":
		image = "lscr.io/linuxserver/sonarr:latest"
		port = 8989
		return fmt.Sprintf(`services:
  sonarr:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:8989"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=Etc/UTC
    volumes:
      - /mnt/tank/apps/%s/config:/config
      - /mnt/tank/media:/media
`, image, appID, port, appID), port

	case "dockge":
		image = "louislam/dockge:latest"
		port = 5001
		return fmt.Sprintf(`services:
  dockge:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:5001"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/tank/apps/%s/data:/app/data
      - /mnt/tank/apps/stacks:/opt/stacks
`, image, appID, port, appID), port

	case "portainer":
		image = "portainer/portainer-ce:latest"
		port = 9443
		return fmt.Sprintf(`x-nas-management-port: 9443
x-portals:
  - name: "Web UI (HTTPS)"
    scheme: "https"
    port: 9443
    path: "/"
  - name: "Web UI (HTTP)"
    scheme: "http"
    port: 9000
    path: "/"
services:
  portainer:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:9443"
      - "9000:9000"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/tank/apps/%s/data:/data
`, image, appID, port, appID), port

	case "audiobookshelf":
		image = "ghcr.io/advplyr/audiobookshelf:latest"
		port = 13378
		return fmt.Sprintf(`services:
  audiobookshelf:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:80"
    environment:
      - AUDIOBOOKSHELF_UID=1000
      - AUDIOBOOKSHELF_GID=1000
    volumes:
      - /mnt/tank/apps/%s/config:/config
      - /mnt/tank/apps/%s/metadata:/metadata
      - /mnt/tank/media/audiobooks:/audiobooks
`, image, appID, port, appID, appID), port

	case "paperless-ngx":
		image = "ghcr.io/paperless-ngx/paperless-ngx:latest"
		port = 8000
		return fmt.Sprintf(`services:
  paperless-ngx:
    image: %s
    container_name: %s
    restart: unless-stopped
    ports:
      - "%d:8000"
    environment:
      - USERMAP_UID=1000
      - USERMAP_GID=1000
    volumes:
      - /mnt/tank/apps/%s/data:/usr/src/paperless/data
      - /mnt/tank/apps/%s/media:/usr/src/paperless/media
      - /mnt/tank/apps/%s/consume:/usr/src/paperless/consume
`, image, appID, port, appID, appID, appID), port
	}

	// Default template generator for any other TrueNAS app
	if app.ContainerImage != "" {
		image = app.ContainerImage
	} else {
		image = resolveContainerImage(appID, app.Sources, app.Home)
	}
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
      - /mnt/tank/apps/%s:/data
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
      - /mnt/tank/apps/nextcloud:/var/www/html
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
      - /mnt/tank/apps/nextcloud-db:/var/lib/postgresql/data
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
      - /mnt/tank/apps/jellyfin/config:/config
      - /mnt/tank/apps/jellyfin/cache:/cache
      - /mnt/tank/media:/media:ro
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
			AppID:          "nginx-proxy-manager",
			Title:          "Nginx Proxy Manager",
			Description:    "Expose your web services easily and securely with SSL certificates, access lists, and proxy hosts.",
			Category:       "networking",
			Categories:     []string{"networking", "security"},
			IconURL:        "https://media.sys.truenas.net/apps/nginx-proxy-manager/icons/icon.svg",
			Home:           "https://nginxproxymanager.com/",
			Train:          "community",
			Version:        "1.3.14",
			AppVersion:     "2.16.0",
			DefaultPort:    81,
			Sources:        []string{"https://apps.truenas.com/catalog/nginx-proxy-manager_community/", "https://hub.docker.com/r/jc21/nginx-proxy-manager", "https://nginxproxymanager.com/"},
			ContainerImage: "jc21/nginx-proxy-manager:latest",
			ComposeYAML: `services:
  nginx-proxy-manager:
    image: jc21/nginx-proxy-manager:latest
    container_name: nginx-proxy-manager
    restart: unless-stopped
    ports:
      - "80:80"
      - "81:81"
      - "443:443"
    environment:
      - DISABLE_IPV6=true
    volumes:
      - /mnt/tank/apps/nginx-proxy-manager/data:/data
      - /mnt/tank/apps/nginx-proxy-manager/letsencrypt:/etc/letsencrypt
`,
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
		{
			AppID:       "uptime-kuma",
			Title:       "Uptime Kuma (CasaOS)",
			Description: "A self-hosted monitoring tool like 'Uptime Robot'.",
			Category:    "Utilities",
			Categories:  []string{"Utilities", "Network"},
			IconURL:     "https://raw.githubusercontent.com/IceWhaleTech/CasaOS-AppStore/main/Apps/UptimeKuma/icon.png",
			Train:       "casaos",
			Version:     "1.23.16",
			DefaultPort: 3001,
		},
		{
			AppID:       "bento-pdf",
			Title:       "BentoPDF",
			Description: "Your all-in-one privacy-first PDF toolkit. View, edit, and organize PDFs locally.",
			Category:    "Utilities",
			Categories:  []string{"Utilities", "Productivity"},
			IconURL:     "https://raw.githubusercontent.com/IceWhaleTech/CasaOS-AppStore/main/Apps/BentoPDF/icon.png",
			Train:       "casaos",
			Version:     "1.0.0",
			DefaultPort: 8080,
		},
		{
			AppID:       "grafana",
			Title:       "Grafana",
			Description: "The open and composable observability and data visualization platform.",
			Category:    "Monitoring",
			Categories:  []string{"Monitoring", "Analytics"},
			IconURL:     "https://raw.githubusercontent.com/Dokploy/templates/main/blueprints/grafana/logo.png",
			Train:       "dokploy",
			Version:     "11.0.0",
			DefaultPort: 3000,
		},
		{
			AppID:       "ackee",
			Title:       "Ackee",
			Description: "Self-hosted, Node.js based analytics tool for those who care about privacy.",
			Category:    "Analytics",
			Categories:  []string{"Analytics", "Privacy"},
			IconURL:     "https://raw.githubusercontent.com/Dokploy/templates/main/blueprints/ackee/logo.png",
			Train:       "dokploy",
			Version:     "3.4.2",
			DefaultPort: 3000,
		},
		{
			AppID:       "n8n",
			Title:       "n8n",
			Description: "Free and source-available workflow automation tool. Easily automate tasks across different services.",
			Category:    "Automation",
			Categories:  []string{"Automation", "Integration"},
			IconURL:     "https://raw.githubusercontent.com/Dokploy/templates/main/blueprints/n8n/logo.png",
			Train:       "dokploy",
			Version:     "1.48.0",
			DefaultPort: 5678,
		},
	}

	for _, app := range defaults {
		key := fmt.Sprintf("%s/%s", app.Train, app.AppID)
		a.apps[key] = app
	}
}
