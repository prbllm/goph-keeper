## Postgres storage: слой доступа к БД

Пакет `internal/server/storage/postgres` реализует доступ к PostgreSQL для серверных доменов.

- управление пулом соединений (`Pool` / `Connector`);
- CRUD‑операции для доменных сущностей (auth, vault и др.);

### Интерфейсы

- `Pool` — обёртка над `*sql.DB` с методами `PingContext`, `ExecContext`, `QueryRowContext`, `BeginTx`;
- `Connector` — отвечает за открытие пула `OpenPing(ctx, databaseURL)` с проверкой доступности БД;

### VaultRepository

`VaultRepository` обслуживает таблицу `vault_items` и связанную с ней логику:

- `CreateItem` — вставка новой записи с уже сгенерированными `item_id` и версией;
- `GetItem` — чтение одной записи пользователя по `user_id` и `item_id`, маппинг `NULL`‑полей на `*string` / `*time.Time`;
- `ListItems` — пагинация по `(updated_at, item_id)` с поддержкой soft‑delete (`deleted_at`) и `pageToken`;
- `UpdateItem` — оптимистичная блокировка по версии, при конфликте возвращает `vault.ErrConflict`;
- `DeleteItem` — tombstone‑удаление: выставляет `deleted_at`, инкрементирует `version`, возвращает новую версию.

### RevisionLogRepository и ProcessedOperationsRepository

- `RevisionLogRepository` пишет события в `revision_log` и заполняет `RevisionID` / `ChangedAt` через `RETURNING`;
- `ProcessedOperationsRepository` обслуживает таблицу `processed_operations` и даёт идемпотентность по `(user_id, operation_id)`:
  - `MarkProcessed` вставляет запись, игнорируя конфликты по уникальному ключу;
  - `Lookup` возвращает последнюю запись или `nil`, если операция ещё не обрабатывалась.

### Инварианты и ошибки

- все запросы принимают `context.Context` и используют `*Context`‑методы из `database/sql`;
- отсутствие строки трактуется как доменная ошибка (`vault.ErrNotFound` / `vault.ErrConflict`), а не как `sql.ErrNoRows`;
- репозитории не создают схемы — любые изменения структуры БД делаются только через миграции.

