package config

import (
	"encoding/json"
	"fmt"
	"os"

	"watchdawg/internal/models"
)

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(path string) (*models.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config models.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validateConfig performs basic validation on the configuration
func validateConfig(config *models.Config) error {
	if len(config.HealthChecks) == 0 {
		return fmt.Errorf("no health checks defined")
	}

	for i, check := range config.HealthChecks {
		if check.Name == "" {
			return fmt.Errorf("healthcheck[%d]: name is required", i)
		}

		if check.Schedule == "" {
			return fmt.Errorf("healthcheck[%d] (%s): schedule is required", i, check.Name)
		}

		switch check.Type {
		case models.CheckTypeHTTP:
			if check.HTTP == nil {
				return fmt.Errorf("healthcheck[%d] (%s): HTTP config is required for type 'http'", i, check.Name)
			}
			if check.HTTP.URL == "" {
				return fmt.Errorf("healthcheck[%d] (%s): HTTP URL is required", i, check.Name)
			}
			if check.HTTP.Method == "" {
				check.HTTP.Method = "GET" // Default to GET
			}
		case models.CheckTypeStarlark:
			if check.Starlark == nil {
				return fmt.Errorf("healthcheck[%d] (%s): Starlark config is required for type 'starlark'", i, check.Name)
			}
			if check.Starlark.Script == "" {
				return fmt.Errorf("healthcheck[%d] (%s): Starlark script is required", i, check.Name)
			}
		case models.CheckTypeGRPC, models.CheckTypeKafka:
			return fmt.Errorf("healthcheck[%d] (%s): type '%s' not yet implemented", i, check.Name, check.Type)
		default:
			return fmt.Errorf("healthcheck[%d] (%s): unknown check type '%s'", i, check.Name, check.Type)
		}

		// Set defaults
		if check.Retries < 0 {
			check.Retries = 0
		}
		if check.Timeout == 0 {
			check.Timeout = 30 * 1000000000 // 30 seconds in nanoseconds
		}
	}

	return nil
}
