package catalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/sigxcpu/nas/internal/adapters/outbound/docker"
	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

var (
	defaultDokployMetaURL      = "https://templates.dokploy.com/meta.json"
	defaultDokployBlueprintURL = "https://raw.githubusercontent.com/Dokploy/templates/main/blueprints"
)

// SetDokployBlueprintURLForTest overrides the Dokploy blueprint URL for testing.
func SetDokployBlueprintURLForTest(url string) {
	defaultDokployBlueprintURL = url
}


type dokployMetaEntry struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Logo        string            `json:"logo"`
	Links       map[string]string `json:"links"`
	Tags        []string          `json:"tags"`
}

type dokployTemplateTOML struct {
	Variables map[string]any `toml:"variables"`
	Config    struct {
		Domains []struct {
			ServiceName string `toml:"serviceName"`
			Port        any    `toml:"port"`
			Host        string `toml:"host"`
		} `toml:"domains"`
		Env    any `toml:"env"`
		Mounts any `toml:"mounts"`
	} `toml:"config"`
}

// fetchDokployIndex retrieves the Dokploy templates registry.
func fetchDokployIndex(ctx context.Context, client *http.Client, url string) ([]dokployMetaEntry, []byte, error) {
	if url == "" {
		url = defaultDokployMetaURL
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
		return nil, nil, fmt.Errorf("dokploy catalog returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var entries []dokployMetaEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, nil, fmt.Errorf("failed to parse dokploy meta.json: %w", err)
	}
	return entries, body, nil
}

// enrichFromDokploy fetches the Docker Compose and template.toml for a Dokploy blueprint.
func enrichFromDokploy(ctx context.Context, client *http.Client, app *domain.CatalogApp, cacheDir, appsPool, appdataPath string) {
	if app.Train != "dokploy" {
		return
	}

	valuesCacheDir := filepath.Join(cacheDir, "values")
	_ = os.MkdirAll(valuesCacheDir, 0755)

	composeCacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("dokploy-%s-compose.yaml", app.AppID))
	tomlCacheFile := filepath.Join(valuesCacheDir, fmt.Sprintf("dokploy-%s-template.toml", app.AppID))

	var composeData []byte
	var tomlData []byte

	if cached, err := os.ReadFile(composeCacheFile); err == nil && len(cached) > 0 {
		composeData = cached
	} else {
		composeURL := fmt.Sprintf("%s/%s/docker-compose.yml", defaultDokployBlueprintURL, app.AppID)
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

	if cached, err := os.ReadFile(tomlCacheFile); err == nil && len(cached) > 0 {
		tomlData = cached
	} else {
		tomlURL := fmt.Sprintf("%s/%s/template.toml", defaultDokployBlueprintURL, app.AppID)
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, tomlURL, nil); err == nil {
			if resp, err := client.Do(req); err == nil {
				if resp.StatusCode == http.StatusOK {
					if b, err := io.ReadAll(resp.Body); err == nil {
						tomlData = b
						_ = os.WriteFile(tomlCacheFile, b, 0644)
					}
				}
				resp.Body.Close()
			}
		}
	}

	if len(composeData) == 0 {
		return
	}

	normalizedCompose, defaultPort, _, questions, containerImage := normalizeDokployBlueprint(
		string(composeData), string(tomlData), app.AppID, appsPool, appdataPath,
	)

	if defaultPort > 0 {
		app.DefaultPort = defaultPort
	}
	if containerImage != "" {
		app.ContainerImage = containerImage
	}
	app.ComposeYAML = normalizedCompose

	if len(questions) > 0 {
		groupMap := make(map[string][]domain.CatalogQuestion)
		for _, q := range questions {
			groupMap[q.Group] = append(groupMap[q.Group], q)
		}
		var groups []domain.CatalogQuestionGroup
		for gName, qs := range groupMap {
			groups = append(groups, domain.CatalogQuestionGroup{
				Name:        gName,
				Description: getGroupDescription(gName),
				Questions:   qs,
			})
		}
		app.QuestionGroups = groups
		app.Questions = questions
	}
}

