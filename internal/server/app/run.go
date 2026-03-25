package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/grpcserver"
	"github.com/prbllm/goph-keeper/internal/server/logging"
	"github.com/prbllm/goph-keeper/internal/server/migrations"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Run starts the server runtime and blocks until shutdown.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	tlsCreds, err := grpcserver.LoadServerTransportCredentials(cfg)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}

	startCtx := context.Background()
	db, err := postgres.DefaultConnector.OpenPing(startCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()
	logger.Info("postgres ready")

	if err := migrations.Run(logger, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	minioClient, err := s3minio.DefaultConnector.Connect(startCtx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		return fmt.Errorf("minio: %w", err)
	}
	logger.Info("minio ready", zap.String("bucket", cfg.MinioBucket))

	deps := &grpcserver.Deps{
		Logger: logger,
		Cfg:    cfg,
		DB:     db,
		MinIO:  minioClient,
	}

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.GRPCAddr, err)
	}

	grpcServer, err := grpcserver.New(deps, tlsCreds)
	if err != nil {
		return fmt.Errorf("grpc server: %w", err)
	}

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
