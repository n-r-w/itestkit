---
name: itestkit
description: How to use `github.com/n-r-w/itestkit` to run integration tests from JSONC cases.
metadata:
  version: "1.3"
---

<itestkit_usage>
  <when_to_use>
    Implementing data-driven integration tests at the API level.
  </when_to_use>

  <guidelines>
    1. Unified case shape:
      1) top-level `steps[]`: full execution pipeline (ordered)
      2) top-level `assert`: expected outcome
    2. JSONC + strict JSON:
      1) comments are stripped
      2) unknown fields are rejected
      3) trailing JSON is rejected
    3. Required fields:
      1) case `name`: TrimSpace()'d, unique
      2) `steps`: non-empty
      3) each step: `id`, `kind`, `handler`, `request`
      4) `assert.code`: parsed by `StatusCodec.Parse`
    4. Allowed step kinds (full list):
      1) `prepare`
      2) `action`
      3) `publish`
      4) `await`
      5) `verify`
      6) `cleanup`
    5. Await retry semantics:
      1) `retry` is allowed only for `kind=await`
      2) `retry` is required for `kind=await`
      3) all retry fields must be positive: `timeout_ms`, `interval_ms`, `max_attempts`
    6. Assert semantics:
      1) `code == Success()`:
          - case execution must finish without unexpected error
          - `assert.response` is required
          - response comparison target:
            - `assert.response_from_step` when set
            - otherwise output of the last `action` step
          - `assert.response_mode` values:
            - absent or `exact`: compare full normalized expected and actual responses. Use `{ "$present": true }` as an object field value to assert that the field exists with any value. The marker is not valid at the root or as an array element. A present field with `null` passes.
            - `partial`: compare only fields present in `assert.response`
          - Semantic matcher objects are opt-in expected values in both modes:
            * `{ "$same_instant": "2026-05-30T10:00:00Z" }`: actual and expected values must be RFC3339 strings that represent the same instant.
            * `{ "$rfc3339": true }`: actual value must be an RFC3339 date-time string. Use it when only the format matters and the instant is dynamic.
            * `{ "$matches": "^trace-\\\\d+$" }`: actual value must be a string that matches the regular expression.
            * `{ "$not_empty": true }`: actual value must be a non-empty string, array, or object. `"   "` passes; `""`, `[]`, `{}`, `null`, numbers, and booleans fail.
          - In `partial` mode:
            * object fields are recursive subsets, arrays require exact length and order, and scalar values use strict equality unless a semantic matcher object is used.
            * use `{ "$present": true }` with the same placement rules as in `exact`.
            * use `{ "$absent": true }` as an object field value to assert that the field is absent from the normalized actual response. A present field, including `null`, fails.
            * expected response stays as raw JSON after source preprocessing and is not decoded through `DecodeExpectedResponse`.
          - Actual response is normalized through `NormalizeResponse`.
      2) `code != Success()`:
          - case must return an execution error
          - `ErrorInspector.FromError` must extract `(code, message)`
          - `assert.response` is ignored
          - `message_contains` is optional substring check for error message
    7. Runner behavior details:
      1) non-cleanup steps stop after first unrecoverable failure
      2) cleanup steps still run in declared order after failure
      3) step outputs are stored internally by `step.id`
    8. Discovery: `*.jsonc` files are found recursively under rootDir and sorted; `LoadCases` fails if none found.
    9. Runtime request templates:
      1) Supported template forms in `steps[].request`:
          - `{{steps.<step-id>.response}}`
          - `{{steps.<step-id>.response.<path>}}`
      2) Template resolution happens during case execution, using outputs of already completed steps.
      3) If template references an unavailable step output or invalid path, case execution fails on that step.
    10. Runtime calendar macros in JSONC:
      1) If cases use `<test_date...>`, `<test_timestamp...>`, or `<test_rfc3339_timestamp...>`, wrap `CaseSource` with `testcalendar.New().WrapSource(...)` before `itestkit.LoadCases(...)`.
      2) `test_date` renders `YYYY-MM-DD`, supports day offsets only, and may use explicit fixed-offset timezone suffix `@±HH:MM`.
      3) `test_timestamp` renders `YYYY-MM-DD HH:MM:SS+TZ`, supports day/hour/minute offsets in `d -> h -> m` order, and may use explicit fixed-offset timezone suffix `@±HH:MM`.
      4) `test_rfc3339_timestamp` renders RFC3339, supports day/hour/minute offsets in `d -> h -> m` order, and may use explicit fixed-offset timezone suffix `@±HH:MM`.
      5) The macro must occupy the whole string value; partial interpolation inside a larger string is not supported.
      6) Inside `{"$date": ...}` only `test_date` is allowed; the selected calendar day may use explicit timezone, but the rendered value is RFC3339 midnight UTC.
      7) Fixed anchor: `2026-03-01 03:00:00 +06:00`.
  </guidelines>

  <critical_requirements>
    1. MUST use `embed` and `itestkit.LoadCases` to include case files in the binary for reliable access in all environments.
    2. By default, JSONC fixtures MUST assert full API response bodies in `assert.response`, not only status codes or summaries. Use `assert.response_mode: "partial"` only when the project explicitly allows partial assertions and the selected fields prove the scenario behavior. Do not hardcode expected responses in test code.
    3. MUST NOT create:
      1) Custom data seeding in Go code instead of `prepare` step with jsonc request.
      2) Custom API calls in Go code instead of `action` step with jsonc request.
      3) Custom response checks in Go code instead of `assert.response` in jsonc.
      Otherwise, you CAN'T expand test coverage without code changes.
    5. MUST make code maintainable and readable:
      1) Split jsonc cases by folders according to api handlers, queues, or features under test. Don't put all cases in one folder!
      2) Split go code by files according to their role in the test suite: suite lifecycle, case harness factory, handlers, custom assertions, etc. Don't put all code in one file!
      3) Make separate folders for different test suites if you have multiple: one suite per API, queue, etc.
      4) Use helper functions and types to reduce boilerplate and improve readability. Don't repeat yourself in test code!
      5) Add comments not only to code, but also to jsonc cases to explain each step's purpose and the overall test scenario. Don't make others guess your intentions!
    6. If cases use calendar macros, MUST preprocess the source with `testcalendar.New().WrapSource(...)` before `itestkit.LoadCases(...)`.
    7. If your SUT computes time-dependent fields using its own clock, MUST pass `testcalendar.FixedNow()` into that clock too. `WrapSource(...)` changes fixtures only; it does NOT freeze the application's current time.
    8. If integration tests require docker for their operations, all go files in integration test folders MUST have the build tag `//go:build integration` or similar.
  </critical_requirements>

  <fixtures>
    Minimal JSONC case:
    ```jsonc
    {
      "name": "health check ok",
      "steps": [
        { "id": "prepare-1", "kind": "prepare", "handler": "Noop", "request": {} },
        { "id": "action-1", "kind": "action", "handler": "Check", "request": { "service": "" } }
      ],
      "assert": {
        "code": "OK",
        "response_from_step": "action-1",
        "response": { "status": "SERVING" }
      }
    }
    ```
  </fixtures>

