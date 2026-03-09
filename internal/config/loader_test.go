package config

import (
	"strings"
	"testing"

	"watchdawg/internal/models"
)

// minimalConfig returns a valid minimal config for use as a baseline in tests.
func minimalConfig() *models.Config {
	return &models.Config{
		HealthChecks: []models.HealthCheck{
			{
				Name:     "test-check",
				Schedule: "30s",
				HTTP:     &models.HTTPCheckConfig{URL: "http://example.com", Method: "GET", Expected: models.ExpectedHTTPResponse{StatusCode: models.StatusCodeMatcher{Codes: []int{200}}}},
			},
		},
	}
}

func TestHistoryConfig_DBPathRequired(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: ""}

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected error for missing db_path, got nil")
	} else if !strings.Contains(err.Error(), "db_path") {
		t.Errorf("error should mention db_path, got: %v", err)
	}
}

func TestHistoryConfig_InvalidRetention(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db", Retention: -1}

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected error for negative retention, got nil")
	} else if !strings.Contains(err.Error(), "retention") {
		t.Errorf("error should mention retention, got: %v", err)
	}
}

func TestHistoryConfig_RetentionDefaultsTo1000(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db"}

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.History.Retention != 1000 {
		t.Errorf("expected default retention 1000, got %d", cfg.History.Retention)
	}
}

func TestHistoryConfig_ValidBlock(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db", Retention: 500}

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error for valid history block: %v", err)
	}
}

func TestHistoryConfig_RecordAllDefault(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db"}

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RecordAll defaults to false (zero value); no explicit default needed
	if cfg.History.RecordAllChecks {
		t.Error("expected RecordAll to default to false")
	}
}

func TestHistoryConfig_PerCheckRetentionOverride(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db", Retention: 1000}
	cfg.HealthChecks[0].Retention = 250

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error for valid per-check retention: %v", err)
	}
}

func TestHistoryConfig_PerCheckRetentionNegative(t *testing.T) {
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db"}
	cfg.HealthChecks[0].Retention = -5

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected error for negative per-check retention, got nil")
	} else if !strings.Contains(err.Error(), "retention") {
		t.Errorf("error should mention retention, got: %v", err)
	}
}

func TestHistoryConfig_PerCheckRetentionZeroAllowed(t *testing.T) {
	// 0 means "use global default at write time" — not an error at config load
	cfg := minimalConfig()
	cfg.History = &models.HistoryConfig{DBPath: "./test.db"}
	cfg.HealthChecks[0].Retention = 0

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error for zero per-check retention: %v", err)
	}
}

func TestNoHistoryBlock_ZeroImpact(t *testing.T) {
	// No history block must not affect existing checks in any way
	cfg := minimalConfig()

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error with no history block: %v", err)
	}
	if cfg.History != nil {
		t.Error("History should remain nil when not configured")
	}
}
