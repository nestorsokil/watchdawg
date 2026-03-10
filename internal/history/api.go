package history

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultLimit = 100

// Handler serves the execution history REST API.
type Handler struct {
	store  ExecutionStore
	logger *slog.Logger
}

// NewHandler creates a Handler backed by the given store.
func NewHandler(store ExecutionStore, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handleHistory(w, r)
}

// handleHistory dispatches GET /history/{check_name} and GET /history/*.
func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.logger.Debug("History API request", "method", r.Method, "path", r.URL.Path)

	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Strip the "/history/" prefix to get the check name or "*".
	checkName := strings.TrimPrefix(r.URL.Path, "/history/")

	if checkName == "*" {
		h.handleAll(w, r, limit)
	} else {
		if len(checkName) > 256 || strings.ContainsAny(checkName, "/\x00") {
			writeError(w, "invalid check name", http.StatusBadRequest)
			return
		}
		h.handleCheck(w, r, checkName, limit)
	}
}

func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request, checkName string, limit int) {
	records, err := h.store.QueryCheck(r.Context(), checkName, limit)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, "no history found for check \""+checkName+"\"", http.StatusNotFound)
			return
		}
		h.logger.Error("QueryCheck failed", "check", checkName, "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, buildResponse(map[string][]Record{checkName: records}))
}

func (h *Handler) handleAll(w http.ResponseWriter, r *http.Request, limit int) {
	all, err := h.store.QueryAll(r.Context(), limit)
	if err != nil {
		h.logger.Error("QueryAll failed", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, buildResponse(all))
}

// --- response shapes ---

type checksResponse struct {
	Checks map[string][]recordJSON `json:"checks"`
}

type recordJSON struct {
	Timestamp  time.Time `json:"timestamp"`
	Healthy    bool      `json:"healthy"`
	DurationMs int64     `json:"duration_ms"`
	Error      string    `json:"error"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func buildResponse(data map[string][]Record) checksResponse {
	checks := make(map[string][]recordJSON, len(data))
	for name, records := range data {
		recs := make([]recordJSON, len(records))
		for i, r := range records {
			recs[i] = recordJSON{
				Timestamp:  r.Timestamp,
				Healthy:    r.Healthy,
				DurationMs: r.DurationMs,
				Error:      r.Error,
			}
		}
		checks[name] = recs
	}
	return checksResponse{Checks: checks}
}

// --- helpers ---

func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid limit: must be a positive integer")
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers already sent; nothing further we can do.
		return
	}
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg}) //nolint:errcheck
}
