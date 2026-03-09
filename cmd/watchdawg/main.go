package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"watchdawg/internal/config"
	"watchdawg/internal/healthcheck"
	"watchdawg/internal/history"
	"watchdawg/internal/metrics"
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

	httpCtx, httpCancel := context.WithCancel(context.Background())
	defer httpCancel()

	// History setup: open store, wire recorder, start background goroutine.
	var histRecorder *history.Recorder
	if cfg.History != nil {
		store, err := history.NewSQLiteStore(cfg.History, logger)
		if err != nil {
			logger.Error("Failed to open history store", "path", cfg.History.DBPath, "error", err)
			os.Exit(1)
		}
		histRecorder = history.NewRecorder(store, cfg.History.Retention, logger)
		scheduler.SetHistoryRecorder(histRecorder, cfg.History)

		go histRecorder.Start(httpCtx)
		defer histRecorder.Stop()

		if metricsServer != nil {
			mux := http.NewServeMux()
			mux.Handle("/metrics", metricsServer.Handler())
			mux.Handle("/history/", history.NewHandler(store, logger).Handler())
			startHTTPServer(metricsServer.Address(), mux, httpCtx, logger)
		}
	} else if metricsServer != nil {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metricsServer.Handler())
		startHTTPServer(metricsServer.Address(), mux, httpCtx, logger)
	}

	scheduler.Start()
	logger.Info("Health checks are running. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Received shutdown signal")
	httpCancel()
	scheduler.Stop()
	logger.Info("WatchDawg stopped")
}

// startHTTPServer starts a shared http.Server on addr in a goroutine and shuts it down when ctx is cancelled.
func startHTTPServer(addr string, mux *http.ServeMux, ctx context.Context, logger *slog.Logger) {
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		logger.Info("HTTP server listening", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server stopped with error", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			logger.Error("HTTP server shutdown error", "error", err)
		}
	}()
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
