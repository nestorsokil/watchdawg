package healthcheck

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"go.starlark.net/starlark"

	"watchdawg/internal/models"
	"watchdawg/internal/starlarkeval"
)

// kafkaReader is the minimal interface for consuming Kafka messages.
// The interface enables injecting mock readers in unit tests.
type kafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	Close() error
}

// receivedMessage holds the content of the last consumed Kafka message.
type receivedMessage struct {
	Value   []byte
	Key     []byte
	Headers map[string]string
}

// kafkaConsumerState tracks the live state for one Kafka check's background consumer.
type kafkaConsumerState struct {
	mu                 sync.RWMutex
	lastMessageTime    time.Time
	hasReceivedMessage bool
	lastMessage        *receivedMessage
	expectedInterval   time.Duration
	cancel             context.CancelFunc
}

// KafkaChecker monitors Kafka topics for message liveness.
//
// For each registered check a background consumer goroutine tracks the
// most-recently received message. On each Execute call the checker
// verifies that a message arrived within the expected schedule interval.
// If no message has been received since startup the check is considered
// healthy (waiting for the first message), matching the user-selected
// "healthy until first violation" policy.
type KafkaChecker struct {
	consumers map[string]*kafkaConsumerState
	mu        sync.RWMutex
	// newReader is injectable so tests can replace the real kafka.Reader.
	newReader func(brokers []string, topic, groupID string) kafkaReader
}

func NewKafkaChecker() *KafkaChecker {
	return &KafkaChecker{
		consumers: make(map[string]*kafkaConsumerState),
		newReader: func(brokers []string, topic, groupID string) kafkaReader {
			return kafka.NewReader(kafka.ReaderConfig{
				Brokers:     brokers,
				Topic:       topic,
				GroupID:     groupID,
				StartOffset: kafka.LastOffset, // ignore backlog; only new messages matter
				MinBytes:    1,
				MaxBytes:    10 * 1024 * 1024, // 10 MiB
			})
		},
	}
}

// StartConsumer registers and launches a background consumer for the given
// kafka check. The consumer runs until Stop is called.
func (k *KafkaChecker) StartConsumer(check models.HealthCheck) error {
	interval, err := time.ParseDuration(check.Schedule)
	if err != nil {
		// Config validation should have caught this; guard defensively.
		return fmt.Errorf("invalid schedule duration for kafka check '%s': %w", check.Name, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	state := &kafkaConsumerState{
		expectedInterval: interval,
		cancel:           cancel,
	}

	reader := k.newReader(check.Kafka.Brokers, check.Kafka.Topic, check.Kafka.GroupID)

	k.mu.Lock()
	k.consumers[check.Name] = state
	k.mu.Unlock()

	log.Printf("Starting Kafka consumer for check '%s' on topic '%s' via brokers %v",
		check.Name, check.Kafka.Topic, check.Kafka.Brokers)

	go k.consumeMessages(ctx, check.Name, reader, state)

	return nil
}

func (k *KafkaChecker) consumeMessages(ctx context.Context, checkName string, reader kafkaReader, state *kafkaConsumerState) {
	defer reader.Close()

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("Kafka consumer for check '%s' stopped", checkName)
				return
			}
			log.Printf("Kafka consumer error for check '%s': %v — will retry", checkName, err)
			continue
		}

		headers := make(map[string]string, len(msg.Headers))
		for _, h := range msg.Headers {
			headers[h.Key] = string(h.Value)
		}

		state.mu.Lock()
		state.lastMessageTime = time.Now()
		state.hasReceivedMessage = true
		state.lastMessage = &receivedMessage{
			Value:   msg.Value,
			Key:     msg.Key,
			Headers: headers,
		}
		state.mu.Unlock()
	}
}

