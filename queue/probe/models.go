// Package probe contains a transport-agnostic contract and a matcher for checking outbound messages.
package probe

import "encoding/json"

// HeadersMode defines the message header comparison strategy.
type HeadersMode string

const (
	// HeadersModeExact requires a complete match of the set of headers.
	HeadersModeExact HeadersMode = "exact"
	// HeadersModeSubset requires only the expected subset of headers to match.
	HeadersModeSubset HeadersMode = "subset"
)

// Ordering defines the message order checking mode.
type Ordering string

const (
	// OrderingStrict requires strict order and the absence of unnecessary messages in the topic.
	OrderingStrict Ordering = "strict"
	// OrderingAny ignores the order of messages and only considers the number of matches.
	OrderingAny Ordering = "any"
)

// Message represents the broker's observable message in transport-agnostic form.
type Message struct {
	Topic   string            `json:"topic"`
	Key     *string           `json:"key,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Payload json.RawMessage   `json:"payload,omitempty"`
	Offset  int64             `json:"offset"`
}

// OutboundExpectation describes the expectations contract for checking an outgoing publication.
type OutboundExpectation struct {
	Topic         string            `json:"topic"`
	Key           *string           `json:"key,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HeadersMode   HeadersMode       `json:"headers_mode,omitempty"`
	ExpectedCount int               `json:"expected_count"`
	Payload       json.RawMessage   `json:"payload,omitempty"`
	PayloadSubset json.RawMessage   `json:"payload_subset,omitempty"`
	Ordering      Ordering          `json:"ordering,omitempty"`
}

// CheckResult contains the result of comparing the observed messages with the expectations contract.
type CheckResult struct {
	MatchedCount       int       `json:"matched_count"`
	ObservedMessages   []Message `json:"observed_messages"`
	LastMismatchReason string    `json:"last_mismatch_reason"`
}
