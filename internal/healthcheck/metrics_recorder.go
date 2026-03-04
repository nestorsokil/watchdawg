package healthcheck

// MetricsRecorder is the interface through which schedulers, checkers, and hook notifiers
// emit observability data. The no-op implementation is used when metrics are disabled.
type MetricsRecorder interface {
	// RecordCheckAttempt records a single execution attempt (including retried attempts).
	RecordCheckAttempt(checkName string, healthy bool, durationSec float64)

	// RecordCheckUp records the current health state after all attempts for a scheduled tick.
	RecordCheckUp(checkName string, healthy bool)

	// RecordHookExecution records the outcome of a single hook delivery.
	// trigger is "on_success" or "on_failure"; hookResult is "success" or "failure".
	RecordHookExecution(checkName, hookType, target, trigger, hookResult string)

	// RecordHookDuration records how long a single hook execution took in seconds.
	// trigger is "on_success" or "on_failure".
	RecordHookDuration(checkName, hookType, target, trigger string, durationSec float64)

	// RecordMessageAge records the age of the most recently received message in seconds.
	// Only called after at least one message has been received.
	RecordMessageAge(checkName string, ageSec float64)
}

// NoopMetricsRecorder discards all metrics. Used when the metrics block is absent from config.
type NoopMetricsRecorder struct{}

func (NoopMetricsRecorder) RecordCheckAttempt(_ string, _ bool, _ float64)      {}
func (NoopMetricsRecorder) RecordCheckUp(_ string, _ bool)                       {}
func (NoopMetricsRecorder) RecordHookExecution(_, _, _, _, _ string)             {}
func (NoopMetricsRecorder) RecordHookDuration(_, _, _, _ string, _ float64)      {}
func (NoopMetricsRecorder) RecordMessageAge(_ string, _ float64)                 {}
