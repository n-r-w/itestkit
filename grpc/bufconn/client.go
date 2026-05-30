package bufconn

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const (
	defaultBufSize = 1024 * 1024
	defaultTarget  = "passthrough:///bufnet"
)

// Option describes the bufconn helper configuration.
type Option func(*config)

// config stores bufconn helper parameters.
type config struct {
	bufSize       int
	target        string
	serverOptions []grpc.ServerOption
	dialOptions   []grpc.DialOption
}

// WithBufSize sets the buffer size for bufconn.
func WithBufSize(size int) Option {
	return func(cfg *config) {
		if size > 0 {
			cfg.bufSize = size
		}
	}
}

// WithTarget sets the target for grpc.NewClient.
func WithTarget(target string) Option {
	return func(cfg *config) {
		if target != "" {
			cfg.target = target
		}
	}
}

// WithServerOptions specifies gRPC server options.
func WithServerOptions(opts ...grpc.ServerOption) Option {
	return func(cfg *config) {
		cfg.serverOptions = append(cfg.serverOptions, opts...)
	}
}

// WithDialOptions specifies gRPC client options.
//
// Important: NewClient uses grpc.NewClient which ignores some DialOption
// (eg grpc.WithBlock, grpc.WithTimeout, grpc.WithReturnConnectionError,
// grpc.FailOnNonTempDialError). Do not rely on these options to control your connection;
// set RPC deadlines (context.WithTimeout) in your tests instead.
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(cfg *config) {
		cfg.dialOptions = append(cfg.dialOptions, opts...)
	}
}

// NewClient brings up the gRPC server in bufconn and returns the client.
func NewClient[Client any](
	t *testing.T,
	register func(*grpc.Server),
	newClient func(grpc.ClientConnInterface) Client,
	opts ...Option,
) Client {
	t.Helper()

	if register == nil {
		t.Fatal("register function is nil")
	}
	if newClient == nil {
		t.Fatal("newClient function is nil")
	}

	cfg := newConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	listener := bufconn.Listen(cfg.bufSize)
	grpcServer := grpc.NewServer(cfg.serverOptions...)

	var (
		conn         *grpc.ClientConn
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

			if conn != nil {
				if closeErr := conn.Close(); closeErr != nil {
					t.Errorf("close conn: %v", closeErr)
				}
			}
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

	dialOptions := dialOptionsForListener(listener, cfg.dialOptions...)

	conn, err := grpc.NewClient(cfg.target, dialOptions...)
	if err != nil {
		cleanup()
		t.Fatalf("dial bufconn: %v", err)
	}

	return newClient(conn)
}

// newConfig returns the default configuration.
func newConfig() config {
	return config{
		bufSize:       defaultBufSize,
		target:        defaultTarget,
		serverOptions: nil,
		dialOptions:   nil,
	}
}

// dialOptionsForListener generates basic dial options for connecting to bufconn listener.
func dialOptionsForListener(listener *bufconn.Listener, extraOptions ...grpc.DialOption) []grpc.DialOption {
	options := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	return append(options, extraOptions...)
}
