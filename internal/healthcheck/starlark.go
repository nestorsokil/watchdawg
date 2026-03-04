package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"watchdawg/internal/models"
	"watchdawg/internal/starlarkeval"
)

type StarlarkChecker struct {
	NoOpInitializer
	logger   *slog.Logger
	recorder MetricsRecorder
}

func NewStarlarkChecker(logger *slog.Logger, recorder MetricsRecorder) *StarlarkChecker {
	return &StarlarkChecker{logger: logger, recorder: recorder}
}

func (k *StarlarkChecker) IsMatching(check *models.HealthCheck) bool { return check.Starlark != nil }

func (s *StarlarkChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	return executeWithRetry(ctx, check, s.executeOnce)
}

func (s *StarlarkChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	attemptStart := time.Now()
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: attemptStart,
		Attempt:   attempt,
	}
	defer func() {
		s.recorder.RecordCheckAttempt(check.Name, result.Healthy, time.Since(attemptStart).Seconds())
	}()

	globals := s.buildGlobals(check)

	healthy, message, err := starlarkeval.RunCheckScript(
		ctx,
		fmt.Sprintf("healthcheck-%s", check.Name),
		check.Name+".star",
		check.Starlark.Script,
		globals,
	)
	if err != nil {
		result.Healthy = false
		result.Error = err.Error()
		result.Message = result.Error
		return result
	}

	result.Healthy = healthy
	result.Message = message

	return result
}

func (s *StarlarkChecker) buildGlobals(check *models.HealthCheck) starlark.StringDict {
	globals := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}

	if check.Starlark.Globals != nil {
		for key, value := range check.Starlark.Globals {
			globals[key] = starlarkeval.ToStarlarkValue(value)
		}
	}

	// TODO: Add HTTP client function for making requests from Starlark
	// This will allow Starlark scripts to make HTTP calls

	return globals
}
