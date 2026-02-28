package healthcheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"watchdawg/internal/models"
)

// mockKafkaReader is a controllable fake kafkaReader for unit tests.
// Push messages via the messages channel; send on errCh to simulate errors.
type mockKafkaReader struct {
	messages chan kafka.Message
	errCh    chan error
	closed   bool
	mu       sync.Mutex
}

func newMockReader() *mockKafkaReader {
	return &mockKafkaReader{
		messages: make(chan kafka.Message, 10),
		errCh:    make(chan error, 1),
	}
}

func (m *mockKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	select {
	case msg := <-m.messages:
		return msg, nil
	case err := <-m.errCh:
		return kafka.Message{}, err
	case <-ctx.Done():
		return kafka.Message{}, ctx.Err()
	}
}

func (m *mockKafkaReader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockKafkaReader) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// newMockedKafkaChecker returns a KafkaChecker whose reader factory always
// returns the provided mock reader (ignoring broker/topic/group arguments).
func newMockedKafkaChecker(mock *mockKafkaReader) *KafkaChecker {
	return &KafkaChecker{
		consumers: make(map[string]*kafkaConsumerState),
		logger:    testLogger(),
		newReader: func(brokers []string, topic, groupID string) kafkaReader {
			return mock
		},
	}
}

func kafkaCheck(name, schedule string) models.HealthCheck {
	return models.HealthCheck{
		Name:     name,
		Type:     models.CheckTypeKafka,
		Schedule: schedule,
		Timeout:  5 * time.Second,
		Kafka: &models.KafkaCheckConfig{
			Brokers: []string{"localhost:9092"},
			Topic:   "test-topic",
			GroupID: "watchdawg-" + name,
		},
	}
}

// sendAndWait pushes a message to the mock and waits until the consumer
// goroutine has processed it (visible via the check's consumer state).
func sendAndWait(t *testing.T, checker *KafkaChecker, mock *mockKafkaReader, checkName string, msg kafka.Message) {
	t.Helper()
	mock.messages <- msg
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		checker.mu.RLock()
		state := checker.consumers[checkName]
		checker.mu.RUnlock()
		if state != nil {
			state.mu.RLock()
			has := state.hasReceivedMessage
			state.mu.RUnlock()
			if has {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for consumer to process message for check '%s'", checkName)
}

// ── Liveness tests ────────────────────────────────────────────────────────────

func TestKafkaChecker_NoMessagesYet_ReportsHealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("no-msg", "30s")
	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	result := checker.Execute(context.Background(), &check)
	if !result.Healthy {
		t.Fatalf("expected healthy while waiting for first message, got: %s", result.Message)
	}
	if result.CheckName != check.Name {
		t.Fatalf("expected CheckName=%q, got %q", check.Name, result.CheckName)
	}
}

func TestKafkaChecker_MessageWithinInterval_ReportsHealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("within-interval", "30s")
	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("hello")})

	result := checker.Execute(context.Background(), &check)
	if !result.Healthy {
		t.Fatalf("expected healthy after recent message, got: %s", result.Message)
	}
}

func TestKafkaChecker_MessageTooOld_ReportsUnhealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("stale-msg", "30s")
	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("old")})

	// Backdate the lastMessageTime to simulate a stale message.
	checker.mu.RLock()
	state := checker.consumers[check.Name]
	checker.mu.RUnlock()

	state.mu.Lock()
	state.lastMessageTime = time.Now().Add(-60 * time.Second)
	state.mu.Unlock()

	result := checker.Execute(context.Background(), &check)
	if result.Healthy {
		t.Fatalf("expected unhealthy for stale message, got: %s", result.Message)
	}
}

func TestKafkaChecker_ConsumerNotRunning_ReportsUnhealthy(t *testing.T) {
	checker := newMockedKafkaChecker(newMockReader())

	check := kafkaCheck("no-consumer", "30s")
	// Intentionally do NOT call StartConsumer.

	result := checker.Execute(context.Background(), &check)
	if result.Healthy {
		t.Fatal("expected unhealthy when consumer was never started")
	}
}

// ── Assertion tests ───────────────────────────────────────────────────────────

func TestKafkaChecker_AssertionPasses_ReportsHealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("assert-pass", "30s")
	check.Kafka.Assertion = `"hello" in value`

	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("hello world")})

	result := checker.Execute(context.Background(), &check)
	if !result.Healthy {
		t.Fatalf("expected healthy when assertion passes, got: %s", result.Message)
	}
}

func TestKafkaChecker_AssertionFails_ReportsUnhealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("assert-fail", "30s")
	check.Kafka.Assertion = `"secret" in value`

	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("hello world")})

	result := checker.Execute(context.Background(), &check)
	if result.Healthy {
		t.Fatalf("expected unhealthy when assertion fails, got: %s", result.Message)
	}
}

func TestKafkaChecker_JSONAssertion_ReportsHealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("json-assert", "30s")
	check.Kafka.Format = models.ResponseFormatJSON
	check.Kafka.Assertion = `result["status"] == "ok"`

	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte(`{"status":"ok"}`)})

	result := checker.Execute(context.Background(), &check)
	if !result.Healthy {
		t.Fatalf("expected healthy for JSON assertion pass, got: %s", result.Message)
	}
}

func TestKafkaChecker_InvalidJSONWithFormat_ReportsUnhealthy(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("bad-json", "30s")
	check.Kafka.Format = models.ResponseFormatJSON
	check.Kafka.Assertion = `result["status"] == "ok"`

	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("not json")})

	result := checker.Execute(context.Background(), &check)
	if result.Healthy {
		t.Fatalf("expected unhealthy for invalid JSON, got: %s", result.Message)
	}
}

// ── Stop / cleanup tests ──────────────────────────────────────────────────────

func TestKafkaChecker_Stop_ClosesReader(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("stop-test", "30s")
	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}

	checker.Stop()

	// Give the goroutine a moment to react to context cancellation.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.isClosed() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("reader was not closed after Stop()")
}

// ── Consumer error resilience ─────────────────────────────────────────────────

func TestKafkaChecker_ConsumerError_DoesNotCrash(t *testing.T) {
	mock := newMockReader()
	checker := newMockedKafkaChecker(mock)

	check := kafkaCheck("error-resilience", "30s")
	if err := checker.StartConsumer(context.Background(), check); err != nil {
		t.Fatalf("StartConsumer: %v", err)
	}
	defer checker.Stop()

	// Inject a transient error; consumer should continue after logging.
	mock.errCh <- errors.New("simulated broker error")

	// Then send a valid message; consumer should still process it.
	sendAndWait(t, checker, mock, check.Name, kafka.Message{Value: []byte("recovered")})

	result := checker.Execute(context.Background(), &check)
	if !result.Healthy {
		t.Fatalf("expected healthy after error + recovery, got: %s", result.Message)
	}
}
