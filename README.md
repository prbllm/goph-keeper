# goph-keeper

Хранилище секретов GophKeeper.

## Оглавление репозитория

| Раздел | Описание |
| --- | --- |
| [docs/server](docs/server/README.md) | Документация сервера |
| [cmd/server](cmd/server/README.md) | Точка входа сервера |
| [cmd/client](cmd/client/README.md) | Точка входа CLI-клиента |
| [internal/server](internal/server/README.md) | Ядро сервера |
| [internal/server/config](internal/server/config/README.md) | Переменные окружения и именованные константы |
| [internal/client](internal/client/README.md) | Ядро CLI-клиента |
| [internal/client/transport](internal/client/transport/README.md) | gRPC-транспорт клиента (TLS, JWT) |
| [api/proto](api/proto/gophkeeper/v1/gophkeeper.proto) | Контракт gRPC |
| [migrations/server](migrations/server/) | Схема PostgreSQL |
| [build/server/Dockerfile](build/server/Dockerfile) | Docker-образ сервера |
| [docker-compose.yml](docker-compose.yml) | Локальный запуск сервера |

---

## Сервер

### TLS и сертификаты

gRPC принимается по TLS. Пути к сертификату и ключу — `GOPHKEEPER_GRPC_TLS_CERT_PATH` / `GOPHKEEPER_GRPC_TLS_KEY_PATH`, по умолчанию `certs/server.crt` и `certs/server.key` относительно рабочей директории процесса.

```bash
scripts/generate_certs.sh
```

Подробности и сценарий доверия CA на клиенте: [docs/server/OPERATIONS.md](docs/server/OPERATIONS.md).

### Переменные окружения

**Обязательные:** `GOPHKEEPER_DATABASE_URL`, `GOPHKEEPER_MINIO_ACCESS_KEY`, `GOPHKEEPER_MINIO_SECRET_KEY`, `GOPHKEEPER_JWT_SECRET` (не короче 32 символов).

**Часто используемые необязательные:** `GOPHKEEPER_GRPC_ADDR`, пути TLS, уровень логов, параметры MinIO, TTL токенов, лимиты vault/blob/chunk/gRPC, TTL и интервал cleanup upload sessions.

Полная таблица: [internal/server/config/README.md](internal/server/config/README.md). Шаблон: [.env.example](.env.example).

### Локальный запуск через Docker Compose

```bash
bash scripts/generate_certs.sh
cp .env.example .env
# задайте GOPHKEEPER_JWT_SECRET и при необходимости остальное
docker compose up
```

У сервиса `server` каталог `./certs` смонтирован в `/certs`; в `docker-compose.yml` по умолчанию заданы `GOPHKEEPER_GRPC_TLS_CERT_PATH=/certs/server.crt` и `GOPHKEEPER_GRPC_TLS_KEY_PATH=/certs/server.key`.

### Интеграционные e2e-тесты

```bash
go test -tags=integration -count=1 -timeout=600s ./internal/integration/... -v
```

Нужен запущенный Docker для testcontainers. См. [docs/server/OPERATIONS.md](docs/server/OPERATIONS.md).

---

## Клиент

Точка входа CLI в [`cmd/client`](cmd/client/main.go); обзор пакетов — [internal/client/README.md](internal/client/README.md), транспорт — [internal/client/transport/README.md](internal/client/transport/README.md).

### TLS и доверие к серверу

Сервер принимает только TLS. Укажите путь к CA, которой подписан сертификат сервера (после `scripts/generate_certs.sh` это обычно `certs/ca.crt` из корня репозитория):

```bash
go run ./cmd/client --tls-ca certs/ca.crt -s 127.0.0.1:50051
```

Флаг `--tls-ca` по умолчанию в коде — `cert.pem`; для локальной разработки задайте явно путь к `ca.crt`. Скрытый флаг `--insecure` отключает TLS (только для отладки, не к продакшену).

### Запуск

Из корня репозитория:

```bash
go run ./cmd/client --help
```

Данные CLI по умолчанию хранятся в `~/.gophkeeper` (флаг `--data-dir` / `-d`).

### Глобальные флаги

| Флаг | По умолчанию | Назначение |
| --- | --- | --- |
| `-s` / `--server` | `localhost:50051` | Адрес gRPC сервера |
| `-d` / `--data-dir` | `~/.gophkeeper` | Каталог локальных данных |
| `--tls-ca` | `cert.pem` | Путь к PEM-файлу CA для проверки сертификата сервера |

### Лимиты gRPC

Размеры `MaxCallSendMsgSize` / `MaxCallRecvMsgSize` на клиенте должны быть **не меньше** `GOPHKEEPER_GRPC_MAX_MESSAGE_BYTES` на сервере — иначе возможен `RESOURCE_EXHAUSTED` на крупных сообщениях. См. [.env.example](.env.example) и [internal/server/config/README.md](internal/server/config/README.md).
