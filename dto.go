package itestkit

import "encoding/json"

// caseFile describes the JSONC contract of the integration case.
type caseFile struct {
	Name        string            `json:"name"`
	Labels      []string          `json:"labels,omitempty"`
	Description string            `json:"description,omitempty"`
	Steps       []stepFile        `json:"steps"`
	Assert      assertFileSection `json:"assert"`
}

// stepKindFile describes the valid step types of a JSONC contract.
type stepKindFile string

// awaitRetryFile describes the retry policy for the kind=await step.
type awaitRetryFile struct {
	TimeoutMS   int `json:"timeout_ms"`
	IntervalMS  int `json:"interval_ms"`
	MaxAttempts int `json:"max_attempts"`
}

// stepFile describes one step with a handler and a request payload.
type stepFile struct {
	ID      string          `json:"id"`
	Kind    stepKindFile    `json:"kind"`
	Handler string          `json:"handler"`
	Request json.RawMessage `json:"request"`
	Retry   *awaitRetryFile `json:"retry,omitempty"`
}

// assertFileSection describes the expected status and response.
type assertFileSection struct {
	Code             string          `json:"code"`
	MessageContains  string          `json:"message_contains"`
	Response         json.RawMessage `json:"response"`
	ResponseFromStep string          `json:"response_from_step"`
	ResponseMode     ResponseMode    `json:"response_mode"`
}
