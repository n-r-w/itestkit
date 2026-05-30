package kafkaproducer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/n-r-w/itestkit/queue/itest"
	"github.com/n-r-w/itestkit/queue/probe"
)

const (
	pollBatchTimeout     = 50 * time.Millisecond
	saramaNetworkTimeout = 5 * time.Second
	namespaceJoiner      = "-"
)

// PublishMessage describes the message data to be sent to Kafka.
type PublishMessage struct {
	Topic   string
	Key     *string
	Headers map[string]string
	Payload []byte
}

// Harness implements outbound checks and publishing via Kafka for a single case.
type Harness struct {
	brokers       []string
	caseNamespace string

	writer messageWriter
	reader messageReader

	ensureTopicFn func(ctx context.Context, topic string) error
	newReaderFn   func(topic string) (messageReader, error)

	mu       sync.Mutex
	plan     *probe.OutboundExpectation
	observed []probe.Message
}

// compileTimeHarnessCheck confirms the implementation of outbound harness contracts.
var _ itest.OutboundHarness = (*Harness)(nil)

// NewHarness creates a case-level helper with producer and consumer for outbound scenarios.
func NewHarness(brokers []string, caseNamespace string) (*Harness, error) {
	normalizedBrokers := normalizeBrokers(brokers)
	if len(normalizedBrokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}

	producer, err := sarama.NewSyncProducer(normalizedBrokers, newSaramaConfig())
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	harness := &Harness{
		brokers:       normalizedBrokers,
		caseNamespace: normalizeNamespace(caseNamespace),
		writer:        newSaramaWriter(producer),
		reader:        nil,
		ensureTopicFn: nil,
		newReaderFn:   nil,
		mu:            sync.Mutex{},
		plan:          nil,
		observed:      nil,
	}
	harness.ensureTopicFn = func(ctx context.Context, topic string) error {
		return ensureTopic(ctx, harness.brokers, topic)
	}
	harness.newReaderFn = func(topic string) (messageReader, error) {
		return newSaramaReader(harness.brokers, topic)
	}

	return harness, nil
}

// ResolveTopic adds a namespace prefix to the base topic to isolate cases.
func (harness *Harness) ResolveTopic(baseTopic string) string {
	normalizedTopic := strings.TrimSpace(baseTopic)
	if harness.caseNamespace == "" {
		return normalizedTopic
	}
	if normalizedTopic == "" {
		return harness.caseNamespace
	}

	return harness.caseNamespace + namespaceJoiner + normalizedTopic
}

// PublishJSON serializes the payload and sends the message to Kafka.
func (harness *Harness) PublishJSON(
	ctx context.Context,
	topic string,
	key *string,
	headers map[string]string,
	payload any,
) error {
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return harness.PublishRaw(ctx, PublishMessage{
		Topic:   topic,
		Key:     key,
		Headers: cloneHeaders(headers),
		Payload: payloadRaw,
	})
}

// PublishRaw sends the already serialized payload to Kafka.
func (harness *Harness) PublishRaw(ctx context.Context, message PublishMessage) error {
	normalizedTopic := harness.ResolveTopic(message.Topic)
	if normalizedTopic == "" {
		return errors.New("publish topic is required")
	}

	if err := harness.ensureTopicFn(ctx, normalizedTopic); err != nil {
		return fmt.Errorf("ensure topic %q: %w", normalizedTopic, err)
	}

	kafkaMessage := harnessMessage{
		Topic:   normalizedTopic,
		Key:     cloneOptionalStringAsBytes(message.Key),
		Value:   append([]byte(nil), message.Payload...),
		Headers: mapToKafkaHeaders(message.Headers),
		Offset:  0,
	}
	if err := harness.writer.WriteMessages(ctx, kafkaMessage); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}

	return nil
}

