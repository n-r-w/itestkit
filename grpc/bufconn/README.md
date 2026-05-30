# gRPC bufconn client for `bufconn` integration tests

`bufconn` is a compact helper for gRPC service tests over an in-memory transport. It starts a `grpc.Server`, connects to it through `bufconn`, returns a typed client, and releases resources through `t.Cleanup`.

## Scope

- Creates a `bufconn.Listener` with a configurable buffer size.
- Initializes `grpc.Server` with custom options and registers services.
- Starts the server in a separate goroutine.
- Creates `grpc.ClientConnInterface` through `grpc.NewClient` with a `bufconn` dialer.
- Shuts down cleanly: `grpcServer.Stop()`, connection close, and listener close.

## What the service must provide

Usage requires a minimal set of functions:

- `register func(*grpc.Server)` — registers gRPC services on the server.
- `newClient func(grpc.ClientConnInterface) Client` — creates a typed client.
- Optional settings through `WithBufSize`, `WithServerOptions`, and `WithDialOptions`.

## Defaults

- Buffer size: 1 MB (`1024 * 1024`).
- Transport credentials: `insecure.NewCredentials()`.

## Minimal usage

```go
client := bufconn.NewClient(
    t,
    func(s *grpc.Server) { pb.RegisterSearchServer(s, handler) },
    pb.NewSearchClient,
    bufconn.WithServerOptions(grpc.UnaryInterceptor(myInterceptor)),
    bufconn.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
)
```

`client` is ready to use in tests, and all resources are released automatically in `t.Cleanup`.

## How to choose between `NewClient` and `NewServer`

- Use `NewClient` when your test needs a typed gRPC client right away.
    - Best fit for direct gRPC integrations where your harness depends on `pb.ServiceClient`.
- Use `NewServer` when your SUT uses adapter-style constructors with transport inputs.
    - Best fit for constructors that accept `target string` and `[]grpc.DialOption`.
    - Typical gateway-style setup: start in-memory server once, then pass `Target()` + `DialOptions()` into adapter constructors.

Quick decision:
- `typed client needed` -> `NewClient`
- `target + dial options needed` -> `NewServer`
