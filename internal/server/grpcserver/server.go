package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

// LoadServerTransportCredentials loads server TLS material from paths in cfg.
func LoadServerTransportCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	return credentials.NewServerTLSFromFile(cfg.GRPCTLSCertPath, cfg.GRPCTLSKeyPath)
}

// New creates a gRPC server and registers core infra services.
// tlsCreds must be non-nil (typically from LoadServerTransportCredentials).
// Deps.Logger, Deps.Cfg, Deps.DB, and Deps.MinIO must be non-nil.
func New(deps *Deps, tlsCreds credentials.TransportCredentials) (*grpc.Server, error) {
	if deps == nil {
		return nil, fmt.Errorf("grpcserver: deps is nil")
	}
	if deps.Logger == nil {
		return nil, fmt.Errorf("grpcserver: deps.Logger is nil")
	}
	if deps.Cfg == nil {
		return nil, fmt.Errorf("grpcserver: deps.Cfg is nil")
	}
	if tlsCreds == nil {
		return nil, fmt.Errorf("grpcserver: tlsCreds is nil")
	}
	if deps.DB == nil {
		return nil, fmt.Errorf("grpcserver: deps.DB is nil")
	}
	if deps.MinIO == nil {
		return nil, fmt.Errorf("grpcserver: deps.MinIO is nil")
	}

	deps.Logger.Debug("gRPC: registering health services")

	s := grpc.NewServer(grpc.Creds(tlsCreds))
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

	return s, nil
}