// PlanOutbound saves the wait and activates the reader with a "from now" boundary.
func (harness *Harness) PlanOutbound(ctx context.Context, expectation probe.OutboundExpectation) error {
	normalized, err := probe.NormalizeExpectation(expectation)
	if err != nil {
		return err
	}
	normalized.Topic = harness.ResolveTopic(normalized.Topic)

	if err = harness.ensureTopicFn(ctx, normalized.Topic); err != nil {
		return fmt.Errorf("ensure topic %q: %w", normalized.Topic, err)
	}

	reader, err := harness.newReaderFn(normalized.Topic)
	if err != nil {
		return fmt.Errorf("create reader: %w", err)
	}

	harness.mu.Lock()
	defer harness.mu.Unlock()

	if harness.reader != nil {
		if closeErr := harness.reader.Close(); closeErr != nil {
			return fmt.Errorf("replace reader: %w", closeErr)
		}
	}

	harness.reader = reader
	harness.plan = &normalized
	harness.observed = nil

	return nil
}

// AwaitOutbound performs intermediate checks on published messages.
func (harness *Harness) AwaitOutbound(ctx context.Context) (probe.CheckResult, error) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if err := harness.pollMessagesLocked(ctx); err != nil {
		return probe.CheckResult{}, err
	}

	normalizedPlan, err := harness.requirePlanLocked()
	if err != nil {
		return probe.CheckResult{}, err
	}

	windowExpired := false
	if deadline, ok := ctx.Deadline(); ok {
		windowExpired = !time.Now().Before(deadline)
	}

	result, err := probe.EvaluateAwait(normalizedPlan, harness.observed, windowExpired)
	return harness.normalizeResultTopics(result), err
}

// VerifyOutbound performs final verification of published messages.
func (harness *Harness) VerifyOutbound(ctx context.Context) (probe.CheckResult, error) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if err := harness.pollMessagesLocked(ctx); err != nil {
		return probe.CheckResult{}, err
	}

	normalizedPlan, err := harness.requirePlanLocked()
	if err != nil {
		return probe.CheckResult{}, err
	}

	result, err := probe.EvaluateVerify(normalizedPlan, harness.observed)
	return harness.normalizeResultTopics(result), err
}

// normalizeResultTopics returns messages with logical topics without a case namespace prefix.
func (harness *Harness) normalizeResultTopics(result probe.CheckResult) probe.CheckResult {
	if harness.caseNamespace == "" {
		return result
	}

	prefix := harness.caseNamespace + namespaceJoiner
	normalizedMessages := make([]probe.Message, len(result.ObservedMessages))
	for index, message := range result.ObservedMessages {
		normalizedMessages[index] = message
		normalizedMessages[index].Topic = strings.TrimPrefix(message.Topic, prefix)
	}

	result.ObservedMessages = normalizedMessages
	return result
}

// CleanupBroker releases the consumer/producer harness resources.
func (harness *Harness) CleanupBroker(_ context.Context) error {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	var joinedErr error
	if harness.reader != nil {
		if err := harness.reader.Close(); err != nil {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("close reader: %w", err))
		}
		harness.reader = nil
	}
	if harness.writer != nil {
		if err := harness.writer.Close(); err != nil {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("close writer: %w", err))
		}
		harness.writer = nil
	}
	harness.plan = nil
	harness.observed = nil

	return joinedErr
}

// pollMessagesLocked reads available messages and updates the local snapshot.
func (harness *Harness) pollMessagesLocked(ctx context.Context) error {
	if harness.reader == nil {
		return errors.New("kafka reader is not initialized")
	}

	for {
		fetchCtx, cancel := context.WithTimeout(ctx, pollBatchTimeout)
		message, err := harness.reader.FetchMessage(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("fetch kafka message: %w", err)
		}

		harness.observed = append(harness.observed, mapFromKafkaMessage(message))
	}
}

// requirePlanLocked checks for an active wait plan.
func (harness *Harness) requirePlanLocked() (probe.OutboundExpectation, error) {
	if harness.plan == nil {
		return probe.OutboundExpectation{}, errors.New("outbound plan is not initialized")
	}

	return *harness.plan, nil
}

// normalizeBrokers filters empty broker addresses and returns a copy with normalization.
func normalizeBrokers(brokers []string) []string {
	normalized := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		trimmedBroker := strings.TrimSpace(broker)
		if trimmedBroker == "" {
			continue
		}
		normalized = append(normalized, trimmedBroker)
	}

	return normalized
}

