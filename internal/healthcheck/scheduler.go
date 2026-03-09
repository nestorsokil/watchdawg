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

// Checker executes a single health check and returns its result.
type Checker interface {
	IsMatching(check *models.HealthCheck) bool
	Init(ctx context.Context, check *models.HealthCheck) error
	Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult
	Cleanup(ctx context.Context) error
}

// noOpInitializer provides no-op Init and Cleanup for checkers that have no lifecycle.
type noOpInitializer struct{}

func (noOpInitializer) Init(ctx context.Context, check *models.HealthCheck) error { return nil }
func (noOpInitializer) Cleanup(ctx context.Context) error                         { return nil }

type Scheduler struct {
	cron     *cron.Cron
	checkers []Checker
	notifier *HookNotifier
	recorder MetricsRecorder
	history    HistoryRecorder       // nil when history is not configured
	historyCfg *models.HistoryConfig // nil when history is not configured
	logger   *slog.Logger
	// rootCtx is cancelled in Stop to signal background workers (e.g. Kafka consumers).
	rootCtx    context.Context
	rootCancel context.CancelFunc
}

func NewScheduler(logger *slog.Logger, recorder MetricsRecorder) *Scheduler {
	adapter := cronSlogAdapter{logger: logger}
	rootCtx, rootCancel := context.WithCancel(context.Background())

	return &Scheduler{
		cron: cron.New(
			cron.WithSeconds(),
			cron.WithChain(cron.Recover(adapter)),
			cron.WithLogger(adapter),
		),
		checkers: []Checker{
			NewHTTPChecker(logger, recorder),
			NewGRPCChecker(logger, recorder),
			NewKafkaChecker(logger, recorder),
			NewStarlarkChecker(logger, recorder),
		},
		notifier:   NewHookNotifier(logger, recorder),
		recorder:   recorder,
		logger:     logger,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}
}

// SetHistoryRecorder attaches a HistoryRecorder and its config to the scheduler.
// Must be called before Start(). Passing nil disables recording.
func (s *Scheduler) SetHistoryRecorder(h HistoryRecorder, cfg *models.HistoryConfig) {
	s.history = h
	s.historyCfg = cfg
}

func (s *Scheduler) AddHealthCheck(check models.HealthCheck) error {
	var matchedChecker Checker
	for _, checker := range s.checkers {
		if checker.IsMatching(&check) {
			matchedChecker = checker
			break
		}
	}

	if matchedChecker == nil {
		return fmt.Errorf("no checker found for health check '%s': no recognised check type configured", check.Name)
	}

	if err := matchedChecker.Init(s.rootCtx, &check); err != nil {
		return fmt.Errorf("failed to initialise checker for '%s': %w", check.Name, err)
	}

	schedule := s.parseSchedule(check.Schedule)

	_, err := s.cron.AddFunc(schedule, func() {
		s.executeHealthCheck(check, matchedChecker)
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
	// Cancel background workers (e.g. Kafka consumers) before draining cron jobs,
	// so they begin shutting down in parallel with the job drain.
	s.rootCancel()
	cronCtx := s.cron.Stop()
	<-cronCtx.Done()
	for _, checker := range s.checkers {
		if err := checker.Cleanup(context.Background()); err != nil {
			s.logger.Error("Failed cleanup on checker", "checker", fmt.Sprintf("%T", checker), "error", err)
		}
	}
	s.notifier.Close()
	s.logger.Info("Scheduler stopped")
}

func (s *Scheduler) parseSchedule(schedule string) string {
	schedule = strings.TrimSpace(schedule)

	// Attempt duration parsing first; this handles "30s", "5m", "2h", "500ms", etc.
	// Only fall through to cron parsing when it is not a valid duration string.
	if duration, err := time.ParseDuration(schedule); err == nil {
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

	// Standard cron: "minute hour day month weekday"
	// We support: "second minute hour day month weekday"
	parts := strings.Fields(schedule)
	if len(parts) == 5 {
		return "0 " + schedule
	}

	return schedule
}

func (s *Scheduler) executeHealthCheck(check models.HealthCheck, checker Checker) {
	s.logger.Info("Executing health check", "check", check.Name)

	ctx, cancel := context.WithTimeout(context.Background(), check.Timeout)
	defer cancel()

	result := checker.Execute(ctx, &check)

	s.recorder.RecordCheckUp(check.Name, result.Healthy)

	if s.history != nil && (s.historyCfg.RecordAll || check.Record) {
		s.history.Record(&check, result)
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
