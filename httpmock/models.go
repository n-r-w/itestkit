package httpmock

import "encoding/json"

// QueryMode controls query parameter matching.
type QueryMode string

// HeadersMode controls HTTP header matching.
type HeadersMode string

// Ordering controls request order verification.
type Ordering string

// CountMode controls how expected_count is checked.
type CountMode string

const (
	// QueryModeExact requires exact parsed query equality.
	QueryModeExact QueryMode = "exact"
	// QueryModeSubset allows extra actual query parameters.
	QueryModeSubset QueryMode = "subset"
	// HeadersModeExact requires exact header equality.
	HeadersModeExact HeadersMode = "exact"
	// HeadersModeSubset allows extra actual headers.
	HeadersModeSubset HeadersMode = "subset"
	// OrderingStrict requires the observed request order to match the plan order.
	OrderingStrict Ordering = "strict"
	// OrderingAny ignores observed request order during verification.
	OrderingAny Ordering = "any"
	// CountModeExact requires exactly expected_count matching requests.
	CountModeExact CountMode = "exact"
	// CountModeAtLeast requires at least expected_count matching requests.
	CountModeAtLeast CountMode = "at_least"
)

// Plan describes planned HTTP calls for one test case.
type Plan struct {
	Calls    []CallExpectation `json:"calls"`
	Ordering Ordering          `json:"ordering,omitempty"`
}

// CallExpectation describes one expected HTTP request and its stub response.
type CallExpectation struct {
	Name           string              `json:"name,omitempty"`
	ExpectedCount  int                 `json:"expected_count"`
	CountMode      CountMode           `json:"count_mode,omitempty"`
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	Query          map[string][]string `json:"query,omitempty"`
	QueryMode      QueryMode           `json:"query_mode,omitempty"`
	Headers        map[string][]string `json:"headers,omitempty"`
	HeadersMode    HeadersMode         `json:"headers_mode,omitempty"`
	Body           json.RawMessage     `json:"body,omitempty"`
	BodySubset     json.RawMessage     `json:"body_subset,omitempty"`
	RawBody        *string             `json:"raw_body,omitempty"`
	FormBody       map[string][]string `json:"form_body,omitempty"`
	FormBodySubset map[string][]string `json:"form_body_subset,omitempty"`
	Response       Response            `json:"response"`
}

// Response describes the HTTP response returned for a planned request.
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    json.RawMessage     `json:"body,omitempty"`
	RawBody *string             `json:"raw_body,omitempty"`
}

// ObservedRequest describes a request recorded by the mock server.
type ObservedRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Query   map[string][]string `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

// CheckResult describes observed request matching state.
type CheckResult struct {
	MatchedCount       int               `json:"matched_count"`
	ObservedRequests   []ObservedRequest `json:"observed_requests"`
	LastMismatchReason string            `json:"last_mismatch_reason,omitempty"`
}

// PlanHTTPCallsResponse describes a successful plan step result.
type PlanHTTPCallsResponse struct {
	Planned       bool `json:"planned"`
	ExpectedCalls int  `json:"expected_calls"`
}
