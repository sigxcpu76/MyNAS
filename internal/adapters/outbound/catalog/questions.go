package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

// Raw TrueNAS SCALE questions.yaml schema representation
type rawQuestionsFile struct {
	Groups    []rawGroup    `yaml:"groups"`
	Questions []rawQuestion `yaml:"questions"`
}

type rawGroup struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type rawQuestion struct {
	Variable    string     `yaml:"variable"`
	Label       string     `yaml:"label"`
	Group       string     `yaml:"group"`
	Description string     `yaml:"description"`
	Schema      *rawSchema `yaml:"schema"`
}

type rawSchema struct {
	Type      string        `yaml:"type"` // string, int, boolean, hostpath, dict, list
	Default   any           `yaml:"default"`
	Required  bool          `yaml:"required"`
	Enum      []rawEnumItem `yaml:"enum"`
	MountPath string        `yaml:"mount_path"`
	Subpath   string        `yaml:"subpath"`
	Attrs     []rawQuestion `yaml:"attrs"`
	Items     []rawQuestion `yaml:"items"`
}

type rawEnumItem struct {
	Value       any    `yaml:"value"`
	Description string `yaml:"description"`
}

// ParseQuestionsYAML parses TrueNAS SCALE questions.yaml into structured domain question groups.
func ParseQuestionsYAML(data []byte, appID, appsPool, appdataPath string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
	if len(data) == 0 {
		return nil, nil
	}

	var raw rawQuestionsFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil
	}

	if appsPool == "" {
		appsPool = "tank"
	}
	if appdataPath == "" {
		appdataPath = fmt.Sprintf("/nas/%s/appdata", appsPool)
	}

	var flatList []domain.CatalogQuestion
	flattenQuestions("", raw.Questions, raw.Groups, appID, appdataPath, &flatList)

	// Filter out internal TrueNAS noise (certificates, ix internal perms, etc.)
	var filtered []domain.CatalogQuestion
	for _, q := range flatList {
		if isIgnoredQuestion(q.Variable) {
			continue
		}
		filtered = append(filtered, q)
	}

	// Group into clean categories
	groupMap := make(map[string][]domain.CatalogQuestion)
	for _, q := range filtered {
		grp := categorizeQuestion(q)
		q.Group = grp
		groupMap[grp] = append(groupMap[grp], q)
	}

	groupNames := []string{
		"Network & Web Portal",
		"Persistent Storage & Datasets",
		"Application Configuration & Credentials",
		"User Permissions & System",
	}

	var resultGroups []domain.CatalogQuestionGroup
	for _, gName := range groupNames {
		if qs, ok := groupMap[gName]; ok && len(qs) > 0 {
			resultGroups = append(resultGroups, domain.CatalogQuestionGroup{
				Name:        gName,
				Description: getGroupDescription(gName),
				Questions:   qs,
			})
		}
	}

	// Append any other groups
	for gName, qs := range groupMap {
		isStandard := false
		for _, sn := range groupNames {
			if sn == gName {
				isStandard = true
				break
			}
		}
		if !isStandard && len(qs) > 0 {
			resultGroups = append(resultGroups, domain.CatalogQuestionGroup{
				Name:        gName,
				Description: "",
				Questions:   qs,
			})
		}
	}

	return resultGroups, filtered
}

func flattenQuestions(prefix string, questions []rawQuestion, groups []rawGroup, appID, appdataPath string, out *[]domain.CatalogQuestion) {
	for _, q := range questions {
		varPath := q.Variable
		if prefix != "" {
			varPath = prefix + "." + q.Variable
		}

		schema := q.Schema
		if schema == nil {
			continue
		}

		// Recurse into dict attrs
		if schema.Type == "dict" && len(schema.Attrs) > 0 {
			flattenQuestions(varPath, schema.Attrs, groups, appID, appdataPath, out)
			continue
		}

		// Normal scalar or hostpath question
		qType := normalizeQuestionType(schema.Type)
		if qType == "" {
			continue
		}

		label := q.Label
		if label == "" {
			label = humanizeVariable(q.Variable)
		}

		defVal := formatDefaultValue(schema.Default, qType, varPath, appID, appdataPath)
		mountPath := schema.MountPath
		if mountPath == "" && strings.Contains(varPath, "storage") {
			// Extract mount path if specified in variable or subpath
			if schema.Subpath != "" {
				mountPath = schema.Subpath
			} else {
				parts := strings.Split(varPath, ".")
				mountPath = "/" + parts[len(parts)-1]
			}
		}

		var options []string
		for _, opt := range schema.Enum {
			if opt.Value != nil {
				options = append(options, fmt.Sprintf("%v", opt.Value))
			}
		}

		*out = append(*out, domain.CatalogQuestion{
			Variable:     varPath,
			Label:        label,
			Description:  q.Description,
			Type:         qType,
			DefaultValue: defVal,
			Required:     schema.Required,
			Group:        q.Group,
			Options:      options,
			MountPath:    mountPath,
		})
	}
}

