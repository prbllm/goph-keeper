# Документация сервера GophKeeper

Сервер — gRPC **только по TLS**, Postgres, MinIO, единый vault engine, фоновая очистка upload sessions.

## Оглавление


| Документ                       | Содержание                                              |
| ------------------------------ | ------------------------------------------------------- |
| [AUTH.md](AUTH.md)             | Аутентификация, токены, связь с env                     |
| [SYNC.md](SYNC.md)             | Push/pull, идемпотентность, ревизии                     |
| [OPERATIONS.md](OPERATIONS.md) | TLS, Docker, переменные окружения, интеграционные тесты |


## Быстрые ссылки на код

- Точка входа: [cmd/server](../../cmd/server/README.md)
- Конфиг и константы: [internal/server/config](../../internal/server/config/README.md)
- gRPC: [internal/server/grpcserver](../../internal/server/grpcserver/README.md)
- Схема БД: [migrations/server](../../migrations/server/)

