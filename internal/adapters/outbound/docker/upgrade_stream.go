package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
	"gopkg.in/yaml.v3"
)

type dockerPullProgressEvent struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorDetail *struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
	ProgressDetail *struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
}

// UpgradeAppStream performs a synchronous upgrade of the Docker Compose stack while
// streaming incremental progress updates (snapshot, image layer download bytes, container recreation).
func (a *Adapter) UpgradeAppStream(
	ctx context.Context,
	id string,
	takeSnapshot bool,
	onProgress domain.DockerAppUpgradeProgressCallback,
) (*domain.DockerApp, error) {
	emit := func(p domain.DockerAppUpgradeProgress) {
		if onProgress != nil {
			onProgress(p)
		}
	}

	appDir := filepath.Join(a.unitsDir, id)
	composePath := filepath.Join(appDir, "compose.yaml")

	a.mu.Lock()
	if _, err := os.Stat(composePath); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("app %q not found or has no compose.yaml", id)
	}
	if !a.hasSocket {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker daemon socket %s is not available", a.socketPath)
	}
	if !a.hasCompose {
		a.mu.Unlock()
		return nil, fmt.Errorf("docker compose CLI is not available on host")
	}
	if _, busy := a.deployingApps[id]; busy {
		a.mu.Unlock()
		return nil, fmt.Errorf("app %q already has an upgrade or deployment in progress", id)
	}

	content, err := os.ReadFile(composePath)
	if err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to read compose.yaml: %w", err)
	}

	// Normalize compose volumes & environment to ensure health before upgrade.
	normalized := a.NormalizeComposeVolumes(id, string(content))
	normalized = NormalizeComposeEnvironment(normalized)
	if normalized != string(content) {
		_ = os.WriteFile(composePath, []byte(normalized), 0644)
		content = []byte(normalized)
	}

	_, _, vols, _, _, _, _, _, _, _, _ := parseComposeYAML(string(content))

	if a.deployCancel == nil {
		a.deployCancel = make(map[string]context.CancelFunc)
	}
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	a.deployingApps[id] = "Upgrading stack (pulling latest images & restarting containers)..."
	a.deployCancel[id] = cancel
	delete(a.deployingErrors, id)
	a.mu.Unlock()

	defer cancel()

	// Notify watchers that the app entered DEPLOYING state.
	go a.emitEvent(domain.NewEvent("docker", "MODIFIED", "dockerApps/"+id, "Upgrading stack..."))

	emit(domain.DockerAppUpgradeProgress{
		Stage:   "preparing",
		Message: "Preparing stack upgrade...",
		Percent: 2,
	})

	if takeSnapshot {
		emit(domain.DockerAppUpgradeProgress{
			Stage:   "snapshot",
			Message: "Creating pre-upgrade ZFS snapshot...",
			Percent: 5,
		})
		a.snapshotAppVolumes(bgCtx, id, vols)
		emit(domain.DockerAppUpgradeProgress{
			Stage:   "snapshot",
			Message: "Pre-upgrade ZFS snapshot complete",
			Percent: 10,
		})
	}

	// Step 1: Pull images with byte-level progress via Docker Engine API, falling back to `docker compose pull` if needed.
	images := a.extractStackImages(bgCtx, appDir, id, string(content))
	totalImages := len(images)
	if totalImages > 0 {
		pullFallbackNeeded := false
		for idx, img := range images {
			basePct := 10 + (idx*75)/totalImages
			emit(domain.DockerAppUpgradeProgress{
				Stage:           "pulling",
				Message:         fmt.Sprintf("Pulling %s (%d/%d)...", img, idx+1, totalImages),
				Percent:         basePct,
				CurrentImage:    img,
				CompletedImages: idx,
				TotalImages:     totalImages,
			})

			pullErr := a.pullImageWithProgress(bgCtx, img, func(curBytes, totBytes int64) {
				imgFrac := 0.0
				if totBytes > 0 {
					imgFrac = float64(curBytes) / float64(totBytes)
					if imgFrac > 1.0 {
						imgFrac = 1.0
					}
				}
				pct := 10 + int(((float64(idx)+imgFrac)/float64(totalImages))*75.0)
				if pct > 84 {
					pct = 84
				}
				emit(domain.DockerAppUpgradeProgress{
					Stage:           "pulling",
					Message:         fmt.Sprintf("Pulling %s (%d/%d)...", img, idx+1, totalImages),
					Percent:         pct,
					CurrentBytes:    curBytes,
					TotalBytes:      totBytes,
					CurrentImage:    img,
					CompletedImages: idx,
					TotalImages:     totalImages,
				})
			})
			if pullErr != nil {
				slog.Warn("[docker] direct image pull failed, falling back to docker compose pull", "app", id, "image", img, "error", pullErr)
				pullFallbackNeeded = true
				break
			}
		}

		if pullFallbackNeeded {
			emit(domain.DockerAppUpgradeProgress{
				Stage:       "pulling",
				Message:     "Pulling images via Docker Compose...",
				Percent:     40,
				TotalImages: totalImages,
			})
			if pullErr := a.runCompose(bgCtx, appDir, id, "pull"); pullErr != nil {
				a.finishUpgradeState(id, pullErr)
				emit(domain.DockerAppUpgradeProgress{
					Stage:   "error",
					Message: fmt.Sprintf("Image pull failed: %v", pullErr),
					Percent: 0,
					Done:    true,
					Error:   pullErr.Error(),
				})
				return nil, pullErr
			}
		}
	} else {
		emit(domain.DockerAppUpgradeProgress{
			Stage:   "pulling",
			Message: "Pulling latest images...",
			Percent: 25,
		})
		if pullErr := a.runCompose(bgCtx, appDir, id, "pull"); pullErr != nil {
			a.finishUpgradeState(id, pullErr)
			emit(domain.DockerAppUpgradeProgress{
				Stage:   "error",
				Message: fmt.Sprintf("Image pull failed: %v", pullErr),
				Percent: 0,
				Done:    true,
				Error:   pullErr.Error(),
			})
			return nil, pullErr
		}
	}

	// Step 2: Recreate containers with zero orphaned containers.
	emit(domain.DockerAppUpgradeProgress{
		Stage:           "recreating",
		Message:         "Recreating containers with zero orphaned containers...",
		Percent:         85,
		CompletedImages: totalImages,
		TotalImages:     totalImages,
	})

	runErr := a.runCompose(bgCtx, appDir, id, "up", "-d", "--remove-orphans")
	a.finishUpgradeState(id, runErr)

	if runErr != nil {
		if !isComposeCanceled(runErr, "") {
			emit(domain.DockerAppUpgradeProgress{
				Stage:   "error",
				Message: fmt.Sprintf("Upgrade failed: %v", runErr),
				Percent: 85,
				Done:    true,
				Error:   runErr.Error(),
			})
		}
		a.emitEvent(domain.NewEvent("docker", "MODIFIED", "dockerApps/"+id, "Upgrade failed"))
		return nil, runErr
	}

	a.mu.Lock()
	liveContainers := a.fetchLiveContainers(bgCtx)
	built, buildErr := a.buildApp(bgCtx, id, liveContainers)
	a.mu.Unlock()

	a.emitEvent(domain.NewEvent("docker", "MODIFIED", "dockerApps/"+id, "Upgrade completed"))

	emit(domain.DockerAppUpgradeProgress{
		Stage:           "completed",
		Message:         "Upgrade completed successfully",
		Percent:         100,
		CompletedImages: totalImages,
		TotalImages:     totalImages,
		Done:            true,
		App:             built,
	})

	return built, buildErr
}

