package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sigxcpu/nas/internal/domain"
)

type activeRun struct {
	runID         string
	containerName string
	abortCancel   context.CancelFunc
	daemonCancel  context.CancelFunc
	doneCh        chan struct{}
	startTime     time.Time
}

type Adapter struct {
	mu          sync.RWMutex
	stateDir    string
	logger      *slog.Logger
	jobs        map[string]*domain.CronJob
	runs        map[string][]*domain.CronJobRun // jobID -> runs
	activeRuns  map[string]*activeRun           // jobID -> activeRun
	cronEngine  *cron.Cron
	cronEntries map[string]cron.EntryID
	eventFn     func(action, jobID, message string)
	started     bool
}

func NewAdapter(stateDir string) *Adapter {
	if stateDir == "" {
		stateDir = "/tmp/nas-cron"
	}
	_ = os.MkdirAll(stateDir, 0755)

	a := &Adapter{
		stateDir:    stateDir,
		logger:      slog.Default().With("subsystem", "cron"),
		jobs:        make(map[string]*domain.CronJob),
		runs:        make(map[string][]*domain.CronJobRun),
		activeRuns:  make(map[string]*activeRun),
		cronEntries: make(map[string]cron.EntryID),
		cronEngine: cron.New(cron.WithParser(cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		))),
	}

	a.loadAllJobs()
	return a
}

func (a *Adapter) SetLogger(l *slog.Logger) {
	a.logger = l
}

func (a *Adapter) SetEventCallback(fn func(action, jobID, message string)) {
	a.eventFn = fn
}

func (a *Adapter) emitEvent(action, jobID, message string) {
	if a.eventFn != nil {
		a.eventFn(action, jobID, message)
	}
}

func readLogSnippet(logFilePath string) string {
	snippetData, err := os.ReadFile(logFilePath)
	if err != nil {
		return ""
	}
	s := string(snippetData)
	if len(s) > 4000 {
		return s[len(s)-4000:]
	}
	return s
}

func (a *Adapter) findJobContainers(jobID string) []struct {
	name string
	ts   int64
} {
	prefix := fmt.Sprintf("nas-cron-%s-", jobID)
	checkCmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", prefix), "--format", "{{.Names}}")
	out, err := checkCmd.Output()
	if err != nil {
		return nil
	}
	var matches []struct {
		name string
		ts   int64
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		cName := strings.TrimSpace(line)
		if !strings.HasPrefix(cName, prefix) {
			continue
		}
		tsStr := strings.TrimPrefix(cName, prefix)
		ts, parseErr := strconv.ParseInt(tsStr, 10, 64)
		if parseErr != nil {
			continue
		}
		matches = append(matches, struct {
			name string
			ts   int64
		}{name: cName, ts: ts})
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ts > matches[j].ts
	})
	return matches
}