// Execute evaluates the liveness of the configured Kafka topic.
//
// The check passes when:
//   - No messages have been received yet (waiting for first message), OR
//   - A message was received within the expected interval AND any configured
//     assertion passes against the most recently received message.
func (k *KafkaChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	startTime := time.Now()
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: startTime,
		Attempt:   1,
	}

	k.mu.RLock()
	state, ok := k.consumers[check.Name]
	k.mu.RUnlock()

	if !ok {
		result.Healthy = false
		result.Error = "kafka consumer not running for this check"
		result.Message = result.Error
		result.Duration = time.Since(startTime).Milliseconds()
		return result
	}

	state.mu.RLock()
	hasMsg := state.hasReceivedMessage
	lastTime := state.lastMessageTime
	expectedInterval := state.expectedInterval
	var lastMsg *receivedMessage
	if state.lastMessage != nil {
		msgCopy := *state.lastMessage
		lastMsg = &msgCopy
	}
	state.mu.RUnlock()

	// No messages yet: report healthy while waiting for the producer to start.
	if !hasMsg {
		result.Healthy = true
		result.Message = fmt.Sprintf("waiting for first message on topic '%s'", check.Kafka.Topic)
		result.Duration = time.Since(startTime).Milliseconds()
		return result
	}

	age := time.Since(lastTime)
	if age > expectedInterval {
		result.Healthy = false
		result.Message = fmt.Sprintf("no message on topic '%s' for %v (expected at least every %v)",
			check.Kafka.Topic, age.Truncate(time.Millisecond), expectedInterval)
		result.Duration = time.Since(startTime).Milliseconds()
		return result
	}

	if check.Kafka.Assertion != "" && lastMsg != nil {
		valid, assertionMsg, err := k.validateWithStarlark(check.Kafka.Assertion, check.Kafka.Format, lastMsg)
		if err != nil {
			result.Healthy = false
			result.Error = fmt.Sprintf("assertion error: %v", err)
			result.Message = result.Error
			result.Duration = time.Since(startTime).Milliseconds()
			return result
		}
		result.Healthy = valid
		if assertionMsg != "" {
			result.Message = assertionMsg
		} else if valid {
			result.Message = fmt.Sprintf("Kafka check passed: message received %v ago, assertion passed",
				age.Truncate(time.Millisecond))
		} else {
			result.Message = fmt.Sprintf("Kafka assertion failed on message received %v ago",
				age.Truncate(time.Millisecond))
		}
	} else {
		result.Healthy = true
		result.Message = fmt.Sprintf("Kafka check passed: message received %v ago on topic '%s'",
			age.Truncate(time.Millisecond), check.Kafka.Topic)
	}

	result.Duration = time.Since(startTime).Milliseconds()
	return result
}

// Stop cancels all background consumers.
func (k *KafkaChecker) Stop() {
	k.mu.RLock()
	defer k.mu.RUnlock()
	for name, state := range k.consumers {
		log.Printf("Stopping Kafka consumer for check '%s'", name)
		state.cancel()
	}
}

// validateWithStarlark runs a Starlark assertion against a received Kafka message.
func (k *KafkaChecker) validateWithStarlark(script string, format models.ResponseFormat, msg *receivedMessage) (valid bool, message string, err error) {
	headersDict := &starlark.Dict{}
	for key, value := range msg.Headers {
		headersDict.SetKey(starlark.String(key), starlark.String(value))
	}

	globals := starlark.StringDict{
		"value":   starlark.String(string(msg.Value)),
		"key":     starlark.String(string(msg.Key)),
		"headers": headersDict,
	}

	if format != models.ResponseFormatNone {
		parsedResult, parseErr := starlarkeval.ParseResponseBody(string(msg.Value), format)
		if parseErr != nil {
			return false, "", fmt.Errorf("failed to parse message value as %s: %w", format, parseErr)
		}
		globals["result"] = parsedResult
	}

	return starlarkeval.RunAssertionScript("kafka-validation", "kafka-validation.star", script, globals)
}

// KafkaPublisher publishes messages to Kafka topics. It is co-located with the
// consumer so all Kafka I/O lives in one file.
type KafkaPublisher struct{}

func NewKafkaPublisher() *KafkaPublisher {
	return &KafkaPublisher{}
}

// Publish writes a single message to the given topic via the provided brokers.
func (p *KafkaPublisher) Publish(ctx context.Context, brokers []string, topic string, message []byte) error {
	writer := &kafka.Writer{
		Addr:  kafka.TCP(brokers...),
		Topic: topic,
	}
	defer writer.Close()
	return writer.WriteMessages(ctx, kafka.Message{Value: message})
}
