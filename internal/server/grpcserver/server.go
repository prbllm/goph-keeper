package grpcserver

import (
	"context"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Deps groups runtime dependencies wired at startup for gRPC and services.
type Deps struct {
	Logger *zap.Logger
	Cfg    *config.Config
	DB     postgres.Pool
	MinIO  s3minio.Client
}

type healthService struct {
	gophkeeperv1.UnimplementedHealthServiceServer
}

func (healthService) Check(context.Context, *gophkeeperv1.HealthCheckRequest) (*gophkeeperv1.HealthCheckResponse, error) {
	return &gophkeeperv1.HealthCheckResponse{Status: "SERVING"}, nil
}

// New creates a gRPC server and registers core infra services.
func New(deps *Deps) *grpc.Server {
	deps.Logger.Debug("gRPC: registering health services")

	s := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	gophkeeperv1.RegisterHealthServiceServer(s, healthService{})

	now := time.Now
	authRepos := postgres.NewAuthRepositories(deps.DB)
	authService := auth.NewService(
		authRepos,
		authRepos.SessionRepository(),
		authRepos,
		auth.BcryptHasher{},
		auth.NewJWTIssuer(deps.Cfg.JWTSecret, now),
		now,
		time.Duration(deps.Cfg.AccessTTLSec)*time.Second,
		time.Duration(deps.Cfg.RefreshTTLSec)*time.Second,
		deps.Cfg.InlineThresholdBytes,
		deps.Cfg.MaxBlobSizeBytes,
		deps.Cfg.MaxChunkSizeBytes,
	)
	gophkeeperv1.RegisterAuthServiceServer(s, authHandler{svc: authService})

	return s
}
