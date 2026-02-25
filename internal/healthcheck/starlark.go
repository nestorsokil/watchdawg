package healthcheck

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"watchdawg/internal/models"
)

// StarlarkChecker executes Starlark-based health checks
type StarlarkChecker struct{}

// NewStarlarkChecker creates a new Starlark health checker
func NewStarlarkChecker() *StarlarkChecker {
	return &StarlarkChecker{}
}

// Execute performs a Starlark health check with retries
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

	// Create Starlark thread
	thread := &starlark.Thread{
		Name: fmt.Sprintf("healthcheck-%s", check.Name),
	}

	// Prepare global variables
	globals := s.buildGlobals(check)

	// Execute the Starlark script
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

	// If no check() function found, fall back to global variables
	if !found {
		healthy, message = s.extractResult(globals)
	}

	result.Healthy = healthy
	result.Message = message

	return result
}

// buildGlobals creates the global environment for the Starlark script
func (s *StarlarkChecker) buildGlobals(check *models.HealthCheck) starlark.StringDict {
	globals := starlark.StringDict{
		// Add built-in functions that health checks might need
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}

	// Add user-defined globals
	if check.Starlark.Globals != nil {
		for key, value := range check.Starlark.Globals {
			globals[key] = s.toStarlarkValue(value)
		}
	}

	// TODO: Add HTTP client function for making requests from Starlark
	// This will allow Starlark scripts to make HTTP calls

	return globals
}

// callCheckFunction attempts to call a check() function from the script
// Returns (healthy, message, error)
// If check() function doesn't exist, returns ("", "", nil) to signal fallback
func (s *StarlarkChecker) callCheckFunction(thread *starlark.Thread, globals starlark.StringDict) (checkFunctionFound bool, healthy bool, message string, err error) {
	// Look for check() function in globals
	checkFunc, ok := globals["check"]
	if !ok {
		// No check() function found, signal to use fallback
		return false, false, "", nil
	}

	// Verify it's actually a callable
	callable, ok := checkFunc.(starlark.Callable)
	if !ok {
		return false, false, "", fmt.Errorf("'check' is not a function")
	}

	// Call the check() function with no arguments
	result, err := starlark.Call(thread, callable, nil, nil)
	if err != nil {
		return true, false, "", err
	}

	// Parse the return value
	// Expected: dict with "healthy" and optional "message" fields
	// Or: just a boolean value
	if boolVal, ok := result.(starlark.Bool); ok {
		// Simple boolean return
		return true,bool(boolVal), "", nil
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		return true,false, "", fmt.Errorf("check() must return a dict or bool, got %s", result.Type())
	}

	// Extract healthy field (required)
	healthyVal, found, err := dict.Get(starlark.String("healthy"))
	if err != nil {
		return true, false, "", err
	}
	if !found {
		return true, false, "", fmt.Errorf("check() result dict must contain 'healthy' field")
	}

	boolVal, ok := healthyVal.(starlark.Bool)
	if !ok {
		return true,false, "", fmt.Errorf("'healthy' field must be a boolean")
	}
	healthy = bool(boolVal)

	// Extract optional message field
	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return true, healthy, message, nil
}

// toStarlarkValue converts Go values to Starlark values
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

// extractResult extracts the health check result from Starlark execution
func (s *StarlarkChecker) extractResult(globals starlark.StringDict) (healthy bool, message string) {
	// Check if there's a "result" global variable
	if resultVal, ok := globals["result"]; ok {
		return s.parseResultValue(resultVal)
	}

	// Check if there's a "healthy" global variable
	if healthyVal, ok := globals["healthy"]; ok {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			healthy = bool(boolVal)
		}
	}

	// Check if there's a "message" global variable
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

// parseResultValue parses a result dict {"healthy": bool, "message": string}
func (s *StarlarkChecker) parseResultValue(val starlark.Value) (healthy bool, message string) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return false, "result must be a dict"
	}

	// Extract "healthy" field
	if healthyVal, found, _ := dict.Get(starlark.String("healthy")); found {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			healthy = bool(boolVal)
		}
	}

	// Extract "message" field
	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return healthy, message
}
