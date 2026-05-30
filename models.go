package itestkit

// Case describes a loaded integration case.
type Case[C any, S comparable] struct {
	Name       string
	SourcePath string
	Steps      []Step[C]
	Assert     Assert[S]
}

// StepKind describes the type of step in the unified step-based model.
type StepKind string

// StepKind* constants define all supported step kinds in unified step-based execution.
const (
	StepKindPrepare StepKind = "prepare"
	StepKindAction  StepKind = "action"
	StepKindPublish StepKind = "publish"
	StepKindAwait   StepKind = "await"
	StepKindVerify  StepKind = "verify"
	StepKindCleanup StepKind = "cleanup"
)

// AwaitRetry describes the retry policy for the kind=await step.
type AwaitRetry struct {
	TimeoutMS   int
	IntervalMS  int
	MaxAttempts int
}

// Step stores the handler and the prepared request.
type Step[C any] struct {
	ID          string
	Kind        StepKind
	HandlerName string
	Handler     Handler[C]
	Request     any
	Retry       *AwaitRetry
}

// ResponseMode specifies how a successful response is compared.
type ResponseMode string

const (
	// ResponseModeExact requires an exact match of the normalized response.
	ResponseModeExact ResponseMode = "exact"
	// ResponseModePartial only checks the fields listed in assert.response.
	ResponseModePartial ResponseMode = "partial"
)

// Assert stores the expected status and expected response for a successful scenario.
type Assert[S comparable] struct {
	Code             S
	MessageContains  string
	Response         any
	ResponseFromStep string
	ResponseMode     ResponseMode
}