// normalizeNamespace brings the namespace to a Kafka topic-safe format.
func normalizeNamespace(namespace string) string {
	normalized := strings.ToLower(strings.TrimSpace(namespace))
	if normalized == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(normalized))

	previousDash := false
	for _, symbol := range normalized {
		isAlphaNum := symbol >= 'a' && symbol <= 'z' || symbol >= '0' && symbol <= '9'
		isAllowedPunctuation := symbol == '-' || symbol == '_' || symbol == '.'
		if isAlphaNum || isAllowedPunctuation {
			builder.WriteRune(symbol)
			previousDash = symbol == '-'
			continue
		}

		if previousDash {
			continue
		}

		builder.WriteByte('-')
		previousDash = true
	}

	return strings.Trim(builder.String(), "-")
}

// ensureTopic ensures that topic exists in the Kafka cluster.
func ensureTopic(ctx context.Context, brokers []string, topic string) error {
	if topic == "" {
		return errors.New("topic is required")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	admin, err := sarama.NewClusterAdmin(brokers, newSaramaConfig())
	if err != nil {
		return fmt.Errorf("create cluster admin: %w", err)
	}
	defer func() {
		_ = admin.Close()
	}()

	topicConfig := &sarama.TopicDetail{ //nolint:exhaustruct // optional topic fields use zero-values
		NumPartitions:     1,
		ReplicationFactor: 1,
	}
	err = runWithContext(ctx, func() error {
		return admin.CreateTopic(topic, topicConfig, false)
	})
	if err != nil {
		if isTopicAlreadyExistsError(err) {
			return nil
		}

		return fmt.Errorf("create topic: %w", err)
	}

	return nil
}

// isTopicAlreadyExistsError specifies an error for an already existing topic.
func isTopicAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sarama.ErrTopicAlreadyExists) || strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// cloneHeaders copies the map of headers.
func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	maps.Copy(cloned, headers)
	return cloned
}

// cloneOptionalStringAsBytes copies the optional string key to []byte.
func cloneOptionalStringAsBytes(value *string) []byte {
	if value == nil {
		return nil
	}

	return []byte(*value)
}

// saramaMessageWriter adapts sarama.SyncProducer to messageWriter.
type saramaMessageWriter struct {
	producer sarama.SyncProducer
}

// newSaramaWriter creates a writer adapter over sarama.SyncProducer.
func newSaramaWriter(producer sarama.SyncProducer) *saramaMessageWriter {
	return &saramaMessageWriter{producer: producer}
}

