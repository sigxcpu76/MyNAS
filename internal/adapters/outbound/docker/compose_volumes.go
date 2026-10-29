package docker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NormalizeComposeVolumes rewrites TrueNAS volume descriptors, filters out setting artifacts,
// and normalizes all volume mounts into valid host path bindings under the NAS pool.
func NormalizeComposeVolumes(appID, pool, composeYAML string) string {
	if strings.TrimSpace(composeYAML) == "" {
		return composeYAML
	}

	if pool == "" {
		pool = "tank"
	}
	pool = strings.TrimPrefix(pool, "zfsPools/")
	cleanAppID := appID
	if cleanAppID == "" {
		cleanAppID = "app"
	}
	appdataPath := fmt.Sprintf("/mnt/%s/apps/%s", pool, cleanAppID)

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeYAML), &root); err != nil {
		return composeYAML
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return composeYAML
	}

	docMap := root.Content[0]
	var servicesNode *yaml.Node
	var volumesNodeIdx = -1

	for i := 0; i < len(docMap.Content); i += 2 {
		key := docMap.Content[i].Value
		if key == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
			servicesNode = docMap.Content[i+1]
		} else if key == "volumes" {
			volumesNodeIdx = i
		}
	}

	if servicesNode == nil {
		return composeYAML
	}

	// isSettingTarget checks whether a mount target is an internal TrueNAS setting rather than a real directory
	isSettingTarget := func(target string) bool {
		clean := strings.TrimPrefix(strings.TrimSpace(target), "/")
		switch clean {
		case "type", "acl_enable", "dataset_name", "acl_entries", "cifs_config", "nfs_config", "read_only", "host_path_config", "ix_volume_config":
			return true
		default:
			return false
		}
	}

	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcMap := servicesNode.Content[i]
		if svcMap.Kind != yaml.MappingNode {
			continue
		}

		svcName := ""
		if i-1 >= 0 && i-1 < len(servicesNode.Content) {
			svcName = servicesNode.Content[i-1].Value
		}
		image := ""
		for k := 0; k < len(svcMap.Content); k += 2 {
			if svcMap.Content[k].Value == "image" && k+1 < len(svcMap.Content) {
				image = svcMap.Content[k+1].Value
			}
		}

		isElastic := cleanAppID == "elastic-search" || strings.Contains(cleanAppID, "elasticsearch") ||
			svcName == "elastic-search" || strings.Contains(svcName, "elasticsearch") ||
			strings.Contains(image, "elasticsearch")

		if isElastic {
			// 1. Fix ports if TrueNAS template or fallback defaulted target port to 8080 instead of 9200
			for j := 0; j < len(svcMap.Content); j += 2 {
				if svcMap.Content[j].Value == "ports" && svcMap.Content[j+1].Kind == yaml.SequenceNode {
					for _, pNode := range svcMap.Content[j+1].Content {
						if pNode.Kind == yaml.ScalarNode {
							v := pNode.Value
							if strings.HasSuffix(v, ":8080") {
								pNode.Value = strings.TrimSuffix(v, ":8080") + ":9200"
							} else if v == "8080" {
								pNode.Value = "9200"
							}
						}
					}
				}
			}

			// 2. Ensure single-node discovery, disabled xpack security (for standalone), and valid ES_JAVA_OPTS
			var envSeq *yaml.Node
			for j := 0; j < len(svcMap.Content); j += 2 {
				if svcMap.Content[j].Value == "environment" {
					envSeq = svcMap.Content[j+1]
					break
				}
			}

			if envSeq == nil || envSeq.Kind != yaml.SequenceNode {
				newEnv := &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "discovery.type=single-node"},
						{Kind: yaml.ScalarNode, Value: "xpack.security.enabled=false"},
						{Kind: yaml.ScalarNode, Value: "ES_JAVA_OPTS=-Xms512m -Xmx512m"},
					},
				}
				svcMap.Content = append(svcMap.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "environment"}, newEnv)
			} else {
				hasDiscovery := false
				hasXpack := false
				hasJavaOpts := false
				heapSize := ""
				for _, eNode := range envSeq.Content {
					if eNode.Kind == yaml.ScalarNode {
						val := eNode.Value
						if strings.HasPrefix(val, "discovery.type") || strings.HasPrefix(val, "ES_SETTING_DISCOVERY_TYPE") {
							hasDiscovery = true
						}
						if strings.HasPrefix(val, "xpack.security.enabled") || strings.HasPrefix(val, "ES_SETTING_XPACK_SECURITY_ENABLED") {
							hasXpack = true
						}
						if strings.HasPrefix(val, "ES_JAVA_OPTS") {
							hasJavaOpts = true
						}
						if strings.HasPrefix(val, "HEAP_SIZE=") {
							heapSize = strings.TrimPrefix(val, "HEAP_SIZE=")
						} else if strings.HasPrefix(val, "ES_HEAP_SIZE=") {
							heapSize = strings.TrimPrefix(val, "ES_HEAP_SIZE=")
						}
					}
				}
				if !hasDiscovery {
					envSeq.Content = append(envSeq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "discovery.type=single-node"})
				}
				if !hasXpack {
					envSeq.Content = append(envSeq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "xpack.security.enabled=false"})
				}
				if !hasJavaOpts {
					if heapSize == "" {
						heapSize = "512m"
					}
					envSeq.Content = append(envSeq.Content, &yaml.Node{
						Kind:  yaml.ScalarNode,
						Value: fmt.Sprintf("ES_JAVA_OPTS=-Xms%s -Xmx%s", heapSize, heapSize),
					})
				}
			}
		}

		for j := 0; j < len(svcMap.Content); j += 2 {
			if svcMap.Content[j].Value != "volumes" {
				continue
			}

			volSeq := svcMap.Content[j+1]
			if volSeq.Kind != yaml.SequenceNode {
				continue
			}

			var cleaned []*yaml.Node
			var seen = make(map[string]bool)

			for _, vNode := range volSeq.Content {
				if vNode.Kind != yaml.ScalarNode {
					cleaned = append(cleaned, vNode)
					continue
				}

				rawVal := strings.TrimSpace(vNode.Value)
				parts := strings.Split(rawVal, ":")
				source := strings.TrimSpace(parts[0])
				target := ""
				mode := ""
				if len(parts) > 1 {
					target = strings.TrimSpace(parts[1])
				}
				if len(parts) > 2 {
					mode = strings.TrimSpace(parts[2])
				}

				// 1. Drop settings mistakenly serialized as volumes
				if source == "true" || source == "false" {
					continue
				}
				if isSettingTarget(target) {
					continue
				}

				// 2. Check if source is a TrueNAS volume reference (ix_volume, ix-volume)
				isIxVolume := (source == "ix_volume" || source == "ix-volume" || strings.HasPrefix(source, "ix_volume"))

				// 3. Handle TrueNAS bug where target was defaulted to /path
				if target == "/path" && strings.HasSuffix(source, "/path") {
					if cleanAppID == "elastic-search" || strings.Contains(cleanAppID, "elasticsearch") {
						target = "/usr/share/elasticsearch/data"
					} else {
						target = "/data"
					}
					source = filepath.Join(filepath.Dir(source), "data")
				}

				// 4. Rewrite TrueNAS ix_volume reference into NAS host path
				if isIxVolume {
					leaf := "data"
					if target != "" {
						tParts := strings.Split(strings.Trim(target, "/"), "/")
						if len(tParts) > 0 && tParts[len(tParts)-1] != "" {
							leaf = tParts[len(tParts)-1]
						}
					}
					source = fmt.Sprintf("%s/%s", appdataPath, leaf)
				} else if !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "~") {
					// Relative or named volume without host path (e.g. "data:/var/lib/...")
					// Rewrite to NAS persistent app storage
					source = fmt.Sprintf("%s/%s", appdataPath, source)
				}

				// 5. Rewrite legacy /tank/, /mnt/tank/, /nas/, /mnt/nas/ prefixes to /mnt/<pool>/
				if strings.HasPrefix(source, "/tank/") {
					source = strings.Replace(source, "/tank/", fmt.Sprintf("/mnt/%s/", pool), 1)
				} else if strings.HasPrefix(source, "/nas/") {
					source = strings.Replace(source, "/nas/", fmt.Sprintf("/mnt/%s/", pool), 1)
				} else if strings.HasPrefix(source, "/mnt/") {
					parts := strings.Split(strings.TrimPrefix(source, "/mnt/"), "/")
					if len(parts) > 0 && parts[0] != pool && (parts[0] == "tank" || parts[0] == "nas" || parts[0] == "mypool") {
						source = fmt.Sprintf("/mnt/%s/%s", pool, strings.Join(parts[1:], "/"))
					}
				}

				if target == "" {
					continue
				}

				// Ensure target mount path is absolute to prevent Docker daemon rejection
				if !strings.HasPrefix(target, "/") {
					target = "/" + target
				}

				newVal := fmt.Sprintf("%s:%s", source, target)
				if mode != "" {
					newVal = fmt.Sprintf("%s:%s:%s", source, target, mode)
				}

				if !seen[newVal] {
					seen[newVal] = true
					vNode.Value = newVal
					cleaned = append(cleaned, vNode)
				}
			}

			// Filter out generic fallback appdataPath:/data if a more specific mount under appdataPath exists
			hasSpecificMount := false
			for _, v := range cleaned {
				if v.Kind == yaml.ScalarNode {
					p := strings.Split(v.Value, ":")
					if len(p) >= 2 && strings.HasPrefix(p[0], appdataPath+"/") {
						hasSpecificMount = true
						break
					}
				}
			}
			if hasSpecificMount {
				var filtered []*yaml.Node
				for _, v := range cleaned {
					if v.Kind == yaml.ScalarNode {
						p := strings.Split(v.Value, ":")
						if len(p) >= 2 && p[0] == appdataPath && p[1] == "/data" {
							continue
						}
					}
					filtered = append(filtered, v)
				}
				cleaned = filtered
			}

			// If all volumes were dropped (they were all TrueNAS settings), inject proper app data volume
			if len(cleaned) == 0 {
				defaultTarget := "/data"
				if cleanAppID == "elastic-search" || strings.Contains(cleanAppID, "elasticsearch") {
					defaultTarget = "/usr/share/elasticsearch/data"
				}
				defaultMount := fmt.Sprintf("%s/data:%s", appdataPath, defaultTarget)
				cleaned = append(cleaned, &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: defaultMount,
				})
			}

			volSeq.Content = cleaned
		}
	}

	// If top-level volumes exists, clean up any TrueNAS setting keys
	if volumesNodeIdx != -1 && volumesNodeIdx+1 < len(docMap.Content) {
		topVols := docMap.Content[volumesNodeIdx+1]
		if topVols.Kind == yaml.MappingNode {
			var keptContent []*yaml.Node
			for k := 0; k < len(topVols.Content); k += 2 {
				kName := topVols.Content[k].Value
				if kName == "ix_volume" || kName == "ix-volume" || kName == "false" || kName == "true" || isSettingTarget(kName) {
					continue
				}
				keptContent = append(keptContent, topVols.Content[k], topVols.Content[k+1])
			}
			topVols.Content = keptContent
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return composeYAML
	}
	_ = enc.Close()

	res := buf.String()
	res = strings.TrimPrefix(res, "---\n")
	return res
}

