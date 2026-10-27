package docker

import (
	"regexp"
	"strings"
)

var (
	// Layer progress lines: "6b37362b3da7 Downloading [===>   ] 1MB/2MB", "... Pull complete", etc.
	composeLayerProgressRe = regexp.MustCompile(`^\s*[0-9a-f]{12}\s+(Pulling fs layer|Waiting|Downloading|Verifying Checksum|Download complete|Extracting|Pull complete|Already exists|Retrying)\b`)
	// Resource status lines: "npm Pulling", "Container x Creating", "Network y Created", ...
	composeStatusRe = regexp.MustCompile(`^\s*(Container|Network|Volume|Image)?\s*\S+\s+(Pulling|Pulled|Creating|Created|Starting|Started|Running|Recreate|Recreated|Stopping|Stopped|Removing|Removed|Healthy|Waiting|Built|Building)\s*$`)
)

const maxComposeErrorDetails = 4000

// filterComposeOutput strips docker compose progress noise (layer pulls, status transitions)
// and returns only the lines that are likely to explain a failure.
func filterComposeOutput(raw string) string {
	raw = strings.ReplaceAll(raw, "\r", "\n")
	seen := make(map[string]struct{})
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if composeLayerProgressRe.MatchString(line) || composeStatusRe.MatchString(line) {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")
	if len(out) > maxComposeErrorDetails {
		out = "..." + out[len(out)-maxComposeErrorDetails:]
	}
	return out
}

// isComposeCanceled reports whether the failure was caused by an intentional interrupt/cancel
// (SIGINT exit 130, SIGTERM exit 143, or context cancellation) rather than a real error.
func isComposeCanceled(err error, details string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "exit status 130") || strings.Contains(msg, "exit status 143") ||
		strings.Contains(msg, "signal: killed") || strings.Contains(msg, "context canceled") {
		return true
	}
	return strings.TrimSpace(details) == "canceled"
}
