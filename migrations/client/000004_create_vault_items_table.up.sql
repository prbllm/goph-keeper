-- Таблица элементов хранилища
CREATE TABLE IF NOT EXISTS vault_items (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL DEFAULT 1,
    item_type INTEGER NOT NULL,
    title_cipher BLOB,
    title_nonce BLOB,
    metadata_cipher BLOB,
    metadata_nonce BLOB,
    payload_cipher BLOB,
    payload_nonce BLOB,
    blob_id TEXT,
    deleted INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для производительности
CREATE INDEX IF NOT EXISTS idx_vault_items_deleted ON vault_items(deleted);
CREATE INDEX IF NOT EXISTS idx_vault_items_item_type ON vault_items(item_type);