package grpcserver

import (
	"context"
	"errors"
	"fmt"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// AuthService defines the subset of auth service methods required by gRPC handlers.
type AuthService interface {
	Register(ctx context.Context, in auth.RegisterInput) (*auth.RegisterOutput, error)
	Login(ctx context.Context, in auth.LoginInput) (*auth.LoginOutput, error)
	Refresh(ctx context.Context, in auth.RefreshInput) (*auth.RefreshOutput, error)
	Logout(ctx context.Context, refreshToken string) error
	Limits() *gophkeeperv1.Limits
}

// Deps groups runtime dependencies wired at startup for gRPC and services.
type Deps struct {
	Logger      *zap.Logger
	Cfg         *config.Config
	AuthService AuthService
	VaultEngine vault.Engine
	BlobRepo    blob.Repository
	BlobStorage blob.ObjectStorage
}

var (
	_ AuthService = (*auth.Service)(nil)
)

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
// Deps.Logger, Deps.Cfg, Deps.AuthService, Deps.VaultEngine, Deps.BlobRepo, and Deps.BlobStorage must be non-nil.
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
	if deps.AuthService == nil {
		return nil, fmt.Errorf("grpcserver: deps.AuthService is nil")
	}
	if deps.VaultEngine == nil {
		return nil, fmt.Errorf("grpcserver: deps.VaultEngine is nil")
	}
	if deps.BlobRepo == nil {
		return nil, fmt.Errorf("grpcserver: deps.BlobRepo is nil")
	}
	if deps.BlobStorage == nil {
		return nil, fmt.Errorf("grpcserver: deps.BlobStorage is nil")
	}
	if tlsCreds == nil {
		return nil, fmt.Errorf("grpcserver: tlsCreds is nil")
	}

	deps.Logger.Debug("gRPC: registering health services")

	s := grpc.NewServer(
		grpc.Creds(tlsCreds),
		grpc.ChainUnaryInterceptor(
			newLoggingUnaryInterceptor(deps.Logger),
			newRecoveryUnaryInterceptor(deps.Logger),
			newAuthUnaryInterceptor(deps.Cfg.JWTSecret),
		),
		grpc.ChainStreamInterceptor(
			newLoggingStreamInterceptor(deps.Logger),
			newRecoveryStreamInterceptor(deps.Logger),
			newAuthStreamInterceptor(deps.Cfg.JWTSecret),
		),
	)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	gophkeeperv1.RegisterHealthServiceServer(s, healthService{})

	gophkeeperv1.RegisterAuthServiceServer(s, authHandler{svc: deps.AuthService})

	deps.Logger.Debug("gRPC: registering vault service")
	gophkeeperv1.RegisterVaultServiceServer(s, vaultHandler{engine: deps.VaultEngine})

	return s, nil
}
