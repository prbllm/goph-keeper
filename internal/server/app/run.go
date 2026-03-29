// Package app wires the production server: config, logging, TLS, Postgres migrations, MinIO, auth,
// vault engine, background jobs, and the gRPC stack. Run and RunContext are the main entrypoints used by cmd/server.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/grpcserver"
	"github.com/prbllm/goph-keeper/internal/server/jobs"
	"github.com/prbllm/goph-keeper/internal/server/logging"
	"github.com/prbllm/goph-keeper/internal/server/migrations"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Run starts the server runtime and blocks until SIGINT or SIGTERM.
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return RunContext(ctx, nil)
}

// RunContext starts the full server stack like Run, but shuts down when ctx is cancelled.
// If onListen is non-nil, it is called once with the bound listener address after Listen succeeds
// (useful with GOPHKEEPER_GRPC_ADDR=127.0.0.1:0 to discover the ephemeral port).
func RunContext(ctx context.Context, onListen func(net.Addr)) error {
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

	now := time.Now

	authRepos := postgres.NewAuthRepositories(db)
	authService := auth.NewService(
		authRepos,
		authRepos.SessionRepository(),
		authRepos,
		auth.BcryptHasher{},
		auth.NewJWTIssuer(cfg.JWTSecret, now),
		now,
		time.Duration(cfg.AccessTTLSec)*time.Second,
		time.Duration(cfg.RefreshTTLSec)*time.Second,
		cfg.InlineThresholdBytes,
		cfg.MaxBlobSizeBytes,
		cfg.MaxChunkSizeBytes,
	)

	vaultRepos := postgres.NewVaultRepositories(db)
	vaultEngine := vault.NewEngine(
		vaultRepos.Vault(),
		vaultRepos.Revisions(),
		vaultRepos.ProcessedOperations(),
		now,
		logger,
	)

	blobRepo := postgres.NewBlobRepository(db)
	blobStorage := s3minio.NewObjectStorage(minioClient, cfg.MinioBucket)
	uploadStore := postgres.NewUploadStore(db)

	deps := &grpcserver.Deps{
		Logger:       logger,
		Cfg:          cfg,
		AuthService:  authService,
		DeviceAdmin:  authRepos,
		VaultEngine:  vaultEngine,
		PostgresPool: db,
		Now:          now,
		BlobRepo:     blobRepo,
		BlobStorage:  blobStorage,
		UploadStore:  uploadStore,
	}

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.GRPCAddr, err)
	}
	if onListen != nil {
		onListen(lis.Addr())
	}

	grpcServer, err := grpcserver.New(deps, tlsCreds)
	if err != nil {
		return fmt.Errorf("grpc server: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gRPC server listening", zap.String("addr", lis.Addr().String()))
		errCh <- grpcServer.Serve(lis)
	}()

	go jobs.RunUploadSessionCleanup(
		ctx,
		logger,
		uploadStore,
		blobStorage,
		time.Duration(cfg.UploadSessionCleanupIntervalMinutes)*time.Minute,
		time.Duration(config.UploadSessionCleanupRunTimeoutSeconds)*time.Second,
	)

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
