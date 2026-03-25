package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var publicMethods = map[string]struct{}{
	"/gophkeeper.v1.AuthService/Register": {},
	"/gophkeeper.v1.AuthService/Login":    {},
	"/gophkeeper.v1.AuthService/Refresh":  {},
	"/gophkeeper.v1.HealthService/Check":  {},
	"/grpc.health.v1.Health/Check":        {},
}

func newAuthUnaryInterceptor(secret string) grpc.UnaryServerInterceptor {
	secretBytes := []byte(secret)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if _, ok := publicMethods[info.FullMethod]; ok {
			return handler(ctx, req)
		}

		token, err := bearerTokenFromMetadata(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "missing or invalid authorization metadata")
		}

		claims, err := parseAccessToken(token, secretBytes)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}

		ctx = WithAuthContext(ctx, claims.userID, claims.deviceID)
		return handler(ctx, req)
	}
}

type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authServerStream) Context() context.Context {
	return s.ctx
}

func newAuthStreamInterceptor(secret string) grpc.StreamServerInterceptor {
	secretBytes := []byte(secret)

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if _, ok := publicMethods[info.FullMethod]; ok {
			return handler(srv, ss)
		}

		token, err := bearerTokenFromMetadata(ss.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, "missing or invalid authorization metadata")
		}

		claims, err := parseAccessToken(token, secretBytes)
		if err != nil {
			return status.Error(codes.Unauthenticated, "invalid access token")
		}

		ctx := WithAuthContext(ss.Context(), claims.userID, claims.deviceID)
		return handler(srv, &authServerStream{ServerStream: ss, ctx: ctx})
	}
}

type accessTokenClaims struct {
	userID   string
	deviceID string
}

func parseAccessToken(token string, secret []byte) (*accessTokenClaims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("token invalid")
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("claims type is invalid")
	}

	userID, _ := mapClaims[config.JWTClaimSubject].(string)
	deviceID, _ := mapClaims[config.JWTClaimDeviceID].(string)
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(deviceID) == "" {
		return nil, errors.New("required claims are missing")
	}

	return &accessTokenClaims{
		userID:   userID,
		deviceID: deviceID,
	}, nil
}

func bearerTokenFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("metadata is missing")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("authorization header is missing")
	}
	raw := strings.TrimSpace(values[0])
	if raw == "" {
		return "", errors.New("authorization header is empty")
	}
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return "", errors.New("authorization scheme is not bearer")
	}
	token := strings.TrimSpace(raw[len("bearer "):])
	if token == "" {
		return "", errors.New("bearer token is empty")
	}
	return token, nil
}
