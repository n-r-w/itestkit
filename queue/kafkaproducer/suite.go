// Package kafkaproducer contains helpers for running Kafka in tests and checking outbound messages.
package kafkaproducer

import (
	"context"
	"errors"
	"fmt"

	testkafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

// Suite manages the lifecycle of a Kafka container at the test suite level.
type Suite struct {
	container *testkafka.KafkaContainer
	brokers   []string
}

// StartSuite picks up a Kafka container and returns a suite helper.
func StartSuite(ctx context.Context) (*Suite, error) {
	container, err := testkafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		return nil, fmt.Errorf("start kafka container: %w", err)
	}

	brokers, err := container.Brokers(ctx)
	if err != nil {
		if terminateErr := container.Terminate(ctx); terminateErr != nil {
			return nil, errors.Join(
				fmt.Errorf("read kafka brokers: %w", err),
				fmt.Errorf("terminate kafka container: %w", terminateErr),
			)
		}
		return nil, fmt.Errorf("read kafka brokers: %w", err)
	}

	return &Suite{container: container, brokers: append([]string(nil), brokers...)}, nil
}

// Brokers returns a copy of the bootstrap brokers list.
func (suite *Suite) Brokers() []string {
	if suite == nil {
		return nil
	}

	return append([]string(nil), suite.brokers...)
}

// NewHarness creates a case-level harness with namespace-isolated topics.
func (suite *Suite) NewHarness(caseNamespace string) (*Harness, error) {
	if suite == nil {
		return nil, errors.New("kafka suite is nil")
	}

	return NewHarness(suite.brokers, caseNamespace)
}

// Close terminates the Kafka container and releases resources.
func (suite *Suite) Close(ctx context.Context) error {
	if suite == nil || suite.container == nil {
		return nil
	}

	if err := suite.container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminate kafka container: %w", err)
	}

	suite.container = nil
	suite.brokers = nil
	return nil
}
