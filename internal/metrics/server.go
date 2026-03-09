package metrics

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"watchdawg/internal/models"
)

// MetricsServer exposes Prometheus metrics over HTTP and implements healthcheck.MetricsRecorder.
type MetricsServer struct {
	cfg    *models.MetricsConfig
	logger *slog.Logger

	registry       *prometheus.Registry
	checkUp        *prometheus.GaugeVec
	checkExecTotal *prometheus.CounterVec
	checkDuration  *prometheus.HistogramVec
	hookExecTotal  *prometheus.CounterVec
	hookDuration   *prometheus.HistogramVec
	kafkaMessageAge *prometheus.GaugeVec
}

func NewMetricsServer(cfg *models.MetricsConfig, logger *slog.Logger) *MetricsServer {
	registry := prometheus.NewRegistry()

	checkUp := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchdawg",
		Name:      "check_up",
		Help:      "Whether the check is currently healthy (1=up, 0=down)",
	}, []string{"check"})

	checkExecTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "watchdawg",
		Name:      "check_executions_total",
		Help:      "Total number of check execution attempts",
	}, []string{"check", "result"})

	checkDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "watchdawg",
		Name:      "check_duration_seconds",
		Help:      "Duration of each check execution attempt in seconds",
		Buckets:   prometheus.DefBuckets,
	}, []string{"check"})

	hookExecTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "watchdawg",
		Name:      "hook_executions_total",
		Help:      "Total number of hook executions",
	}, []string{"check", "type", "target", "trigger", "result"})

	hookDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "watchdawg",
		Name:      "hook_duration_seconds",
		Help:      "Duration of each hook execution in seconds",
		Buckets:   prometheus.DefBuckets,
	}, []string{"check", "type", "target", "trigger"})

	kafkaMessageAge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "watchdawg",
		Name:      "check_message_age_seconds",
		Help:      "Age of the most recently received message in seconds",
	}, []string{"check"})

	registry.MustRegister(checkUp, checkExecTotal, checkDuration, hookExecTotal, hookDuration, kafkaMessageAge)

	return &MetricsServer{
		cfg:             cfg,
		logger:          logger,
		registry:        registry,
		checkUp:         checkUp,
		checkExecTotal:  checkExecTotal,
		checkDuration:   checkDuration,
		hookExecTotal:   hookExecTotal,
		hookDuration:    hookDuration,
		kafkaMessageAge: kafkaMessageAge,
	}
}

// Address returns the configured host:port for the metrics HTTP server.
func (s *MetricsServer) Address() string {
	return s.cfg.Address
}

// Handler returns the Prometheus HTTP handler for the /metrics endpoint.
func (s *MetricsServer) Handler() http.Handler {
	return promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		ErrorLog: slog.NewLogLogger(s.logger.Handler(), slog.LevelError),
	})
}

func (s *MetricsServer) RecordCheckAttempt(checkName string, healthy bool, durationSec float64) {
	resultLabel := resultLabel(healthy)
	s.checkExecTotal.WithLabelValues(checkName, resultLabel).Inc()
	s.checkDuration.WithLabelValues(checkName).Observe(durationSec)
}

func (s *MetricsServer) RecordCheckUp(checkName string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	s.checkUp.WithLabelValues(checkName).Set(value)
}

func (s *MetricsServer) RecordHookExecution(checkName, hookType, target, trigger, hookResult string) {
	s.hookExecTotal.WithLabelValues(checkName, hookType, target, trigger, hookResult).Inc()
}

func (s *MetricsServer) RecordHookDuration(checkName, hookType, target, trigger string, durationSec float64) {
	s.hookDuration.WithLabelValues(checkName, hookType, target, trigger).Observe(durationSec)
}

func (s *MetricsServer) RecordMessageAge(checkName string, ageSec float64) {
	s.kafkaMessageAge.WithLabelValues(checkName).Set(ageSec)
}

func resultLabel(healthy bool) string {
	if healthy {
		return "success"
	}
	return "failure"
}
