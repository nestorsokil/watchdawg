package healthcheck

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"watchdawg/internal/models"
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

	thread := &starlark.Thread{
		Name: fmt.Sprintf("healthcheck-%s", check.Name),
	}

	globals := s.buildGlobals(check)

	// IMPORTANT: ExecFile returns the updated globals with new definitions
	var err error
	globals, err = starlark.ExecFile(thread, check.Name+".star", check.Starlark.Script, globals)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("script execution failed: %v", err)
		result.Message = result.Error
		return result
	}

	// Try to call check() function first (preferred approach)
	// If not found, fall back to reading global variables
	found, healthy, message, err := s.callCheckFunction(thread, globals)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("check function failed: %v", err)
		result.Message = result.Error
		return result
	}

	if !found {
		healthy, message = s.extractResult(globals)
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
			globals[key] = s.toStarlarkValue(value)
		}
	}

	// TODO: Add HTTP client function for making requests from Starlark
	// This will allow Starlark scripts to make HTTP calls

	return globals
}

func (s *StarlarkChecker) callCheckFunction(thread *starlark.Thread, globals starlark.StringDict) (checkFunctionFound bool, healthy bool, message string, err error) {
	checkFunc, ok := globals["check"]
	if !ok {
		return false, false, "", nil
	}

	callable, ok := checkFunc.(starlark.Callable)
	if !ok {
		return false, false, "", fmt.Errorf("'check' is not a function")
	}

	result, err := starlark.Call(thread, callable, nil, nil)
	if err != nil {
		return true, false, "", err
	}

	if boolVal, ok := result.(starlark.Bool); ok {
		return true, bool(boolVal), "", nil
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		return true, false, "", fmt.Errorf("check() must return a dict or bool, got %s", result.Type())
	}

	healthyVal, found, err := dict.Get(starlark.String("healthy"))
	if err != nil {
		return true, false, "", err
	}
	if !found {
		return true, false, "", fmt.Errorf("check() result dict must contain 'healthy' field")
	}

	boolVal, ok := healthyVal.(starlark.Bool)
	if !ok {
		return true, false, "", fmt.Errorf("'healthy' field must be a boolean")
	}
	healthy = bool(boolVal)

	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return true, healthy, message, nil
}

func (s *StarlarkChecker) toStarlarkValue(v interface{}) starlark.Value {
	switch val := v.(type) {
	case string:
		return starlark.String(val)
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case float64:
		return starlark.Float(val)
	case bool:
		return starlark.Bool(val)
	case map[string]interface{}:
		dict := &starlark.Dict{}
		for k, v := range val {
			dict.SetKey(starlark.String(k), s.toStarlarkValue(v))
		}
		return dict
	case []interface{}:
		var elems []starlark.Value
		for _, elem := range val {
			elems = append(elems, s.toStarlarkValue(elem))
		}
		return starlark.NewList(elems)
	default:
		return starlark.None
	}
}

func (s *StarlarkChecker) extractResult(globals starlark.StringDict) (healthy bool, message string) {
	if resultVal, ok := globals["result"]; ok {
		return s.parseResultValue(resultVal)
	}

	if healthyVal, ok := globals["healthy"]; ok {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			healthy = bool(boolVal)
		}
	}

	if msgVal, ok := globals["message"]; ok {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	// If no explicit result found, assume success
	if message == "" {
		healthy = true
		message = "Starlark check completed"
	}

	return healthy, message
}

func (s *StarlarkChecker) parseResultValue(val starlark.Value) (healthy bool, message string) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return false, "result must be a dict"
	}

	if healthyVal, found, _ := dict.Get(starlark.String("healthy")); found {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			healthy = bool(boolVal)
		}
	}

	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return healthy, message
}
