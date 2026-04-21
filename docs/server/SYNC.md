# Sync: push и pull

Реализация: [`internal/server/grpcserver/sync_handler.go`](../../internal/server/grpcserver/sync_handler.go), движок — [`internal/server/vault`](../../internal/server/vault/README.md).

## Push (`PushChanges` / часть `Sync`)

Каждая операция клиента выполняется в **одной SQL-транзакции** вместе с записью в `revision_log` и `processed_operations`:

- **Идемпотентность** — ключ `(user_id, operation_id)`. Повтор с тем же `operation_id` возвращает уже применённый результат без повторной мутации.  
- **Конфликты версий** — оптимистичная блокировка; при расхождении версии клиент получает описание конфликта (см. proto).  
- **Payload** — тот же путь, что и в Vault RPC: inline до порога из `GOPHKEEPER_INLINE_THRESHOLD_BYTES`, иначе blob в MinIO (см. `resolveVaultPayload`).

## Pull

Выборка изменений после `since_revision` из `revision_log` с пагинацией (`page_token`). Размеры страниц ограничены константами пакета `config` (`DefaultSyncPullPageSize`, `MaxSyncPullPageSize`, `MaxSyncBundleRevisionEvents`).

## Связь с Vault

Все мутации из sync проходят через **`VaultEngine`**, чтобы бизнес-правила не расходились с прямым `VaultService`.
