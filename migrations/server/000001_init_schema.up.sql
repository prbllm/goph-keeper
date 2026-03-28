CREATE TABLE users (
    user_id TEXT PRIMARY KEY,
    login TEXT NOT NULL UNIQUE,
    password_hash BYTEA NOT NULL,
    password_salt BYTEA NOT NULL,
    kdf_algorithm TEXT NOT NULL,
    kdf_memory_kib INTEGER NOT NULL,
    kdf_iterations INTEGER NOT NULL,
    kdf_parallelism INTEGER NOT NULL,
    kdf_key_length INTEGER NOT NULL,
    encrypted_vault_key BYTEA NOT NULL,
    encrypted_vault_key_nonce BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE devices (
    device_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    device_name TEXT NOT NULL,
    platform SMALLINT NOT NULL,
    client_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_devices_user_id ON devices(user_id);

CREATE TABLE refresh_tokens (
    token_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_device_id ON refresh_tokens(device_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

CREATE TABLE blobs (
    blob_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    object_key TEXT NOT NULL UNIQUE,
    size_bytes BIGINT NOT NULL,
    checksum BYTEA NOT NULL,
    content_kind TEXT NOT NULL,
    file_name TEXT,
    mime_type TEXT,
    status SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    committed_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ
);

CREATE INDEX idx_blobs_user_id ON blobs(user_id);
CREATE INDEX idx_blobs_status ON blobs(status);

CREATE TABLE upload_sessions (
    upload_session_id TEXT PRIMARY KEY,
    blob_id TEXT NOT NULL REFERENCES blobs(blob_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    expected_size BIGINT NOT NULL,
    expected_checksum BYTEA NOT NULL,
    received_size BIGINT NOT NULL DEFAULT 0,
    status SMALLINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_upload_sessions_blob_id ON upload_sessions(blob_id);
CREATE INDEX idx_upload_sessions_user_id ON upload_sessions(user_id);
CREATE INDEX idx_upload_sessions_expires_at ON upload_sessions(expires_at);

CREATE TABLE vault_items (
    item_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    item_type SMALLINT NOT NULL,
    title_ciphertext BYTEA NOT NULL,
    title_nonce BYTEA NOT NULL,
    metadata_ciphertext BYTEA NOT NULL,
    metadata_nonce BYTEA NOT NULL,
    payload_ciphertext BYTEA,
    payload_nonce BYTEA,
    blob_id TEXT REFERENCES blobs(blob_id) ON DELETE SET NULL,
    version BIGINT NOT NULL DEFAULT 1,
    checksum BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_vault_items_user_id ON vault_items(user_id);
CREATE INDEX idx_vault_items_user_id_updated_at ON vault_items(user_id, updated_at DESC);
CREATE INDEX idx_vault_items_user_id_deleted_at ON vault_items(user_id, deleted_at);

CREATE TABLE revision_log (
    revision_id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    operation_id TEXT,
    item_id TEXT NOT NULL REFERENCES vault_items(item_id) ON DELETE CASCADE,
    change_type SMALLINT NOT NULL,
    item_version BIGINT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_revision_log_user_id_revision_id ON revision_log(user_id, revision_id);
CREATE INDEX idx_revision_log_user_id_changed_at ON revision_log(user_id, changed_at DESC);
CREATE UNIQUE INDEX idx_revision_log_user_id_operation_id
    ON revision_log(user_id, operation_id)
    WHERE operation_id IS NOT NULL;

CREATE TABLE processed_operations (
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    operation_id TEXT NOT NULL,
    item_id TEXT,
    revision_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, operation_id)
);

CREATE INDEX idx_processed_operations_created_at ON processed_operations(created_at DESC);
