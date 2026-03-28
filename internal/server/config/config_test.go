package config_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/prbllm/goph-keeper/internal/server/config"
)

// minimalValidEnv sets required vars; other fields rely on defaults.
func minimalValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvDatabaseURL, "postgres://localhost/gophkeeper")
	t.Setenv(config.EnvMinioAccessKey, "access")
	t.Setenv(config.EnvMinioSecretKey, "secret")
	t.Setenv(config.EnvJWTSecret, strings.Repeat("x", config.MinJWTSecretLen))
}

func TestLoad_success_defaults(t *testing.T) {
	minimalValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCAddr != config.DefaultGRPCAddr {
		t.Errorf("GRPCAddr = %q, want default %q", cfg.GRPCAddr, config.DefaultGRPCAddr)
	}
	if cfg.LogLevel != config.DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, config.DefaultLogLevel)
	}
	if cfg.MinioEndpoint != config.DefaultMinioEndpoint {
		t.Errorf("MinioEndpoint = %q, want %q", cfg.MinioEndpoint, config.DefaultMinioEndpoint)
	}
	if cfg.MinioBucket != config.DefaultMinioBucket {
		t.Errorf("MinioBucket = %q, want %q", cfg.MinioBucket, config.DefaultMinioBucket)
	}
	if cfg.MinioUseSSL {
		t.Error("MinioUseSSL: expected false from default")
	}
	if cfg.AccessTTLSec != config.DefaultAccessTTLSec {
		t.Errorf("AccessTTLSec = %d, want %d", cfg.AccessTTLSec, config.DefaultAccessTTLSec)
	}
	if cfg.RefreshTTLSec != config.DefaultRefreshTTLSec {
		t.Errorf("RefreshTTLSec = %d, want %d", cfg.RefreshTTLSec, config.DefaultRefreshTTLSec)
	}
	if cfg.InlineThresholdBytes != config.DefaultInlineThresholdBytes {
		t.Errorf("InlineThresholdBytes = %d, want %d", cfg.InlineThresholdBytes, config.DefaultInlineThresholdBytes)
	}
	if cfg.MaxBlobSizeBytes != config.DefaultMaxBlobSizeBytes {
		t.Errorf("MaxBlobSizeBytes = %d, want %d", cfg.MaxBlobSizeBytes, config.DefaultMaxBlobSizeBytes)
	}
	if cfg.MaxChunkSizeBytes != config.DefaultMaxChunkSizeBytes {
		t.Errorf("MaxChunkSizeBytes = %d, want %d", cfg.MaxChunkSizeBytes, config.DefaultMaxChunkSizeBytes)
	}
	if cfg.GRPCMaxMessageBytes != config.DefaultGRPCMaxMessageBytes {
		t.Errorf("GRPCMaxMessageBytes = %d, want %d", cfg.GRPCMaxMessageBytes, config.DefaultGRPCMaxMessageBytes)
	}
	if cfg.GRPCTLSCertPath != config.DefaultGRPCTLSCertPath {
		t.Errorf("GRPCTLSCertPath = %q, want %q", cfg.GRPCTLSCertPath, config.DefaultGRPCTLSCertPath)
	}
	if cfg.GRPCTLSKeyPath != config.DefaultGRPCTLSKeyPath {
		t.Errorf("GRPCTLSKeyPath = %q, want %q", cfg.GRPCTLSKeyPath, config.DefaultGRPCTLSKeyPath)
	}
	if cfg.UploadSessionTTLHours != config.DefaultUploadSessionTTLHours {
		t.Errorf("UploadSessionTTLHours = %d, want %d", cfg.UploadSessionTTLHours, config.DefaultUploadSessionTTLHours)
	}
}

func TestLoad_success_overrides(t *testing.T) {
	minimalValidEnv(t)
	t.Setenv(config.EnvGRPCAddr, "0.0.0.0:9999")
	t.Setenv(config.EnvLogLevel, "WARN")
	t.Setenv(config.EnvMinioEndpoint, "minio.local:9000")
	t.Setenv(config.EnvMinioBucket, "custom")
	t.Setenv(config.EnvMinioUseSSL, "true")
	t.Setenv(config.EnvAccessTTLSec, "60")
	t.Setenv(config.EnvRefreshTTLSec, "600")
	t.Setenv(config.EnvInlineThresholdBytes, "4096")
	t.Setenv(config.EnvMaxBlobSizeBytes, "8192")
	t.Setenv(config.EnvMaxChunkSizeBytes, "1024")
	t.Setenv(config.EnvGRPCTLSCertPath, "/tls/cert.pem")
	t.Setenv(config.EnvGRPCTLSKeyPath, "/tls/key.pem")
	t.Setenv(config.EnvUploadSessionTTLHours, "48")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCAddr != "0.0.0.0:9999" {
		t.Errorf("GRPCAddr = %q", cfg.GRPCAddr)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want lowercased warn", cfg.LogLevel)
	}
	if cfg.MinioEndpoint != "minio.local:9000" {
		t.Errorf("MinioEndpoint = %q", cfg.MinioEndpoint)
	}
	if cfg.MinioBucket != "custom" {
		t.Errorf("MinioBucket = %q", cfg.MinioBucket)
	}
	if !cfg.MinioUseSSL {
		t.Error("MinioUseSSL: expected true")
	}
	if cfg.AccessTTLSec != 60 {
		t.Errorf("AccessTTLSec = %d", cfg.AccessTTLSec)
	}
	if cfg.RefreshTTLSec != 600 {
		t.Errorf("RefreshTTLSec = %d", cfg.RefreshTTLSec)
	}
	if cfg.InlineThresholdBytes != 4096 {
		t.Errorf("InlineThresholdBytes = %d", cfg.InlineThresholdBytes)
	}
	if cfg.MaxBlobSizeBytes != 8192 {
		t.Errorf("MaxBlobSizeBytes = %d", cfg.MaxBlobSizeBytes)
	}
	if cfg.MaxChunkSizeBytes != 1024 {
		t.Errorf("MaxChunkSizeBytes = %d", cfg.MaxChunkSizeBytes)
	}
	if cfg.GRPCTLSCertPath != "/tls/cert.pem" {
		t.Errorf("GRPCTLSCertPath = %q", cfg.GRPCTLSCertPath)
	}
	if cfg.GRPCTLSKeyPath != "/tls/key.pem" {
		t.Errorf("GRPCTLSKeyPath = %q", cfg.GRPCTLSKeyPath)
	}
	if cfg.UploadSessionTTLHours != 48 {
		t.Errorf("UploadSessionTTLHours = %d, want 48", cfg.UploadSessionTTLHours)
	}
}

