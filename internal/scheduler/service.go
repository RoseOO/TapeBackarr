package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/models"

	"github.com/robfig/cron/v3"
)

// JobRunner is a function that runs a backup job
type JobRunner func(ctx context.Context, job *models.BackupJob) error

// Service manages job scheduling
type Service struct {
	db        *database.DB
	logger    *logging.Logger
	cron      *cron.Cron
	jobRunner JobRunner
	mu        sync.RWMutex
	entries   map[int64]cron.EntryID
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewService creates a new scheduler service
func NewService(db *database.DB, logger *logging.Logger, jobRunner JobRunner) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Service{
		db:        db,
		logger:    logger,
		cron:      cron.New(cron.WithSeconds()),
		jobRunner: jobRunner,
		entries:   make(map[int64]cron.EntryID),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the scheduler
func (s *Service) Start() error {
	s.logger.Info("Starting scheduler", nil)

	// Load all enabled jobs
	if err := s.loadJobs(); err != nil {
		return err
	}

	s.cron.Start()

	// Start next run updater
	go s.updateNextRuns()

	return nil
}

// Stop stops the scheduler
func (s *Service) Stop() {
	s.logger.Info("Stopping scheduler", nil)
	s.cancel()
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// loadJobs loads all enabled jobs from the database
func (s *Service) loadJobs() error {
	rows, err := s.db.Query(`
		SELECT id, name, source_id, pool_id, backup_type, schedule_cron, retention_days, enabled
		FROM backup_jobs WHERE enabled = 1 AND schedule_cron IS NOT NULL AND schedule_cron != ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var job models.BackupJob
		if err := rows.Scan(&job.ID, &job.Name, &job.SourceID, &job.PoolID, &job.BackupType, &job.ScheduleCron, &job.RetentionDays, &job.Enabled); err != nil {
			s.logger.Warn("Failed to scan job", map[string]interface{}{"error": err.Error()})
			continue
		}

		if err := s.scheduleJob(&job); err != nil {
			s.logger.Warn("Failed to schedule job", map[string]interface{}{
				"job_id": job.ID,
				"error":  err.Error(),
			})
		}
	}

	return nil
}

// scheduleJob adds a job to the scheduler
func (s *Service) scheduleJob(job *models.BackupJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing schedule if any
	if entryID, exists := s.entries[job.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.entries, job.ID)
	}

	if !job.Enabled || job.ScheduleCron == "" {
		return nil
	}

	// Create a copy of the job for the closure
	jobCopy := *job

	entryID, err := s.cron.AddFunc(job.ScheduleCron, func() {
		s.runJob(&jobCopy)
	})
	if err != nil {
		return err
	}

	s.entries[job.ID] = entryID

	s.logger.Info("Scheduled job", map[string]interface{}{
		"job_id":   job.ID,
		"job_name": job.Name,
		"schedule": job.ScheduleCron,
	})

	return nil
}

// runJob executes a backup job
func (s *Service) runJob(job *models.BackupJob) {
	s.logger.Info("Running scheduled job", map[string]interface{}{
		"job_id":   job.ID,
		"job_name": job.Name,
	})

	ctx, cancel := context.WithTimeout(s.ctx, 24*time.Hour)
	defer cancel()

	if err := s.jobRunner(ctx, job); err != nil {
		s.logger.Error("Scheduled job failed", map[string]interface{}{
			"job_id": job.ID,
			"error":  err.Error(),
		})
	}

	// Update last run time
	s.db.Exec("UPDATE backup_jobs SET last_run_at = CURRENT_TIMESTAMP WHERE id = ?", job.ID)
}

// AddJob adds or updates a job schedule
func (s *Service) AddJob(job *models.BackupJob) error {
	return s.scheduleJob(job)
}

// RemoveJob removes a job from the scheduler
func (s *Service) RemoveJob(jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entries[jobID]; exists {
		s.cron.Remove(entryID)
		delete(s.entries, jobID)
		s.logger.Info("Removed job from scheduler", map[string]interface{}{"job_id": jobID})
	}
}

// GetNextRun returns the next scheduled run time for a job
func (s *Service) GetNextRun(jobID int64) *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entryID, exists := s.entries[jobID]; exists {
		entry := s.cron.Entry(entryID)
		if !entry.Next.IsZero() {
			return &entry.Next
		}
	}
	return nil
}

// updateNextRuns periodically updates next run times in the database
func (s *Service) updateNextRuns() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			for jobID, entryID := range s.entries {
				entry := s.cron.Entry(entryID)
				if !entry.Next.IsZero() {
					s.db.Exec("UPDATE backup_jobs SET next_run_at = ? WHERE id = ?", entry.Next, jobID)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// ReloadJobs reloads all jobs from the database
func (s *Service) ReloadJobs() error {
	// Clear all existing entries
	s.mu.Lock()
	for jobID, entryID := range s.entries {
		s.cron.Remove(entryID)
		delete(s.entries, jobID)
	}
	s.mu.Unlock()

	// Reload from database
	return s.loadJobs()
}

// ListScheduledJobs returns info about all scheduled jobs
func (s *Service) ListScheduledJobs() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []map[string]interface{}
	for jobID, entryID := range s.entries {
		entry := s.cron.Entry(entryID)
		jobs = append(jobs, map[string]interface{}{
			"job_id":   jobID,
			"next_run": entry.Next,
			"prev_run": entry.Prev,
		})
	}

	return jobs
}

// ParseCron validates a cron expression
func ParseCron(expr string) error {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err
}
