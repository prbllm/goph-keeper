package grpcserver

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecoveryUnaryInterceptor_recoversPanic(t *testing.T) {
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	interceptor := newRecoveryUnaryInterceptor(logger)

	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.VaultService/CreateItem"}
	_, err := interceptor(context.Background(), struct{}{}, info, func(context.Context, any) (any, error) {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Internal)
	}

	if observed.Len() != 1 {
		t.Fatalf("entries: got %d want 1", observed.Len())
	}
	fields := observed.All()[0].ContextMap()
	if got := fields["method"]; got != info.FullMethod {
		t.Fatalf("method: got %v want %v", got, info.FullMethod)
	}
}

func TestRecoveryStreamInterceptor_recoversPanic(t *testing.T) {
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	interceptor := newRecoveryStreamInterceptor(logger)

	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.VaultService/CreateItem"}
	ss := &testServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Internal)
	}

	if observed.Len() != 1 {
		t.Fatalf("entries: got %d want 1", observed.Len())
	}
	fields := observed.All()[0].ContextMap()
	if got := fields["method"]; got != info.FullMethod {
		t.Fatalf("method: got %v want %v", got, info.FullMethod)
	}
}