func (a *Adapter) loadAllJobs() {
	entries, err := os.ReadDir(a.stateDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobID := entry.Name()
		jobFile := filepath.Join(a.stateDir, jobID, "job.json")
		data, err := os.ReadFile(jobFile)
		if err != nil {
			continue
		}

		var job domain.CronJob
		if err := json.Unmarshal(data, &job); err != nil {
			a.logger.Warn("failed to parse job file", "file", jobFile, "error", err)
			continue
		}

		if job.ID == "" {
			job.ID = jobID
		}
		job.Name = domain.FormatCronJobResourceName(job.ID)

		// Load recent runs and recover any orphaned .log files lacking a .json record
		runsDir := filepath.Join(a.stateDir, jobID, "runs")
		runMap := make(map[string]*domain.CronJobRun)
		var jobRuns []*domain.CronJobRun
		if runEntries, err := os.ReadDir(runsDir); err == nil {
			for _, rEntry := range runEntries {
				if strings.HasSuffix(rEntry.Name(), ".json") {
					rData, err := os.ReadFile(filepath.Join(runsDir, rEntry.Name()))
					if err == nil {
						var r domain.CronJobRun
						if json.Unmarshal(rData, &r) == nil && r.RunID != "" {
							runCopy := r
							runMap[runCopy.RunID] = &runCopy
							jobRuns = append(jobRuns, &runCopy)
						}
					}
				}
			}
			// Synthesize run records for orphaned .log files created before a crash/restart
			prefix := job.ID + "-"
			for _, rEntry := range runEntries {
				if !strings.HasSuffix(rEntry.Name(), ".log") {
					continue
				}
				runID := strings.TrimSuffix(rEntry.Name(), ".log")
				if _, exists := runMap[runID]; exists {
					continue
				}
				if !strings.HasPrefix(runID, prefix) {
					continue
				}
				tsStr := strings.TrimPrefix(runID, prefix)
				ts, parseErr := strconv.ParseInt(tsStr, 10, 64)
				if parseErr != nil {
					continue
				}
				startTime := time.Unix(ts, 0)
				r := &domain.CronJobRun{
					RunID:     runID,
					JobName:   job.Name,
					StartTime: startTime,
					Status:    "RUNNING",
				}
				runMap[runID] = r
				jobRuns = append(jobRuns, r)
			}
		}

		sort.Slice(jobRuns, func(i, j int) bool {
			return jobRuns[i].StartTime.After(jobRuns[j].StartTime)
		})

		// Check if a cron container for this job is still present in Docker
		containers := a.findJobContainers(job.ID)
		if len(containers) > 0 && job.State == "RUNNING" {
			latest := containers[0]
			// Clean up any older stale containers
			for _, stale := range containers[1:] {
				_ = exec.Command("docker", "rm", "-f", stale.name).Run()
			}
			runID := fmt.Sprintf("%s-%d", job.ID, latest.ts)
			run, exists := runMap[runID]
			if !exists {
				run = &domain.CronJobRun{
					RunID:     runID,
					JobName:   job.Name,
					StartTime: time.Unix(latest.ts, 0),
					Status:    "RUNNING",
				}
				jobRuns = append([]*domain.CronJobRun{run}, jobRuns...)
			} else {
				run.Status = "RUNNING"
			}
			startTime := run.StartTime
			job.State = "RUNNING"
			job.LastRunStatus = "RUNNING"
			job.LastScheduleTime = &startTime
			job.LastRunDuration = ""
			job.LastRunError = ""
			job.LastRunExitCode = 0
			job.NextScheduleTime = domain.ComputeNextSchedule(job.Schedule, time.Now())

			if len(jobRuns) > 20 {
				jobRuns = jobRuns[:20]
			}
			a.jobs[job.ID] = &job
			a.runs[job.ID] = jobRuns
			a.saveRun(job.ID, run)
			_ = a.saveJobLocked(&job)

			abortCtx, abortCancel := context.WithCancel(context.Background())
			daemonCtx, daemonCancel := context.WithCancel(context.Background())
			doneCh := make(chan struct{})
			a.activeRuns[job.ID] = &activeRun{
				runID:         runID,
				containerName: latest.name,
				abortCancel:   abortCancel,
				daemonCancel:  daemonCancel,
				doneCh:        doneCh,
				startTime:     startTime,
			}
			logFilePath := filepath.Join(runsDir, runID+".log")
			a.logger.Info("re-attaching to cron container across daemon restart", "job_id", job.ID, "run_id", runID, "container", latest.name)
			go a.monitorContainer(job.ID, run, latest.name, logFilePath, job.TimeoutSeconds, abortCtx, daemonCtx, doneCh, true)
			continue
		}

		// Clean up any leftover containers if job was not in RUNNING state
		for _, stale := range containers {
			_ = exec.Command("docker", "rm", "-f", stale.name).Run()
		}

		// Finalize any runs that were left in RUNNING state without a backing container
		for _, r := range jobRuns {
			if r.Status == "RUNNING" {
				logPath := filepath.Join(runsDir, r.RunID+".log")
				endTime := time.Now()
				if info, statErr := os.Stat(logPath); statErr == nil {
					endTime = info.ModTime()
				}
				if endTime.Before(r.StartTime) {
					endTime = r.StartTime
				}
				r.EndTime = &endTime
				r.Duration = endTime.Sub(r.StartTime).Round(time.Millisecond).String()
				r.Status = "ABORTED"
				r.Error = "daemon restarted during execution"
				r.LogSnippet = readLogSnippet(logPath)
				a.saveRun(job.ID, r)
			}
		}

		jobChanged := false
		if job.State == "RUNNING" {
			job.State = "IDLE"
			if job.LastRunStatus == "RUNNING" {
				job.LastRunStatus = "ABORTED"
				job.LastRunError = "daemon restarted during execution"
				if len(jobRuns) > 0 && jobRuns[0].Duration != "" {
					job.LastRunDuration = jobRuns[0].Duration
				}
			}
			jobChanged = true
		}

		job.NextScheduleTime = domain.ComputeNextSchedule(job.Schedule, time.Now())
		if len(jobRuns) > 20 {
			jobRuns = jobRuns[:20]
		}
		a.jobs[job.ID] = &job
		a.runs[job.ID] = jobRuns
		if jobChanged {
			_ = a.saveJobLocked(&job)
		}
	}
}

