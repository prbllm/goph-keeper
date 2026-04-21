package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAuthUnaryInterceptor_publicMethodWithoutToken(t *testing.T) {
	interceptor := newAuthUnaryInterceptor("secret")
	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Register"}

	_, err := interceptor(context.Background(), struct{}{}, info, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthUnaryInterceptor_grpcHealthV1_publicMethodWithoutToken(t *testing.T) {
	interceptor := newAuthUnaryInterceptor("secret")
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}

	_, err := interceptor(context.Background(), struct{}{}, info, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthUnaryInterceptor_privateMethodWithoutToken(t *testing.T) {
	interceptor := newAuthUnaryInterceptor("secret")
	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}

	_, err := interceptor(context.Background(), struct{}{}, info, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthStreamInterceptor_publicMethodWithoutToken(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Register"}
	ss := &testServerStream{ctx: context.Background()}

	err := interceptor(nil, ss, info, func(_ any, stream grpc.ServerStream) error {
		if _, ok := UserIDFromContext(stream.Context()); ok {
			t.Fatalf("expected no auth context to be set")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStreamInterceptor_grpcHealthV1_publicMethodWithoutToken(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	ss := &testServerStream{ctx: context.Background()}

	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStreamInterceptor_privateMethodWithoutToken(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}
	ss := &testServerStream{ctx: context.Background()}

	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthStreamInterceptor_blobUploadWithoutToken(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.BlobService/UploadBlob"}
	ss := &testServerStream{ctx: context.Background()}

	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthStreamInterceptor_blobDownloadWithoutToken(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.BlobService/DownloadBlob"}
	ss := &testServerStream{ctx: context.Background()}

	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthUnaryInterceptor_privateMethodWithInvalidBearer(t *testing.T) {
	interceptor := newAuthUnaryInterceptor("secret")
	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic abc"))

	_, err := interceptor(ctx, struct{}{}, info, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthStreamInterceptor_privateMethodWithInvalidBearer(t *testing.T) {
	interceptor := newAuthStreamInterceptor("secret")
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic abc"))
	ss := &testServerStream{ctx: ctx}

	err := interceptor(nil, ss, info, func(_ any, _ grpc.ServerStream) error { return nil })
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code: got %v want %v", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthUnaryInterceptor_privateMethodWithValidToken(t *testing.T) {
	const secret = "super-secret"
	interceptor := newAuthUnaryInterceptor(secret)
	info := &grpc.UnaryServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}

	token, err := auth.NewJWTIssuer(secret, time.Now).IssueAccessToken(
		auth.AccessTokenClaims{UserID: "user-1", DeviceID: "device-1"},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))

	_, err = interceptor(ctx, struct{}{}, info, func(ctx context.Context, _ any) (any, error) {
		userID, ok := UserIDFromContext(ctx)
		if !ok || userID != "user-1" {
			t.Fatalf("user id from context: got=%q ok=%v", userID, ok)
		}
		deviceID, ok := DeviceIDFromContext(ctx)
		if !ok || deviceID != "device-1" {
			t.Fatalf("device id from context: got=%q ok=%v", deviceID, ok)
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStreamInterceptor_privateMethodWithValidToken(t *testing.T) {
	const secret = "super-secret"
	interceptor := newAuthStreamInterceptor(secret)
	info := &grpc.StreamServerInfo{FullMethod: "/gophkeeper.v1.AuthService/Logout"}

	token, err := auth.NewJWTIssuer(secret, time.Now).IssueAccessToken(
		auth.AccessTokenClaims{UserID: "user-1", DeviceID: "device-1"},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
	ss := &testServerStream{ctx: ctx}

	err = interceptor(nil, ss, info, func(_ any, stream grpc.ServerStream) error {
		userID, ok := UserIDFromContext(stream.Context())
		if !ok || userID != "user-1" {
			t.Fatalf("user id from context: got=%q ok=%v", userID, ok)
		}
		deviceID, ok := DeviceIDFromContext(stream.Context())
		if !ok || deviceID != "device-1" {
			t.Fatalf("device id from context: got=%q ok=%v", deviceID, ok)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
