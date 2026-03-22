package main

import (
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// serverDeps groups runtime dependencies wired at startup for gRPC and services.
type serverDeps struct {
	Logger *zap.Logger
	Cfg    *config.Config
	DB     postgres.Pool
	MinIO  s3minio.Client
}

func newGRPCServer(deps *serverDeps) *grpc.Server {
	deps.Logger.Debug("gRPC: registering health and reflection")

	s := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(s)
	return s
}
