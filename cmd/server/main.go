package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger, err := newLogger(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	startCtx := context.Background()
	db, err := postgres.DefaultConnector.OpenPing(startCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()
	logger.Info("postgres ready")

	_, err = s3minio.DefaultConnector.Connect(startCtx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		return fmt.Errorf("minio: %w", err)
	}
	logger.Info("minio ready", zap.String("bucket", cfg.MinioBucket))

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.GRPCAddr, err)
	}

	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gRPC server listening", zap.String("addr", cfg.GRPCAddr))
		errCh <- grpcServer.Serve(lis)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.GRPCShutdownTimeoutSeconds)*time.Second)
		defer cancel()
		select {
		case <-shutdownCtx.Done():
			logger.Warn("graceful stop timed out, forcing stop")
			grpcServer.Stop()
		case <-stopped:
		}
		logger.Info("server stopped")
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

func newLogger(level string) (*zap.Logger, error) {
	switch strings.ToLower(level) {
	case config.LogLevelDebug:
		log, err := zap.NewDevelopment()
		if err != nil {
			return nil, fmt.Errorf("zap development: %w", err)
		}
		return log, nil
	default:
		var zl zapcore.Level
		if err := zl.UnmarshalText([]byte(level)); err != nil {
			return nil, fmt.Errorf("parse log level: %w", err)
		}
		zcfg := zap.NewProductionConfig()
		zcfg.Level = zap.NewAtomicLevelAt(zl)
		return zcfg.Build()
	}
}