func (a *Adapter) saveJobLocked(job *domain.CronJob) error {
	jobDir := filepath.Join(a.stateDir, job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("create job dir: %w", err)
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	jobFile := filepath.Join(jobDir, "job.json")
	return os.WriteFile(jobFile, data, 0644)
}

func (a *Adapter) saveRun(jobID string, run *domain.CronJobRun) {
	runsDir := filepath.Join(a.stateDir, jobID, "runs")
	_ = os.MkdirAll(runsDir, 0755)

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(runsDir, run.RunID+".json"), data, 0644)
}

// Start begins the cron scheduler loop and registers all enabled jobs.
func (a *Adapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}

	for _, job := range a.jobs {
		if job.Enabled {
			a.scheduleJobLocked(job)
		}
	}

	a.cronEngine.Start()
	a.started = true
	a.logger.Info("cron scheduler engine started", "jobs_count", len(a.jobs))
	return nil
}

// Stop cleanly terminates the cron scheduler loop while leaving any running
// detached Docker cron containers intact across daemon restarts.
func (a *Adapter) Stop() error {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return nil
	}

	cronCtx := a.cronEngine.Stop()
	var waitChans []chan struct{}
	for jobID, active := range a.activeRuns {
		a.logger.Info("detaching from running cron job on daemon stop", "job_id", jobID, "run_id", active.runID)
		active.daemonCancel()
		waitChans = append(waitChans, active.doneCh)
	}
	a.started = false
	a.mu.Unlock()

	<-cronCtx.Done()
	for _, ch := range waitChans {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
		}
	}

	a.logger.Info("cron scheduler engine stopped")
	return nil
}

func (a *Adapter) scheduleJobLocked(job *domain.CronJob) {
	// Remove existing schedule if any
	if entryID, exists := a.cronEntries[job.ID]; exists {
		a.cronEngine.Remove(entryID)
		delete(a.cronEntries, job.ID)
	}

	if !job.Enabled {
		return
	}

	jobID := job.ID
	entryID, err := a.cronEngine.AddFunc(job.Schedule, func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
			defer cancel()
			if _, runErr := a.ExecuteJob(ctx, jobID, false); runErr != nil {
				a.logger.Error("cron execution failed", "job_id", jobID, "error", runErr)
			}
		}()
	})

	if err != nil {
		a.logger.Warn("could not schedule cron job", "job_id", job.ID, "schedule", job.Schedule, "error", err)
		return
	}

	a.cronEntries[job.ID] = entryID
	job.NextScheduleTime = domain.ComputeNextSchedule(job.Schedule, time.Now())
}

