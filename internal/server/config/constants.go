package config

const (
	EnvGRPCAddr       = "GOPHKEEPER_GRPC_ADDR"
	EnvLogLevel       = "GOPHKEEPER_LOG_LEVEL"
	EnvDatabaseURL    = "GOPHKEEPER_DATABASE_URL"
	EnvMinioEndpoint  = "GOPHKEEPER_MINIO_ENDPOINT"
	EnvMinioAccessKey = "GOPHKEEPER_MINIO_ACCESS_KEY"
	EnvMinioSecretKey = "GOPHKEEPER_MINIO_SECRET_KEY"
	EnvMinioBucket    = "GOPHKEEPER_MINIO_BUCKET"
	EnvMinioUseSSL    = "GOPHKEEPER_MINIO_USE_SSL"
)

const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

const (
	DefaultGRPCAddr      = "127.0.0.1:50051"
	DefaultLogLevel      = LogLevelInfo
	DefaultMinioEndpoint = "127.0.0.1:9000"
	DefaultMinioBucket   = "gophkeeper"
	DefaultMinioUseSSL   = "false"
)

const (
	PostgresMaxOpenConns       = 25
	PostgresMaxIdleConns       = 5
	PostgresConnMaxLifetimeMin = 30
)

const (
	PostgresPingTimeoutSeconds = 10
	MinIOConnectTimeoutSeconds = 15
	GRPCShutdownTimeoutSeconds = 10
)
