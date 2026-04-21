package grpcserver

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newRecoveryUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (_ any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error(
					"grpc unary panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", rec),
					zap.Stack("stack"),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()

		return handler(ctx, req)
	}
}

func newRecoveryStreamInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error(
					"grpc stream panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", rec),
					zap.Stack("stack"),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}
