# Аутентификация и сессии

Серверный слой реализован в [`internal/server/auth`](../../internal/server/auth/README.md) и выставляется наружу как gRPC `AuthService`.

## Потоки

1. **Register** — создание пользователя, устройства и refresh-сессии; выдача пары access + refresh.  
2. **Login** — проверка пароля, при необходимости обновление записи устройства, новая пара токенов.  
3. **Refresh** — одноразовая ротация refresh: старый токен инвалидируется, выдаётся новая пара.  
4. **Logout** — отзыв refresh по хэшу.

Если `device_id` не передан при register/login, он генерируется на сервере.

## Токены

- **Access** — JWT (HS256), claims: `sub` (user id), `device_id`, стандартные `iat` / `exp`.  
- **Refresh** — opaque строка; в БД хранится **только хэш**, plaintext не сохраняется и не логируется.

## Переменные окружения

Обязательно: `GOPHKEEPER_JWT_SECRET` (не короче 32 символов).

Опционально: `GOPHKEEPER_ACCESS_TOKEN_TTL_SECONDS` (по умолчанию `900`), `GOPHKEEPER_REFRESH_TOKEN_TTL_SECONDS` (по умолчанию `2592000`).

Лимиты vault/blob после успешной аутентификации отдаются клиенту в ответе (см. proto `Limits`). Полный список env — [`internal/server/config/README.md`](../../internal/server/config/README.md).

## Ошибки (доменный слой)

- Некорректный ввод → `ErrInvalidArgument`.  
- Неверные креды / просрочка / несовпадение устройства → `ErrUnauthorized`.  
- Конфликт при регистрации (занятый логин) → `ErrAlreadyExists`.

Подробнее о поведении `Logout` и маппинге на gRPC — в README пакета `auth`.
