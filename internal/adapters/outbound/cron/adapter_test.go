package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

func TestCronAdapter_CrudAndSchedule(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nas-cron-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	adapter := NewAdapter(tempDir)
	ctx := context.Background()

	// 1. Create a job
	job := domain.CronJob{
		ID:       "backup-test",
		Title:    "Test Backup",
		Schedule: "0 23 * * *",
		Image:    "rclone/rclone:1",
		Args:     []string{"sync", "/data", "remote:/data"},
		Enabled:  true,
		Volumes: []domain.VolumeMount{
			{HostPath: "/tmp", MountPath: "/data", ReadOnly: true},
		},
	}

	created, err := adapter.CreateJob(ctx, job)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}
	if created.ID != "backup-test" {
		t.Errorf("expected ID 'backup-test', got %q", created.ID)
	}
	if created.NextScheduleTime == nil {
		t.Errorf("expected NextScheduleTime to be computed")
	}

	// 2. Get the job
	got, err := adapter.GetJob(ctx, "cronJobs/backup-test")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.Title != "Test Backup" {
		t.Errorf("expected Title 'Test Backup', got %q", got.Title)
	}

	// 3. Update the job
	got.Title = "Updated Backup"
	got.Schedule = "*/15 * * * *"
	got.TimeoutSeconds = 3600
	updated, err := adapter.UpdateJob(ctx, *got, []string{"title", "schedule", "timeout_seconds"})
	if err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}
	if updated.Title != "Updated Backup" {
		t.Errorf("expected Title 'Updated Backup', got %q", updated.Title)
	}
	if updated.Schedule != "*/15 * * * *" {
		t.Errorf("expected Schedule '*/15 * * * *', got %q", updated.Schedule)
	}
	if updated.TimeoutSeconds != 3600 {
		t.Errorf("expected TimeoutSeconds 3600, got %d", updated.TimeoutSeconds)
	}

	// 4. Persistence check: create a new adapter pointing to same stateDir
	adapter2 := NewAdapter(tempDir)
	jobs, err := adapter2.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs on reloaded adapter failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Title != "Updated Backup" {
		t.Errorf("expected reloaded title 'Updated Backup', got %q", jobs[0].Title)
	}
	if jobs[0].TimeoutSeconds != 3600 {
		t.Errorf("expected reloaded TimeoutSeconds 3600, got %d", jobs[0].TimeoutSeconds)
	}

	// 5. Start and Stop scheduler
	if err := adapter2.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := adapter2.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 6. Delete job
	if err := adapter2.DeleteJob(ctx, "backup-test"); err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	jobPath := filepath.Join(tempDir, "backup-test")
	if _, err := os.Stat(jobPath); !os.IsNotExist(err) {
		t.Errorf("expected job directory %q to be deleted", jobPath)
	}
}

func TestCronAdapter_OrphanLogRecoveryOnRestart(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nas-cron-recovery-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	jobDir := filepath.Join(tempDir, "rclone-test")
	runsDir := filepath.Join(jobDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	// Simulate a job that was RUNNING when the daemon crashed/restarted and container is gone
	jobJSON := `{
  "name": "cronJobs/rclone-test",
  "id": "rclone-test",
  "title": "Rclone Backup",
  "schedule": "0 23 * * *",
  "image": "rclone/rclone:1",
  "enabled": true,
  "state": "RUNNING",
  "lastRunStatus": "RUNNING"
}`
	if err := os.WriteFile(filepath.Join(jobDir, "job.json"), []byte(jobJSON), 0644); err != nil {
		t.Fatalf("failed to write job.json: %v", err)
	}

	// Write an orphaned .log file without a .json file
	logPath := filepath.Join(runsDir, "rclone-test-1793480400.log")
	if err := os.WriteFile(logPath, []byte("Transferred: 100 MiB\n"), 0644); err != nil {
		t.Fatalf("failed to write orphan log: %v", err)
	}

	adapter := NewAdapter(tempDir)
	ctx := context.Background()

	got, err := adapter.GetJob(ctx, "rclone-test")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if got.State != "IDLE" {
		t.Errorf("expected State 'IDLE', got %q", got.State)
	}
	if got.LastRunStatus != "ABORTED" {
		t.Errorf("expected LastRunStatus 'ABORTED', got %q", got.LastRunStatus)
	}

	logs, run, err := adapter.GetJobLogs(ctx, "rclone-test", "", 100)
	if err != nil {
		t.Fatalf("GetJobLogs failed: %v", err)
	}
	if run == nil || run.RunID != "rclone-test-1793480400" {
		t.Fatalf("expected recovered run 'rclone-test-1793480400', got %+v", run)
	}
	if run.Status != "ABORTED" {
		t.Errorf("expected recovered run Status 'ABORTED', got %q", run.Status)
	}
	if logs != "Transferred: 100 MiB\n" {
		t.Errorf("expected recovered logs 'Transferred: 100 MiB\\n', got %q", logs)
	}

	// Verify that the recovered run .json was persisted to disk
	if _, err := os.Stat(filepath.Join(runsDir, "rclone-test-1793480400.json")); err != nil {
		t.Errorf("expected recovered run json file to exist on disk: %v", err)
	}
}
