package healthcheck

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"watchdawg/internal/models"
	"watchdawg/internal/starlarkeval"
)

type StarlarkChecker struct{}

func NewStarlarkChecker() *StarlarkChecker {
	return &StarlarkChecker{}
}

func (s *StarlarkChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	startTime := time.Now()

	var lastResult *models.CheckResult
	maxAttempts := check.Retries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result := s.executeOnce(ctx, check, attempt)
		lastResult = result

		if result.Healthy {
			result.Duration = time.Since(startTime).Milliseconds()
			return result
		}

		if attempt < maxAttempts {
			time.Sleep(1 * time.Second)
		}
	}

	lastResult.Duration = time.Since(startTime).Milliseconds()
	return lastResult
}

func (s *StarlarkChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: time.Now(),
		Attempt:   attempt,
	}

	globals := s.buildGlobals(check)

	healthy, message, err := starlarkeval.RunCheckScript(
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