func (a *Adapter) finishUpgradeState(id string, runErr error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.deployCancel, id)
	delete(a.deployingApps, id)
	if runErr != nil && !isComposeCanceled(runErr, "") {
		a.deployingErrors[id] = fmt.Sprintf("Upgrade failed: %v", runErr)
	}
}

// extractStackImages resolves the list of unique container images used by a compose stack.
func (a *Adapter) extractStackImages(ctx context.Context, unitDir, projectName, composeContent string) []string {
	if out, err := a.runComposeOutput(ctx, unitDir, projectName, "config", "--images"); err == nil {
		seen := make(map[string]bool)
		var images []string
		for _, line := range strings.Split(out, "\n") {
			img := strings.TrimSpace(line)
			if img != "" && !seen[img] {
				seen[img] = true
				images = append(images, img)
			}
		}
		if len(images) > 0 {
			return images
		}
	}
	return extractComposeImagesFromYAML(composeContent)
}

func extractComposeImagesFromYAML(composeContent string) []string {
	var parsed struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(composeContent), &parsed); err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var images []string
	for _, svc := range parsed.Services {
		img := strings.TrimSpace(svc.Image)
		if img != "" && !seen[img] {
			seen[img] = true
			images = append(images, img)
		}
	}
	return images
}

func splitImageForPull(image string) (fromImage, tag string) {
	image = strings.TrimSpace(image)
	if strings.Contains(image, "@") {
		return image, ""
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[:lastColon], image[lastColon+1:]
	}
	return image, "latest"
}

