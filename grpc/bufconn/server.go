package bufconn

import (
	"sync"
	"testing"

	"google.golang.org/grpc"
	ggrpcbufconn "google.golang.org/grpc/test/bufconn"
)

// Server stores connection data to the bufconn gRPC server.
type Server struct {
	target      string
	dialOptions []grpc.DialOption
}

// NewServer brings up the gRPC server in bufconn and returns connection parameters.
func NewServer(t *testing.T, register func(*grpc.Server), opts ...Option) Server {
	t.Helper()

	if register == nil {
		t.Fatal("register function is nil")
	}

	cfg := newConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	listener := ggrpcbufconn.Listen(cfg.bufSize)
	grpcServer := grpc.NewServer(cfg.serverOptions...)

	var (
		serveStarted bool
		cleanupOnce  sync.Once
	)
	cleanup := func() {
		cleanupOnce.Do(func() {
			if !serveStarted {
				if closeErr := listener.Close(); closeErr != nil {
					t.Errorf("close listener: %v", closeErr)
				}
			}

			grpcServer.Stop()
		})
	}
	t.Cleanup(cleanup)

	defer func() {
		if recovered := recover(); recovered != nil {
			cleanup()
			panic(recovered)
		}
	}()

	register(grpcServer)

	serveStarted = true
	go func() {
		_ = grpcServer.Serve(listener)
	}()

	return Server{
		target:      cfg.target,
		dialOptions: dialOptionsForListener(listener, cfg.dialOptions...),
	}
}

// Target returns the target for the grpc.NewClient and adapter-style constructors.
func (server Server) Target() string {
	return server.target
}

// DialOptions returns a copy of the dial options for connecting to the in-memory server.
func (server Server) DialOptions(extraOptions ...grpc.DialOption) []grpc.DialOption {
	options := make([]grpc.DialOption, 0, len(server.dialOptions)+len(extraOptions))
	options = append(options, server.dialOptions...)
	options = append(options, extraOptions...)

	return options
}
