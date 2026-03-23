package config_test

import (
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
}

func TestLoad_success_overrides(t *testing.T) {
	minimalValidEnv(t)
	t.Setenv(config.EnvGRPCAddr, "0.0.0.0:9999")
	t.Setenv(config.EnvLogLevel, "WARN")
	t.Setenv(config.EnvMinioEndpoint, "minio.local:9000")
	t.Setenv(config.EnvMinioBucket, "custom")
	t.Setenv(config.EnvMinioUseSSL, "true")

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
			name: "invalid log level",
			prep: func(t *testing.T) {
				minimalValidEnv(t)
				t.Setenv(config.EnvLogLevel, "verbose")
			},
			wantSub: config.EnvLogLevel,
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
