# Эксплуатация: TLS, окружение, тесты

## TLS

gRPC принимается **только с TLS**. Пути к сертификату и ключу задаются переменными `GOPHKEEPER_GRPC_TLS_CERT_PATH` и `GOPHKEEPER_GRPC_TLS_KEY_PATH`; по умолчанию — `certs/server.crt` и `certs/server.key` относительно рабочей директории процесса. При отсутствии файлов сервер завершает работу на старте.

Генерация локальных сертификатов из корня репозитория:

```bash
scripts/generate_certs.sh
```

Клиент должен доверять CA из `certs/ca.crt` и использовать имя из SAN сертификата (`localhost`, `127.0.0.1` — см. скрипт).

## Переменные окружения

Полная таблица: [`internal/server/config/README.md`](../../internal/server/config/README.md). Шаблон: [`.env.example`](../../.env.example).

## Docker Compose

```bash
bash scripts/generate_certs.sh
cp .env.example .env
# задайте GOPHKEEPER_JWT_SECRET и при необходимости остальное
docker compose up
```

Образ: [`build/server/Dockerfile`](../../build/server/Dockerfile), оркестрация: [`docker-compose.yml`](../../docker-compose.yml). У сервиса `server` каталог `./certs` обычно монтируется в `/certs`; пути TLS в compose должны указывать на смонтированные файлы.

## Интеграционные e2e (testcontainers)

Требуются Docker и тег `integration`:

```bash
go test -tags=integration -count=1 -timeout=600s ./internal/integration/... -v
```

Код: [`internal/integration`](../../internal/integration).

## Миграции

SQL лежит в [`migrations/server`](../../migrations/server). При старте сервера миграции применяются автоматически (`internal/server/migrations`).
