package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigxcpu/nas/internal/domain"
	"github.com/sigxcpu/nas/internal/ports"
)

type CronJobService struct {
	port ports.CronJobPort
}

func NewCronJobService(port ports.CronJobPort) *CronJobService {
	return &CronJobService{port: port}
}

func (s *CronJobService) ListJobs(ctx context.Context) ([]domain.CronJob, error) {
	return s.port.ListJobs(ctx)
}

func (s *CronJobService) GetJob(ctx context.Context, id string) (*domain.CronJob, error) {
	return s.port.GetJob(ctx, id)
}

func (s *CronJobService) CreateJob(ctx context.Context, job domain.CronJob) (*domain.CronJob, error) {
	cleanID := domain.CleanCronJobID(job.ID)
	if cleanID == "" {
		cleanID = domain.CleanCronJobID(job.Name)
	}
	if cleanID == "" {
		return nil, fmt.Errorf("cron job id is required")
	}
	job.ID = cleanID

	if strings.TrimSpace(job.Schedule) == "" {
		return nil, fmt.Errorf("schedule expression is required")
	}
	if _, err := domain.ValidateCronSchedule(job.Schedule); err != nil {
		return nil, err
	}

	if strings.TrimSpace(job.Image) == "" {
		return nil, fmt.Errorf("container image is required")
	}

	return s.port.CreateJob(ctx, job)
}

func (s *CronJobService) UpdateJob(ctx context.Context, job domain.CronJob, fields []string) (*domain.CronJob, error) {
	return s.port.UpdateJob(ctx, job, fields)
}

func (s *CronJobService) DeleteJob(ctx context.Context, id string) error {
	return s.port.DeleteJob(ctx, id)
}

func (s *CronJobService) RunJob(ctx context.Context, id string) (*domain.CronJobRun, error) {
	return s.port.RunJob(ctx, id)
}

func (s *CronJobService) GetJobLogs(ctx context.Context, id string, runID string, lines int) (string, *domain.CronJobRun, error) {
	return s.port.GetJobLogs(ctx, id, runID, lines)
}

func (s *CronJobService) Start(ctx context.Context) error {
	return s.port.Start(ctx)
}

func (s *CronJobService) Stop() error {
	return s.port.Stop()
}
