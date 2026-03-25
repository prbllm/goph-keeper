# goph-keeper

## Сервер

### TLS и сертификаты

gRPC принимается по TLS. Пути к сертификату и закрытому ключу задаются переменными `GOPHKEEPER_GRPC_TLS_CERT_PATH` и `GOPHKEEPER_GRPC_TLS_KEY_PATH`, по умолчанию — `certs/server.crt` и `certs/server.key` относительно текущей рабочей директории процесса.

Из корня репозитория сгенерируйте сертификаты и ключи:

```bash
scripts/generate_certs.sh
```

Скрипт: [scripts/generate_certs.sh](scripts/generate_certs.sh).

После выполнения (относительно текущей рабочей директории процесса):

- [certs/server.crt](certs/server.crt) — сертификат сервера  
- [certs/server.key](certs/server.key) — закрытый ключ сервера
- [certs/ca.crt](certs/ca.crt) — CA для доверия на **клиенте**  
- [certs/ca.key](certs/ca.key) — ключ CA

Клиент должен доверять CA из `certs/ca.crt` и подключаться к имени из SAN сертификата (`localhost` и `127.0.0.1`).

### Переменные окружения

**Обязательные**

| Переменная | Назначение |
| --- | --- |
| `GOPHKEEPER_DATABASE_URL` | DSN PostgreSQL, напр. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `GOPHKEEPER_MINIO_ACCESS_KEY` | Access key MinIO |
| `GOPHKEEPER_MINIO_SECRET_KEY` | Secret key MinIO |
| `GOPHKEEPER_JWT_SECRET` | Секрет подписи JWT, **не короче 32 символов** |

**Необязательные**

| Переменная | Назначение |
| --- | --- |
| `GOPHKEEPER_GRPC_ADDR` | Адрес прослушивания (`127.0.0.1:50051`) |
| `GOPHKEEPER_GRPC_TLS_CERT_PATH` | Путь к PEM сертификата сервера (`certs/server.crt`) |
| `GOPHKEEPER_GRPC_TLS_KEY_PATH` | Путь к PEM закрытому ключу сервера (`certs/server.key`) |
| `GOPHKEEPER_LOG_LEVEL` | `debug`, `info`, `warn` или `error` (`info`) |
| `GOPHKEEPER_MINIO_ENDPOINT` | Хост:порт MinIO (`127.0.0.1:9000`) |
| `GOPHKEEPER_MINIO_BUCKET` | Имя бакета (`gophkeeper`) |
| `GOPHKEEPER_MINIO_USE_SSL` | `true` / `false` (`false`) |
| `GOPHKEEPER_ACCESS_TOKEN_TTL_SECONDS` | TTL access-токена, сек (`900`) |
| `GOPHKEEPER_REFRESH_TOKEN_TTL_SECONDS` | TTL refresh-токена, сек (`2592000`) |
| `GOPHKEEPER_INLINE_THRESHOLD_BYTES` | Порог inline для vault/blob (`65536`) |
| `GOPHKEEPER_MAX_BLOB_SIZE_BYTES` | Макс. размер blob (`104857600`) |
| `GOPHKEEPER_MAX_CHUNK_SIZE_BYTES` | Макс. размер chunk (`8388608`) |

Шаблон окружения: [.env.example](.env.example) — скопируйте в `.env` и заполните.

### Локальный запуск через Docker Compose

Сборка образа: [build/server/Dockerfile](build/server/Dockerfile). Оркестрация: [docker-compose.yml](docker-compose.yml) (Postgres, MinIO, сервер).

```bash
bash scripts/generate_certs.sh
cp .env.example .env
# задайте GOPHKEEPER_JWT_SECRET и при необходимости остальное
docker compose up --build
```

У сервиса `server` каталог `./certs` смонтирован в `/certs`; в [docker-compose.yml](docker-compose.yml) по умолчанию заданы `GOPHKEEPER_GRPC_TLS_CERT_PATH=/certs/server.crt` и `GOPHKEEPER_GRPC_TLS_KEY_PATH=/certs/server.key`.
