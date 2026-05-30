package bufconn

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// rpcTimeout sets an upper limit on the time per RPC to prevent tests from hanging.
	rpcTimeout = 2 * time.Second

	// testServiceName describes the name of the test service.
	testServiceName = "itestkit.TestService"
	// testServiceFullMethod describes the full path of a unary method.
	testServiceFullMethod = "/itestkit.TestService/UnaryCall"
)

// newRPCCtx creates a context for one RPC with a deadline and registers cancel via Cleanup.
func newRPCCtx(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), rpcTimeout)
	t.Cleanup(cancel)

	return ctx
}

// testServiceServer describes the test service contract.
type testServiceServer interface {
	UnaryCall(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

// testService implements a test service to test a client call.
type testService struct {
	called atomic.Bool
}

// UnaryCall marks the call and returns an empty response.
func (service *testService) UnaryCall(
	_ context.Context,
	_ *emptypb.Empty,
) (*emptypb.Empty, error) {
	service.called.Store(true)
	return &emptypb.Empty{}, nil
}

// testServiceDesc contains a description of the test service for gRPC.
var testServiceDesc = grpc.ServiceDesc{
	ServiceName: testServiceName,
	HandlerType: (*testServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "UnaryCall",
			Handler:    testServiceUnaryCallHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "itestkit.proto",
}

// testServiceUnaryCallHandler adapts the unary method of the test service.
func testServiceUnaryCallHandler(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	request := new(emptypb.Empty)
	if err := dec(request); err != nil {
		return nil, err
	}

	service, ok := srv.(testServiceServer)
	if !ok {
		return nil, fmt.Errorf("unexpected server type %T", srv)
	}
	if interceptor == nil {
		response, err := service.UnaryCall(ctx, request)
		if err != nil {
			return nil, err
		}
		return response, nil
	}

	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: testServiceFullMethod,
	}
	handler := func(ctx context.Context, req any) (any, error) {
		typedRequest, ok := req.(*emptypb.Empty)
		if !ok {
			return nil, fmt.Errorf("unexpected request type %T", req)
		}
		response, err := service.UnaryCall(ctx, typedRequest)
		if err != nil {
			return nil, err
		}
		return response, nil
	}

	return interceptor(ctx, request, info, handler)
}

// testServiceClient describes a minimal client for calling a unary method.
type testServiceClient interface {
	UnaryCall(ctx context.Context, request *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

// testClient implements a test client on top of grpc.ClientConnInterface.
type testClient struct {
	conn grpc.ClientConnInterface
}

// newTestClient creates a test client to call a unary method.
func newTestClient(conn grpc.ClientConnInterface) testServiceClient {
	return testClient{conn: conn}
}

// UnaryCall makes a unary call via conn.Invoke.
func (client testClient) UnaryCall(
	ctx context.Context,
	request *emptypb.Empty,
	opts ...grpc.CallOption,
) (*emptypb.Empty, error) {
	response := new(emptypb.Empty)
	if err := client.conn.Invoke(ctx, testServiceFullMethod, request, response, opts...); err != nil {
		return nil, err
	}
	return response, nil
}

// registerTestService registers a test service on the gRPC server.
func registerTestService(server *testService) func(*grpc.Server) {
	return func(grpcServer *grpc.Server) {
		grpcServer.RegisterService(&testServiceDesc, server)
	}
}

// withCleanupBarrier runs a separate test scope so that Cleanup has time to free up resources before checking the parent.
func withCleanupBarrier(t *testing.T, callback func(t *testing.T)) {
	t.Helper()

	if ok := t.Run("cleanup-barrier", callback); !ok {
		t.FailNow()
	}
}

// TestNewClient_UnaryCall tests a successful RPC call through the bufconn client.
func TestNewClient_UnaryCall(t *testing.T) {
	t.Parallel()

	server := &testService{called: atomic.Bool{}}
	client := NewClient(
		t,
		registerTestService(server),
		newTestClient,
	)

	ctx := newRPCCtx(t)

	_, err := client.UnaryCall(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, server.called.Load())
}

// TestNewClient_UsesOptions checks the use of server/dial options.
func TestNewClient_UsesOptions(t *testing.T) {
	t.Parallel()

	var serverInterceptorCalled atomic.Bool
	var clientInterceptorCalled atomic.Bool

	serverInterceptor := grpc.UnaryInterceptor(func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		serverInterceptorCalled.Store(true)
		return handler(ctx, req)
	})

	clientInterceptor := grpc.WithUnaryInterceptor(func(
		ctx context.Context,
		method string,
		req, reply any,
		conn *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		clientInterceptorCalled.Store(true)
		return invoker(ctx, method, req, reply, conn, opts...)
	})

	server := &testService{called: atomic.Bool{}}
	client := NewClient(
		t,
		registerTestService(server),
		newTestClient,
		WithBufSize(1024*128),
		WithServerOptions(serverInterceptor),
		WithDialOptions(clientInterceptor),
	)

	ctx := newRPCCtx(t)

	_, err := client.UnaryCall(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, serverInterceptorCalled.Load())
	require.True(t, clientInterceptorCalled.Load())
}

// TestNewClient_CleanupClosesConnection tests whether the connection is closed after cleanup.
func TestNewClient_CleanupClosesConnection(t *testing.T) {
	t.Parallel()

	var client testServiceClient

	withCleanupBarrier(t, func(t *testing.T) {
		client = NewClient(
			t,
			registerTestService(&testService{called: atomic.Bool{}}),
			newTestClient,
		)
		ctx := newRPCCtx(t)

		_, err := client.UnaryCall(ctx, &emptypb.Empty{})
		require.NoError(t, err)
	})

	ctx := newRPCCtx(t)

	_, err := client.UnaryCall(ctx, &emptypb.Empty{})
	require.Error(t, err)
}

// TestNewClient_WithTarget checks that the custom target is used when creating the conn.
func TestNewClient_WithTarget(t *testing.T) {
	t.Parallel()

	server := &testService{called: atomic.Bool{}}
	client := NewClient(
		t,
		registerTestService(server),
		newTestClient,
		WithTarget("passthrough:///bufnet-custom"),
	)

	ctx := newRPCCtx(t)

	_, err := client.UnaryCall(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, server.called.Load())
}

// TestNewServer_UnaryCall checks that NewServer returns a working target and dial options.
func TestNewServer_UnaryCall(t *testing.T) {
	t.Parallel()

	serverImpl := &testService{called: atomic.Bool{}}
	server := NewServer(
		t,
		registerTestService(serverImpl),
		WithTarget("passthrough:///bufnet-custom"),
	)

	conn, err := grpc.NewClient(server.Target(), server.DialOptions()...)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("close conn: %v", closeErr)
		}
	})

	client := newTestClient(conn)
	ctx := newRPCCtx(t)

	_, err = client.UnaryCall(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, serverImpl.called.Load())
}