func (a *Adapter) ListJobs(ctx context.Context) ([]domain.CronJob, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make([]domain.CronJob, 0, len(a.jobs))
	for _, j := range a.jobs {
		copied := *j
		if active, running := a.activeRuns[j.ID]; running {
			copied.State = "RUNNING"
			copied.LastRunStatus = "RUNNING"
			copied.LastScheduleTime = &active.startTime
			copied.LastRunDuration = ""
			copied.LastRunError = ""
			copied.LastRunExitCode = 0
		}
		if entryID, exists := a.cronEntries[j.ID]; exists {
			entry := a.cronEngine.Entry(entryID)
			if !entry.Next.IsZero() {
				next := entry.Next
				copied.NextScheduleTime = &next
			}
		}
		out = append(out, copied)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (a *Adapter) GetJob(ctx context.Context, id string) (*domain.CronJob, error) {
	cleanID := domain.CleanCronJobID(id)
	a.mu.RLock()
	defer a.mu.RUnlock()

	job, exists := a.jobs[cleanID]
	if !exists {
		return nil, fmt.Errorf("%w: cron job %q", domain.ErrNotFound, cleanID)
	}
	copied := *job
	if active, running := a.activeRuns[job.ID]; running {
		copied.State = "RUNNING"
		copied.LastRunStatus = "RUNNING"
		copied.LastScheduleTime = &active.startTime
		copied.LastRunDuration = ""
		copied.LastRunError = ""
		copied.LastRunExitCode = 0
	}
	if entryID, exists := a.cronEntries[job.ID]; exists {
		entry := a.cronEngine.Entry(entryID)
		if !entry.Next.IsZero() {
			next := entry.Next
			copied.NextScheduleTime = &next
		}
	}
	return &copied, nil
}

func (a *Adapter) CreateJob(ctx context.Context, job domain.CronJob) (*domain.CronJob, error) {
	cleanID := domain.CleanCronJobID(job.ID)
	if cleanID == "" {
		return nil, fmt.Errorf("job id cannot be empty")
	}

	if _, err := domain.ValidateCronSchedule(job.Schedule); err != nil {
		return nil, err
	}

	if strings.TrimSpace(job.Image) == "" {
		return nil, fmt.Errorf("image cannot be empty")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.jobs[cleanID]; exists {
		return nil, fmt.Errorf("cron job %q already exists", cleanID)
	}

	now := time.Now()
	job.ID = cleanID
	job.Name = domain.FormatCronJobResourceName(cleanID)
	job.State = "IDLE"
	job.CreatedAt = now
	job.UpdatedAt = now
	job.NextScheduleTime = domain.ComputeNextSchedule(job.Schedule, now)
	if job.ConcurrencyPolicy == "" {
		job.ConcurrencyPolicy = "Forbid"
	}
	if job.RestartPolicy == "" {
		job.RestartPolicy = "Never"
	}

	if err := a.saveJobLocked(&job); err != nil {
		return nil, err
	}

	a.jobs[cleanID] = &job
	if job.Enabled && a.started {
		a.scheduleJobLocked(&job)
	}

	a.emitEvent("CREATE", cleanID, "Cron job created")
	return &job, nil
}

func (a *Adapter) UpdateJob(ctx context.Context, job domain.CronJob, fields []string) (*domain.CronJob, error) {
	cleanID := domain.CleanCronJobID(job.ID)
	if cleanID == "" {
		cleanID = domain.CleanCronJobID(job.Name)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	existing, exists := a.jobs[cleanID]
	if !exists {
		return nil, fmt.Errorf("%w: cron job %q", domain.ErrNotFound, cleanID)
	}

	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[strings.ToLower(strings.ReplaceAll(f, "_", ""))] = true
	}

	if len(fields) == 0 || fieldSet["title"] {
		existing.Title = job.Title
	}
	if len(fields) == 0 || fieldSet["description"] {
		existing.Description = job.Description
	}
	if len(fields) == 0 || fieldSet["schedule"] {
		if _, err := domain.ValidateCronSchedule(job.Schedule); err != nil {
			return nil, err
		}
		existing.Schedule = job.Schedule
	}
	if len(fields) == 0 || fieldSet["image"] {
		if strings.TrimSpace(job.Image) != "" {
			existing.Image = job.Image
		}
	}
	if len(fields) == 0 || fieldSet["command"] {
		existing.Command = job.Command
	}
	if len(fields) == 0 || fieldSet["args"] {
		existing.Args = job.Args
	}
	if len(fields) == 0 || fieldSet["env"] {
		existing.Env = job.Env
	}
	if len(fields) == 0 || fieldSet["volumes"] {
		existing.Volumes = job.Volumes
	}
	if len(fields) == 0 || fieldSet["enabled"] {
		existing.Enabled = job.Enabled
	}
	if len(fields) == 0 || fieldSet["concurrencypolicy"] {
		if job.ConcurrencyPolicy != "" {
			existing.ConcurrencyPolicy = job.ConcurrencyPolicy
		}
	}
	if len(fields) == 0 || fieldSet["restartpolicy"] {
		if job.RestartPolicy != "" {
			existing.RestartPolicy = job.RestartPolicy
		}
	}
	if len(fields) == 0 || fieldSet["timeoutseconds"] || fieldSet["timeout"] {
		if job.TimeoutSeconds < 0 {
			existing.TimeoutSeconds = 0
		} else {
			existing.TimeoutSeconds = job.TimeoutSeconds
		}
	}

	existing.UpdatedAt = time.Now()
	existing.NextScheduleTime = domain.ComputeNextSchedule(existing.Schedule, time.Now())

	if err := a.saveJobLocked(existing); err != nil {
		return nil, err
	}

	if a.started {
		if existing.Enabled {
			a.scheduleJobLocked(existing)
		} else if entryID, exists := a.cronEntries[existing.ID]; exists {
			a.cronEngine.Remove(entryID)
			delete(a.cronEntries, existing.ID)
		}
	}

	a.emitEvent("UPDATE", cleanID, "Cron job updated")
	return existing, nil
}

func (a *Adapter) DeleteJob(ctx context.Context, id string) error {
	cleanID := domain.CleanCronJobID(id)

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.jobs[cleanID]; !exists {
		return fmt.Errorf("%w: cron job %q", domain.ErrNotFound, cleanID)
	}

	if active, isRunning := a.activeRuns[cleanID]; isRunning {
		active.abortCancel()
		delete(a.activeRuns, cleanID)
	}

	if entryID, exists := a.cronEntries[cleanID]; exists {
		a.cronEngine.Remove(entryID)
		delete(a.cronEntries, cleanID)
	}

	delete(a.jobs, cleanID)
	delete(a.runs, cleanID)

	jobDir := filepath.Join(a.stateDir, cleanID)
	_ = os.RemoveAll(jobDir)

	a.emitEvent("DELETE", cleanID, "Cron job deleted")
	return nil
}

func (a *Adapter) RunJob(ctx context.Context, id string) (*domain.CronJobRun, error) {
	cleanID := domain.CleanCronJobID(id)
	return a.ExecuteJob(ctx, cleanID, true)
}

func (a *Adapter) upsertRunLocked(jobID string, run *domain.CronJobRun) {
	existingRuns := a.runs[jobID]
	for i, r := range existingRuns {
		if r.RunID == run.RunID {
			existingRuns[i] = run
			return
		}
	}
	a.runs[jobID] = append([]*domain.CronJobRun{run}, existingRuns...)
	if len(a.runs[jobID]) > 20 {
		a.runs[jobID] = a.runs[jobID][:20]
	}
}

func (a *Adapter) ExecuteJob(parentCtx context.Context, jobID string, isManual bool) (*domain.CronJobRun, error) {
	a.mu.Lock()
	job, exists := a.jobs[jobID]
	if !exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: cron job %q", domain.ErrNotFound, jobID)
	}

	// Concurrency policy check
	var prevDoneCh chan struct{}
	if active, running := a.activeRuns[jobID]; running {
		policy := strings.ToLower(job.ConcurrencyPolicy)
		if policy == "allow" {
			// allow concurrent execution
		} else if policy == "replace" {
			a.logger.Info("replacing running cron job", "job_id", jobID, "previous_run_id", active.runID)
			active.abortCancel()
			prevDoneCh = active.doneCh
			delete(a.activeRuns, jobID)
		} else { // default: "forbid"
			a.mu.Unlock()
			a.logger.Warn("skipping cron execution due to ConcurrencyForbid policy", "job_id", jobID)
			return nil, fmt.Errorf("cron job %q is already running (run_id: %s)", jobID, active.runID)
		}
	}

	startTime := time.Now()
	unixTs := startTime.Unix()
	if len(a.runs[jobID]) > 0 && a.runs[jobID][0].RunID == fmt.Sprintf("%s-%d", jobID, unixTs) {
		unixTs++
	}
	runID := fmt.Sprintf("%s-%d", jobID, unixTs)
	containerName := fmt.Sprintf("nas-cron-%s", runID)

	abortCtx, abortCancel := context.WithCancel(context.Background())
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})

	a.activeRuns[jobID] = &activeRun{
		runID:         runID,
		containerName: containerName,
		abortCancel:   abortCancel,
		daemonCancel:  daemonCancel,
		doneCh:        doneCh,
		startTime:     startTime,
	}

	run := &domain.CronJobRun{
		RunID:     runID,
		JobName:   job.Name,
		StartTime: startTime,
		Status:    "RUNNING",
	}

	job.State = "RUNNING"
	job.LastRunStatus = "RUNNING"
	job.LastRunDuration = ""
	job.LastRunError = ""
	job.LastRunExitCode = 0
	job.LastScheduleTime = &startTime
	a.upsertRunLocked(jobID, run)
	_ = a.saveJobLocked(job)
	a.mu.Unlock()

	a.saveRun(jobID, run)
	a.emitEvent("START", jobID, fmt.Sprintf("Cron job run %s started", runID))

	runsDir := filepath.Join(a.stateDir, jobID, "runs")
	_ = os.MkdirAll(runsDir, 0755)
	logFilePath := filepath.Join(runsDir, runID+".log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		abortCancel()
		daemonCancel()
		close(doneCh)
		a.mu.Lock()
		delete(a.activeRuns, jobID)
		if j, ok := a.jobs[jobID]; ok {
			j.State = "ERROR"
			j.LastRunStatus = "FAILED"
			j.LastRunError = err.Error()
			_ = a.saveJobLocked(j)
		}
		a.mu.Unlock()
		return nil, fmt.Errorf("create log file: %w", err)
	}

	// Build detached docker run command so the container runs in the Docker engine
	// independently of the nasctl daemon process and survives nasctl restarts.
	var dockerArgs []string
	dockerArgs = append(dockerArgs, "run", "-d", "--name", containerName)

	// Volume mounts
	for _, v := range job.Volumes {
		if v.HostPath != "" && v.MountPath != "" {
			m := fmt.Sprintf("%s:%s", v.HostPath, v.MountPath)
			if v.ReadOnly {
				m += ":ro"
			}
			dockerArgs = append(dockerArgs, "-v", m)
		}
	}

	// Environment variables
	for _, e := range job.Env {
		if e.Name != "" {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", e.Name, e.Value))
		}
	}

	// Entrypoint command override
	if len(job.Command) > 0 {
		dockerArgs = append(dockerArgs, "--entrypoint", job.Command[0])
	}

	// Image
	dockerArgs = append(dockerArgs, job.Image)

	// Extra command arguments
	if len(job.Command) > 1 {
		dockerArgs = append(dockerArgs, job.Command[1:]...)
	}

	// Args
	dockerArgs = append(dockerArgs, job.Args...)

	a.logger.Info("executing ephemeral cron container", "job_id", jobID, "run_id", runID, "container", containerName, "image", job.Image, "timeout_seconds", job.TimeoutSeconds)
	timeoutSeconds := job.TimeoutSeconds

	// Execute container asynchronously so HTTP calls return immediately without timing out
	go func() {
		if prevDoneCh != nil {
			select {
			case <-prevDoneCh:
			case <-time.After(3 * time.Second):
			}
		}

		_ = exec.Command("docker", "rm", "-f", containerName).Run()

		startExecCtx := abortCtx
		var startTimeoutCancel context.CancelFunc
		if timeoutSeconds > 0 {
			startExecCtx, startTimeoutCancel = context.WithDeadline(abortCtx, startTime.Add(time.Duration(timeoutSeconds)*time.Second))
			defer startTimeoutCancel()
		}

		startCmd := exec.CommandContext(startExecCtx, "docker", dockerArgs...)
		startCmd.Stdout = io.Discard
		startCmd.Stderr = logFile

		startErr := startCmd.Run()
		_ = logFile.Sync()
		_ = logFile.Close()

		if startErr != nil {
			defer close(doneCh)
			defer abortCancel()
			defer daemonCancel()

			endTime := time.Now()
			duration := endTime.Sub(startTime).Round(time.Millisecond).String()
			var exitCode int32 = 1
			var status = "FAILED"
			var errMsg = startErr.Error()

			if startExecCtx.Err() == context.DeadlineExceeded && abortCtx.Err() == nil {
				status = "TIMEOUT"
				errMsg = fmt.Sprintf("execution timed out after %s", (time.Duration(timeoutSeconds) * time.Second).String())
				exitCode = 124
			} else if abortCtx.Err() != nil {
				status = "ABORTED"
				errMsg = "execution cancelled"
				exitCode = 130
			} else if exitError, ok := startErr.(*exec.ExitError); ok {
				if statusWait, ok := exitError.Sys().(syscall.WaitStatus); ok {
					exitCode = int32(statusWait.ExitStatus())
				}
			}
			_ = exec.Command("docker", "rm", "-f", containerName).Run()
			a.finishRun(jobID, run, logFilePath, endTime, duration, status, exitCode, errMsg)
			return
		}

		a.monitorContainer(jobID, run, containerName, logFilePath, timeoutSeconds, abortCtx, daemonCtx, doneCh, false)
	}()

	return run, nil
}