// EnsureVolumeHostPaths parses compose YAML and ensures all host paths mounted into
// containers exist on disk and have permissive read/write permissions for non-root containers.
func EnsureVolumeHostPaths(composeYAML string) {
	if strings.TrimSpace(composeYAML) == "" {
		return
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeYAML), &root); err != nil {
		return
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return
	}
	docMap := root.Content[0]
	var servicesNode *yaml.Node
	for i := 0; i < len(docMap.Content); i += 2 {
		if docMap.Content[i].Value == "services" && docMap.Content[i+1].Kind == yaml.MappingNode {
			servicesNode = docMap.Content[i+1]
			break
		}
	}
	if servicesNode == nil {
		return
	}
	for i := 1; i < len(servicesNode.Content); i += 2 {
		svcMap := servicesNode.Content[i]
		if svcMap.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(svcMap.Content); j += 2 {
			if svcMap.Content[j].Value == "volumes" && svcMap.Content[j+1].Kind == yaml.SequenceNode {
				for _, vNode := range svcMap.Content[j+1].Content {
					if vNode.Kind != yaml.ScalarNode {
						continue
					}
					parts := strings.Split(strings.TrimSpace(vNode.Value), ":")
					if len(parts) >= 2 {
						src := strings.TrimSpace(parts[0])
						if strings.HasPrefix(src, "/") {
							if err := os.MkdirAll(src, 0777); err == nil {
								_ = os.Chmod(src, 0777)
							}
						}
					}
				}
			}
		}
	}
}
