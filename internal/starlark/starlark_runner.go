// package starlark

// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"strings"
// 	"time"

// 	"go.starlark.net/starlark"
// )

// func RunStarlarkScript(script string, globals starlark.StringDict) (bool, string, error) {
// 	thread := &starlark.Thread{
// 		Name: fmt.Sprintf("starlark-runner-%s", time.Now().UnixNano()),
// 	}

// 	_, execErr := starlark.ExecFile(thread, "validation.star", script, globals)
// 	if execErr != nil {
// 		return false, "", fmt.Errorf("script execution failed: %w", execErr)
// 	}

// 	return true, "", nil
// }