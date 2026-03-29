# Vault engine (`internal/server/vault`)

Единый **`EngineService`** обслуживает и прямые RPC-мутации, и операции sync push: создание/обновление/удаление элементов с **оптимистичной конкуренцией по версии**, запись в **`revision_log`**, идемпотентность по **`processed_operations`** (`user_id` + `operation_id`).

## Поведение

- Удаление — **tombstone** (`deleted_at`), без физического удаления строки.  
- Повтор push с тем же `operation_id` возвращает уже зафиксированный результат без двойной мутации.  
- Логирование аномалий (несовпадение `item_id` при idempotent lookup, отсутствие item) — через `zap`, логгер передаётся в `NewEngine`.

## Транзакции

В sync-handlers движок создаётся внутри одной SQL-транзакции — см. `postgres.WithVaultEngineTx` в [`storage/postgres/vault_engine_tx.go`](../storage/postgres/vault_engine_tx.go).

## См. также

- [internal/server/storage/postgres/README.md](../storage/postgres/README.md)  
- [docs/server/SYNC.md](../../docs/server/SYNC.md)  
