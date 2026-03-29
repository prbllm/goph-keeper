# Конфигурация сервера (`internal/server/config`)

Пакет загружает [`Config`](config.go) из переменных окружения через `Load()`. Имена ключей env, значения по умолчанию и внутренние константы заданы в [`constants.go`](constants.go).

## Переменные окружения

| Переменная | Обязательная | Значение по умолчанию | Назначение |
| --- | --- | --- | --- |
| `GOPHKEEPER_DATABASE_URL` | да | — | DSN PostgreSQL, напр. `postgres://user:pass@host:5432/db?sslmode=disable`. |
| `GOPHKEEPER_MINIO_ACCESS_KEY` | да | — | Access key MinIO. |
| `GOPHKEEPER_MINIO_SECRET_KEY` | да | — | Secret key MinIO. |
| `GOPHKEEPER_JWT_SECRET` | да | — | Секрет HMAC для access JWT; минимальная длина — `MinJWTSecretLen` (32 байта). |
| `GOPHKEEPER_GRPC_ADDR` | нет | `127.0.0.1:50051` | Адрес прослушивания gRPC. |
| `GOPHKEEPER_GRPC_TLS_CERT_PATH` | нет | `certs/server.crt` | PEM сертификат сервера. |
| `GOPHKEEPER_GRPC_TLS_KEY_PATH` | нет | `certs/server.key` | PEM закрытый ключ сервера. |
| `GOPHKEEPER_LOG_LEVEL` | нет | `info` | Один из `debug`, `info`, `warn`, `error`. |
| `GOPHKEEPER_MINIO_ENDPOINT` | нет | `127.0.0.1:9000` | Хост:порт MinIO. |
| `GOPHKEEPER_MINIO_BUCKET` | нет | `gophkeeper` | Имя бакета (создаётся при отсутствии). |
| `GOPHKEEPER_MINIO_USE_SSL` | нет | `false` | `true` / `false` / `1` / `0` / `yes` / `no`. |
| `GOPHKEEPER_ACCESS_TOKEN_TTL_SECONDS` | нет | `900` | TTL access JWT (секунды). |
| `GOPHKEEPER_REFRESH_TOKEN_TTL_SECONDS` | нет | `2592000` | TTL refresh (~30 суток). |
| `GOPHKEEPER_INLINE_THRESHOLD_BYTES` | нет | `262144` (256 KiB) | Максимальный размер шифротекста payload, хранимого inline в `vault_items`; больше — объект в MinIO. |
| `GOPHKEEPER_MAX_BLOB_SIZE_BYTES` | нет | `104857600` (100 MiB) | Верхняя граница размера blob / vault payload. |
| `GOPHKEEPER_MAX_CHUNK_SIZE_BYTES` | нет | `8388608` (8 MiB) | Максимальный размер одного чанка `UploadBlob`. |
| `GOPHKEEPER_GRPC_MAX_MESSAGE_BYTES` | нет | `104857600` (100 MiB) | Лимиты `MaxRecvMsgSize` / `MaxSendMsgSize` на сервере; у клиента вызовы должны быть не меньше этого значения. |
| `GOPHKEEPER_UPLOAD_SESSION_TTL_HOURS` | нет | `24` | Срок жизни сессии загрузки blob. |
| `GOPHKEEPER_UPLOAD_SESSION_CLEANUP_INTERVAL_MINUTES` | нет | `15` | Интервал фоновой очистки просроченных upload sessions; `0` отключает задачу. |

Проверки при `Load()`:

- пути к TLS cert/key не пустые;
- `GOPHKEEPER_INLINE_THRESHOLD_BYTES` не больше `GOPHKEEPER_MAX_BLOB_SIZE_BYTES`.

## Константы

Задают пулы, таймауты, имена JWT-claims и лимиты sync — см. идентификаторы в `constants.go`.

| Имя | Значение | Роль |
| --- | --- | --- |
| `EnvGRPCAddr`, … | строки | Имена переменных окружения (дублируют таблицу выше). |
| `LogLevelDebug`, `LogLevelInfo`, `LogLevelWarn`, `LogLevelError` | строки | Допустимые уровни логирования. |
| `DefaultGRPCAddr` | `127.0.0.1:50051` | Адрес gRPC по умолчанию. |
| `DefaultGRPCTLSCertPath` | `certs/server.crt` | Путь к сертификату по умолчанию. |
| `DefaultGRPCTLSKeyPath` | `certs/server.key` | Путь к ключу по умолчанию. |
| `DefaultLogLevel` | `info` | Уровень логов по умолчанию. |
| `DefaultMinioEndpoint` | `127.0.0.1:9000` | Endpoint MinIO. |
| `DefaultMinioBucket` | `gophkeeper` | Бакет по умолчанию. |
| `DefaultMinioUseSSL` | `false` | SSL к MinIO по умолчанию. |
| `DefaultAccessTTLSec` | `900` | TTL access (сек). |
| `DefaultRefreshTTLSec` | `2592000` | TTL refresh (сек). |
| `DefaultInlineThresholdBytes` | `256 * 1024` | 256 KiB, порог inline. |
| `DefaultMaxBlobSizeBytes` | `104857600` | 100 MiB. |
| `DefaultMaxChunkSizeBytes` | `8388608` | 8 MiB. |
| `DefaultGRPCMaxMessageBytes` | `104857600` | 100 MiB gRPC. |
| `DefaultUploadSessionTTLHours` | `24` | TTL upload session (часы). |
| `DefaultUploadSessionCleanupIntervalMinutes` | `15` | Интервал cleanup (минуты). |
| `DefaultSyncPullPageSize` | `100` | Размер страницы pull по умолчанию. |
| `MaxSyncPullPageSize` | `500` | Верхняя граница размера страницы pull. |
| `MaxSyncBundleRevisionEvents` | `500` | Лимит событий ревизий в одном ответе sync. |
| `PostgresMaxOpenConns` | `25` | Размер пула соединений Postgres. |
| `PostgresMaxIdleConns` | `5` | Idle-соединения в пуле. |
| `PostgresConnMaxLifetimeMin` | `30` | Максимальное время жизни соединения (минуты). |
| `PostgresPingTimeoutSeconds` | `10` | Таймаут `Ping` при старте. |
| `MinIOConnectTimeoutSeconds` | `15` | Таймаут подключения к MinIO. |
| `GRPCShutdownTimeoutSeconds` | `10` | Таймаут graceful shutdown gRPC. |
| `UploadSessionCleanupRunTimeoutSeconds` | `120` | Таймаут одного прохода cleanup (SQL/хранилище). |
| `UploadSessionCleanupListBatchSize` | `100` | Размер батча при выборке сессий на удаление. |
| `JWTClaimSubject` | `sub` | Claim user id в JWT. |
| `JWTClaimDeviceID` | `device_id` | Claim устройства. |
| `JWTClaimIssuedAt` | `iat` | Claim времени выпуска. |
| `JWTClaimExpiry` | `exp` | Claim истечения. |
| `MinJWTSecretLen` | `32` | Минимальная длина `GOPHKEEPER_JWT_SECRET`. |
| `VaultGRPCProtoReserveBytes` | `512 * 1024` | Запас под protobuf/метаданные при расчёте макс. ciphertext vault от `GRPCMaxMessageBytes`. |

Точные выражения в коде — в [`constants.go`](constants.go) (в т.ч. `512 * 1024` для `VaultGRPCProtoReserveBytes`).
