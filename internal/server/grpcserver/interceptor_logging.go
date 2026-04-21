package grpcserver

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func newLoggingUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)

		logger.Info(
			"grpc request",
			zap.String("kind", "unary"),
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.String("status_code", code.String()),
		)

		return resp, err
	}
}

func newLoggingStreamInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		code := status.Code(err)

		logger.Info(
			"grpc request",
			zap.String("kind", "stream"),
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.String("status_code", code.String()),
		)

		return err
	}
}
