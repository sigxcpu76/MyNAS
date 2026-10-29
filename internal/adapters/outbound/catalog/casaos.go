package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

var (
	defaultCasaOSIndexURL = "https://raw.githubusercontent.com/IceWhaleTech/CasaOS-AppStore/gh-pages/index.json"
	defaultCasaOSBaseURL  = "https://raw.githubusercontent.com/IceWhaleTech/CasaOS-AppStore/gh-pages"
)

// SetCasaOSBaseURLForTest overrides the CasaOS raw base URL for unit tests.
func SetCasaOSBaseURLForTest(url string) {
	defaultCasaOSBaseURL = url
}


type casaosIndex struct {
	Version   int              `json:"version"`
	UpdatedAt string           `json:"updated_at"`
	AppCount  int              `json:"app_count"`
	BaseURL   string           `json:"base_url"`
	Apps      []casaosAppEntry `json:"apps"`
}

type casaosAppEntry struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Tagline       string   `json:"tagline"`
	Category      string   `json:"category"`
	Categories    []string `json:"categories"`
	Author        string   `json:"author"`
	Developer     string   `json:"developer"`
	Architectures []string `json:"architectures"`
	Icon          string   `json:"icon"`
	Thumbnail     string   `json:"thumbnail"`
	ComposeURL    string   `json:"compose_url"`
	MetaURL       string   `json:"meta_url"`
	Version       string   `json:"version"`
}

type casaosMetaFile struct {
	Title          string   `json:"title"`
	Tagline        string   `json:"tagline"`
	Description    string   `json:"description"`
	Category       string   `json:"category"`
	Categories     []string `json:"categories"`
	Author         string   `json:"author"`
	Developer      string   `json:"developer"`
	Icon           string   `json:"icon"`
	Thumbnail      string   `json:"thumbnail"`
	ScreenshotLink []string `json:"screenshot_link"`
	Website        string   `json:"website"`
	Repo           string   `json:"repo"`
	Support        string   `json:"support"`
	Docs           string   `json:"docs"`
	Version        string   `json:"version"`
}

