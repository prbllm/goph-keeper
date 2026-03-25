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

func TestLoggingUnaryInterceptor_logsStatusAndMethod(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	interceptor := newLoggingUnaryInterceptor(logger)

	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Login"}
	_, err := interceptor(context.Background(), struct{}{}, info, func(context.Context, any) (any, error) {
		return struct{}{}, status.Error(codes.InvalidArgument, "bad request")
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}

	if observed.Len() != 1 {
		t.Fatalf("entries: got %d want 1", observed.Len())
	}
	fields := observed.All()[0].ContextMap()
	if got := fields["method"]; got != info.FullMethod {
		t.Fatalf("method: got %v want %v", got, info.FullMethod)
	}
	if got := fields["status_code"]; got != codes.InvalidArgument.String() {
		t.Fatalf("status_code: got %v want %v", got, codes.InvalidArgument.String())
	}
}

func TestLoggingStreamInterceptor_logsStatusAndMethod(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	interceptor := newLoggingStreamInterceptor(logger)

	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Login"}
	ss := &testServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.InvalidArgument, "bad request")
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.InvalidArgument)
	}

	if observed.Len() != 1 {
		t.Fatalf("entries: got %d want 1", observed.Len())
	}
	fields := observed.All()[0].ContextMap()
	if got := fields["method"]; got != info.FullMethod {
		t.Fatalf("method: got %v want %v", got, info.FullMethod)
	}
	if got := fields["status_code"]; got != codes.InvalidArgument.String() {
		t.Fatalf("status_code: got %v want %v", got, codes.InvalidArgument.String())
	}
}