func (a *Adapter) monitorContainer(
	jobID string,
	run *domain.CronJobRun,
	containerName string,
	logFilePath string,
	timeoutSeconds int32,
	abortCtx context.Context,
	daemonCtx context.Context,
	doneCh chan struct{},
	isResumed bool,
) {
	defer close(doneCh)

	openFlags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if isResumed {
		openFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	logFile, err := os.OpenFile(logFilePath, openFlags, 0644)

	timeoutCtx := context.Background()
	var timeoutCancel context.CancelFunc = func() {}
	if timeoutSeconds > 0 {
		deadline := run.StartTime.Add(time.Duration(timeoutSeconds) * time.Second)
		timeoutCtx, timeoutCancel = context.WithDeadline(context.Background(), deadline)
	}
	defer timeoutCancel()

	followCtx, followCancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-abortCtx.Done():
			followCancel()
		case <-daemonCtx.Done():
			followCancel()
		case <-timeoutCtx.Done():
			followCancel()
		case <-followCtx.Done():
		}
	}()

	if err == nil {
		logsCmd := exec.CommandContext(followCtx, "docker", "logs", "-f", containerName)
		logsCmd.Stdout = logFile
		logsCmd.Stderr = logFile
		_ = logsCmd.Run()
		_ = logFile.Sync()
		_ = logFile.Close()
	} else {
		waitCmd := exec.CommandContext(followCtx, "docker", "wait", containerName)
		_ = waitCmd.Run()
	}
	followCancel()

	// If daemon is stopping (and job was not explicitly aborted or timed out), leave the container
	// running in Docker so the next nasctl instance re-attaches on startup.
	if daemonCtx.Err() != nil && abortCtx.Err() == nil && timeoutCtx.Err() == nil {
		return
	}

	endTime := time.Now()
	duration := endTime.Sub(run.StartTime).Round(time.Millisecond).String()

	var exitCode int32 = 0
	var status = "SUCCESS"
	var errMsg = ""

	if timeoutCtx.Err() == context.DeadlineExceeded && abortCtx.Err() == nil {
		status = "TIMEOUT"
		errMsg = fmt.Sprintf("execution timed out after %s", (time.Duration(timeoutSeconds) * time.Second).String())
		exitCode = 124
		a.logger.Warn("cron job timed out, stopping container", "job_id", jobID, "run_id", run.RunID, "container", containerName, "timeout_seconds", timeoutSeconds)
		_ = exec.Command("docker", "stop", "-t", "2", containerName).Run()
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	} else if abortCtx.Err() != nil {
		status = "ABORTED"
		errMsg = "execution cancelled"
		exitCode = 130
		_ = exec.Command("docker", "stop", "-t", "2", containerName).Run()
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	} else {
		inspectOut, inspectErr := exec.Command("docker", "inspect", "-f", "{{.State.ExitCode}}|{{.State.Error}}", containerName).Output()
		if inspectErr != nil {
			status = "FAILED"
			exitCode = 1
			errMsg = "container terminated unexpectedly"
		} else {
			parts := strings.SplitN(strings.TrimSpace(string(inspectOut)), "|", 2)
			if code, parseErr := strconv.Atoi(parts[0]); parseErr == nil {
				exitCode = int32(code)
			} else {
				exitCode = 1
			}
			stateErr := ""
			if len(parts) > 1 {
				stateErr = strings.TrimSpace(parts[1])
			}
			if exitCode != 0 {
				status = "FAILED"
				if stateErr != "" {
					errMsg = stateErr
				} else {
					errMsg = fmt.Sprintf("exit status %d", exitCode)
				}
			}
		}
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	}

	a.finishRun(jobID, run, logFilePath, endTime, duration, status, exitCode, errMsg)
}

