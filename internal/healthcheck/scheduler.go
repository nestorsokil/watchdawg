package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"watchdawg/internal/models"
)

// cronSlogAdapter bridges robfig/cron's Logger interface to slog.
// It is used to route cron internals (including recovered panics) through slog.
type cronSlogAdapter struct {
	logger *slog.Logger
}

func (a cronSlogAdapter) Info(msg string, keysAndValues ...interface{}) {
	a.logger.Info(msg, keysAndValues...)
}

func (a cronSlogAdapter) Error(err error, msg string, keysAndValues ...interface{}) {
	args := make([]interface{}, 0, len(keysAndValues)+2)
	args = append(args, "error", err)
	args = append(args, keysAndValues...)
	a.logger.Error(msg, args...)
}

type Scheduler struct {
	cron            *cron.Cron
	httpChecker     *HTTPChecker
	starlarkChecker *StarlarkChecker
	kafkaChecker    *KafkaChecker
	grpcChecker     *GRPCChecker
	notifier        *HookNotifier
	logger          *slog.Logger
	// rootCtx is cancelled in Stop to signal background workers (e.g. Kafka consumers).
	rootCtx    context.Context
	rootCancel context.CancelFunc
}

func NewScheduler(logger *slog.Logger) *Scheduler {
	adapter := cronSlogAdapter{logger: logger}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron: cron.New(
			cron.WithSeconds(),
			cron.WithChain(cron.Recover(adapter)),
			cron.WithLogger(adapter),
		),
		httpChecker:     NewHTTPChecker(logger),
		starlarkChecker: NewStarlarkChecker(logger),
		kafkaChecker:    NewKafkaChecker(logger),
		grpcChecker:     NewGRPCChecker(logger),
		notifier:        NewHookNotifier(logger),
		logger:          logger,
		rootCtx:         rootCtx,
		rootCancel:      rootCancel,
	}
}

func (s *Scheduler) AddHealthCheck(check models.HealthCheck) error {
	// Kafka checks require a background consumer started before the first
	// scheduled tick so the consumer is already listening when Execute runs.
	if check.Kafka != nil {
		if err := s.kafkaChecker.StartConsumer(s.rootCtx, check); err != nil {
			return fmt.Errorf("failed to start kafka consumer for check '%s': %w", check.Name, err)
		}
	}

	schedule := s.parseSchedule(check.Schedule)

	_, err := s.cron.AddFunc(schedule, func() {
		s.executeHealthCheck(check)
	})

	if err != nil {
		return fmt.Errorf("failed to schedule check '%s': %w", check.Name, err)
	}

	s.logger.Info("Scheduled health check", "check", check.Name, "schedule", check.Schedule)
	return nil
}

func (s *Scheduler) Start() {
	s.logger.Info("Starting health check scheduler")
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.logger.Info("Stopping health check scheduler")
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.rootCancel()
	s.kafkaChecker.Stop()
	s.logger.Info("Scheduler stopped")
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
	s.logger.Info("Executing health check", "check", check.Name)

	ctx, cancel := context.WithTimeout(context.Background(), check.Timeout)
	defer cancel()

	var result *models.CheckResult

	switch {
	case check.HTTP != nil:
		result = s.httpChecker.Execute(ctx, &check)
	case check.Starlark != nil:
		result = s.starlarkChecker.Execute(ctx, &check)
	case check.Kafka != nil:
		result = s.kafkaChecker.Execute(ctx, &check)
	case check.GRPC != nil:
		result = s.grpcChecker.Execute(ctx, &check)
	default:
		s.logger.Error("Check has no recognizable sub-config; skipping", "check", check.Name)
		return
	}

	if result.Healthy {
		s.logger.Info("Health check passed",
			"check", check.Name,
			"message", result.Message,
			"duration_ms", result.Duration,
		)
	} else {
		attrs := []interface{}{
			"check", check.Name,
			"message", result.Message,
			"duration_ms", result.Duration,
		}
		if result.Error != "" {
			attrs = append(attrs, "error", result.Error)
		}
		s.logger.Warn("Health check failed", attrs...)
	}

	if result.Healthy && len(check.OnSuccess) > 0 {
		if err := s.notifier.NotifySuccess(ctx, check.OnSuccess, result); err != nil {
			s.logger.Error("Failed to send success hooks", "check", check.Name, "error", err)
		} else {
			s.logger.Info("Sent success hooks", "check", check.Name)
		}
	}

	if !result.Healthy && len(check.OnFailure) > 0 {
		if err := s.notifier.NotifyFailure(ctx, check.OnFailure, result); err != nil {
			s.logger.Error("Failed to send failure hooks", "check", check.Name, "error", err)
		} else {
			s.logger.Info("Sent failure hooks", "check", check.Name)
		}
	}
}
