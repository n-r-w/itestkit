# Project Rules

## Description

`itestkit` - a portable core for running integration tests on JSONC cases. The package is independent of any specific service, transport, or internal packages and operates through a set of interfaces.

See `README.md` for more details.

## Rules

1. Run `task test`, `task lint` before finalizing.
2. Tests in `docs/itestkit/examples/` will fail. It OK due to special negative tests-cases in `docs/itestkit/examples/grpc`.
3. Run `task fmt` to fix linter formatting issues.