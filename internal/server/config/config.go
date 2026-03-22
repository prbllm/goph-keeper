package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds server settings loaded from the environment.
type Config struct {
	GRPCAddr       string
	LogLevel       string // debug | info | warn | error
	Environment    string
	DatabaseURL    string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCAddr:       getEnv(EnvGRPCAddr, DefaultGRPCAddr),
		LogLevel:       strings.ToLower(getEnv(EnvLogLevel, DefaultLogLevel)),
		Environment:    strings.ToLower(getEnv(EnvEnvironment, DefaultEnvironment)),
		DatabaseURL:    strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		MinioEndpoint:  strings.TrimSpace(getEnv(EnvMinioEndpoint, DefaultMinioEndpoint)),
		MinioAccessKey: strings.TrimSpace(os.Getenv(EnvMinioAccessKey)),
		MinioSecretKey: strings.TrimSpace(os.Getenv(EnvMinioSecretKey)),
		MinioBucket:    getEnv(EnvMinioBucket, DefaultMinioBucket),
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

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("%s is required", EnvDatabaseURL)
	}
	if cfg.MinioEndpoint == "" {
		return nil, fmt.Errorf("%s is required", EnvMinioEndpoint)
	}
	if cfg.MinioAccessKey == "" || cfg.MinioSecretKey == "" {
		return nil, fmt.Errorf("%s and %s are required", EnvMinioAccessKey, EnvMinioSecretKey)
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
