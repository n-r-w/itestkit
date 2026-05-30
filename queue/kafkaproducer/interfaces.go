package kafkaproducer

//go:generate go tool mockgen -source=interfaces.go -destination=interfaces_mock.go -package=kafkaproducer

import (
	"context"
)

// harnessHeader describes the kafka message header in transport-agnostic format.
type harnessHeader struct {
	Key   string
	Value []byte
}

// harnessMessage describes a kafka message in the transport-agnostic harness format.
type harnessMessage struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []harnessHeader
	Offset  int64
}

// messageWriter describes the minimal Kafka producer contract for harness.
type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...harnessMessage) error
	Close() error
}

// messageReader describes the minimal Kafka reader contract for harness.
type messageReader interface {
	FetchMessage(ctx context.Context) (harnessMessage, error)
	Close() error
}
