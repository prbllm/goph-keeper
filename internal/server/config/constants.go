package config

const (
	EnvGRPCAddr              = "GOPHKEEPER_GRPC_ADDR"
	EnvGRPCTLSCertPath       = "GOPHKEEPER_GRPC_TLS_CERT_PATH"
	EnvGRPCTLSKeyPath        = "GOPHKEEPER_GRPC_TLS_KEY_PATH"
	EnvLogLevel              = "GOPHKEEPER_LOG_LEVEL"
	EnvDatabaseURL           = "GOPHKEEPER_DATABASE_URL"
	EnvMinioEndpoint         = "GOPHKEEPER_MINIO_ENDPOINT"
	EnvMinioAccessKey        = "GOPHKEEPER_MINIO_ACCESS_KEY"
	EnvMinioSecretKey        = "GOPHKEEPER_MINIO_SECRET_KEY"
	EnvMinioBucket           = "GOPHKEEPER_MINIO_BUCKET"
	EnvMinioUseSSL           = "GOPHKEEPER_MINIO_USE_SSL"
	EnvJWTSecret             = "GOPHKEEPER_JWT_SECRET"
	EnvAccessTTLSec          = "GOPHKEEPER_ACCESS_TOKEN_TTL_SECONDS"
	EnvRefreshTTLSec         = "GOPHKEEPER_REFRESH_TOKEN_TTL_SECONDS"
	EnvInlineThresholdBytes  = "GOPHKEEPER_INLINE_THRESHOLD_BYTES"
	EnvMaxBlobSizeBytes      = "GOPHKEEPER_MAX_BLOB_SIZE_BYTES"
	EnvMaxChunkSizeBytes     = "GOPHKEEPER_MAX_CHUNK_SIZE_BYTES"
	EnvGRPCMaxMessageBytes   = "GOPHKEEPER_GRPC_MAX_MESSAGE_BYTES"
	EnvUploadSessionTTLHours = "GOPHKEEPER_UPLOAD_SESSION_TTL_HOURS"
)

const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

const (
	DefaultGRPCAddr              = "127.0.0.1:50051"
	DefaultGRPCTLSCertPath       = "certs/server.crt"
	DefaultGRPCTLSKeyPath        = "certs/server.key"
	DefaultLogLevel              = LogLevelInfo
	DefaultMinioEndpoint         = "127.0.0.1:9000"
	DefaultMinioBucket           = "gophkeeper"
	DefaultMinioUseSSL           = "false"
	DefaultAccessTTLSec          = 900
	DefaultRefreshTTLSec         = 2592000
	DefaultInlineThresholdBytes  = 256 * 1024 // 256 KiB
	DefaultMaxBlobSizeBytes      = 104857600  // 100 MiB
	DefaultMaxChunkSizeBytes     = 8388608    // 8 MiB
	DefaultGRPCMaxMessageBytes   = 104857600  // 100 MiB
	DefaultUploadSessionTTLHours = 24
	DefaultSyncPullPageSize      = 100
	MaxSyncPullPageSize          = 500
	MaxSyncBundleRevisionEvents  = 500
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

const (
	JWTClaimSubject  = "sub"
	JWTClaimDeviceID = "device_id"
	JWTClaimIssuedAt = "iat"
	JWTClaimExpiry   = "exp"
)

// MinJWTSecretLen is the minimum accepted length for HS256 signing keys.
const MinJWTSecretLen = 32

// VaultGRPCProtoReserveBytes is reserved when computing max vault payload ciphertext from GRPCMaxMessageBytes
// (title, metadata, protobuf framing).
const VaultGRPCProtoReserveBytes = 512 * 1024
