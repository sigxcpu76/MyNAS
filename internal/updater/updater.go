package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/version"
)

const (
	DefaultGitHubRepo = "sigxcpu76/MyNAS"
	DefaultCacheTTL   = 10 * time.Minute
)

// GitHubAsset represents a downloadable release file from GitHub Releases API.
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GitHubRelease represents a release object from GitHub Releases API.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// CheckResult represents the outcome of an update check.
type CheckResult struct {
	CurrentVersion   string    `json:"current_version"`
	LatestVersion    string    `json:"latest_version"`
	UpdateAvailable  bool      `json:"update_available"`
	ReleaseURL       string    `json:"release_url"`
	ReleaseNotes     string    `json:"release_notes"`
	PublishedAt      time.Time `json:"published_at"`
	DownloadURL      string    `json:"download_url"`
	BinarySizeBytes  int64     `json:"binary_size_bytes"`
	ExpectedSHA256   string    `json:"expected_sha256"`
	CurrentCommit    string    `json:"current_commit"`
	CurrentBuildDate string    `json:"current_build_date"`
	BinaryPath       string    `json:"binary_path"`
}

// ApplyResult represents the result of applying an update.
type ApplyResult struct {
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	UpdatedVersion    string `json:"updated_version"`
	PreviousVersion   string `json:"previous_version"`
	ServiceRestarting bool   `json:"service_restarting"`
}

// Updater handles OTA update checks and in-place atomic application.
type Updater struct {
	repo       string
	httpClient *http.Client
	apiBaseURL string

	mu         sync.Mutex
	lastCheck  time.Time
	cachedResp *CheckResult
}

// New creates an Updater for the specified repo (e.g. "sigxcpu76/MyNAS").
func New(repo string) *Updater {
	if repo == "" {
		repo = DefaultGitHubRepo
	}
	return &Updater{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		apiBaseURL: "https://api.github.com",
	}
}

// Check queries GitHub Releases to determine if an update is available.
func (u *Updater) Check(ctx context.Context, forceRefresh bool, targetVersion string) (*CheckResult, error) {
	u.mu.Lock()
	if !forceRefresh && targetVersion == "" && u.cachedResp != nil && time.Since(u.lastCheck) < DefaultCacheTTL {
		cached := *u.cachedResp
		u.mu.Unlock()
		return &cached, nil
	}
	u.mu.Unlock()

	var rel *GitHubRelease
	var err error

	if targetVersion != "" {
		rel, err = u.fetchReleaseByTag(ctx, targetVersion)
	} else {
		rel, err = u.fetchLatestRelease(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release info: %w", err)
	}

	execPath, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	curVer := version.Version
	latestVer := rel.TagName
	updateAvailable := false
	if targetVersion != "" {
		updateAvailable = version.CleanTag(curVer) != version.CleanTag(targetVersion)
	} else {
		updateAvailable = version.IsNewer(latestVer, curVer)
	}

	// Locate candidate assets for current GOOS and GOARCH
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	targetAsset, archiveAsset, shaAsset := findMatchingAssets(rel.Assets, goos, goarch)

	downloadURL := ""
	var binarySize int64
	if archiveAsset != nil {
		downloadURL = archiveAsset.BrowserDownloadURL
		binarySize = archiveAsset.Size
	} else if targetAsset != nil {
		downloadURL = targetAsset.BrowserDownloadURL
		binarySize = targetAsset.Size
	}

	expectedSHA := ""
	if shaAsset != nil {
		if hash, err := u.fetchSHA256(ctx, shaAsset.BrowserDownloadURL); err == nil {
			expectedSHA = hash
		}
	}

	res := &CheckResult{
		CurrentVersion:   curVer,
		LatestVersion:    latestVer,
		UpdateAvailable:  updateAvailable,
		ReleaseURL:       rel.HTMLURL,
		ReleaseNotes:     rel.Body,
		PublishedAt:      rel.PublishedAt,
		DownloadURL:      downloadURL,
		BinarySizeBytes:  binarySize,
		ExpectedSHA256:   expectedSHA,
		CurrentCommit:    version.GitCommit,
		CurrentBuildDate: version.BuildDate,
		BinaryPath:       execPath,
	}

	u.mu.Lock()
	if targetVersion == "" {
		u.lastCheck = time.Now()
		u.cachedResp = res
	}
	u.mu.Unlock()

	return res, nil
}

// Apply downloads, verifies, atomically installs the update, and triggers a service restart.
func (u *Updater) Apply(ctx context.Context, targetVersion string, restartService bool) (*ApplyResult, error) {
	checkRes, err := u.Check(ctx, true, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("update check failed: %w", err)
	}

	if checkRes.DownloadURL == "" {
		return nil, fmt.Errorf("no compatible release asset found for %s/%s in release %s",
			runtime.GOOS, runtime.GOARCH, checkRes.LatestVersion)
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not determine running binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	binDir := filepath.Dir(execPath)
	tempPath := filepath.Join(binDir, fmt.Sprintf(".nasctl-%d.update", time.Now().UnixNano()))
	backupPath := filepath.Join(binDir, "nasctl.bak")

	slog.Info("[updater] starting binary download",
		"target_version", checkRes.LatestVersion,
		"url", checkRes.DownloadURL,
		"target_path", tempPath)

	// Download to temp file and compute checksum
	computedSHA, err := u.downloadAndExtract(ctx, checkRes.DownloadURL, tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("download/extraction failed: %w", err)
	}

	// Verify SHA256 checksum if one was provided in the release
	if checkRes.ExpectedSHA256 != "" {
		expected := strings.ToLower(strings.TrimSpace(checkRes.ExpectedSHA256))
		if computedSHA != expected {
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", expected, computedSHA)
		}
		slog.Info("[updater] binary checksum verified successfully", "sha256", computedSHA)
	}

	// Set executable bit
	if err := os.Chmod(tempPath, 0755); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("failed to make downloaded binary executable: %w", err)
	}

	// Pre-flight validation: run the new binary with -h to verify ELF architecture and dependencies
	testCtx, testCancel := context.WithTimeout(ctx, 8*time.Second)
	defer testCancel()
	cmd := exec.CommandContext(testCtx, tempPath, "-h")
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("new binary failed pre-flight verification: %v (output: %s)", err, strings.TrimSpace(string(out)))
	}
	slog.Info("[updater] pre-flight execution test passed successfully")

	// Create backup of current binary
	if err := copyFile(execPath, backupPath); err != nil {
		slog.Warn("[updater] could not create backup of existing binary", "path", backupPath, "err", err)
	} else {
		_ = os.Chmod(backupPath, 0755)
		slog.Info("[updater] created backup of current binary", "path", backupPath)
	}

	// Atomic rename over running binary
	if err := os.Rename(tempPath, execPath); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("failed to atomic replace %s: %w", execPath, err)
	}
	slog.Info("[updater] atomic binary replacement complete", "path", execPath, "version", checkRes.LatestVersion)

	// Trigger restart if requested
	if restartService {
		go func() {
			// Delay slightly to allow the HTTP response to be flushed to the client
			time.Sleep(1 * time.Second)
			TriggerRestart()
		}()
	}

	return &ApplyResult{
		Success:           true,
		Message:           fmt.Sprintf("Successfully updated nasctl to %s. Service is restarting.", checkRes.LatestVersion),
		UpdatedVersion:    checkRes.LatestVersion,
		PreviousVersion:   checkRes.CurrentVersion,
		ServiceRestarting: restartService,
	}, nil
}

