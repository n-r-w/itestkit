# itestkit

`itestkit` runs integration tests from JSONC files.

It is useful when many integration scenarios have the same execution shape but different input and expected output. Test code wires your service once. JSONC cases describe the scenarios.

The core runner does not know anything about your service, transport, database, or message broker. You connect it through small interfaces: handlers execute steps, a lifecycle prepares shared resources, and a status codec maps fixture status strings to your status type.

## When to use it

Use `itestkit` when you want to:

- keep integration scenarios as readable JSONC fixtures;
- run the same runner against HTTP, gRPC, queue, or custom service calls;
- describe multi-step flows with setup, actions, polling, verification, and cleanup;
- compare normalized responses in `exact` or `partial` mode;
- reuse one suite setup for many cases.

## Install

```bash
go get github.com/n-r-w/itestkit
```

Requires Go 1.26.0 or newer.

## Quick start

1. Put cases into `.jsonc` files.
2. Implement handlers for the operations used by those cases.
3. Register handlers by name.
4. Load cases with `itestkit.LoadCases`.
5. Run them with `itestkit.RunCases`.

```go
func TestIntegration(t *testing.T) {
    t.Parallel()

    codec := statusCodec{}
    registry := itestkit.NewMapRegistry(map[string]itestkit.Handler[*Client]{
        "CreateOrder": createOrderHandler{},
        "GetOrder":    getOrderHandler{},
    })

    cases, err := itestkit.LoadCases(casesFS, "cases", registry, codec)
    require.NoError(t, err)

    itestkit.RunCases(
        t,
        cases,
        suiteLifecycle{},
        caseHarnessFactory{},
        errorInspector{},
        codec,
    )
}
```

`casesFS` can be `embed.FS` or another source that implements `ReadDir` and `ReadFile`.

## Case format

A case contains a name, ordered steps, and an expected result.

```jsonc
{
  "name": "create and read order",
  "steps": [
    {
      "id": "create",
      "kind": "action",
      "handler": "CreateOrder",
      "request": {
        "order_id": "order-1",
        "amount": 1000
      }
    },
    {
      "id": "read",
      "kind": "verify",
      "handler": "GetOrder",
      "request": {
        "order_id": "order-1"
      }
    }
  ],
  "assert": {
    "code": "OK",
    "response_from_step": "read",
    "response_mode": "partial",
    "response": {
      "order_id": "order-1",
      "amount": 1000
    }
  }
}
```

Supported step kinds:

- `prepare` — prepare data or environment before the main action;
- `action` — call the operation under test;
- `publish` — send an event or message;
- `await` — retry a check until it succeeds or the retry policy is exhausted;
- `verify` — check side effects or final state;
- `cleanup` — clean resources after the case; cleanup steps still run after earlier step errors.

For `await`, add a retry policy:

```jsonc
{
  "id": "wait-for-processing",
  "kind": "await",
  "handler": "AwaitOrderProcessed",
  "request": { "order_id": "order-1" },
  "retry": {
    "timeout_ms": 5000,
    "interval_ms": 100,
    "max_attempts": 50
  }
}
```

A step request can use a response from an earlier step:

```jsonc
{
  "id": "read-created-order",
  "kind": "verify",
  "handler": "GetOrder",
  "request": {
    "order_id": "{{steps.create.response.order_id}}"
  }
}
```

## What you implement

### `Handler`

A handler binds one fixture operation to your code.

```go
type createOrderHandler struct{}

func (createOrderHandler) DecodeRequest(raw json.RawMessage) (any, error) {
    var request CreateOrderRequest
    if err := itestkit.DecodeStrictJSON(raw, &request); err != nil {
        return nil, err
    }
    return &request, nil
}

func (createOrderHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
    var response OrderResponse
    if err := itestkit.DecodeStrictJSON(raw, &response); err != nil {
        return nil, err
    }
    return &response, nil
}

func (createOrderHandler) Invoke(ctx context.Context, client *Client, request any) (any, error) {
    typedRequest, ok := request.(*CreateOrderRequest)
    if !ok {
        return nil, fmt.Errorf("CreateOrder received invalid request type: %T", request)
    }
    return client.CreateOrder(ctx, typedRequest)
}

func (createOrderHandler) NormalizeResponse(response any) (any, error) {
    return response, nil
}
```

`NormalizeResponse` should return a stable value for comparison. Use it to remove volatile fields or convert transport-specific responses to simple Go values.

### Suite and case lifecycle

`RunCases` separates suite setup from per-case setup.

- `SuiteLifecycle` starts and stops shared resources once for the whole case set.
- `SuiteCaseHarnessFactory` creates the client or harness for one case.
- `ErrorInspector` converts runtime errors to the same status type that fixtures use.
- `StatusCodec` parses `assert.code` and returns the success status.

## Assertions

`assert.code` is always checked.

For successful cases, `assert.response` is checked against a normalized step response. If `response_from_step` is empty, `itestkit` uses the last `action` step.

`response_mode` values:

- `exact` — the full normalized response must match. Use `{ "$present": true }` as an object field value to assert that the field exists with any value. The marker is not valid at the root or as an array element. A present field with `null` passes.
- `partial` — object fields from `assert.response` must match, and extra object fields in the actual response are allowed. Use `{ "$present": true }` with the same rules as in `exact`. Use `{ "$absent": true }` as an object field value to assert that the field is absent from the normalized actual response. A present field, including `null`, fails.

