package healthcheck

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"watchdawg/internal/models"
)

type Scheduler struct {
	cron            *cron.Cron
	httpChecker     *HTTPChecker
	starlarkChecker *StarlarkChecker
	notifier        *HookNotifier
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:            cron.New(cron.WithSeconds()),
		httpChecker:     NewHTTPChecker(),
		starlarkChecker: NewStarlarkChecker(),
		notifier:        NewHookNotifier(),
	}
}

func (s *Scheduler) AddHealthCheck(check models.HealthCheck) error {
	schedule := s.parseSchedule(check.Schedule)

	_, err := s.cron.AddFunc(schedule, func() {
		s.executeHealthCheck(check)
	})

	if err != nil {
		return fmt.Errorf("failed to schedule check '%s': %w", check.Name, err)
	}

	log.Printf("Scheduled health check '%s' with schedule: %s", check.Name, check.Schedule)
	return nil
}

func (s *Scheduler) Start() {
	log.Println("Starting health check scheduler...")
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	log.Println("Stopping health check scheduler...")
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) parseSchedule(schedule string) string {
	schedule = strings.TrimSpace(schedule)

	if strings.HasSuffix(schedule, "s") || strings.HasSuffix(schedule, "m") || strings.HasSuffix(schedule, "h") {
		duration, err := time.ParseDuration(schedule)
		if err == nil {
			seconds := int(duration.Seconds())
			if seconds < 60 {
				return fmt.Sprintf("*/%d * * * * *", seconds)
			}
			minutes := seconds / 60
			if minutes < 60 {
				return fmt.Sprintf("0 */%d * * * *", minutes)
			}
			hours := minutes / 60
			return fmt.Sprintf("0 0 */%d * * *", hours)
		}
	}

	// Standard cron: "minute hour day month weekday"
	// We support: "second minute hour day month weekday"
	parts := strings.Fields(schedule)
	if len(parts) == 5 {
		return "0 " + schedule
	}

	return schedule
}

func (s *Scheduler) executeHealthCheck(check models.HealthCheck) {
	log.Printf("Executing health check: %s", check.Name)

	ctx, cancel := context.WithTimeout(context.Background(), check.Timeout)
	defer cancel()

	var result *models.CheckResult

	switch check.Type {
	case models.CheckTypeHTTP:
		result = s.httpChecker.Execute(ctx, &check)
	case models.CheckTypeStarlark:
		result = s.starlarkChecker.Execute(ctx, &check)
	default:
		log.Printf("ERROR: Unknown check type '%s' for check '%s'", check.Type, check.Name)
		return
	}

	if result.Healthy {
		log.Printf("✓ Health check '%s' PASSED: %s (took %dms)", check.Name, result.Message, result.Duration)
	} else {
		log.Printf("✗ Health check '%s' FAILED: %s (took %dms)", check.Name, result.Message, result.Duration)
		if result.Error != "" {
			log.Printf("  Error: %s", result.Error)
		}
	}

	if result.Healthy && len(check.OnSuccess) > 0 {
		if err := s.notifier.NotifySuccess(check.OnSuccess, result); err != nil {
			log.Printf("Failed to send success hook(s) for '%s': %v", check.Name, err)
		} else {
			log.Printf("Sent success hook(s) for '%s'", check.Name)
		}
	}

	if !result.Healthy && len(check.OnFailure) > 0 {
		if err := s.notifier.NotifyFailure(check.OnFailure, result); err != nil {
			log.Printf("Failed to send failure hook(s) for '%s': %v", check.Name, err)
		} else {
			log.Printf("Sent failure hook(s) for '%s'", check.Name)
		}
	}
}
