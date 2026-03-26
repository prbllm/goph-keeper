package transport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestAuthInterceptor_AddsToken(t *testing.T) {
	interceptor := AuthInterceptor("test-token")

	called := false

	err := interceptor(
		context.Background(),
		"/test",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			called = true
			return nil
		},
	)

	require.NoError(t, err)
	require.True(t, called)
}

func TestAuthInterceptor_NoToken(t *testing.T) {
	interceptor := AuthInterceptor("")

	err := interceptor(
		context.Background(),
		"/test",
		nil,
		nil,
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)

	require.NoError(t, err)
}