// TestNewServer_DialOptions checks that additional dial options are applied to the connection.
func TestNewServer_DialOptions(t *testing.T) {
	t.Parallel()

	var clientInterceptorCalled atomic.Bool

	server := NewServer(t, registerTestService(&testService{called: atomic.Bool{}}))

	conn, err := grpc.NewClient(
		server.Target(),
		server.DialOptions(
			grpc.WithUnaryInterceptor(func(
				ctx context.Context,
				method string,
				req, reply any,
				conn *grpc.ClientConn,
				invoker grpc.UnaryInvoker,
				opts ...grpc.CallOption,
			) error {
				clientInterceptorCalled.Store(true)
				return invoker(ctx, method, req, reply, conn, opts...)
			}),
		)...,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("close conn: %v", closeErr)
		}
	})

	client := newTestClient(conn)
	ctx := newRPCCtx(t)

	_, err = client.UnaryCall(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, clientInterceptorCalled.Load())
}

// TestNewServer_CleanupStopsServer verifies that cleanup NewServer stops the server.
func TestNewServer_CleanupStopsServer(t *testing.T) {
	t.Parallel()

	var (
		client testServiceClient
		conn   *grpc.ClientConn
	)

	withCleanupBarrier(t, func(t *testing.T) {
		server := NewServer(t, registerTestService(&testService{called: atomic.Bool{}}))

		var err error
		conn, err = grpc.NewClient(server.Target(), server.DialOptions()...)
		require.NoError(t, err)

		client = newTestClient(conn)
		ctx := newRPCCtx(t)

		_, err = client.UnaryCall(ctx, &emptypb.Empty{})
		require.NoError(t, err)
	})

	t.Cleanup(func() {
		if conn != nil {
			if closeErr := conn.Close(); closeErr != nil {
				t.Errorf("close conn: %v", closeErr)
			}
		}
	})

	ctx := newRPCCtx(t)

	_, err := client.UnaryCall(ctx, &emptypb.Empty{})
	require.Error(t, err)
}