func (a *Adapter) finishRun(
	jobID string,
	run *domain.CronJobRun,
	logFilePath string,
	endTime time.Time,
	duration string,
	status string,
	exitCode int32,
	errMsg string,
) {
	run.EndTime = &endTime
	run.Duration = duration
	run.Status = status
	run.ExitCode = exitCode
	run.Error = errMsg
	run.LogSnippet = readLogSnippet(logFilePath)

	a.saveRun(jobID, run)

	a.mu.Lock()
	if active, ok := a.activeRuns[jobID]; ok && active.runID == run.RunID {
		delete(a.activeRuns, jobID)
	}

	if j, ok := a.jobs[jobID]; ok {
		j.LastRunStatus = status
		j.LastRunExitCode = exitCode
		j.LastRunDuration = duration
		j.LastRunError = errMsg
		if _, stillActive := a.activeRuns[jobID]; !stillActive {
			j.State = "IDLE"
		}
		if status == "SUCCESS" {
			j.LastSuccessfulTime = &endTime
		}
		j.NextScheduleTime = domain.ComputeNextSchedule(j.Schedule, time.Now())

		a.upsertRunLocked(jobID, run)
		_ = a.saveJobLocked(j)
	}
	a.mu.Unlock()

	a.emitEvent("COMPLETE", jobID, fmt.Sprintf("Cron job run %s completed with status %s", run.RunID, status))
}

