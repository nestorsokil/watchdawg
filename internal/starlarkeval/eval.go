// Package starlarkeval provides shared utilities for executing Starlark scripts
// in Watchdawg health checks. It consolidates all starlark.ExecFile calls and
// result-extraction logic that is otherwise duplicated across HTTP assertions,
// Starlark checks, and Kafka assertions.
package starlarkeval

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"go.starlark.net/starlark"

	"watchdawg/internal/models"
)

// RunAssertionScript executes a validation/assertion script against a pre-built
// globals dict and extracts a boolean validity result plus an optional message.
//
// If the script is a simple single-line expression (no assignments, definitions,
// or imports) it is automatically wrapped as `valid = <expr>`.
//
// Result extraction order:
//  1. `result` dict containing a "valid" or "healthy" key
//  2. `valid` or `healthy` bool global
//  3. `message` string global
func RunAssertionScript(ctx context.Context, threadName, filename, script string, globals starlark.StringDict) (valid bool, message string, err error) {
	if ctx.Err() != nil {
		return false, "", ctx.Err()
	}

	if IsSimpleExpression(script) {
		script = fmt.Sprintf("valid = %s", script)
	}

	thread := &starlark.Thread{Name: threadName}

	moduleGlobals, execErr := starlark.ExecFile(thread, filename, script, globals)
	if execErr != nil {
		return false, "", fmt.Errorf("script execution failed: %w", execErr)
	}

	// A script-set "result" dict with validation fields takes precedence,
	// but only when it contains "valid"/"healthy" to avoid collisions with a
	// pre-injected "result" variable holding parsed response bodies.
	if resultVal, ok := moduleGlobals["result"]; ok {
		if dict, isDict := resultVal.(*starlark.Dict); isDict {
			if HasValidationFields(dict) {
				return ParseValidationResult(resultVal)
			}
		}
	}

	if validVal, ok := moduleGlobals["valid"]; ok {
		if boolVal, ok := validVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	} else if healthyVal, ok := moduleGlobals["healthy"]; ok {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	}

	if msgVal, ok := moduleGlobals["message"]; ok {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return valid, message, nil
}

// RunCheckScript executes a full Starlark health-check script and returns the
// health status. It mirrors the StarlarkChecker execution model:
//  1. Execute the script
//  2. If a callable `check()` function was defined, call it and use its return value
//  3. Otherwise fall back to reading global variables: `result`, `healthy`, `message`
//
// When no explicit result is set the check defaults to healthy.
func RunCheckScript(ctx context.Context, threadName, filename, script string, globals starlark.StringDict) (healthy bool, message string, err error) {
	if ctx.Err() != nil {
		return false, "", ctx.Err()
	}

	thread := &starlark.Thread{Name: threadName}

	moduleGlobals, execErr := starlark.ExecFile(thread, filename, script, globals)
	if execErr != nil {
		return false, "", fmt.Errorf("script execution failed: %w", execErr)
	}

	found, checkHealthy, checkMsg, callErr := callCheckFunction(thread, moduleGlobals)
	if callErr != nil {
		return false, "", fmt.Errorf("check function failed: %w", callErr)
	}
	if found {
		return checkHealthy, checkMsg, nil
	}

	healthy, message = extractCheckResult(moduleGlobals)
	return healthy, message, nil
}

// scriptKeywordPattern matches assignment targets (valid/healthy/message with optional
// surrounding whitespace around =), function definitions, and import statements.
// Used to distinguish simple expressions from full scripts.
var scriptKeywordPattern = regexp.MustCompile(`\b(valid|healthy|message)\s*=|def |\bimport\b`)

// IsSimpleExpression reports whether script is a single-line expression that
// should be auto-wrapped as `valid = <expr>`. Multi-line scripts, scripts with
// assignments/definitions, and import statements are not considered simple.
func IsSimpleExpression(script string) bool {
	script = strings.TrimSpace(script)

	if strings.Contains(script, "\n") {
		return false
	}

	return !scriptKeywordPattern.MatchString(script)
}

// toStarlark is the shared conversion core. onUnknown determines what Starlark
// value is returned for types that have no natural mapping (e.g. structs).
func toStarlark(v interface{}, onUnknown func(interface{}) starlark.Value) starlark.Value {
	switch val := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(val)
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case float64:
		return starlark.Float(val)
	case string:
		return starlark.String(val)
	case []interface{}:
		elems := make([]starlark.Value, len(val))
		for i, item := range val {
			elems[i] = toStarlark(item, onUnknown)
		}
		return starlark.NewList(elems)
	case map[string]interface{}:
		dict := starlark.NewDict(len(val))
		for key, value := range val {
			dict.SetKey(starlark.String(key), toStarlark(value, onUnknown))
		}
		return dict
	default:
		return onUnknown(v)
	}
}

