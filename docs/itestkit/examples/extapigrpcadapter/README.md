# Adapter-style external gRPC call example

This example shows when to use `itestkit/grpc/bufconn.NewServer` instead of `NewClient`.

Use this pattern when your service dependencies are constructed in adapter style:
- constructor input is `target + []grpc.DialOption`
- the adapter itself manages `grpc.NewClient(...)`

The harness starts an in-memory external gRPC server via `NewServer`, then creates an adapter from `Target()` and `DialOptions()`.

This mirrors integration setups like gateway-style services where adapters own transport clients.
