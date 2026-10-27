package docker

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// prettifyComposeYAML normalizes compose content for display/editing: it strips leading blank
// lines and converts flow-style/JSON documents (as emitted by TrueNAS rendered templates)
// into readable block-style YAML. Key order is preserved. On any parse failure the trimmed
// input is returned unchanged.
func prettifyComposeYAML(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &doc); err != nil || doc.Kind == 0 {
		return trimmed + "\n"
	}
	if !hasFlowStyle(&doc) {
		return trimmed + "\n"
	}
	clearFlowStyle(&doc)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return trimmed + "\n"
	}
	_ = enc.Close()
	return buf.String()
}

func hasFlowStyle(n *yaml.Node) bool {
	if n.Style&yaml.FlowStyle != 0 {
		return true
	}
	for _, c := range n.Content {
		if hasFlowStyle(c) {
			return true
		}
	}
	return false
}

func clearFlowStyle(n *yaml.Node) {
	n.Style &^= yaml.FlowStyle
	// JSON strings are double-quoted; keep quoting only where YAML requires it.
	if n.Kind == yaml.ScalarNode && n.Style&yaml.DoubleQuotedStyle != 0 && n.Tag == "!!str" {
		n.Style &^= yaml.DoubleQuotedStyle
	}
	for _, c := range n.Content {
		clearFlowStyle(c)
	}
}