func getCasaOSI18n(val any) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return strings.TrimSpace(s)
	}
	if m, ok := val.(map[string]any); ok {
		for _, k := range []string{"en_US", "en_GB", "en", "default"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		for _, v := range m {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// fetchCasaOSIndex retrieves the CasaOS AppStore index.
func fetchCasaOSIndex(ctx context.Context, client *http.Client, url string) (*casaosIndex, []byte, error) {
	if url == "" {
		url = defaultCasaOSIndexURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("casaos catalog returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var idx casaosIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, nil, fmt.Errorf("failed to parse casaos index: %w", err)
	}
	return &idx, body, nil
}

// enrichFromCasaOS fetches the Compose YAML and metadata for a CasaOS application
// and converts them to our standardized catalog model.
func enrichFromCasaOS(ctx context.Context, client *http.Client, app *domain.CatalogApp, cacheDir, appsPool, appdataPath string) {
	if app.Train != "casaos" {
		return
	}

	valuesCacheDir := filepath.Join(cacheDir, "values")
	_ = os.MkdirAll(valuesCacheDir, 0755)

	composeCacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("casaos-%s-compose.yaml", app.AppID))
	var composeData []byte

	if cached, err := os.ReadFile(composeCacheFile); err == nil && len(cached) > 0 {
		composeData = cached
	} else {
		composeURL := ""
		if len(app.Sources) > 0 && strings.HasPrefix(app.Sources[0], "http") {
			composeURL = app.Sources[0]
		} else {
			composeURL = fmt.Sprintf("%s/apps/%s/docker-compose.yml", defaultCasaOSBaseURL, app.AppID)
		}

		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, composeURL, nil); err == nil {
			if resp, err := client.Do(req); err == nil {
				if resp.StatusCode == http.StatusOK {
					if b, err := io.ReadAll(resp.Body); err == nil {
						composeData = b
						_ = os.WriteFile(composeCacheFile, b, 0644)
					}
				}
				resp.Body.Close()
			}
		}
	}

	if len(composeData) == 0 {
		return
	}

	normalizedCompose, defaultPort, portals, containerImage := normalizeCasaOSCompose(string(composeData), app.AppID, appsPool, appdataPath)
	if defaultPort > 0 {
		app.DefaultPort = defaultPort
	}
	if containerImage != "" {
		app.ContainerImage = containerImage
	}
	app.ComposeYAML = normalizedCompose

	// Extract questions & groups from CasaOS Compose specification
	groups, questions := extractCasaOSQuestions(normalizedCompose, *app, defaultPort, portals, appsPool, appdataPath)
	if len(groups) > 0 {
		app.QuestionGroups = groups
		app.Questions = questions
	}
}

// normalizeCasaOSCompose converts CasaOS volume paths and x-casaos portals into NAS standard format.
func normalizeCasaOSCompose(rawCompose, appID, pool, appdataPath string) (string, int, []ExtractedPortal, string) {
	if strings.TrimSpace(rawCompose) == "" {
		return rawCompose, 0, nil, ""
	}

	if pool == "" {
		pool = "tank"
	}
	pool = strings.TrimPrefix(pool, "zfsPools/")
	cleanID := appID
	if parts := strings.Split(cleanID, "."); len(parts) > 1 {
		cleanID = parts[len(parts)-1]
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(rawCompose), &root); err != nil {
		return rawCompose, 0, nil, ""
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return rawCompose, 0, nil, ""
	}

	docMap := root.Content[0]
	defaultPort := 0
	scheme := "http"
	webPath := "/"
	var portals []ExtractedPortal
	primaryImage := ""

	// 1. Inspect root x-casaos metadata
	for i := 0; i < len(docMap.Content); i += 2 {
		if docMap.Content[i].Value == "x-casaos" && docMap.Content[i+1].Kind == yaml.MappingNode {
			metaMap := docMap.Content[i+1]
			for k := 0; k < len(metaMap.Content); k += 2 {
				kName := metaMap.Content[k].Value
				valNode := metaMap.Content[k+1]
				switch kName {
				case "port_map":
					if p, err := strconv.Atoi(strings.TrimSpace(valNode.Value)); err == nil && p > 0 {
						defaultPort = p
					}
				case "scheme":
					if valNode.Value != "" {
						scheme = strings.ToLower(strings.TrimSpace(valNode.Value))
					}
				case "index":
					if valNode.Value != "" {
						webPath = strings.TrimSpace(valNode.Value)
					}
				}
			}
		}
	}

	// 2. Normalize service volumes and environment variables
	for i := 0; i < len(docMap.Content); i += 2 {
		if docMap.Content[i].Value != "services" || docMap.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		servicesNode := docMap.Content[i+1]
		for j := 1; j < len(servicesNode.Content); j += 2 {
			svcMap := servicesNode.Content[j]
			if svcMap.Kind != yaml.MappingNode {
				continue
			}

			// Extract primary image if not set yet
			if primaryImage == "" {
				for k := 0; k < len(svcMap.Content); k += 2 {
					if svcMap.Content[k].Value == "image" && svcMap.Content[k+1].Kind == yaml.ScalarNode {
						primaryImage = strings.TrimSpace(svcMap.Content[k+1].Value)
						break
					}
				}
			}

			// Look for service-level x-casaos port if not set
			if defaultPort == 0 {
				for k := 0; k < len(svcMap.Content); k += 2 {
					if svcMap.Content[k].Value == "ports" && svcMap.Content[k+1].Kind == yaml.SequenceNode {
						portsSeq := svcMap.Content[k+1]
						if len(portsSeq.Content) > 0 {
							firstP := portsSeq.Content[0]
							if firstP.Kind == yaml.ScalarNode {
								parts := strings.Split(firstP.Value, ":")
								if p, err := strconv.Atoi(parts[0]); err == nil && p > 0 {
									defaultPort = p
								}
							}
						}
					}
				}
			}

			// Clean and rewrite volumes
			for k := 0; k < len(svcMap.Content); k += 2 {
				if svcMap.Content[k].Value != "volumes" {
					continue
				}
				volSeq := svcMap.Content[k+1]
				if volSeq.Kind != yaml.SequenceNode {
					continue
				}

				var newVolContent []*yaml.Node
				for _, vNode := range volSeq.Content {
					if vNode.Kind == yaml.ScalarNode {
						val := vNode.Value
						val = strings.ReplaceAll(val, "$AppID", cleanID)
						val = strings.ReplaceAll(val, "/DATA/AppData", fmt.Sprintf("/mnt/%s/apps", pool))
						val = strings.ReplaceAll(val, "/DATA/Media", fmt.Sprintf("/mnt/%s/media", pool))
						val = strings.ReplaceAll(val, "/DATA/Downloads", fmt.Sprintf("/mnt/%s/media/downloads", pool))
						val = strings.ReplaceAll(val, "/DATA/Gallery", fmt.Sprintf("/mnt/%s/media/pictures", pool))
						val = strings.ReplaceAll(val, "/DATA/Documents", fmt.Sprintf("/mnt/%s/media/documents", pool))
						val = strings.ReplaceAll(val, "/DATA/", fmt.Sprintf("/mnt/%s/data/", pool))
						vNode.Value = val
						newVolContent = append(newVolContent, vNode)
					} else if vNode.Kind == yaml.MappingNode {
						// Bind syntax: {type: bind, source: ..., target: ...}
						src := ""
						tgt := ""
						mode := ""
						for m := 0; m < len(vNode.Content); m += 2 {
							mk := vNode.Content[m].Value
							mv := vNode.Content[m+1].Value
							switch mk {
							case "source":
								src = mv
							case "target":
								tgt = mv
							case "mode", "read_only":
								if mv == "true" || mv == "ro" {
									mode = "ro"
								}
							}
						}
						if src != "" && tgt != "" {
							src = strings.ReplaceAll(src, "$AppID", cleanID)
							src = strings.ReplaceAll(src, "/DATA/AppData", fmt.Sprintf("/mnt/%s/apps", pool))
							src = strings.ReplaceAll(src, "/DATA/Media", fmt.Sprintf("/mnt/%s/media", pool))
							src = strings.ReplaceAll(src, "/DATA/Downloads", fmt.Sprintf("/mnt/%s/media/downloads", pool))
							src = strings.ReplaceAll(src, "/DATA/Gallery", fmt.Sprintf("/mnt/%s/media/pictures", pool))
							src = strings.ReplaceAll(src, "/DATA/Documents", fmt.Sprintf("/mnt/%s/media/documents", pool))
							src = strings.ReplaceAll(src, "/DATA/", fmt.Sprintf("/mnt/%s/data/", pool))

							line := fmt.Sprintf("%s:%s", src, tgt)
							if mode != "" {
								line = fmt.Sprintf("%s:%s:%s", src, tgt, mode)
							}
							newVolContent = append(newVolContent, &yaml.Node{
								Kind:  yaml.ScalarNode,
								Value: line,
							})
						}
					}
				}
				volSeq.Content = newVolContent
			}

			// Clean environment variables ($PUID, $PGID, $TZ)
			for k := 0; k < len(svcMap.Content); k += 2 {
				if svcMap.Content[k].Value != "environment" {
					continue
				}
				envNode := svcMap.Content[k+1]
				if envNode.Kind == yaml.SequenceNode {
					for _, eNode := range envNode.Content {
						if eNode.Kind == yaml.ScalarNode {
							eNode.Value = strings.ReplaceAll(eNode.Value, "$PUID", "1000")
							eNode.Value = strings.ReplaceAll(eNode.Value, "$PGID", "1000")
							eNode.Value = strings.ReplaceAll(eNode.Value, "$TZ", "Etc/UTC")
							eNode.Value = strings.ReplaceAll(eNode.Value, "$AppID", cleanID)
						}
					}
				} else if envNode.Kind == yaml.MappingNode {
					for ek := 1; ek < len(envNode.Content); ek += 2 {
						v := envNode.Content[ek].Value
						v = strings.ReplaceAll(v, "$PUID", "1000")
						v = strings.ReplaceAll(v, "$PGID", "1000")
						v = strings.ReplaceAll(v, "$TZ", "Etc/UTC")
						v = strings.ReplaceAll(v, "$AppID", cleanID)
						envNode.Content[ek].Value = v
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(&root)
	_ = enc.Close()

	resYAML := strings.TrimPrefix(buf.String(), "---\n")

	// Inject x-portals header if port is known
	if defaultPort > 0 {
		portal := ExtractedPortal{
			Name:   "Web UI",
			Port:   defaultPort,
			Scheme: scheme,
			Path:   webPath,
		}
		portals = append(portals, portal)
		portalBlock := fmt.Sprintf("x-portals:\n  - name: %q\n    scheme: %q\n    port: %d\n    path: %q\n",
			portal.Name, portal.Scheme, portal.Port, portal.Path)
		resYAML = portalBlock + resYAML
	}

	// Finally run through docker.NormalizeComposeVolumes
	resYAML = docker.NormalizeComposeVolumes(cleanID, pool, resYAML)
	return resYAML, defaultPort, portals, primaryImage
}

// extractCasaOSQuestions creates configuration questions from CasaOS Compose files.
func extractCasaOSQuestions(composeYAML string, app domain.CatalogApp, defaultPort int, portals []ExtractedPortal, pool, appdataPath string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
	var questions []domain.CatalogQuestion
	var groups []domain.CatalogQuestionGroup

	// 1. Port question
	if defaultPort > 0 {
		questions = append(questions, domain.CatalogQuestion{
			Variable:     "web_port",
			Label:        "Web UI Port",
			Description:  "Port mapped to the host for web interface access",
			Type:         "int",
			DefaultValue: strconv.Itoa(defaultPort),
			Required:     true,
			Group:        "Network & Web Portal",
		})
	}

	// 2. Storage questions - extract real volumes from normalized compose
	cleanID := app.AppID
	if parts := strings.Split(cleanID, "."); len(parts) > 1 {
		cleanID = parts[len(parts)-1]
	}

	type volSpec struct {
		hostPath  string
		mountPath string
	}
	var extractedVols []volSpec

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeYAML), &root); err == nil && len(root.Content) > 0 {
		doc := root.Content[0]
		for i := 0; i < len(doc.Content); i += 2 {
			if doc.Content[i].Value == "services" && doc.Content[i+1].Kind == yaml.MappingNode {
				svcs := doc.Content[i+1]
				for j := 1; j < len(svcs.Content); j += 2 {
					svc := svcs.Content[j]
					if svc.Kind != yaml.MappingNode {
						continue
					}
					for k := 0; k < len(svc.Content); k += 2 {
						if svc.Content[k].Value == "volumes" && svc.Content[k+1].Kind == yaml.SequenceNode {
							for _, vNode := range svc.Content[k+1].Content {
								if vNode.Kind == yaml.ScalarNode {
									parts := strings.Split(vNode.Value, ":")
									if len(parts) >= 2 && strings.HasPrefix(parts[0], "/") && strings.HasPrefix(parts[1], "/") {
										extractedVols = append(extractedVols, volSpec{hostPath: parts[0], mountPath: parts[1]})
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if len(extractedVols) > 0 {
		seenMounts := make(map[string]bool)
		for _, v := range extractedVols {
			if seenMounts[v.mountPath] {
				continue
			}
			seenMounts[v.mountPath] = true
			leaf := strings.TrimPrefix(v.mountPath, "/")
			leaf = strings.ReplaceAll(leaf, "/", "_")
			questions = append(questions, domain.CatalogQuestion{
				Variable:     fmt.Sprintf("storage.%s", leaf),
				Label:        fmt.Sprintf("Storage Path (%s)", v.mountPath),
				Description:  fmt.Sprintf("Host storage dataset for container %s", v.mountPath),
				Type:         "hostpath",
				DefaultValue: v.hostPath,
				MountPath:    v.mountPath,
				Required:     true,
				Group:        "Persistent Storage & Datasets",
			})
		}
	} else {
		defaultStorage := fmt.Sprintf("/mnt/%s/apps/%s/data", pool, cleanID)
		questions = append(questions, domain.CatalogQuestion{
			Variable:     "storage.data",
			Label:        "Persistent Data Path",
			Description:  "Host storage dataset for persistent application state",
			Type:         "hostpath",
			DefaultValue: defaultStorage,
			MountPath:    "/data",
			Required:     true,
			Group:        "Persistent Storage & Datasets",
		})
	}

	// 3. User permission questions
	questions = append(questions, domain.CatalogQuestion{
		Variable:     "puid",
		Label:        "User ID (PUID)",
		Description:  "Process user ID inside the container",
		Type:         "int",
		DefaultValue: "1000",
		Group:        "User Permissions & System",
	})
	questions = append(questions, domain.CatalogQuestion{
		Variable:     "pgid",
		Label:        "Group ID (PGID)",
		Description:  "Process group ID inside the container",
		Type:         "int",
		DefaultValue: "1000",
		Group:        "User Permissions & System",
	})

	// Group questions
	groupMap := make(map[string][]domain.CatalogQuestion)
	for _, q := range questions {
		groupMap[q.Group] = append(groupMap[q.Group], q)
	}
	for gName, qs := range groupMap {
		groups = append(groups, domain.CatalogQuestionGroup{
			Name:        gName,
			Description: getGroupDescription(gName),
			Questions:   qs,
		})
	}

	return groups, questions
}