// normalizeDokployBlueprint resolves template variables, extracts ports, adds x-portals,
// and normalizes volume paths.
func normalizeDokployBlueprint(rawCompose, rawTOML, appID, pool, appdataPath string) (string, int, []ExtractedPortal, []domain.CatalogQuestion, string) {
	if strings.TrimSpace(rawCompose) == "" {
		return rawCompose, 0, nil, nil, ""
	}

	if pool == "" {
		pool = "tank"
	}
	pool = strings.TrimPrefix(pool, "zfsPools/")

	var parsedTOML dokployTemplateTOML
	if strings.TrimSpace(rawTOML) != "" {
		_ = toml.Unmarshal([]byte(rawTOML), &parsedTOML)
	}

	// 1. Resolve variables defined in template.toml
	resolvedVars := make(map[string]string)
	var questions []domain.CatalogQuestion

	rePassword := regexp.MustCompile(`\$\{password(?::(\d+))?\}`)

	for k, v := range parsedTOML.Variables {
		strVal := fmt.Sprint(v)
		if rePassword.MatchString(strVal) {
			passLen := 16
			if m := rePassword.FindStringSubmatch(strVal); len(m) > 1 && m[1] != "" {
				if l, err := strconv.Atoi(m[1]); err == nil && l > 0 && l <= 64 {
					passLen = l
				}
			}
			strVal = generateRandomPassword(passLen)
		} else if strings.Contains(strVal, "${domain}") {
			strVal = strings.ReplaceAll(strVal, "${domain}", "localhost")
		}

		resolvedVars[k] = strVal

		// Create wizard configuration question
		label := humanizeVarName(k)
		isSecret := strings.Contains(strings.ToLower(k), "pass") || strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "key")
		questions = append(questions, domain.CatalogQuestion{
			Variable:     k,
			Label:        label,
			Description:  fmt.Sprintf("Dokploy template variable for %s", k),
			Type:         "string",
			DefaultValue: strVal,
			Required:     !isSecret,
			Group:        "Application Configuration & Credentials",
		})
	}

	// Resolve compound variable values (e.g. mongodb://user:${PASSWORD}@host)
	for k, v := range resolvedVars {
		for otherK, otherV := range resolvedVars {
			if strings.Contains(v, fmt.Sprintf("${%s}", otherK)) {
				v = strings.ReplaceAll(v, fmt.Sprintf("${%s}", otherK), otherV)
				resolvedVars[k] = v
			}
		}
	}

	// 2. Extract Web Portal Port from config.domains
	defaultPort := 0
	if len(parsedTOML.Config.Domains) > 0 {
		switch pv := parsedTOML.Config.Domains[0].Port.(type) {
		case int64:
			defaultPort = int(pv)
		case int:
			defaultPort = pv
		case float64:
			defaultPort = int(pv)
		case string:
			cleanP := strings.ReplaceAll(strings.TrimSpace(pv), "_", "")
			if p, err := strconv.Atoi(cleanP); err == nil && p > 0 {
				defaultPort = p
			}
		}
	}

	// 3. Substitute variables in compose YAML
	composeStr := rawCompose
	for k, v := range resolvedVars {
		composeStr = strings.ReplaceAll(composeStr, fmt.Sprintf("${%s}", k), v)
	}

	// 4. Parse Compose YAML and ensure ports are published
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeStr), &root); err != nil {
		return composeStr, defaultPort, nil, questions, ""
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return composeStr, defaultPort, nil, questions, ""
	}

	docMap := root.Content[0]
	hasPublishedPort := false
	primaryImage := ""

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

			for k := 0; k < len(svcMap.Content); k += 2 {
				if svcMap.Content[k].Value == "image" && primaryImage == "" && svcMap.Content[k+1].Kind == yaml.ScalarNode {
					primaryImage = strings.TrimSpace(svcMap.Content[k+1].Value)
				}
				if svcMap.Content[k].Value == "ports" && svcMap.Content[k+1].Kind == yaml.SequenceNode {
					if len(svcMap.Content[k+1].Content) > 0 {
						hasPublishedPort = true
						if defaultPort == 0 {
							firstP := svcMap.Content[k+1].Content[0]
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

			// If no published port but defaultPort is known from template.toml, inject it
			if !hasPublishedPort && defaultPort > 0 {
				portVal := fmt.Sprintf("%d:%d", defaultPort, defaultPort)
				svcMap.Content = append(svcMap.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "ports"},
					&yaml.Node{
						Kind: yaml.SequenceNode,
						Content: []*yaml.Node{
							{Kind: yaml.ScalarNode, Value: portVal},
						},
					},
				)
				hasPublishedPort = true
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(&root)
	_ = enc.Close()

	resYAML := strings.TrimPrefix(buf.String(), "---\n")

	// 5. Inject x-portals header
	var portals []ExtractedPortal
	if defaultPort > 0 {
		portal := ExtractedPortal{
			Name:   "Web UI",
			Port:   defaultPort,
			Scheme: "http",
			Path:   "/",
		}
		portals = append(portals, portal)
		portalBlock := fmt.Sprintf("x-portals:\n  - name: %q\n    scheme: %q\n    port: %d\n    path: %q\n",
			portal.Name, portal.Scheme, portal.Port, portal.Path)
		resYAML = portalBlock + resYAML

		questions = append([]domain.CatalogQuestion{
			{
				Variable:     "web_port",
				Label:        "Web UI Port",
				Description:  "Host port for accessing the web interface",
				Type:         "int",
				DefaultValue: strconv.Itoa(defaultPort),
				Required:     true,
				Group:        "Network & Web Portal",
			},
		}, questions...)
	}

	// 6. Normalize volumes via Docker volume normalizer
	resYAML = docker.NormalizeComposeVolumes(appID, pool, resYAML)
	return resYAML, defaultPort, portals, questions, primaryImage
}

func generateRandomPassword(length int) string {
	if length <= 0 {
		length = 16
	}
	bytesNeeded := (length + 1) / 2
	b := make([]byte, bytesNeeded)
	if _, err := rand.Read(b); err != nil {
		return "nasctl_secret_pwd123"
	}
	encoded := hex.EncodeToString(b)
	if len(encoded) > length {
		return encoded[:length]
	}
	return encoded
}

func humanizeVarName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, " ")
}