func (a *Adapter) GetJobLogs(ctx context.Context, id string, runID string, lines int) (string, *domain.CronJobRun, error) {
	cleanID := domain.CleanCronJobID(id)
	a.mu.RLock()
	defer a.mu.RUnlock()

	jobRuns := a.runs[cleanID]
	var targetRun *domain.CronJobRun

	if active, isRunning := a.activeRuns[cleanID]; isRunning && (runID == "" || runID == active.runID) {
		runID = active.runID
		targetRun = &domain.CronJobRun{
			RunID:     active.runID,
			JobName:   fmt.Sprintf("cronJobs/%s", cleanID),
			StartTime: active.startTime,
			Status:    "RUNNING",
		}
	} else if runID != "" {
		for _, r := range jobRuns {
			if r.RunID == runID {
				targetRun = r
				break
			}
		}
	} else if len(jobRuns) > 0 {
		targetRun = jobRuns[0]
		runID = targetRun.RunID
	}

	if runID == "" {
		return "No execution logs available for this cron job.", nil, nil
	}

	logFilePath := filepath.Join(a.stateDir, cleanID, "runs", runID+".log")
	data, err := os.ReadFile(logFilePath)
	if err != nil {
		if targetRun != nil && targetRun.Status == "RUNNING" {
			return "[Task initialized, waiting for container output...]", targetRun, nil
		}
		return "", targetRun, fmt.Errorf("log file not found for run %q: %w", runID, err)
	}

	content := string(data)
	if strings.TrimSpace(content) == "" && targetRun != nil && targetRun.Status == "RUNNING" {
		content = "[Task is running. Waiting for container log output...]"
	} else if lines > 0 {
		split := strings.Split(content, "\n")
		if len(split) > lines {
			split = split[len(split)-lines:]
		}
		content = strings.Join(split, "\n")
	}

	return content, targetRun, nil
}