func normalizeQuestionType(rawType string) string {
	switch strings.ToLower(rawType) {
	case "string", "password", "ipaddr":
		return "string"
	case "int", "integer":
		return "int"
	case "boolean", "bool":
		return "boolean"
	case "hostpath", "path":
		return "hostpath"
	case "select", "enum":
		return "select"
	default:
		return ""
	}
}

func formatDefaultValue(val any, qType, varPath, appID, appdataPath string) string {
	if qType == "hostpath" {
		if valStr, ok := val.(string); ok && valStr != "" && strings.HasPrefix(valStr, "/") {
			// Replace /mnt/<pool> with /nas/<pool>/appdata/<app>
			return strings.ReplaceAll(valStr, "/mnt/", "/nas/")
		}
		leaf := "data"
		parts := strings.Split(varPath, ".")
		if len(parts) > 0 && parts[len(parts)-1] != "" && parts[len(parts)-1] != "host_path" {
			leaf = parts[len(parts)-1]
		}
		return fmt.Sprintf("%s/%s/%s", appdataPath, appID, leaf)
	}

	if val == nil {
		return ""
	}

	switch v := val.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func isIgnoredQuestion(varPath string) bool {
	lower := strings.ToLower(varPath)
	ignoredPrefixes := []string{
		"network.certificate",
		"ix_certificates",
		"resources.limits",
		"resources.reservations",
		"security_context",
		"perms",
		"portal_enabled",
		"probes",
	}
	for _, ign := range ignoredPrefixes {
		if strings.HasPrefix(lower, ign) || strings.Contains(lower, "."+ign) {
			return true
		}
	}
	return false
}

func categorizeQuestion(q domain.CatalogQuestion) string {
	vLower := strings.ToLower(q.Variable)
	if strings.Contains(vLower, "port") || strings.Contains(vLower, "network") || strings.Contains(vLower, "host") && !strings.Contains(vLower, "path") {
		return "Network & Web Portal"
	}
	if strings.Contains(vLower, "storage") || q.Type == "hostpath" || strings.Contains(vLower, "path") || strings.Contains(vLower, "volume") {
		return "Persistent Storage & Datasets"
	}
	if strings.Contains(vLower, "run_as") || strings.Contains(vLower, "user") && !strings.Contains(vLower, "admin") || strings.Contains(vLower, "group") {
		return "User Permissions & System"
	}
	return "Application Configuration & Credentials"
}

func getGroupDescription(groupName string) string {
	switch groupName {
	case "Network & Web Portal":
		return "Configure host listening ports and web portal endpoints"
	case "Persistent Storage & Datasets":
		return "Bind mount persistent ZFS dataset storage paths for config and application data"
	case "Application Configuration & Credentials":
		return "Set application administrative credentials, environment variables, and operational mode"
	case "User Permissions & System":
		return "Configure container process user/group ownership (PUID/PGID)"
	default:
		return ""
	}
}

func humanizeVariable(v string) string {
	parts := strings.Split(v, ".")
	last := parts[len(parts)-1]
	words := strings.Split(last, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// GenerateComposeFromQuestions builds a clean Docker Compose YAML descriptor from answered questions.
func GenerateComposeFromQuestions(app domain.CatalogApp, answers map[string]string, appsPool, appdataPath string) string {
	if appsPool == "" {
		appsPool = "tank"
	}
	if appdataPath == "" {
		appdataPath = fmt.Sprintf("/nas/%s/appdata", appsPool)
	}

	serviceName := app.AppID
	if serviceName == "" {
		serviceName = "app"
	}

	image := app.ContainerImage
	if image == "" {
		image = fmt.Sprintf("%s:latest", serviceName)
	}

	var ports []string
	var volumes []string
	var envs []string
	var portals []string

	puid := "1000"
	pgid := "1000"

	// Parse answers
	for _, q := range app.Questions {
		val := q.DefaultValue
		if userVal, ok := answers[q.Variable]; ok && userVal != "" {
			val = userVal
		}

		vLower := strings.ToLower(q.Variable)

		// 1. Ports
		if strings.Contains(vLower, "port") && q.Type == "int" {
			if p, err := strconv.Atoi(val); err == nil && p > 0 {
				containerPort := p
				if app.DefaultPort > 0 {
					containerPort = app.DefaultPort
				}
				ports = append(ports, fmt.Sprintf("%d:%d", p, containerPort))
				portals = append(portals, fmt.Sprintf("  - name: %q\n    port: %d\n    scheme: \"http\"\n    path: \"/\"", q.Label, p))
			}
		}

		// 2. Storage / Volumes
		if q.Type == "hostpath" || strings.Contains(vLower, "storage") {
			mountPath := q.MountPath
			if mountPath == "" {
				mountPath = "/data"
			}
			hostPath := val
			if hostPath == "" {
				hostPath = fmt.Sprintf("%s/%s/data", appdataPath, serviceName)
			}
			volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, mountPath))
		}

		// 3. User / Group
		if strings.Contains(vLower, "user") && (strings.Contains(vLower, "run_as") || strings.Contains(vLower, "puid")) {
			if val != "" {
				puid = val
			}
		}
		if strings.Contains(vLower, "group") && (strings.Contains(vLower, "run_as") || strings.Contains(vLower, "pgid")) {
			if val != "" {
				pgid = val
			}
		}

		// 4. Environment Variables
		if categorizeQuestion(q) == "Application Configuration & Credentials" && val != "" {
			parts := strings.Split(q.Variable, ".")
			key := strings.ToUpper(parts[len(parts)-1])
			envs = append(envs, fmt.Sprintf("%s=%s", key, val))
		}
	}

	// Fallbacks if no questions provided
	if len(ports) == 0 && app.DefaultPort > 0 {
		ports = append(ports, fmt.Sprintf("%d:%d", app.DefaultPort, app.DefaultPort))
		portals = append(portals, fmt.Sprintf("  - name: \"Web UI\"\n    port: %d\n    scheme: \"http\"\n    path: \"/\"", app.DefaultPort))
	}
	if len(volumes) == 0 {
		volumes = append(volumes, fmt.Sprintf("%s/%s/data:/data", appdataPath, serviceName))
	}

	var sb strings.Builder
	sb.WriteString("services:\n")
	sb.WriteString(fmt.Sprintf("  %s:\n", serviceName))
	sb.WriteString(fmt.Sprintf("    image: %s\n", image))
	sb.WriteString(fmt.Sprintf("    container_name: %s\n", serviceName))
	sb.WriteString("    restart: unless-stopped\n")

	if puid != "" && pgid != "" && puid != "0" {
		sb.WriteString(fmt.Sprintf("    user: \"%s:%s\"\n", puid, pgid))
	}

	if len(ports) > 0 {
		sb.WriteString("    ports:\n")
		for _, p := range ports {
			sb.WriteString(fmt.Sprintf("      - %q\n", p))
		}
	}

	if len(envs) > 0 {
		sb.WriteString("    environment:\n")
		sort.Strings(envs)
		for _, e := range envs {
			sb.WriteString(fmt.Sprintf("      - %s\n", e))
		}
	}

	if len(volumes) > 0 {
		sb.WriteString("    volumes:\n")
		for _, v := range volumes {
			sb.WriteString(fmt.Sprintf("      - %s\n", v))
		}
	}

	if len(portals) > 0 {
		sb.WriteString("x-portals:\n")
		for _, pt := range portals {
			sb.WriteString(pt + "\n")
		}
	}

	return sb.String()
}

// GetCuratedQuestions provides standard questions for apps when questions.yaml is unavailable.
func GetCuratedQuestions(app domain.CatalogApp, appsPool, appdataPath string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
	if appsPool == "" {
		appsPool = "tank"
	}
	if appdataPath == "" {
		appdataPath = fmt.Sprintf("/nas/%s/appdata", appsPool)
	}

	defaultPort := app.DefaultPort
	if defaultPort <= 0 {
		defaultPort = 8080
	}

	netQ := domain.CatalogQuestion{
		Variable:     "network.web_port",
		Label:        "Web UI Port",
		Description:  "Host port to bind the application's primary web interface",
		Type:         "int",
		DefaultValue: strconv.Itoa(defaultPort),
		Required:     true,
		Group:        "Network & Web Portal",
	}

	storageQ := domain.CatalogQuestion{
		Variable:     "storage.data",
		Label:        "Application Data Storage",
		Description:  "Host ZFS dataset directory for persistent configuration and state",
		Type:         "hostpath",
		DefaultValue: fmt.Sprintf("%s/%s/data", appdataPath, app.AppID),
		Required:     true,
		Group:        "Persistent Storage & Datasets",
		MountPath:    "/data",
	}

	userQ := domain.CatalogQuestion{
		Variable:     "run_as.user",
		Label:        "User ID (PUID)",
		Description:  "POSIX User ID for container file ownership",
		Type:         "int",
		DefaultValue: "1000",
		Required:     false,
		Group:        "User Permissions & System",
	}

	groupQ := domain.CatalogQuestion{
		Variable:     "run_as.group",
		Label:        "Group ID (PGID)",
		Description:  "POSIX Group ID for container file ownership",
		Type:         "int",
		DefaultValue: "1000",
		Required:     false,
		Group:        "User Permissions & System",
	}

	flat := []domain.CatalogQuestion{netQ, storageQ, userQ, groupQ}

	// App-specific credential questions
	var extraQs []domain.CatalogQuestion
	switch app.AppID {
	case "nextcloud":
		extraQs = append(extraQs,
			domain.CatalogQuestion{
				Variable:     "nextcloud.admin_user",
				Label:        "Admin Username",
				Description:  "Initial administrative user account",
				Type:         "string",
				DefaultValue: "admin",
				Required:     true,
				Group:        "Application Configuration & Credentials",
			},
			domain.CatalogQuestion{
				Variable:     "nextcloud.admin_password",
				Label:        "Admin Password",
				Description:  "Initial administrative password",
				Type:         "string",
				DefaultValue: "admin",
				Required:     true,
				Group:        "Application Configuration & Credentials",
			},
		)
	case "vaultwarden":
		extraQs = append(extraQs,
			domain.CatalogQuestion{
				Variable:     "vaultwarden.signups_allowed",
				Label:        "Allow New User Signups",
				Description:  "Whether anyone can register an account on this vault",
				Type:         "boolean",
				DefaultValue: "true",
				Required:     false,
				Group:        "Application Configuration & Credentials",
			},
		)
	case "pihole":
		extraQs = append(extraQs,
			domain.CatalogQuestion{
				Variable:     "pihole.web_password",
				Label:        "Admin Web Password",
				Description:  "Password for the Pi-hole web dashboard",
				Type:         "string",
				DefaultValue: "admin",
				Required:     true,
				Group:        "Application Configuration & Credentials",
			},
		)
	case "wg-easy":
		extraQs = append(extraQs,
			domain.CatalogQuestion{
				Variable:     "wgeasy.password",
				Label:        "Admin Dashboard Password",
				Description:  "Password for the WireGuard Easy web management portal",
				Type:         "string",
				DefaultValue: "admin",
				Required:     true,
				Group:        "Application Configuration & Credentials",
			},
		)
	}

	if len(extraQs) > 0 {
		flat = append(flat, extraQs...)
	}

	groupMap := make(map[string][]domain.CatalogQuestion)
	for _, q := range flat {
		groupMap[q.Group] = append(groupMap[q.Group], q)
	}

	groupNames := []string{
		"Network & Web Portal",
		"Persistent Storage & Datasets",
		"Application Configuration & Credentials",
		"User Permissions & System",
	}

	var groups []domain.CatalogQuestionGroup
	for _, gn := range groupNames {
		if qs, ok := groupMap[gn]; ok && len(qs) > 0 {
			groups = append(groups, domain.CatalogQuestionGroup{
				Name:        gn,
				Description: getGroupDescription(gn),
				Questions:   qs,
			})
		}
	}

	return groups, flat
}
