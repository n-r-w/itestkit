# examples/real

This example demonstrates a structure closely resembling a real-world project:

1. `docs/itestkit/examples/real/service` - service code.
2. `docs/itestkit/examples/real/itests` - integration tests using `itestkit`.

## What's in `service`

1. gRPC API (`APIServer`):
   - accepts incoming request (`order_id`),
   - reads order from in-memory database,
   - builds chain of external calls,
   - makes outbound gRPC calls to external API,
   - saves processing result to database,
   - returns final status.
2. In-memory order repository.
3. Simple business logic:
   - standard order -> 2 external calls (`risk`, `reserve`),
   - large order -> 3 external calls (`risk`, `reserve`, `manual`).

## What's in `itests`

1. `harness` starts two gRPC servers via `bufconn`:
   - internal service API,
   - external API mock.
2. Internal API receives `externalClient` directed to the mock.
3. Therefore, outbound gRPC calls from the service are intercepted in the test (without real network).

## How external gRPC call interception works

1. In the `prepare` step `PlanExternalCalls`, a plan of external calls is defined from JSONC:
   - which `expected_service` should arrive,
   - which `stub_response.status` to return.
2. When the service API calls external gRPC during processing:
   - request goes to mock (`externalHealthServer.Check`),
   - mock verifies it against the plan,
   - saves call fact to journal,
   - returns planned response.
3. In the `verify` step `VerifyExternalCalls`, the following are checked:
   - call order,
   - number of calls,
   - specific `service` values.

## What JSONC cases verify

Cases are located in:

- `docs/itestkit/examples/real/itests/cases/api_returns_serving_for_standard_order.jsonc`
- `docs/itestkit/examples/real/itests/cases/api_returns_service_unknown_for_large_order.jsonc`

Each case verifies three levels at once:

1. Service API response (`assert` with `response_from_step: "call-api"`).
2. External API side effects (`verify-external`).
3. Database state after processing (`verify-order-state`).

## What's the "magic" in `{{steps...}}`

The case `docs/itestkit/examples/real/itests/cases/api_returns_service_unknown_for_large_order.jsonc`
uses runtime substitution of values from output of previous steps.

Example:

```jsonc
"{{steps.plan-external.response.calls.2.expected_service}}"
```

Path breakdown:

1. `steps.plan-external` - take output of step with `id = "plan-external"`.
2. `response.calls` - take `calls` array from this step's response.
3. `.2` - take third element of array (zero-indexed).
4. `.expected_service` - take `expected_service` field.

Why this is needed:

1. Avoid manually duplicating the same values in `verify` and `assert` parts of the case.
2. Guarantee that verifications use exactly the external call plan set in `prepare`.
3. Simplify case maintenance when input data changes.

## If external call order is non-deterministic

This happens when:

1. Service iterates over a `map`.
2. Calls are made in parallel (e.g., via `errgroup`).

In this case, fixing the order in the test is not possible if order is not a business requirement.

Recommendations:

1. If order is important for business, make it deterministic in the service and keep strict sequence verification.
2. If order is not important, verify not sequence but composition and quantity of calls:
   - total `expected_calls`,
   - counter for each `service` (`service -> count`).
3. Separate verifications:
   - `assert` — service API response,
   - `verify` — side effects of external calls.
4. Explicitly document chosen verification mode in the case:
   - `strict order` (order is mandatory),
   - `any order` (only composition/quantity matters).

Practical conclusion: if order is random, the test should only fail on incorrect call set, not due to permutation of equally valid elements.

## Step pipeline in case

1. `prepare` `SeedOrders` - database seeding.
2. `prepare` `PlanExternalCalls` - planning mock responses for external API.
3. `action` `CallOrderAPI` - call service API.
4. `verify` `VerifyExternalCalls` - verify outbound calls.
5. `verify` `VerifyOrderState` - verify final database state.
6. `assert` - verify API response.
