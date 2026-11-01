package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type VolumeMount struct {
	HostPath  string `json:"hostPath"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CronJobRun struct {
	RunID      string     `json:"runId"`
	JobName    string     `json:"jobName"`
	StartTime  time.Time  `json:"startTime"`
	EndTime    *time.Time `json:"endTime,omitempty"`
	Duration   string     `json:"duration,omitempty"`
	Status     string     `json:"status"` // "RUNNING", "SUCCESS", "FAILED"
	ExitCode   int32      `json:"exitCode"`
	Error      string     `json:"error,omitempty"`
	LogSnippet string     `json:"logSnippet,omitempty"`
}

type CronJob struct {
	Name              string        `json:"name"` // "cronJobs/{id}"
	ID                string        `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	Schedule          string        `json:"schedule"` // standard 5-part cron or @daily, @hourly
	Image             string        `json:"image"`    // docker image name
	Command           []string      `json:"command"`
	Args              []string      `json:"args"`
	Env               []EnvVar      `json:"env"`
	Volumes           []VolumeMount `json:"volumes"`
	Enabled           bool          `json:"enabled"`
	ConcurrencyPolicy string        `json:"concurrencyPolicy"`        // "Forbid", "Allow", "Replace"
	RestartPolicy     string        `json:"restartPolicy"`            // "Never", "OnFailure"
	TimeoutSeconds    int32         `json:"timeoutSeconds,omitempty"` // 0 = no timeout, >0 = max execution seconds
	State             string        `json:"state"`                    // "IDLE", "RUNNING", "ERROR"

	LastScheduleTime   *time.Time `json:"lastScheduleTime,omitempty"`
	LastSuccessfulTime *time.Time `json:"lastSuccessfulTime,omitempty"`
	NextScheduleTime   *time.Time `json:"nextScheduleTime,omitempty"`

	LastRunDuration string `json:"lastRunDuration,omitempty"`
	LastRunStatus   string `json:"lastRunStatus,omitempty"`
	LastRunExitCode int32  `json:"lastRunExitCode"`
	LastRunError    string `json:"lastRunError,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// ValidateCronSchedule validates whether a cron expression is syntactically correct.
func ValidateCronSchedule(schedule string) (cron.Schedule, error) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return nil, fmt.Errorf("schedule cannot be empty")
	}
	s, err := cronParser.Parse(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", schedule, err)
	}
	return s, nil
}

// ComputeNextSchedule calculates the next scheduled execution time for the given schedule.
func ComputeNextSchedule(schedule string, from time.Time) *time.Time {
	s, err := ValidateCronSchedule(schedule)
	if err != nil {
		return nil
	}
	next := s.Next(from)
	if next.IsZero() {
		return nil
	}
	return &next
}

func CleanCronJobID(nameOrID string) string {
	clean := strings.TrimPrefix(nameOrID, "cronJobs/")
	return strings.TrimSpace(clean)
}

func FormatCronJobResourceName(id string) string {
	clean := CleanCronJobID(id)
	return fmt.Sprintf("cronJobs/%s", clean)
}