// WriteMessages sends messages via sarama.SyncProducer.
func (writer *saramaMessageWriter) WriteMessages(ctx context.Context, messages ...harnessMessage) error {
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}

		producerMessage := &sarama.ProducerMessage{ //nolint:exhaustruct // optional message fields use zero-values
			Topic:   message.Topic,
			Headers: toSaramaRecordHeaders(message.Headers),
		}
		if len(message.Key) > 0 {
			producerMessage.Key = sarama.ByteEncoder(append([]byte(nil), message.Key...))
		}
		if len(message.Value) > 0 {
			producerMessage.Value = sarama.ByteEncoder(append([]byte(nil), message.Value...))
		}

		err := runWithContext(ctx, func() error {
			if _, _, sendErr := writer.producer.SendMessage(producerMessage); sendErr != nil {
				return fmt.Errorf("send kafka message: %w", sendErr)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// runWithContext performs a blocking operation with early exit to cancel the context.
func runWithContext(ctx context.Context, operation func() error) error {
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- operation()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-resultCh:
		return err
	}
}

// Close releases the producer's resources.
func (writer *saramaMessageWriter) Close() error {
	if writer.producer == nil {
		return nil
	}

	return writer.producer.Close()
}

// saramaMessageReader adapts consumer Sarama to messageReader.
type saramaMessageReader struct {
	consumer          sarama.Consumer
	partitionConsumer sarama.PartitionConsumer
}

// newSaramaReader creates a reader by topic and reads from the newest offset.
func newSaramaReader(brokers []string, topic string) (messageReader, error) {
	consumer, err := sarama.NewConsumer(brokers, newSaramaConfig())
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetNewest)
	if err != nil {
		_ = consumer.Close()
		return nil, fmt.Errorf("create partition consumer: %w", err)
	}

	return &saramaMessageReader{
		consumer:          consumer,
		partitionConsumer: partitionConsumer,
	}, nil
}

// FetchMessage receives one message or completes waiting on ctx.
func (reader *saramaMessageReader) FetchMessage(ctx context.Context) (harnessMessage, error) {
	select {
	case <-ctx.Done():
		return harnessMessage{}, ctx.Err()
	case consumerErr, ok := <-reader.partitionConsumer.Errors():
		if !ok {
			return harnessMessage{}, io.EOF
		}

		return harnessMessage{}, consumerErr.Err
	case message, ok := <-reader.partitionConsumer.Messages():
		if !ok {
			return harnessMessage{}, io.EOF
		}

		return harnessMessage{
			Topic:   message.Topic,
			Key:     append([]byte(nil), message.Key...),
			Value:   append([]byte(nil), message.Value...),
			Headers: fromSaramaRecordHeaders(message.Headers),
			Offset:  message.Offset,
		}, nil
	}
}

// Close releases the partition consumer and consumer resources.
func (reader *saramaMessageReader) Close() error {
	var joinedErr error
	if reader.partitionConsumer != nil {
		if err := reader.partitionConsumer.Close(); err != nil {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("close partition consumer: %w", err))
		}
		reader.partitionConsumer = nil
	}
	if reader.consumer != nil {
		if err := reader.consumer.Close(); err != nil {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("close consumer: %w", err))
		}
		reader.consumer = nil
	}

	return joinedErr
}

// newSaramaConfig returns the Sarama configuration for producer/consumer/admin operations.
func newSaramaConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Net.DialTimeout = saramaNetworkTimeout
	config.Net.ReadTimeout = saramaNetworkTimeout
	config.Net.WriteTimeout = saramaNetworkTimeout
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Timeout = saramaNetworkTimeout
	config.Producer.Return.Errors = true
	config.Producer.Return.Successes = true
	config.Consumer.Return.Errors = true
	return config
}

// toSaramaRecordHeaders converts headers harness to sarama headers.
func toSaramaRecordHeaders(headers []harnessHeader) []sarama.RecordHeader {
	if len(headers) == 0 {
		return nil
	}

	result := make([]sarama.RecordHeader, len(headers))
	for index, header := range headers {
		result[index] = sarama.RecordHeader{Key: []byte(header.Key), Value: append([]byte(nil), header.Value...)}
	}

	return result
}

// fromSaramaRecordHeaders converts sarama headers to headers harness.
func fromSaramaRecordHeaders(headers []*sarama.RecordHeader) []harnessHeader {
	if len(headers) == 0 {
		return nil
	}

	result := make([]harnessHeader, 0, len(headers))
	for _, header := range headers {
		if header == nil {
			continue
		}

		result = append(result, harnessHeader{Key: string(header.Key), Value: append([]byte(nil), header.Value...)})
	}

	return result
}

// mapToKafkaHeaders converts the map of headers to []harnessHeader.
func mapToKafkaHeaders(headers map[string]string) []harnessHeader {
	if len(headers) == 0 {
		return nil
	}

	result := make([]harnessHeader, 0, len(headers))
	for key, value := range headers {
		result = append(result, harnessHeader{Key: key, Value: []byte(value)})
	}

	return result
}

// mapFromKafkaMessage converts kafka.Message to a transport-agnostic probe.Message.
func mapFromKafkaMessage(message harnessMessage) probe.Message {
	var keyPtr *string
	if len(message.Key) > 0 {
		keyCopy := string(message.Key)
		keyPtr = &keyCopy
	}

	headers := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headers[header.Key] = string(header.Value)
	}

	return probe.Message{
		Topic:   message.Topic,
		Key:     keyPtr,
		Headers: headers,
		Payload: append([]byte(nil), message.Value...),
		Offset:  message.Offset,
	}
}
