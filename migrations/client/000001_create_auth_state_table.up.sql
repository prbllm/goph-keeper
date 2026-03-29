-- Таблица состояния аутентификации
CREATE TABLE IF NOT EXISTS auth_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    user_id TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    device_id TEXT NOT NULL,
    password_salt BLOB,
    encrypted_dek BLOB,
    encrypted_dek_nonce BLOB,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);