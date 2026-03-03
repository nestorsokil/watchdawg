package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"watchdawg/internal/config"
	"watchdawg/internal/healthcheck"
)

func main() {
	logger := buildLogger()
	slog.SetDefault(logger)

	logger.Info("WatchDawg - Dynamic Health Checking Service")

	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	logger.Info("Loading configuration", "source", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("Configuration loaded", "checks", len(cfg.HealthChecks))

	scheduler := healthcheck.NewScheduler(logger)

	for _, check := range cfg.HealthChecks {
		if err := scheduler.AddHealthCheck(check); err != nil {
			logger.Error("Failed to schedule check", "check", check.Name, "error", err)
			os.Exit(1)
		}
	}

	scheduler.Start()
	logger.Info("Health checks are running. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Received shutdown signal")
	scheduler.Stop()
	logger.Info("WatchDawg stopped")
}

func buildLogger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(handler)
}
