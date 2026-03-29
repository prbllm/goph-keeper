-- Таблица отложенных операций синхронизации
CREATE TABLE IF NOT EXISTS pending_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL UNIQUE,
    operation_type INTEGER NOT NULL,
    item_id TEXT NOT NULL,
    expected_version INTEGER NOT NULL,
    snapshot_type INTEGER,
    snapshot_title_cipher BLOB,
    snapshot_title_nonce BLOB,
    snapshot_metadata_cipher BLOB,
    snapshot_metadata_nonce BLOB,
    snapshot_payload_cipher BLOB,
    snapshot_payload_nonce BLOB,
    snapshot_blob_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для производительности
CREATE INDEX IF NOT EXISTS idx_pending_operations_operation_id ON pending_operations(operation_id);
CREATE INDEX IF NOT EXISTS idx_pending_operations_item_id ON pending_operations(item_id);