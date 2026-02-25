package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"watchdawg/internal/config"
	"watchdawg/internal/healthcheck"
)

func main() {
	log.Println("WatchDawg - Dynamic Health Checking Service")
	log.Println("============================================")

	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	log.Printf("Loading configuration from: %s", *configPath)
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Loaded %d health check(s)", len(cfg.HealthChecks))

	scheduler := healthcheck.NewScheduler()

	for _, check := range cfg.HealthChecks {
		if err := scheduler.AddHealthCheck(check); err != nil {
			log.Fatalf("Failed to schedule check '%s': %v", check.Name, err)
		}
	}

	scheduler.Start()
	log.Println("Health checks are running. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nReceived shutdown signal...")
	scheduler.Stop()
	log.Println("WatchDawg stopped. Goodbye!")
}