func TestLoad_MinioUseSSL_variants(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"1", true},
		{"YES", true},
		{"On", true},
		{"0", false},
		{"no", false},
		{"off", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			minimalValidEnv(t)
			t.Setenv(config.EnvMinioUseSSL, tc.raw)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.MinioUseSSL != tc.want {
				t.Errorf("MinioUseSSL = %v, want %v", cfg.MinioUseSSL, tc.want)
			}
		})
	}
}

func TestLoad_errors(t *testing.T) {
	cases := []struct {
		name    string
		prep    func(*testing.T)
		wantSub string
	}{
		{
			name: "database URL required",
			prep: func(t *testing.T) {
				t.Setenv(config.EnvDatabaseURL, "")
				t.Setenv(config.EnvMinioAccessKey, "a")
				t.Setenv(config.EnvMinioSecretKey, "b")
			},
			wantSub: config.EnvDatabaseURL,
		},
		{
			name: "jwt secret missing",
			prep: func(t *testing.T) {
				t.Setenv(config.EnvDatabaseURL, "postgres://x/y")
				t.Setenv(config.EnvMinioAccessKey, "a")
				t.Setenv(config.EnvMinioSecretKey, "b")
				t.Setenv(config.EnvJWTSecret, "")
			},
			wantSub: config.EnvJWTSecret,
		},
		{
			name: "jwt secret too short",
			prep: func(t *testing.T) {
				t.Setenv(config.EnvDatabaseURL, "postgres://x/y")
				t.Setenv(config.EnvMinioAccessKey, "a")
				t.Setenv(config.EnvMinioSecretKey, "b")
				t.Setenv(config.EnvJWTSecret, "short")
			},
			wantSub: config.EnvJWTSecret,
		},
		{
			name: "invalid log level",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvLogLevel, "verbose")
			},
			wantSub: config.EnvLogLevel,
		},
		{
			name: "invalid access token ttl",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvAccessTTLSec, "0")
			},
			wantSub: config.EnvAccessTTLSec,
		},
		{
			name: "invalid refresh token ttl",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvRefreshTTLSec, "abc")
			},
			wantSub: config.EnvRefreshTTLSec,
		},
		{
			name: "invalid upload session ttl hours",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvUploadSessionTTLHours, "0")
			},
			wantSub: config.EnvUploadSessionTTLHours,
		},
		{
			name: "invalid inline threshold",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvInlineThresholdBytes, "0")
			},
			wantSub: config.EnvInlineThresholdBytes,
		},
		{
			name: "inline threshold exceeds max blob size",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvInlineThresholdBytes, "999999999")
			},
			wantSub: config.EnvInlineThresholdBytes,
		},
		{
			name: "grpc max message bytes too large for platform int",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvGRPCMaxMessageBytes, fmt.Sprintf("%d", uint64(math.MaxInt)+1))
			},
			wantSub: config.EnvGRPCMaxMessageBytes,
		},
		{
			name: "invalid MinioUseSSL",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvMinioUseSSL, "sure")
			},
			wantSub: config.EnvMinioUseSSL,
		},
		{
			name: "minio endpoint whitespace only",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvMinioEndpoint, "   ")
			},
			wantSub: config.EnvMinioEndpoint,
		},
		{
			name: "minio access key missing",
			prep: func(t *testing.T) {
				t.Setenv(config.EnvDatabaseURL, "postgres://x/y")
				t.Setenv(config.EnvMinioAccessKey, "")
				t.Setenv(config.EnvMinioSecretKey, "s")
			},
			wantSub: config.EnvMinioAccessKey,
		},
		{
			name: "minio secret key missing",
			prep: func(t *testing.T) {
				t.Setenv(config.EnvDatabaseURL, "postgres://x/y")
				t.Setenv(config.EnvMinioAccessKey, "a")
				t.Setenv(config.EnvMinioSecretKey, "")
			},
			wantSub: config.EnvMinioSecretKey,
		},
		{
			name: "grpc tls cert path whitespace only",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvGRPCTLSCertPath, "   ")
			},
			wantSub: config.EnvGRPCTLSCertPath,
		},
		{
			name: "grpc tls key path whitespace only",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvGRPCTLSKeyPath, "\t")
			},
			wantSub: config.EnvGRPCTLSKeyPath,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.prep(t)
			_, err := config.Load()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q should mention %q", err.Error(), tc.wantSub)
			}
		})
	}
}
