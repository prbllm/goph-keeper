package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Config holds server settings loaded from the environment.
type Config struct {
	GRPCAddr             string
	GRPCTLSCertPath      string
	GRPCTLSKeyPath       string
	LogLevel             string // debug | info | warn | error
	DatabaseURL          string
	MinioEndpoint        string
	MinioAccessKey       string
	MinioSecretKey       string
	MinioBucket          string
	MinioUseSSL          bool
	JWTSecret            string
	AccessTTLSec         int
	RefreshTTLSec        int
	InlineThresholdBytes uint64
	MaxBlobSizeBytes     uint64
	MaxChunkSizeBytes    uint64
	GRPCMaxMessageBytes  uint64
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCAddr:             getEnv(EnvGRPCAddr, DefaultGRPCAddr),
		GRPCTLSCertPath:      strings.TrimSpace(getEnv(EnvGRPCTLSCertPath, DefaultGRPCTLSCertPath)),
		GRPCTLSKeyPath:       strings.TrimSpace(getEnv(EnvGRPCTLSKeyPath, DefaultGRPCTLSKeyPath)),
		LogLevel:             strings.ToLower(getEnv(EnvLogLevel, DefaultLogLevel)),
		DatabaseURL:          strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		MinioEndpoint:        strings.TrimSpace(getEnv(EnvMinioEndpoint, DefaultMinioEndpoint)),
		MinioAccessKey:       strings.TrimSpace(os.Getenv(EnvMinioAccessKey)),
		MinioSecretKey:       strings.TrimSpace(os.Getenv(EnvMinioSecretKey)),
		MinioBucket:          getEnv(EnvMinioBucket, DefaultMinioBucket),
		JWTSecret:            strings.TrimSpace(os.Getenv(EnvJWTSecret)),
		AccessTTLSec:         DefaultAccessTTLSec,
		RefreshTTLSec:        DefaultRefreshTTLSec,
		InlineThresholdBytes: DefaultInlineThresholdBytes,
		MaxBlobSizeBytes:     DefaultMaxBlobSizeBytes,
		MaxChunkSizeBytes:    DefaultMaxChunkSizeBytes,
		GRPCMaxMessageBytes:  DefaultGRPCMaxMessageBytes,
	}
	useSSL, err := parseBool(getEnv(EnvMinioUseSSL, DefaultMinioUseSSL))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", EnvMinioUseSSL, err)
	}
	cfg.MinioUseSSL = useSSL

	switch cfg.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
	default:
		return nil, fmt.Errorf("%s must be one of %s, %s, %s, %s, got %q",
			EnvLogLevel, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, cfg.LogLevel)
	}

	if cfg.GRPCTLSCertPath == "" {
		return nil, fmt.Errorf("%s must not be empty", EnvGRPCTLSCertPath)
	}
	if cfg.GRPCTLSKeyPath == "" {
		return nil, fmt.Errorf("%s must not be empty", EnvGRPCTLSKeyPath)
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("%s is required", EnvDatabaseURL)
	}
	if cfg.MinioEndpoint == "" {
		return nil, fmt.Errorf("%s is required", EnvMinioEndpoint)
	}
	if cfg.MinioAccessKey == "" || cfg.MinioSecretKey == "" {
		return nil, fmt.Errorf("%s and %s are required", EnvMinioAccessKey, EnvMinioSecretKey)
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("%s is required", EnvJWTSecret)
	}
	if len(cfg.JWTSecret) < MinJWTSecretLen {
		return nil, fmt.Errorf("%s must be at least %d bytes", EnvJWTSecret, MinJWTSecretLen)
	}
	if v := strings.TrimSpace(os.Getenv(EnvAccessTTLSec)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvAccessTTLSec)
		}
		cfg.AccessTTLSec = n
	}
	if v := strings.TrimSpace(os.Getenv(EnvRefreshTTLSec)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvRefreshTTLSec)
		}
		cfg.RefreshTTLSec = n
	}
	if v := strings.TrimSpace(os.Getenv(EnvInlineThresholdBytes)); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvInlineThresholdBytes)
		}
		cfg.InlineThresholdBytes = n
	}
	if v := strings.TrimSpace(os.Getenv(EnvMaxBlobSizeBytes)); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvMaxBlobSizeBytes)
		}
		cfg.MaxBlobSizeBytes = n
	}
	if v := strings.TrimSpace(os.Getenv(EnvMaxChunkSizeBytes)); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvMaxChunkSizeBytes)
		}
		cfg.MaxChunkSizeBytes = n
	}
	if v := strings.TrimSpace(os.Getenv(EnvGRPCMaxMessageBytes)); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("%s must be a positive integer", EnvGRPCMaxMessageBytes)
		}
		cfg.GRPCMaxMessageBytes = n
	}
	if cfg.GRPCMaxMessageBytes > uint64(math.MaxInt) {
		return nil, fmt.Errorf("%s exceeds maximum supported on this platform", EnvGRPCMaxMessageBytes)
	}
	if cfg.InlineThresholdBytes > cfg.MaxBlobSizeBytes {
		return nil, fmt.Errorf("%s must not exceed %s", EnvInlineThresholdBytes, EnvMaxBlobSizeBytes)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", s)
	}
}