// TriggerRestart gracefully restarts the nasctl system service.
func TriggerRestart() {
	slog.Info("[updater] triggering service restart...")

	// 1. Try systemctl restart nasctl
	cmd := exec.Command("systemctl", "restart", "nasctl")
	if err := cmd.Run(); err == nil {
		slog.Info("[updater] systemctl restart nasctl succeeded")
		return
	}

	// 2. If running under systemd or supervisor with Restart=always, exit 0
	slog.Info("[updater] exiting process so systemd supervisor restarts it with new binary")
	os.Exit(0)
}

// downloadAndExtract downloads a release URL and writes the final uncompressed binary to destPath.
func (u *Updater) downloadAndExtract(ctx context.Context, downloadURL, destPath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nasctl-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d downloading asset", resp.StatusCode)
	}

	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)

	isTarGz := strings.HasSuffix(downloadURL, ".tar.gz") || strings.HasSuffix(downloadURL, ".tgz")

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if isTarGz {
		gzr, err := gzip.NewReader(tee)
		if err != nil {
			return "", fmt.Errorf("failed to open gzip stream: %w", err)
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		found := false
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("tar reading error: %w", err)
			}
			if hdr.Typeflag == tar.TypeReg && (strings.Contains(hdr.Name, "nasctl") || strings.HasSuffix(hdr.Name, "nasctl")) {
				if _, err := io.Copy(out, tr); err != nil {
					return "", fmt.Errorf("failed to extract binary from tar: %w", err)
				}
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("no 'nasctl' executable found inside release tarball")
		}

		// Drain remaining gzip stream to finish checksum computation
		_, _ = io.Copy(io.Discard, gzr)
		_, _ = io.Copy(io.Discard, tee)
	} else {
		if _, err := io.Copy(out, tee); err != nil {
			return "", fmt.Errorf("failed to save binary: %w", err)
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (u *Updater) fetchLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBaseURL, u.repo)
	return u.doReleaseRequest(ctx, url)
}

func (u *Updater) fetchReleaseByTag(ctx context.Context, tag string) (*GitHubRelease, error) {
	clean := strings.TrimSpace(tag)
	if !strings.HasPrefix(clean, "v") && !strings.HasPrefix(clean, "V") {
		clean = "v" + clean
	}
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", u.apiBaseURL, u.repo, clean)
	return u.doReleaseRequest(ctx, url)
}

func (u *Updater) doReleaseRequest(ctx context.Context, url string) (*GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "nasctl-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("release not found (HTTP 404)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode release JSON: %w", err)
	}
	return &rel, nil
}

func (u *Updater) fetchSHA256(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nasctl-updater")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching sha256", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return "", err
	}

	line := strings.TrimSpace(buf.String())
	parts := strings.Fields(line)
	if len(parts) > 0 {
		return strings.ToLower(parts[0]), nil
	}
	return strings.ToLower(line), nil
}

// findMatchingAssets finds the matching binary, archive, and sha256 files for the given GOOS/GOARCH.
func findMatchingAssets(assets []GitHubAsset, goos, goarch string) (raw *GitHubAsset, archive *GitHubAsset, sha *GitHubAsset) {
	targetPattern := fmt.Sprintf("%s-%s", goos, goarch)

	for i := range assets {
		a := &assets[i]
		name := strings.ToLower(a.Name)

		if !strings.Contains(name, targetPattern) {
			continue
		}

		if strings.HasSuffix(name, ".sha256") || strings.HasSuffix(name, ".sha256sum") {
			sha = a
		} else if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			archive = a
		} else if !strings.HasSuffix(name, ".iso") && !strings.HasSuffix(name, ".apk") && !strings.HasSuffix(name, ".md") {
			raw = a
		}
	}
	return
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