// pullImageWithProgress pulls a single image via Docker Engine API (/images/create)
// and invokes onBytes with aggregated (currentBytes, totalBytes) across all layers.
func (a *Adapter) pullImageWithProgress(
	ctx context.Context,
	image string,
	onBytes func(currentBytes, totalBytes int64),
) error {
	fromImage, tag := splitImageForPull(image)
	q := url.Values{}
	q.Set("fromImage", fromImage)
	if tag != "" {
		q.Set("tag", tag)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/images/create?"+q.Encode(), nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker API /images/create returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return parseDockerPullStream(resp.Body, onBytes)
}

func parseDockerPullStream(r io.Reader, onBytes func(currentBytes, totalBytes int64)) error {
	type layerState struct {
		current int64
		total   int64
	}
	layers := make(map[string]*layerState)
	lastEmit := time.Time{}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev dockerPullProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Error != "" {
			return fmt.Errorf("%s", ev.Error)
		}
		if ev.ErrorDetail != nil && ev.ErrorDetail.Message != "" {
			return fmt.Errorf("%s", ev.ErrorDetail.Message)
		}
		if ev.ID == "" {
			continue
		}

		ls, ok := layers[ev.ID]
		if !ok {
			ls = &layerState{}
			layers[ev.ID] = ls
		}

		changed := false
		if ev.ProgressDetail != nil && ev.ProgressDetail.Total > 0 && (ev.Status == "Downloading" || ev.Status == "") {
			ls.total = ev.ProgressDetail.Total
			cur := ev.ProgressDetail.Current
			if cur > ls.total {
				cur = ls.total
			}
			if cur >= ls.current {
				ls.current = cur
				changed = true
			}
		} else if ev.Status == "Download complete" || ev.Status == "Pull complete" || ev.Status == "Already exists" {
			if ls.total > 0 && ls.current != ls.total {
				ls.current = ls.total
				changed = true
			}
		}

		if changed && onBytes != nil && time.Since(lastEmit) >= 150*time.Millisecond {
			var curSum, totSum int64
			for _, l := range layers {
				curSum += l.current
				totSum += l.total
			}
			onBytes(curSum, totSum)
			lastEmit = time.Now()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if onBytes != nil {
		var curSum, totSum int64
		for _, l := range layers {
			curSum += l.current
			totSum += l.total
		}
		if totSum > 0 {
			onBytes(curSum, totSum)
		}
	}
	return nil
}
