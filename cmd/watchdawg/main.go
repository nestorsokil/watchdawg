package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"watchdawg/internal/config"
	"watchdawg/internal/healthcheck"
	"watchdawg/internal/metrics"
)

func main() {
	logger := buildLogger()
	slog.SetDefault(logger)

	logger.Info("Watchdawg - Dynamic Health Checking Service")

	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	logger.Info("Loading configuration", "source", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("Configuration loaded", "checks", len(cfg.HealthChecks))

	var recorder healthcheck.MetricsRecorder = healthcheck.NoopMetricsRecorder{}
	var metricsServer *metrics.MetricsServer

	if cfg.Metrics != nil {
		metricsServer = metrics.NewMetricsServer(cfg.Metrics, logger)
		recorder = metricsServer
	}

	scheduler := healthcheck.NewScheduler(logger, recorder)

	for _, check := range cfg.HealthChecks {
		if err := scheduler.AddHealthCheck(check); err != nil {
			logger.Error("Failed to schedule check", "check", check.Name, "error", err)
			os.Exit(1)
		}
	}

	scheduler.Start()
	logger.Info("Health checks are running. Press Ctrl+C to stop.")

	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	defer metricsCancel()

	if metricsServer != nil {
		go func() {
			if err := metricsServer.Start(metricsCtx); err != nil {
				logger.Error("Metrics server stopped with error", "error", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Received shutdown signal")
	metricsCancel()
	scheduler.Stop()
	logger.Info("Watchdawg stopped")
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