Semantic matcher objects are opt-in expected values and work in both modes:

- `{ "$same_instant": "2026-05-30T10:00:00Z" }` — actual and expected values must be RFC3339 strings that represent the same instant.
- `{ "$matches": "^trace-\\d+$" }` — actual value must be a string that matches the regular expression.

For expected errors, use `message_contains` when the error message is part of the contract.

## Run options

```go
itestkit.RunCases(
    t,
    cases,
    lifecycle,
    caseFactory,
    inspector,
    codec,
    itestkit.WithContinueOnFailure(),
    itestkit.WithParallelCases(),
    itestkit.WithParallelismLimit(20),
)
```

Options:

- `WithContinueOnFailure()` — run remaining cases after a failure;
- `WithParallelCases()` — run cases in parallel;
- `WithParallelismLimit(n)` — run cases in parallel with an explicit limit.

## Included helpers

Core helpers:

- `itestkit.NewMapRegistry` — registry backed by a map from handler name to handler;
- `itestkit.DecodeStrictJSON` — JSON decoder that rejects unknown fields and trailing data.

Additional packages:

- `github.com/n-r-w/itestkit/grpc` — gRPC handler helpers and protobuf JSON normalization;
- `github.com/n-r-w/itestkit/grpc/bufconn` — in-memory gRPC server/client helpers;
- `github.com/n-r-w/itestkit/httpmock` — JSONC-driven outbound HTTP mock server;
- `github.com/n-r-w/itestkit/httpserver` — JSONC-driven calls to an in-process `net/http.Handler`;
- `github.com/n-r-w/itestkit/testcalendar` — deterministic date and timestamp macros for JSONC cases;
- `github.com/n-r-w/itestkit/queue/kafkaproducer` — Kafka producer test harness;
- `github.com/n-r-w/itestkit/queue/itest` — ready-made queue step registry.

## HTTP server helper

Use `httpserver.NewRegistry` or `httpserver.NewCallHandler` when JSONC cases need to call an in-process `net/http.Handler`. The case harness must expose `HTTPHandler() http.Handler`.

Preset handler:

- `CallHTTP` — builds an HTTP request from JSONC, calls the handler, and normalizes the response.

Request fields:

- `method`, `path`, `query`, `headers`, `body`, `raw_body`;
- `use_cookies` — attaches cookies stored from earlier responses in the same case;
- `csrf.cookie`, `csrf.header` — copies one stored cookie value into one request header and fails if the header is already set manually;
- `capture_headers`, `capture_cookies` — selects response headers and cookies for assertion. Requested absent headers are returned as empty arrays.

Cookie reuse requires per-case state:

```go
type harness struct {
    handler http.Handler
    cookies *httpserver.CookieJar
}

func (h *harness) HTTPHandler() http.Handler { return h.handler }
func (h *harness) HTTPCookieJar() *httpserver.CookieJar { return h.cookies }
```

See `docs/itestkit/examples/httpserver` for a complete JSONC example.

## HTTP mock helper

Use `httpmock.NewServer(t)` in a per-case harness when the system under test needs a base URL for outbound HTTP calls. The harness must expose `HTTPMock() *httpmock.Server` for preset handlers.

Preset handlers:

- `PlanHTTPCalls` — stores expected requests and stub responses;
- `AwaitHTTPCalls` — checks whether planned requests have been observed;
- `VerifyHTTPCalls` — performs the final request verification.

Plan fields:

- request match: `method`, `path`, `query`, `query_mode`, `headers`, `headers_mode`, `headers_present`, `body`, `body_subset`, `raw_body`, `expected_count`;
- response stub: `response.status`, `response.headers`, `response.body`, `response.raw_body`;
- ordering: `ordering` with values `strict` and `any`.

Mode values:

- `query_mode`: `exact`, `subset`; empty means `exact`;
- `headers_mode`: `exact`, `subset`; empty means `exact`;
- `headers_present` checks that headers exist without comparing their values;
- `ordering`: `strict`, `any`; empty means `any`.

Body rules:

- `body` is exact JSON matching;
- `body_subset` is JSON subset matching;
- `raw_body` is exact string matching;
- set only one of `body`, `body_subset`, and `raw_body` per request expectation.

See `docs/itestkit/examples/httpmock` for a complete JSONC example.

## LLM skill

- `docs/itestkit/SKILL.md`
- `docs/itestkit/examples/custom` — minimal custom integration;
- `docs/itestkit/examples/grpc` — gRPC client integration;
- `docs/itestkit/examples/extapigrpcadapter` — adapter-style gRPC target and dial options;
- `docs/itestkit/examples/httpserver` — inbound HTTP handler flow;
- `docs/itestkit/examples/httpmock` — outbound HTTP mock flow;
- `docs/itestkit/examples/queue/inbound` — inbound queue flow;
- `docs/itestkit/examples/queue/outbound` — outbound Kafka flow;
- `docs/itestkit/examples/bookingcalendar` — calendar macros and fixed test time;
- `docs/itestkit/examples/real` — project-like layout with service and integration tests.
