# extapigrpcsync

This example shows how to test the following scenario:

1. Call the service **internal API** (inbound gRPC).
2. During processing, the service performs an **external outbound gRPC** call.
3. The external call is mocked and validated using JSONC data.

## Why this example exists

It demonstrates a working pattern for a real project:

1. `prepare` configures external API expectations as a `calls[]` list (`expected request` + `stub response` for each call).
2. `prepare` returns a normalized plan that can be reused in later steps.
3. `action` calls your API using `service_chain` from `prepare` data via `{{steps.<id>.response...}}`.
4. `verify` checks external call side effects (whether the call happened, payload content, call count).
5. `assert` validates the `action` step response.

## Why there are three cases

All three cases validate **our service API** (target = `call-api`), but with different external call chain length and content:

1. `external_grpc_sync_ok.jsonc`: 1 external call, final API status = `SERVING`.
2. `external_grpc_sync_plan_response_ok.jsonc`: 2 sequential external calls, final API status = `NOT_SERVING`.
3. `external_grpc_sync_verify_response_ok.jsonc`: 3 sequential external calls, final API status = `SERVICE_UNKNOWN`.

In these cases, `verify-external` does not replace API verification — it only additionally confirms outbound side effects.

## How it works

The `harness` starts two `bufconn` servers:

1. `apiHealthServer` — internal API of the service under test.
2. `externalHealthServer` — external API mock.

`apiHealthServer` receives an injected `externalClient` and performs outbound calls exactly like production code.

`externalHealthServer`:

1. Reads the call plan from harness state (filled by the `prepare` step).
2. Stores the actual inbound payload in the call journal.
3. Returns a predefined stub response.
4. Fails if payload does not match the plan.

## Minimal JSONC pattern

```jsonc
{
  "name": "grpc api handler performs sync outbound grpc call",
  "steps": [
    {
      "id": "plan-external",
      "kind": "prepare",
      "handler": "PlanExternalCheck",
      "request": {
        "calls": [
          {
            "expected_service": "order-100",
            "stub_response": { "status": "SERVING" }
          }
        ]
      }
    },
    {
      "id": "call-api",
      "kind": "action",
      "handler": "CallAPI",
      "request": {
        "service_chain": [
          "{{steps.plan-external.response.calls.0.expected_service}}"
        ]
      }
    },
    {
      "id": "verify-external",
      "kind": "verify",
      "handler": "VerifyExternalCheck",
      "request": {
        "expected_services": [
          "{{steps.plan-external.response.calls.0.expected_service}}"
        ],
        "expected_calls": 1
      }
    }
  ],
  "assert": {
    "code": "OK",
    "response_from_step": "call-api",
    "response": { "status": "SERVING" }
  }
}
```

`PlanExternalCheck` returns the following in this example:

```jsonc
{
  "planned": true,
  "calls": [
    {
      "expected_service": "...",
      "stub_response": { "status": "SERVING" }
    }
  ]
}
```

Therefore, subsequent steps can reference `calls.N.expected_service` without duplicating constants in JSONC.

## How to adapt this to a real service

1. In `NewCaseHarness`, create your API server with real dependencies.
2. Replace the external gRPC dependency with a mock server (`bufconn` or testcontainer).
3. Keep mock state in the harness:
  - expectation plan (array of calls: expected request + stub response),
  - journal of actual calls.
4. Implement handlers:
  - `PlanExternal...` (`prepare`) — writes the plan,
  - `Call...` (`action`) — calls your API,
  - `VerifyExternal...` (`verify`) — validates call journal.
5. In `assert.response_from_step`, explicitly specify which step response should be compared.

## Run

```bash
go test ./examples/extapigrpcsync
```