<dump_usage>
1. ITESTKIT_RESPONSE_DUMP only produces a STRUCTURE, but is NEVER TREATED AS AN EXACT RESULT.
2. Expected values ​​MUST be calculated independently according to requirements, formulas and invariants
</dump_usage>

  <minimal_example>
    <guidelines>
      1. If your suite does not need shared heavy setup, use a no-op suite context (for example `struct{}`) and create per-case harnesses in `NewCaseHarness`.
      2. For SQL DBs or MongoDB in tests, prefer `github.com/n-r-w/testdock/v2`.
      3. For other external integrations (brokers, queues, caches, third-party services), prefer `https://github.com/testcontainers/testcontainers-go` and its modules.
      4. If your handler registry is a simple static map without custom resolve logic, prefer `itestkit.NewMapRegistry(...)` to avoid boilerplate.
      5. For strict decode of non-proto `request` and `assert.response`, prefer `itestkit.DecodeStrictJSON(...)` instead of duplicating local helpers.
      6. For marker-aware JSON expectations inside custom handlers, decode raw expectation JSON with `itestkit.DecodeExpectedJSON(...)` and compare it with JSON-safe actual data through `itestkit.MatchExpectedJSON(expected, actual, itestkit.MatchModeExact)` or `itestkit.MatchModePartial`.
      7. If your JSONC fixtures contain moving dates, prefer `testcalendar.New().WrapSource(source)` before `LoadCases(...)`. If the SUT itself depends on current time, also inject `testcalendar.FixedNow()` into its clock. If fixtures use explicit timezone different from `+06:00`, normalize runtime timestamps to the same timezone before comparing them.
      8. Optional execution options:
        1) `itestkit.WithContinueOnFailure()`
        2) `itestkit.WithParallelCases()`
        3) `itestkit.WithParallelismLimit(n)`
        4) env `ITESTKIT_RESPONSE_DUMP="case name"` runs one case by `Case.Name`, disables parallel execution, and always prints the normalized actual response JSON between dump markers without comparing it with `assert.response`. Command to extract dumped response:
          ```bash
          (ITESTKIT_RESPONSE_DUMP="case name from jsonc" go test ... || true) 2>&1 \
            | awk '/^[[:space:]]*ITESTKIT_RESPONSE_DUMP_BEGIN[[:space:]]*$/{dump=1; found=1; next} /^[[:space:]]*ITESTKIT_RESPONSE_DUMP_END[[:space:]]*$/{dump=0; next} dump {sub(/^        /, ""); print} END {if (!found) exit 2}' \
            | jq . \
            > /tmp/actual-response.json
          ```
    </guidelines>
    <example>
      Minimal (load + run with suite lifecycle):
      ```go
      // 1) Provide integrations:
      //   - itestkit.CaseSource (fs.ReadDirFS + fs.ReadFileFS)
      //   - itestkit.HandlerRegistry[C]
      //   - itestkit.SuiteLifecycle[SC]
      //   - itestkit.SuiteCaseHarnessFactory[SC, C]
      //   - itestkit.StatusCodec[S] + itestkit.ErrorInspector[S]
      // 2) Optional if fixtures use <test_date...>/<test_timestamp...>/<test_rfc3339_timestamp...>:
      //   source = testcalendar.New().WrapSource(source)
      // 3) Load + run cases:
      cases, err := itestkit.LoadCases(source, "cases", registry, statusCodec)
      require.NoError(t, err)
      itestkit.RunCases(t, cases, lifecycle, caseFactory, errorInspector, statusCodec)
      ```
    </example>
  </minimal_example>

  <ext_api>
    How to test external API calls:
      1. Keep external API behavior in service-specific handlers and case harness state; do not add transport-specific logic to itestkit core.
      2. Use `kind=prepare` to:
        1) seed DB/state
        2) configure external request expectation
        3) configure external stub response
      3. Use `kind=action` to call your API/use-case that triggers outbound call.
      4. Use `kind=verify` to check external side effects:
        1) call happened
        2) request payload matched
        3) call count is expected
      5. For eventual consistency use `kind=await` (with retry) before final verify/read step.
      6. Set explicit `assert.response_from_step` for deterministic assert target.
  </ext_api>

  <httpserver>
    How to test inbound HTTP API handlers:
      1. Use `github.com/n-r-w/itestkit/httpserver` when JSONC cases must call an in-process `net/http.Handler`.
      2. Create a per-case harness that exposes `HTTPHandler() http.Handler`.
      3. Use `httpserver.NewRegistry(...)` for the preset `CallHTTP` handler, or `httpserver.NewCallHandler(...)` for manual registration.
      4. Fixture request fields:
        1) `method`, `path`, `query`, `headers`, `body`, `raw_body`
        2) `response_body`
        3) `use_cookies`
        4) `csrf.cookie`, `csrf.header`
        5) `capture_headers`, `capture_cookies`
      5. Set only one of `body` and `raw_body`.
      6. If cases need cookie reuse across steps, create `httpserver.NewCookieJar()` per case and expose `HTTPCookieJar() *httpserver.CookieJar` from the harness. Do not share one jar between cases.
      7. Use `csrf` only for valid CSRF flows where a stored cookie must become a request header. `CallHTTP` fails if the cookie is missing or the same header is already set manually.
      8. Keep missing and mismatch CSRF cases explicit with `headers`, `use_cookies`, or neither. Do not use `csrf` for those negative cases.
      9. JSON response bodies are decoded to JSON-safe values. Non-JSON response bodies are normalized as trimmed strings.
      10. Use `httpserver.WithBodyNormalizer(...)` when binary or project-specific bodies must become JSON-safe summaries. Put normalizer settings in `response_body`; return `handled=false` to use the default JSON/string normalization.
      11. Requested absent headers in `capture_headers` are normalized as empty arrays, not `null`.
      12. Use `httpserver.WithBaseURL(...)` when the handler uses request host.
      13. For success checks, set `assert.response_from_step` to the `CallHTTP` step when it is not the last action step.
  </httpserver>

  <httpmock>
    How to test outbound HTTP calls:
      1. Use `github.com/n-r-w/itestkit/httpmock` when the SUT accepts an outbound HTTP base URL.
      2. Create `httpmock.NewServer(t)` in the per-case harness and expose `HTTPMock() *httpmock.Server`.
      3. Use preset handlers: `PlanHTTPCalls`, `AwaitHTTPCalls`, `VerifyHTTPCalls`.
      4. Fixture fields:
        1) request: `method`, `path`, `query`, `query_mode`, `headers`, `headers_mode`, `headers_present`, `body`, `body_subset`, `raw_body`, `expected_count`
        2) response: `response.status`, `response.headers`, `response.body`, `response.raw_body`
        3) order: `ordering`
      5. Allowed modes:
        1) `query_mode`: `exact`, `subset`; empty means `exact`
        2) `headers_mode`: `exact`, `subset`; empty means `exact`
        3) `ordering`: `strict`, `any`; empty means `any`
      6. Set only one of `body`, `body_subset`, and `raw_body` per expected request.
      7. Use `body_subset` for JSON subset matching and `raw_body` for exact string matching.
      8. Use `headers_present` for dynamic headers whose values must not be compared but whose absence must fail matching.
      9. For success checks, set `assert.response_from_step` to the action or verify step that owns the expected response.
      10. Use `calls: []` to assert that the SUT makes no outbound HTTP requests. Any observed request receives HTTP 500 and makes `VerifyHTTPCalls` fail with an unexpected-request error in both `strict` and `any` ordering modes.
  </httpmock>

  <kafka>
    How to test Kafka interactions:
      1. For producer/outbound, use `queue/kafkaproducer.StartSuite(...)` for suite-level broker setup and `suite.NewHarness(...)` for case-level isolation.
      2. For minimal producer glue-code, use `queue/itest.NewRegistry(...)` with preset handlers: `PlanOutbound`, `AwaitOutbound`, `VerifyOutbound`, `CleanupBroker`.
      3. Recommended outbound flow: `prepare(PlanOutbound)` -> `action(SUT publish via PublishJSON/PublishRaw)` -> `await` -> `verify` -> `cleanup`.
      4. For consumer/inbound, follow `examples/queue/inbound` flow: `prepare` -> `publish` -> `await` -> `verify`.
      5. For success checks, set `assert.response_from_step` to a deterministic step (usually `verify-*`, sometimes `await-*`).
  </kafka>

  <grpc>
    How to test gRPC interactions:
      1. Import aliases used in examples:
        1) itestkitgrpc = "github.com/n-r-w/itestkit/grpc" (avoid name clash with "google.golang.org/grpc")
        2) itestkitbufconn = "github.com/n-r-w/itestkit/grpc/bufconn"
      2. assert.code must match gRPC `codes.Code.String()` exactly (case-sensitive), e.g. "Canceled" (NOT "CANCELLED").
      3. For success assertions, explicitly set `assert.response_from_step` when the target is not the default last `action` step.
      4. Use `itestkitbufconn.NewClient(...)` when your harness needs a typed gRPC client directly.
      5. Use `itestkitbufconn.NewServer(...)` when your SUT/adapter constructor accepts `target string` and `[]grpc.DialOption`.
      6. For adapter-style transport setup, use the dedicated example `examples/extapigrpcadapter`.
        7. For standard proto JSON fixtures, rely on the default decode path in `itestkitgrpc.NewHandlerSpec(...)`.
        8. If proto fixtures need preprocessing before `protojson.Unmarshal`, set `HandlerSpec.DecodeRequestJSON` and/or `HandlerSpec.DecodeExpectedResponseJSON`.
        9. If fixture JSON encodes `google.type.Date` as `YYYY-MM-DD`, use `itestkitgrpc.DecodeProtoJSONWithGoogleDateStrings(...)` as the decode hook.
        10. If you need full stable protobuf response assertions, use `itestkitgrpc.NormalizeProtoMessage(...)` or keep `normalize == nil` in `NewHandlerSpec(...)`.
  </grpc>

  <testcalendar>
    How to use `testcalendar`:
      1. Use it when JSONC fixtures must stay relative to a shared fixed "today" instead of hardcoded historical dates.
      2. Wrap the case source before `LoadCases(...)`:
      ```go
      calendar := testcalendar.New()
      cases, err := itestkit.LoadCases(calendar.WrapSource(source), "cases", registry, statusCodec)
      ```
      3. Supported macros:
        1) `<test_date>`
        2) `<test_date+9d>`
        3) `<test_date@+03:00>`
        4) `<test_date+9d@-05:00>`
        5) `<test_timestamp>`
        6) `<test_timestamp+7d20h30m>`
        7) `<test_timestamp@+03:00>`
        8) `<test_timestamp+7d20h30m@-05:00>`
        9) `<test_rfc3339_timestamp>`
        10) `<test_rfc3339_timestamp+7d20h30m>`
        11) `<test_rfc3339_timestamp@+03:00>`
        12) `<test_rfc3339_timestamp+7d20h30m@-05:00>`
      4. Macro rules:
        1) the macro must be the whole string value
        2) `test_date` supports only `d`
        3) `test_timestamp` and `test_rfc3339_timestamp` support `d`, `h`, `m` in this order, without duplicate units
        4) explicit timezone uses suffix `@±HH:MM` after the relative offset, if any
        5) if timezone is omitted, the default fixed anchor timezone `+06:00` is used
        6) inside Mongo-style `{"$date": ...}` only `test_date` is allowed; it becomes RFC3339 midnight UTC for the selected day
      5. Fixed anchor: `2026-03-01 03:00:00 +06:00`.
      6. `WrapSource(...)` preprocesses fixture JSONC only. If your SUT calculates time-dependent fields using its own clock, pass `testcalendar.FixedNow()` to that clock too. If explicit timezone is different from `+06:00`, convert runtime values to the same timezone before comparing them.
      7. For an end-to-end example that combines calendar macros with a fixed application clock, see `examples/bookingcalendar`.
  </testcalendar>

  <examples>
    Before planning to use itestkit, you MUST read the most relevant examples from the list below. Otherwise, you RISK making CRITICAL MISTAKES.
    The example directories are ACCESSIBLE and located in subdirectories RELATIVE to this file (no need to go fetch them from the web!):
      1. [Simple GRPC client](examples/grpc)
      2. [Custom itestkit implementation](examples/custom)
      3. [In-memory queue consumer and DB simulation](examples/queue/inbound)
      4. [Kafka outbound producer preset](examples/queue/outbound)
      5. [Outbound HTTP mock](examples/httpmock)
      6. [Synchronous external API call](examples/extapisync)
      7. [Calendar macros + now-provider in SUT](examples/bookingcalendar)
      8. [Asynchronous external API call](examples/extapiasync)
      9. [Synchronous external gRPC API call](examples/extapigrpcsync)
      10. [Complex project with grpc api, external api call, database and itests](examples/real)
      11. [Adapter-style external gRPC call (target + dial options)](examples/extapigrpcadapter)
      12. [Inbound HTTP handler](examples/httpserver)
  </examples>

</itestkit_usage>
