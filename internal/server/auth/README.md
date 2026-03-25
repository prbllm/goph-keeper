# Auth: аутентификация и авторизация

Пакет `internal/server/auth` реализует серверный auth-слой для gRPC:

- аутентификация пользователя по `login + password`;
- выдача access JWT;
- ротация и отзыв refresh-токенов;
- привязка сессий к устройству (`device_id`).

## Что делает сервис

Основной use-case слой (`Service`) поддерживает операции:

- `Register` — регистрация пользователя, создание устройства и refresh-сессии;
- `Login` — проверка пароля, upsert устройства и выдача новой пары токенов;
- `Refresh` — ротация refresh-токена и выпуск нового access JWT;
- `Logout` — отзыв refresh-токена по hash.

Если `device_id` не передан в `Register`/`Login`, он генерируется автоматически.

## Модель токенов

- **Access token**: JWT (HS256), содержит `sub` (user id) и `device_id`.
- **Refresh token**: opaque token; в базе хранится только hash (`HashToken`), plaintext не сохраняется.
- Refresh-токены одноразовые: после успешного `Refresh` старый токен становится невалидным.

## Ошибки и поведение

- `ErrInvalidArgument` — некорректные входные данные.
- `ErrUnauthorized` — неверные креды/токен, просрочка, mismatch `device_id` и т.п.
- `ErrAlreadyExists` — конфликт при регистрации (например, логин уже занят).

`Logout` идемпотентен только для случая, когда репозиторий вернул `ErrUnauthorized`
(токен не найден или уже отозван). Другие ошибки хранилища не подавляются и возвращаются вызывающему коду.

## Конфигурация (env)

Ключевые переменные:

- `GOPHKEEPER_JWT_SECRET` (обязательно, минимум 32 байта)
- `GOPHKEEPER_ACCESS_TOKEN_TTL_SECONDS` (по умолчанию `900`)
- `GOPHKEEPER_REFRESH_TOKEN_TTL_SECONDS` (по умолчанию `2592000`)
- `GOPHKEEPER_INLINE_THRESHOLD_BYTES`
- `GOPHKEEPER_MAX_BLOB_SIZE_BYTES`
- `GOPHKEEPER_MAX_CHUNK_SIZE_BYTES`

Значения лимитов возвращаются клиенту в `Limits` после успешной аутентификации.
