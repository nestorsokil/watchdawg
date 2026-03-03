package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"watchdawg/internal/models"
)

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
				check.HTTP.Method = "GET"
			}
		case models.CheckTypeStarlark:
			if check.Starlark == nil {
				return fmt.Errorf("healthcheck[%d] (%s): Starlark config is required for type 'starlark'", i, check.Name)
			}
			if check.Starlark.Script == "" {
				return fmt.Errorf("healthcheck[%d] (%s): Starlark script is required", i, check.Name)
			}
		case models.CheckTypeKafka:
			if check.Kafka == nil {
				return fmt.Errorf("healthcheck[%d] (%s): kafka config is required for type 'kafka'", i, check.Name)
			}
			if len(check.Kafka.Brokers) == 0 {
				return fmt.Errorf("healthcheck[%d] (%s): at least one kafka broker is required", i, check.Name)
			}
			if check.Kafka.Topic == "" {
				return fmt.Errorf("healthcheck[%d] (%s): kafka topic is required", i, check.Name)
			}
			// Kafka checks compare message age against the schedule interval, so the
			// schedule must be a parseable duration rather than a cron expression.
			if _, err := parseDuration(check.Schedule); err != nil {
				return fmt.Errorf("healthcheck[%d] (%s): kafka checks require a duration-format schedule (e.g. '30s', '5m', '1h')", i, check.Name)
			}
			if check.Kafka.GroupID == "" {
				config.HealthChecks[i].Kafka.GroupID = "watchdawg-" + check.Name
			}
		case models.CheckTypeGRPC:
			if check.GRPC == nil {
				return fmt.Errorf("healthcheck[%d] (%s): grpc config is required for type 'grpc'", i, check.Name)
			}
			if check.GRPC.Target == "" {
				return fmt.Errorf("healthcheck[%d] (%s): grpc target is required", i, check.Name)
			}
		default:
			return fmt.Errorf("healthcheck[%d] (%s): unknown check type '%s'", i, check.Name, check.Type)
		}

		if check.Retries < 0 {
			check.Retries = 0
		}
		if check.Timeout == 0 {
			check.Timeout = 30 * 1000000000 // 30 seconds in nanoseconds
		}

		for j, h := range check.OnSuccess {
			if err := validateHookConfig(fmt.Sprintf("healthcheck[%d] (%s) on_success[%d]", i, check.Name, j), h); err != nil {
				return err
			}
		}
		for j, h := range check.OnFailure {
			if err := validateHookConfig(fmt.Sprintf("healthcheck[%d] (%s) on_failure[%d]", i, check.Name, j), h); err != nil {
				return err
			}
		}
	}

	return nil
}

// parseDuration parses duration-format schedules like "30s", "5m", "2h".
// Returns an error if the schedule is not a valid duration string.
func parseDuration(schedule string) (time.Duration, error) {
	schedule = strings.TrimSpace(schedule)
	return time.ParseDuration(schedule)
}

func validateHookConfig(fieldName string, hook models.HookConfig) error {
	set := 0
	if hook.HTTP != nil {
		set++
	}
	if hook.Kafka != nil {
		set++
	}
	if set == 0 {
		return fmt.Errorf("%s: must specify exactly one of 'http' or 'kafka'", fieldName)
	}
	if set > 1 {
		return fmt.Errorf("%s: only one hook type may be specified per entry", fieldName)
	}
	if hook.HTTP != nil {
		if hook.HTTP.URL == "" {
			return fmt.Errorf("%s.http: url is required", fieldName)
		}
		if hook.HTTP.Method == "" {
			hook.HTTP.Method = "POST"
		}
	}
	if hook.Kafka != nil {
		if hook.Kafka.Topic == "" {
			return fmt.Errorf("%s.kafka: topic is required", fieldName)
		}
		if len(hook.Kafka.Brokers) == 0 {
			return fmt.Errorf("%s.kafka: at least one broker is required", fieldName)
		}
	}
	return nil
}
