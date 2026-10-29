package catalog

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
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

// ExtractStorageMountPaths parses a TrueNAS SCALE docker-compose.yaml template
// and returns a map of storage target names (e.g. "data", "config") to their container mount paths.
func ExtractStorageMountPaths(tplContent string, valuesContent ...string) map[string]string {
	result := make(map[string]string)
	if strings.TrimSpace(tplContent) == "" {
		return result
	}

	ixValues := ""
	if len(valuesContent) > 0 {
		ixValues = valuesContent[0]
	}

	consts := make(map[string]string)
	consts["config_path"] = "/config"
	consts["data_path"] = "/data"
	consts["pio_path"] = "/config/.esphome/.platformio"
	consts["pio_globallib_path"] = "/config/.esphome/piolibs"

	if ixValues != "" {
		reConst := regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9_]+_path):\s*["']?(/[^"'\n\r]+)["']?`)
		for _, m := range reConst.FindAllStringSubmatch(ixValues, -1) {
			if len(m) > 2 {
				consts[m[1]] = strings.TrimSpace(m[2])
			}
		}
	}

	// 1. Match: add_storage("/usr/share/elasticsearch/data", values.storage.data)
	// Must require leading slash '/' to guarantee absolute mount path
	reAddStorage := regexp.MustCompile(`add_storage\(\s*["'](/[^"']+)["']\s*,\s*values\.storage\.([a-zA-Z0-9_]+)`)
	for _, m := range reAddStorage.FindAllStringSubmatch(tplContent, -1) {
		if len(m) > 2 {
			result[m[2]] = m[1]
		}
	}

	// 2. Match: add_storage(values.consts.config_path, values.storage.config)
	reAddStorageConst := regexp.MustCompile(`add_storage\(\s*(?:values\.)?consts\.([a-zA-Z0-9_]+)\s*,\s*values\.storage\.([a-zA-Z0-9_]+)`)
	for _, m := range reAddStorageConst.FindAllStringSubmatch(tplContent, -1) {
		if len(m) > 2 {
			constKey := m[1]
			storageKey := m[2]
			if p, ok := consts[constKey]; ok && p != "" {
				result[storageKey] = p
			} else if strings.Contains(constKey, "config") {
				result[storageKey] = "/config"
			} else if strings.Contains(constKey, "data") {
				result[storageKey] = "/data"
			}
		}
	}

	// 3. Match: nc_storage.x.append(("/var/www/html", values.storage.html))
	// NOTE: Must require leading slash '/' so it does NOT match ("config", values.storage.config)
	reAppendStorage := regexp.MustCompile(`\(\s*["'](/[^"']+)["']\s*,\s*values\.storage\.([a-zA-Z0-9_]+)`)
	for _, m := range reAppendStorage.FindAllStringSubmatch(tplContent, -1) {
		if len(m) > 2 {
			if _, exists := result[m[2]]; !exists {
				result[m[2]] = m[1]
			}
		}
	}

	// 4. Match: "volume": values.storage.postgres_data
	reVolumeStorage := regexp.MustCompile(`["']volume["']\s*:\s*values\.storage\.([a-zA-Z0-9_]+)`)
	for _, m := range reVolumeStorage.FindAllStringSubmatch(tplContent, -1) {
		if len(m) > 1 {
			target := m[1]
			if _, exists := result[target]; !exists {
				if strings.Contains(target, "postgres") || strings.Contains(target, "pg") {
					result[target] = "/var/lib/postgresql/data"
				} else if strings.Contains(target, "mysql") || strings.Contains(target, "mariadb") {
					result[target] = "/var/lib/mysql"
				}
			}
		}
	}

	return result
}

func isTrueNASStorageDict(q rawQuestion) bool {
	if q.Schema == nil || q.Schema.Type != "dict" {
		return false
	}
	hasType := false
	hasConfig := false
	for _, attr := range q.Schema.Attrs {
		if attr.Variable == "type" {
			hasType = true
		}
		if attr.Variable == "ix_volume_config" || attr.Variable == "host_path_config" || attr.Variable == "cifs_config" || attr.Variable == "nfs_config" {
			hasConfig = true
		}
	}
	return hasType || hasConfig
}

func resolveContainerMountPath(appID, storageTarget string, schema *rawSchema, mountPaths map[string]string) string {
	ensureAbs := func(p string) string {
		clean := strings.TrimSpace(p)
		if clean == "" {
			return clean
		}
		if !strings.HasPrefix(clean, "/") {
			clean = "/" + clean
		}
		return clean
	}

	if mountPaths != nil {
		if mp, ok := mountPaths[storageTarget]; ok && mp != "" {
			return ensureAbs(mp)
		}
	}
	if schema != nil {
		if schema.MountPath != "" {
			return ensureAbs(schema.MountPath)
		}
		if schema.Subpath != "" {
			return ensureAbs(schema.Subpath)
		}
	}
	// Well-known container mount paths for popular apps
	cleanApp := strings.ToLower(appID)
	cleanTarget := strings.ToLower(storageTarget)
	if strings.Contains(cleanApp, "elastic") && (cleanTarget == "data" || cleanTarget == "storage.data") {
		return "/usr/share/elasticsearch/data"
	}
	switch cleanTarget {
	case "config":
		return "/config"
	case "data":
		return "/data"
	case "media":
		return "/media"
	case "cache":
		return "/cache"
	case "html":
		return "/var/www/html"
	case "postgres_data", "pg_data":
		return "/var/lib/postgresql/data"
	case "mysql_data":
		return "/var/lib/mysql"
	case "transcodes":
		return "/config/transcodes"
	default:
		return ensureAbs(cleanTarget)
	}
}

// ParseQuestionsYAML parses TrueNAS SCALE questions.yaml into structured domain question groups.
func ParseQuestionsYAML(data []byte, appID, appsPool, appdataPath string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
	return ParseQuestionsYAMLWithTemplate(data, appID, appsPool, appdataPath, "")
}

// ParseQuestionsYAMLWithTemplate parses questions.yaml taking into account the docker-compose template to extract real mount paths.
func ParseQuestionsYAMLWithTemplate(data []byte, appID, appsPool, appdataPath, tplContent string, valuesContent ...string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
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
		appdataPath = fmt.Sprintf("/mnt/%s/apps", appsPool)
	}

	mountPaths := ExtractStorageMountPaths(tplContent, valuesContent...)

	var flatList []domain.CatalogQuestion
	flattenQuestions("", raw.Questions, raw.Groups, appID, appdataPath, mountPaths, &flatList)

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

func flattenQuestions(prefix string, questions []rawQuestion, groups []rawGroup, appID, appdataPath string, mountPaths map[string]string, out *[]domain.CatalogQuestion) {
	for _, q := range questions {
		varPath := q.Variable
		if prefix != "" {
			varPath = prefix + "." + q.Variable
		}

		schema := q.Schema
		if schema == nil {
			continue
		}

		// Check if this is a TrueNAS storage descriptor dict (e.g. storage.data with type/ix_volume_config/host_path_config)
		if (prefix == "storage" || strings.HasPrefix(prefix, "storage.")) && isTrueNASStorageDict(q) {
			storageTarget := q.Variable
			label := q.Label
			if label == "" {
				label = humanizeVariable(storageTarget) + " Storage"
			}
			desc := q.Description
			if desc == "" {
				desc = fmt.Sprintf("Persistent storage dataset path for %s", storageTarget)
			}
			defVal := fmt.Sprintf("%s/%s/%s", appdataPath, appID, storageTarget)
			if storageTarget == "media" {
				defVal = strings.Replace(appdataPath, "/apps", "/media", 1)
			}
			mountPath := resolveContainerMountPath(appID, storageTarget, schema, mountPaths)

			*out = append(*out, domain.CatalogQuestion{
				Variable:     varPath,
				Label:        label,
				Description:  desc,
				Type:         "hostpath",
				DefaultValue: defVal,
				Required:     true,
				Group:        "Persistent Storage & Datasets",
				MountPath:    mountPath,
			})
			continue
		}

		// Recurse into dict attrs
		if schema.Type == "dict" && len(schema.Attrs) > 0 {
			flattenQuestions(varPath, schema.Attrs, groups, appID, appdataPath, mountPaths, out)
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
				leaf := parts[len(parts)-1]
				if leaf != "type" && leaf != "acl_enable" && leaf != "dataset_name" && leaf != "host_path" && leaf != "path" {
					mountPath = "/" + leaf
				} else if len(parts) > 1 {
					mountPath = "/" + parts[len(parts)-2]
				} else {
					mountPath = "/data"
				}
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
			return strings.ReplaceAll(valStr, "/nas/", "/mnt/")
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
	// TrueNAS storage plumbing / settings that are not real volume mounts
	if strings.Contains(lower, "storage.") || strings.HasPrefix(lower, "storage") {
		if strings.HasSuffix(lower, ".type") ||
			strings.Contains(lower, "ix_volume") ||
			strings.Contains(lower, "acl_enable") ||
			strings.Contains(lower, "dataset_name") ||
			strings.Contains(lower, "acl_entries") ||
			strings.Contains(lower, "cifs_config") ||
			strings.Contains(lower, "nfs_config") ||
			strings.HasSuffix(lower, ".read_only") {
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

func applyAnswersToCompose(rawCompose string, app domain.CatalogApp, answers map[string]string, appsPool, appdataPath string) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(rawCompose), &root); err != nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return rawCompose
	}
	docMap := root.Content[0]
	if docMap.Kind != yaml.MappingNode {
		return rawCompose
	}

	for _, q := range app.Questions {
		val := q.DefaultValue
		if userVal, ok := answers[q.Variable]; ok && userVal != "" {
			val = userVal
		}
		if val == "" {
			continue
		}
		vLower := strings.ToLower(q.Variable)

		// 1. Ports
		if strings.Contains(vLower, "port") && q.Type == "int" {
			if newP, err := strconv.Atoi(val); err == nil && newP > 0 {
				// Update x-portals
				for i := 0; i < len(docMap.Content); i += 2 {
					if docMap.Content[i].Value == "x-portals" && docMap.Content[i+1].Kind == yaml.SequenceNode {
						for _, pNode := range docMap.Content[i+1].Content {
							if pNode.Kind == yaml.MappingNode {
								for pk := 0; pk < len(pNode.Content); pk += 2 {
									if pNode.Content[pk].Value == "port" {
										pNode.Content[pk+1].Value = strconv.Itoa(newP)
									}
								}
							}
						}
					}
				}

				// Update ports in services
				for i := 0; i < len(docMap.Content); i += 2 {
					if docMap.Content[i].Value == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
						servicesNode := docMap.Content[i+1]
						for j := 1; j < len(servicesNode.Content); j += 2 {
							svcMap := servicesNode.Content[j]
							for k := 0; k < len(svcMap.Content); k += 2 {
								if svcMap.Content[k].Value == "ports" && svcMap.Content[k+1].Kind == yaml.SequenceNode {
									for _, portNode := range svcMap.Content[k+1].Content {
										if portNode.Kind == yaml.ScalarNode {
											cleanVal := strings.Trim(portNode.Value, `"' `)
											parts := strings.Split(cleanVal, ":")
											if len(parts) >= 2 {
												cleanCont := strings.Trim(parts[1], `"' `)
												portNode.Value = fmt.Sprintf("%d:%s", newP, cleanCont)
												break
											}
										} else if portNode.Kind == yaml.MappingNode {
											for mpk := 0; mpk < len(portNode.Content); mpk += 2 {
												if portNode.Content[mpk].Value == "published" {
													portNode.Content[mpk+1].Value = strconv.Itoa(newP)
													break
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// 2. Storage / Volumes
		if q.Type == "hostpath" || (q.MountPath != "" && strings.Contains(vLower, "storage")) {
			targetMount := strings.Trim(q.MountPath, `"' `)
			newHostPath := strings.Trim(val, `"' `)
			if targetMount != "" && !strings.HasPrefix(targetMount, "/") {
				targetMount = "/" + targetMount
			}
			if newHostPath != "" && !strings.HasPrefix(newHostPath, "/") {
				newHostPath = "/" + newHostPath
			}
			if targetMount != "" && newHostPath != "" {
				for i := 0; i < len(docMap.Content); i += 2 {
					if docMap.Content[i].Value == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
						servicesNode := docMap.Content[i+1]
						for j := 1; j < len(servicesNode.Content); j += 2 {
							svcMap := servicesNode.Content[j]
							for k := 0; k < len(svcMap.Content); k += 2 {
								if svcMap.Content[k].Value == "volumes" && svcMap.Content[k+1].Kind == yaml.SequenceNode {
									volsSeq := svcMap.Content[k+1]
									updated := false
									for _, vNode := range volsSeq.Content {
										if vNode.Kind == yaml.ScalarNode {
											cleanV := strings.Trim(vNode.Value, `"' `)
											parts := strings.Split(cleanV, ":")
											if len(parts) >= 2 {
												currTarget := strings.Trim(parts[1], `"' `)
												if !strings.HasPrefix(currTarget, "/") {
													currTarget = "/" + currTarget
												}
												if currTarget == targetMount {
													mode := ""
													if len(parts) > 2 {
														mode = ":" + strings.Trim(parts[2], `"' `)
													}
													vNode.Value = fmt.Sprintf("%s:%s%s", newHostPath, targetMount, mode)
													updated = true
													break
												}
											}
										} else if vNode.Kind == yaml.MappingNode {
											targetMatch := false
											for vk := 0; vk < len(vNode.Content); vk += 2 {
												curT := strings.Trim(vNode.Content[vk+1].Value, `"' `)
												if !strings.HasPrefix(curT, "/") {
													curT = "/" + curT
												}
												if vNode.Content[vk].Value == "target" && curT == targetMount {
													targetMatch = true
													break
												}
											}
											if targetMatch {
												for vk := 0; vk < len(vNode.Content); vk += 2 {
													if vNode.Content[vk].Value == "source" {
														vNode.Content[vk+1].Value = newHostPath
														updated = true
														break
													}
												}
											}
										}
									}

									// If not updated and the only volume is a generic fallback /data, replace it
									if !updated && len(volsSeq.Content) == 1 && volsSeq.Content[0].Kind == yaml.ScalarNode {
										firstVal := strings.Trim(volsSeq.Content[0].Value, `"' `)
										parts := strings.Split(firstVal, ":")
										if len(parts) >= 2 {
											firstTgt := strings.Trim(parts[1], `"' `)
											if firstTgt == "/data" && targetMount != "/data" {
												volsSeq.Content[0].Value = fmt.Sprintf("%s:%s", newHostPath, targetMount)
												updated = true
											}
										}
									}

									if !updated {
										volsSeq.Content = append(volsSeq.Content, &yaml.Node{
											Kind:  yaml.ScalarNode,
											Value: fmt.Sprintf("%s:%s", newHostPath, targetMount),
										})
									}
								}
							}
						}
					}
				}
			}
		}

		// 3. User / Group & Env
		if strings.Contains(vLower, "puid") || strings.Contains(vLower, "pgid") || categorizeQuestion(q) == "Application Configuration & Credentials" {
			parts := strings.Split(q.Variable, ".")
			key := strings.ToUpper(parts[len(parts)-1])
			if strings.Contains(vLower, "puid") {
				key = "PUID"
			} else if strings.Contains(vLower, "pgid") {
				key = "PGID"
			}
			for i := 0; i < len(docMap.Content); i += 2 {
				if docMap.Content[i].Value == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
					servicesNode := docMap.Content[i+1]
					for j := 1; j < len(servicesNode.Content); j += 2 {
						svcMap := servicesNode.Content[j]
						for k := 0; k < len(svcMap.Content); k += 2 {
							if svcMap.Content[k].Value == "environment" {
								envNode := svcMap.Content[k+1]
								if envNode.Kind == yaml.SequenceNode {
									found := false
									for _, e := range envNode.Content {
										if strings.HasPrefix(e.Value, key+"=") {
											e.Value = fmt.Sprintf("%s=%s", key, val)
											found = true
											break
										}
									}
									if !found {
										envNode.Content = append(envNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%s=%s", key, val)})
									}
								} else if envNode.Kind == yaml.MappingNode {
									found := false
									for ek := 0; ek < len(envNode.Content); ek += 2 {
										if envNode.Content[ek].Value == key {
											envNode.Content[ek+1].Value = val
											found = true
											break
										}
									}
									if !found {
										envNode.Content = append(envNode.Content,
											&yaml.Node{Kind: yaml.ScalarNode, Value: key},
											&yaml.Node{Kind: yaml.ScalarNode, Value: val},
										)
									}
								}
							}
						}
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

	out := strings.TrimPrefix(buf.String(), "---\n")
	for k, v := range answers {
		if v != "" && strings.Contains(out, fmt.Sprintf("${%s}", k)) {
			out = strings.ReplaceAll(out, fmt.Sprintf("${%s}", k), v)
		}
	}
	return out
}

// GenerateComposeFromQuestions builds a clean Docker Compose YAML descriptor from answered questions.
func GenerateComposeFromQuestions(app domain.CatalogApp, answers map[string]string, appsPool, appdataPath string) string {
	if appsPool == "" {
		appsPool = "tank"
	}
	if appdataPath == "" {
		appdataPath = fmt.Sprintf("/mnt/%s/apps", appsPool)
	}

	if app.ComposeYAML != "" && strings.Contains(app.ComposeYAML, "services:") {
		return applyAnswersToCompose(app.ComposeYAML, app, answers, appsPool, appdataPath)
	}

	cleanID := app.AppID
	if parts := strings.Split(cleanID, "."); len(parts) > 1 {
		cleanID = parts[len(parts)-1]
	}
	serviceName := cleanID
	if serviceName == "" {
		serviceName = "app"
	}

	image := app.ContainerImage
	if image == "" {
		image = fmt.Sprintf("%s:latest", cleanID)
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
				isPortal := isWebPortalVariable(q.Variable, q.Label)
				containerPort := p
				if isPortal && app.DefaultPort > 0 && app.DefaultPort != p {
					containerPort = app.DefaultPort
				}
				if app.AppID == "elastic-search" || strings.Contains(app.AppID, "elasticsearch") {
					containerPort = 9200
				}
				ports = append(ports, fmt.Sprintf("%d:%d", p, containerPort))
				if isPortal {
					portalName := q.Label
					if portalName == "" {
						portalName = "Web UI"
					}
					portals = append(portals, fmt.Sprintf("  - name: %q\n    port: %d\n    scheme: \"http\"\n    path: \"/\"", portalName, p))
				}
			}
		}

		// 2. Storage / Volumes
		if q.Type == "hostpath" || (q.MountPath != "" && strings.Contains(vLower, "storage")) {
			// Disallow TrueNAS volume settings
			if strings.HasSuffix(vLower, ".type") ||
				strings.Contains(vLower, "ix_volume") ||
				strings.Contains(vLower, "acl_enable") ||
				strings.Contains(vLower, "dataset_name") ||
				strings.Contains(vLower, "acl_entries") ||
				strings.Contains(vLower, "cifs_config") ||
				strings.Contains(vLower, "nfs_config") ||
				strings.HasSuffix(vLower, ".read_only") ||
				val == "ix_volume" ||
				val == "host_path" ||
				val == "true" ||
				val == "false" {
				continue
			}

			mountPath := q.MountPath
			if mountPath == "" || mountPath == "/type" || mountPath == "/acl_enable" || mountPath == "/dataset_name" || mountPath == "/path" {
				parts := strings.Split(q.Variable, ".")
				leaf := parts[len(parts)-1]
				if leaf == "path" || leaf == "host_path" {
					if len(parts) > 1 {
						leaf = parts[len(parts)-2]
					} else {
						leaf = "data"
					}
				}
				mountPath = resolveContainerMountPath(serviceName, leaf, nil, nil)
			}

			hostPath := val
			if hostPath == "" || !strings.HasPrefix(hostPath, "/") {
				parts := strings.Split(q.Variable, ".")
				leaf := parts[len(parts)-1]
				if leaf == "path" || leaf == "host_path" {
					if len(parts) > 1 {
						leaf = parts[len(parts)-2]
					} else {
						leaf = "data"
					}
				}
				hostPath = fmt.Sprintf("%s/%s/%s", appdataPath, serviceName, leaf)
			}

			mapping := fmt.Sprintf("%s:%s", hostPath, mountPath)
			if !slices.Contains(volumes, mapping) {
				volumes = append(volumes, mapping)
			}
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
			envs = append(envs, fmt.Sprintf("%s=%s", key, escapeComposeEnvVal(val)))
		}
	}

	// Standalone Elasticsearch defaults
	if app.AppID == "elastic-search" || strings.Contains(app.AppID, "elasticsearch") {
		hasDiscovery := false
		hasXpack := false
		hasJavaOpts := false
		heapVal := "512m"
		for _, e := range envs {
			if strings.HasPrefix(e, "DISCOVERY_TYPE=") || strings.HasPrefix(e, "discovery.type=") {
				hasDiscovery = true
			}
			if strings.HasPrefix(e, "XPACK_SECURITY_ENABLED=") || strings.HasPrefix(e, "xpack.security.enabled=") {
				hasXpack = true
			}
			if strings.HasPrefix(e, "ES_JAVA_OPTS=") {
				hasJavaOpts = true
			}
			if strings.HasPrefix(e, "HEAP_SIZE=") {
				heapVal = strings.TrimPrefix(e, "HEAP_SIZE=")
			}
		}
		if !hasDiscovery {
			envs = append(envs, "discovery.type=single-node")
		}
		if !hasXpack {
			envs = append(envs, "xpack.security.enabled=false")
		}
		if !hasJavaOpts {
			envs = append(envs, fmt.Sprintf("ES_JAVA_OPTS=-Xms%s -Xmx%s", heapVal, heapVal))
		}
	}

	// Fallbacks if no portals identified
	if len(portals) == 0 && app.DefaultPort > 0 {
		portals = append(portals, fmt.Sprintf("  - name: \"Web UI\"\n    port: %d\n    scheme: \"http\"\n    path: \"/\"", app.DefaultPort))
	} else if len(portals) == 0 && len(ports) == 1 {
		hPort, _ := parsePortMapping(ports[0])
		if p, err := strconv.Atoi(hPort); err == nil && p > 0 {
			portals = append(portals, fmt.Sprintf("  - name: \"Web UI\"\n    port: %d\n    scheme: \"http\"\n    path: \"/\"", p))
		}
	}
	if len(volumes) == 0 {
		defaultMount := resolveContainerMountPath(serviceName, "data", nil, nil)
		volumes = append(volumes, fmt.Sprintf("%s/%s/data:%s", appdataPath, serviceName, defaultMount))
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

func escapeComposeEnvVal(val string) string {
	if val == "" {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(val); i++ {
		if val[i] == '$' {
			if i+1 < len(val) && val[i+1] == '$' {
				sb.WriteString("$$")
				i++
				continue
			}
			if i+1 < len(val) && val[i+1] == '{' {
				closeIdx := strings.IndexByte(val[i+2:], '}')
				if closeIdx != -1 {
					sb.WriteString(val[i : i+2+closeIdx+1])
					i += 2 + closeIdx
					continue
				}
			}
			sb.WriteString("$$")
			continue
		}
		sb.WriteByte(val[i])
	}
	return sb.String()
}

// GetCuratedQuestions provides standard questions for apps when questions.yaml is unavailable.
func GetCuratedQuestions(app domain.CatalogApp, appsPool, appdataPath string) ([]domain.CatalogQuestionGroup, []domain.CatalogQuestion) {
	if appsPool == "" {
		appsPool = "tank"
	}
	if appdataPath == "" {
		appdataPath = fmt.Sprintf("/mnt/%s/apps", appsPool)
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

func isWebPortalVariable(v, label string) bool {
	vLower := strings.ToLower(v)
	lLower := strings.ToLower(label)

	// Explicit non-web port patterns (torrent, peer, dns, dhcp, metrics, exporter, sync, cluster, vpn, etc.)
	nonWebPatterns := []string{
		"bt_port", "torrent", "peer", "dns", "dhcp", "metrics",
		"prometheus", "exporter", "sync", "cluster", "member",
		"wireguard", "vpn", "tunnel", "rtp", "sip", "tftp",
	}
	for _, p := range nonWebPatterns {
		if strings.Contains(vLower, p) || strings.Contains(lLower, p) {
			return false
		}
	}

	// Web portal patterns
	webPatterns := []string{
		"web_port", "webui", "web_ui", "portal", "http", "https",
		"admin", "dashboard", "gui", "ui_port", "frontend",
	}
	for _, p := range webPatterns {
		if strings.Contains(vLower, p) || strings.Contains(lLower, p) {
			return true
		}
	}

	if strings.Contains(lLower, "web ui") || strings.Contains(lLower, "web portal") || strings.Contains(lLower, "web interface") {
		return true
	}

	return false
}

func parsePortMapping(p string) (hostPort, containerPort string) {
	p = strings.Trim(p, `"`)
	if idx := strings.Index(p, "/"); idx != -1 {
		p = p[:idx]
	}
	parts := strings.Split(p, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return p, p
}