// GoToStarlark converts an arbitrary Go value (as produced by json.Unmarshal
// into interface{}) to its Starlark equivalent. Unknown types are stringified
// so that no information from a parsed response body is silently dropped.
func GoToStarlark(v interface{}) starlark.Value {
	return toStarlark(v, func(v interface{}) starlark.Value {
		return starlark.String(fmt.Sprintf("%v", v))
	})
}

// ToStarlarkValue converts a user-provided Go value (from the config globals
// map) to its Starlark equivalent. Unknown types map to starlark.None because
// config values are expected to be well-typed primitives or collections.
func ToStarlarkValue(v interface{}) starlark.Value {
	return toStarlark(v, func(interface{}) starlark.Value {
		return starlark.None
	})
}

// ParseResponseBody parses a raw string body into a Starlark value according to
// the expected format. Used to populate the `result` variable for assertions.
func ParseResponseBody(body string, format models.ResponseFormat) (starlark.Value, error) {
	switch format {
	case models.ResponseFormatJSON:
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			return nil, err
		}
		return GoToStarlark(data), nil

	case models.ResponseFormatXML:
		var data interface{}
		if err := xml.Unmarshal([]byte(body), &data); err != nil {
			return nil, err
		}
		return GoToStarlark(data), nil

	default:
		return starlark.None, nil
	}
}

// HasValidationFields reports whether a Starlark dict contains a "valid" or
// "healthy" key, indicating it is a validation-result dict rather than a
// pre-injected parsed response body.
func HasValidationFields(dict *starlark.Dict) bool {
	_, hasValid, _ := dict.Get(starlark.String("valid"))
	_, hasHealthy, _ := dict.Get(starlark.String("healthy"))
	return hasValid || hasHealthy
}

// ParseValidationResult extracts (valid, message) from a Starlark dict that
// contains validation fields. Used when a script sets a result dict directly.
func ParseValidationResult(val starlark.Value) (valid bool, message string, err error) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return false, "", fmt.Errorf("result must be a dict")
	}

	if validVal, found, _ := dict.Get(starlark.String("valid")); found {
		if boolVal, ok := validVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	} else if healthyVal, found, _ := dict.Get(starlark.String("healthy")); found {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	}

	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return valid, message, nil
}

// callCheckFunction looks for a callable `check` global in the post-execution
// module globals, calls it, and extracts the health result from its return value.
// Returns found=false when no `check` global is present.
func callCheckFunction(thread *starlark.Thread, globals starlark.StringDict) (found bool, healthy bool, message string, err error) {
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

	healthyVal, found2, err := dict.Get(starlark.String("healthy"))
	if err != nil {
		return true, false, "", err
	}
	if !found2 {
		return true, false, "", fmt.Errorf("check() result dict must contain 'healthy' field")
	}

	boolVal, ok := healthyVal.(starlark.Bool)
	if !ok {
		return true, false, "", fmt.Errorf("'healthy' field must be a boolean")
	}
	healthy = bool(boolVal)

	if msgVal, found3, _ := dict.Get(starlark.String("message")); found3 {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return true, healthy, message, nil
}

// extractCheckResult reads the health outcome from module globals when no
// check() function was defined. It checks (in order): "result" dict,
// "healthy" bool, "message" string. Defaults to healthy when nothing is set.
func extractCheckResult(globals starlark.StringDict) (healthy bool, message string) {
	if resultVal, ok := globals["result"]; ok {
		dict, ok := resultVal.(*starlark.Dict)
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

	// No explicit result: default to healthy.
	healthy = true
	if message == "" {
		message = "Starlark check completed"
	}

	return healthy, message
}
