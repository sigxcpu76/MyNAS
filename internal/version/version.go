package version

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	// Version is populated at build time via -ldflags:
	// -X github.com/sigxcpu/nas/internal/version.Version=v1.6.10
	Version   = "v1.6.10-dev"
	GitCommit = "HEAD"
	BuildDate = "unknown"
)

// Info holds full build and runtime version metadata.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo returns the current build and runtime information.
func GetInfo() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("nasctl %s (%s, %s, %s/%s)", i.Version, i.GitCommit, i.BuildDate, i.OS, i.Arch)
}

// CleanTag standardizes version strings, removing leading 'v' and trimming spaces.
func CleanTag(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

var semverRegex = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?`)

// Compare compares two semantic versions (e.g. "v1.5.0" and "v1.6.0" or "1.5").
// Returns:
//   -1 if v1 < v2
//    0 if v1 == v2
//    1 if v1 > v2
func Compare(v1, v2 string) int {
	clean1 := CleanTag(v1)
	clean2 := CleanTag(v2)

	if clean1 == clean2 {
		return 0
	}

	m1 := semverRegex.FindStringSubmatch(clean1)
	m2 := semverRegex.FindStringSubmatch(clean2)

	if m1 == nil && m2 == nil {
		return strings.Compare(clean1, clean2)
	}
	if m1 == nil {
		return -1
	}
	if m2 == nil {
		return 1
	}

	for i := 1; i <= 3; i++ {
		num1 := 0
		if i < len(m1) && m1[i] != "" {
			num1, _ = strconv.Atoi(m1[i])
		}
		num2 := 0
		if i < len(m2) && m2[i] != "" {
			num2, _ = strconv.Atoi(m2[i])
		}
		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}

	// Check pre-release suffix (e.g. "-dev", "-rc1")
	// If one has pre-release and the other does not, the one without pre-release is newer
	pre1 := ""
	if len(m1) > 4 {
		pre1 = m1[4]
	}
	pre2 := ""
	if len(m2) > 4 {
		pre2 = m2[4]
	}

	if pre1 == "" && pre2 != "" {
		return 1
	}
	if pre1 != "" && pre2 == "" {
		return -1
	}
	return strings.Compare(pre1, pre2)
}

// IsNewer returns true if candidate is strictly newer than current.
func IsNewer(candidate, current string) bool {
	return Compare(candidate, current) > 0
}
