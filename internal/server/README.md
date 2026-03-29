# Серверное ядро (`internal/server`)

Здесь собран runtime GophKeeper server: конфигурация, миграции, доступ к Postgres и MinIO, доменные пакеты (`auth`, `vault`, `blob`), фоновые задачи и слой gRPC.

## Структура (ключевые каталоги)

| Каталог | Назначение |
| --- | --- |
| [`app`](app/run.go) | Сборка зависимостей и запуск слушателя gRPC, graceful shutdown |
| [`config`](config/README.md) | Загрузка `Config` из env, именованные константы |
| [`grpcserver`](grpcserver/README.md) | Регистрация сервисов, TLS, unary/stream interceptors, handlers |
| [`auth`](auth/README.md) | Регистрация, логин, refresh, logout, JWT |
| [`vault`](vault/README.md) | Единый движок мутаций vault + revision log + идемпотентность sync |
| [`blob`](blob/storage.go) | Интерфейсы хранилища для зашифрованных объектов |
| [`storage/postgres`](storage/postgres/README.md) | Репозитории и транзакции |
| [`storage/s3minio`](storage/s3minio/object_storage.go) | Реализация `blob.ObjectStorage` для MinIO |
| [`migrations`](migrations/migrate.go) | Применение встроенных SQL-миграций при старте |
| [`jobs`](jobs/upload_session_cleanup.go) | Фоновая очистка upload sessions |

## Документация

- [docs/server](../../docs/server/README.md) — оглавление документации по серверу  
- [internal/server/config/README.md](config/README.md) — полный список env и констант  
