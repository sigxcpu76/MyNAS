package docker

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// EscapeComposeEnvValue escapes literal dollar signs ($) in an environment variable value
// to ($$) so Docker Compose passes them verbatim to the container instead of attempting
// interpolation and corrupting passwords, tokens, or hashes.
// Valid compose interpolations like ${VAR} or ${VAR:-default} are preserved.
func EscapeComposeEnvValue(val string) string {
	if val == "" {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(val); i++ {
		if val[i] == '$' {
			// Already escaped as $$? Keep as $$
			if i+1 < len(val) && val[i+1] == '$' {
				sb.WriteString("$$")
				i++
				continue
			}
			// Braced interpolation like ${VAR} or ${VAR:-default}? Preserve as is
			if i+1 < len(val) && val[i+1] == '{' {
				closeIdx := strings.IndexByte(val[i+2:], '}')
				if closeIdx != -1 {
					fullVar := val[i : i+2+closeIdx+1]
					sb.WriteString(fullVar)
					i += 2 + closeIdx
					continue
				}
			}
			// Single unescaped $: escape to $$
			sb.WriteString("$$")
			continue
		}
		sb.WriteByte(val[i])
	}
	return sb.String()
}

// EscapeComposeEnvString escapes an environment entry in "KEY=VAL" format.
func EscapeComposeEnvString(entry string) string {
	idx := strings.IndexByte(entry, '=')
	if idx == -1 {
		return entry
	}
	return entry[:idx+1] + EscapeComposeEnvValue(entry[idx+1:])
}

// NormalizeComposeEnvironment inspects Compose YAML and ensures all environment variable
// values have bare literal dollar signs safely escaped to $$ so Docker Compose does not wipe them.
func NormalizeComposeEnvironment(rawYAML string) string {
	trimmed := strings.TrimSpace(rawYAML)
	if trimmed == "" {
		return rawYAML
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &doc); err != nil || doc.Kind == 0 {
		return rawYAML
	}

	modified := false
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				v := n.Content[i+1]
				if k.Value == "environment" {
					if v.Kind == yaml.SequenceNode {
						for _, item := range v.Content {
							if item.Kind == yaml.ScalarNode {
								escaped := EscapeComposeEnvString(item.Value)
								if escaped != item.Value {
									item.Value = escaped
									modified = true
								}
							}
						}
					} else if v.Kind == yaml.MappingNode {
						for j := 1; j < len(v.Content); j += 2 {
							valNode := v.Content[j]
							if valNode.Kind == yaml.ScalarNode {
								escaped := EscapeComposeEnvValue(valNode.Value)
								if escaped != valNode.Value {
									valNode.Value = escaped
									modified = true
								}
							}
						}
					}
				} else {
					walk(v)
				}
			}
		} else {
			for _, child := range n.Content {
				walk(child)
			}
		}
	}

	walk(&doc)
	if !modified {
		return rawYAML
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return rawYAML
	}
	_ = enc.Close()
	return buf.String()
}
