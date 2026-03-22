package starlarkeval

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.starlark.net/starlark"
)

// NewHTTPRequestBuiltin constructs the http_request Starlark builtin.
// The builtin is a closure over ctx, client, and maxBodyBytes so callers
// do not need to pass them at call time — the execution environment wires
// them in at script-evaluation time.
//
// Signature: http_request(url, method="GET", body=None, headers=None)
//
// Returns a dict with fields: status_code, headers, body, error.
// Network errors set the error field rather than raising; type errors
// (non-string url/method, non-dict headers) raise hard Starlark errors.
func NewHTTPRequestBuiltin(ctx context.Context, client *http.Client, maxBodyBytes int) *starlark.Builtin {
	return starlark.NewBuiltin("http_request", func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			urlVal     starlark.String
			methodVal  starlark.String = "GET"
			bodyVal    starlark.Value  = starlark.None
			headersVal starlark.Value  = starlark.None
		)

		if err := starlark.UnpackArgs(b.Name(), args, kwargs,
			"url", &urlVal,
			"method?", &methodVal,
			"body?", &bodyVal,
			"headers?", &headersVal,
		); err != nil {
			return nil, err
		}

		return doHTTPRequest(ctx, client, maxBodyBytes, string(urlVal), string(methodVal), bodyVal, headersVal)
	})
}

// errorDict returns a response dict with status_code=0, empty headers/body, and the given error string.
func errorDict(errMsg string) *starlark.Dict {
	d := starlark.NewDict(4)
	d.SetKey(starlark.String("status_code"), starlark.MakeInt(0))
	d.SetKey(starlark.String("headers"), starlark.NewDict(0))
	d.SetKey(starlark.String("body"), starlark.String(""))
	d.SetKey(starlark.String("error"), starlark.String(errMsg))
	return d
}

func doHTTPRequest(ctx context.Context, client *http.Client, maxBodyBytes int, rawURL, method string, bodyVal, headersVal starlark.Value) (starlark.Value, error) {
	// Check context before doing any work.
	if ctx.Err() != nil {
		return errorDict(fmt.Sprintf("request cancelled: %v", ctx.Err())), nil
	}

	// Validate URL scheme.
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return errorDict(fmt.Sprintf("unsupported URL scheme in %q: only http and https are supported", rawURL)), nil
	}

	// Build optional request body.
	var bodyReader io.Reader
	if bodyVal != starlark.None {
		bodyStr, ok := bodyVal.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("http_request: body must be a string or None, got %s", bodyVal.Type())
		}
		bodyReader = strings.NewReader(string(bodyStr))
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), rawURL, bodyReader)
	if err != nil {
		return errorDict(fmt.Sprintf("failed to create request: %v", err)), nil
	}

	// Apply optional headers.
	if headersVal != starlark.None {
		headersDict, ok := headersVal.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("http_request: headers must be a dict or None, got %s", headersVal.Type())
		}
		for _, item := range headersDict.Items() {
			k, ok1 := item[0].(starlark.String)
			v, ok2 := item[1].(starlark.String)
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("http_request: header keys and values must be strings")
			}
			req.Header.Set(string(k), string(v))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return errorDict(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	// Cap body reads to prevent unbounded memory use.
	limited := io.LimitReader(resp.Body, int64(maxBodyBytes)+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return errorDict(fmt.Sprintf("failed to read response body: %v", err)), nil
	}

	var errField starlark.Value = starlark.None
	if len(bodyBytes) > maxBodyBytes {
		bodyBytes = bodyBytes[:maxBodyBytes]
		errField = starlark.String(fmt.Sprintf("response body truncated at %d bytes", maxBodyBytes))
	}

	// Build headers dict (multi-value headers joined with ", ").
	headersDict := starlark.NewDict(len(resp.Header))
	for key, values := range resp.Header {
		headersDict.SetKey(starlark.String(key), starlark.String(strings.Join(values, ", ")))
	}

	result := starlark.NewDict(4)
	result.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	result.SetKey(starlark.String("headers"), headersDict)
	result.SetKey(starlark.String("body"), starlark.String(string(bodyBytes)))
	result.SetKey(starlark.String("error"), errField)
	return result, nil
}
