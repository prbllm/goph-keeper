package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/vault"
)

type VaultRepository struct {
	db Pool
}

func NewVaultRepository(db Pool) *VaultRepository {
	return &VaultRepository{db: db}
}

func (r *VaultRepository) CreateItem(ctx context.Context, it *vault.Item) error {
	const q = `
INSERT INTO vault_items (
	item_id, user_id, item_type,
	title_ciphertext, title_nonce,
	metadata_ciphertext, metadata_nonce,
	payload_ciphertext, payload_nonce,
	blob_id, version, checksum, created_at, updated_at, deleted_at
) VALUES (
	$1, $2, $3,
	$4, $5,
	$6, $7,
	$8, $9,
	$10, $11, $12, $13, $14, $15
)`
	_, err := r.db.ExecContext(ctx, q,
		it.ItemID, it.UserID, it.ItemType,
		it.TitleCiphertext, it.TitleNonce,
		it.MetadataCiphertext, it.MetadataNonce,
		it.PayloadCiphertext, it.PayloadNonce,
		it.BlobID, it.Version, it.Checksum, it.CreatedAt, it.UpdatedAt, it.DeletedAt,
	)
	return err
}

func (r *VaultRepository) GetItem(ctx context.Context, userID, itemID string) (*vault.Item, error) {
	const q = `
SELECT item_id, user_id, item_type,
       title_ciphertext, title_nonce,
       metadata_ciphertext, metadata_nonce,
       payload_ciphertext, payload_nonce,
       blob_id, version, checksum, created_at, updated_at, deleted_at
FROM vault_items
WHERE user_id = $1 AND item_id = $2`
	var it vault.Item
	var blobID sql.NullString
	var deletedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, q, userID, itemID).Scan(
		&it.ItemID, &it.UserID, &it.ItemType,
		&it.TitleCiphertext, &it.TitleNonce,
		&it.MetadataCiphertext, &it.MetadataNonce,
		&it.PayloadCiphertext, &it.PayloadNonce,
		&blobID, &it.Version, &it.Checksum, &it.CreatedAt, &it.UpdatedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, vault.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if blobID.Valid {
		it.BlobID = &blobID.String
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		it.DeletedAt = &t
	}
	return &it, nil
}

func (r *VaultRepository) ListItems(ctx context.Context, userID string, includeDeleted bool, limit int32, pageToken string) ([]*vault.Item, string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	const base = `
SELECT item_id, user_id, item_type,
       title_ciphertext, title_nonce,
       metadata_ciphertext, metadata_nonce,
       payload_ciphertext, payload_nonce,
       blob_id, version, checksum, created_at, updated_at, deleted_at
FROM vault_items
WHERE user_id = $1`

	var (
		args []any
		q    strings.Builder
	)
	args = append(args, userID)
	q.WriteString(base)
	if !includeDeleted {
		q.WriteString(" AND deleted_at IS NULL")
	}
	argPos := 2

	// pageToken format: "<updatedAtUnixNano>:<itemID>"
	if pageToken != "" {
		parts := strings.SplitN(pageToken, ":", 2)
		if len(parts) != 2 {
			return nil, "", vault.ErrInvalidArgument
		}
		updatedAtNano, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, "", vault.ErrInvalidArgument
		}
		lastItemID := parts[1]
		q.WriteString(fmt.Sprintf(" AND (updated_at, item_id) < (to_timestamp($%d / 1e9), $%d)", argPos, argPos+1))
		args = append(args, updatedAtNano, lastItemID)
		argPos += 2
	}

	q.WriteString(fmt.Sprintf(" ORDER BY updated_at DESC, item_id DESC LIMIT $%d", argPos))
	args = append(args, limit+1) // fetch one extra to derive next page token

	db, ok := r.db.(interface {
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	})
	if !ok {
		return nil, "", errors.New("vault: pool does not support QueryContext")
	}

	rows, err := db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var (
		items     []*vault.Item
		nextToken string
	)

	for rows.Next() {
		var it vault.Item
		var blobID sql.NullString
		var deletedAt sql.NullTime
		if err := rows.Scan(
			&it.ItemID, &it.UserID, &it.ItemType,
			&it.TitleCiphertext, &it.TitleNonce,
			&it.MetadataCiphertext, &it.MetadataNonce,
			&it.PayloadCiphertext, &it.PayloadNonce,
			&blobID, &it.Version, &it.Checksum, &it.CreatedAt, &it.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, "", err
		}
		if blobID.Valid {
			it.BlobID = &blobID.String
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			it.DeletedAt = &t
		}
		items = append(items, &it)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if int32(len(items)) > limit {
		last := items[limit]
		items = items[:limit]
		nextToken = fmt.Sprintf("%d:%s", last.UpdatedAt.UnixNano(), last.ItemID)
	}
	return items, nextToken, nil
}

func (r *VaultRepository) UpdateItem(ctx context.Context, it *vault.Item, expectedVersion uint64) error {
	const q = `
UPDATE vault_items
SET title_ciphertext = $1,
    title_nonce = $2,
    metadata_ciphertext = $3,
    metadata_nonce = $4,
    payload_ciphertext = $5,
    payload_nonce = $6,
    blob_id = $7,
    version = $8,
    checksum = $9,
    updated_at = $10
WHERE user_id = $11 AND item_id = $12 AND version = $13 AND deleted_at IS NULL`
	res, err := r.db.ExecContext(ctx, q,
		it.TitleCiphertext, it.TitleNonce,
		it.MetadataCiphertext, it.MetadataNonce,
		it.PayloadCiphertext, it.PayloadNonce,
		it.BlobID, it.Version, it.Checksum, it.UpdatedAt,
		it.UserID, it.ItemID, expectedVersion,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return vault.ErrConflict
	}
	return nil
}

func (r *VaultRepository) DeleteItem(ctx context.Context, userID, itemID string, expectedVersion uint64, deletedAt time.Time) (uint64, error) {
	const q = `
UPDATE vault_items
SET deleted_at = $1,
    version = version + 1,
    updated_at = $1
WHERE user_id = $2 AND item_id = $3 AND version = $4 AND deleted_at IS NULL
RETURNING version`
	var newVersion uint64
	err := r.db.QueryRowContext(ctx, q, deletedAt, userID, itemID, expectedVersion).Scan(&newVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, vault.ErrConflict
	}
	if err != nil {
		return 0, err
	}
	return newVersion, nil
}

type RevisionLogRepository struct {
	db Pool
}

func NewRevisionLogRepository(db Pool) *RevisionLogRepository {
	return &RevisionLogRepository{db: db}
}

func (r *RevisionLogRepository) AppendRevision(ctx context.Context, entry *vault.RevisionLogEntry) error {
	const q = `
INSERT INTO revision_log (user_id, operation_id, item_id, change_type, item_version)
VALUES ($1, $2, $3, $4, $5)
RETURNING revision_id, changed_at`
	var opID any
	if entry.OperationID != nil {
		opID = *entry.OperationID
	}
	if err := r.db.QueryRowContext(ctx, q,
		entry.UserID, opID, entry.ItemID, entry.ChangeType, entry.ItemVersion,
	).Scan(&entry.RevisionID, &entry.ChangedAt); err != nil {
		return err
	}
	return nil
}

type ProcessedOperationsRepository struct {
	db Pool
}

func NewProcessedOperationsRepository(db Pool) *ProcessedOperationsRepository {
	return &ProcessedOperationsRepository{db: db}
}

func (r *ProcessedOperationsRepository) MarkProcessed(ctx context.Context, userID, operationID, itemID string, revisionID uint64) error {
	const q = `
INSERT INTO processed_operations (user_id, operation_id, item_id, revision_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, operation_id) DO NOTHING`
	_, err := r.db.ExecContext(ctx, q, userID, operationID, itemID, revisionID)
	return err
}

func (r *ProcessedOperationsRepository) Lookup(ctx context.Context, userID, operationID string) (*vault.ProcessedOperation, error) {
	const q = `
SELECT user_id, operation_id, item_id, revision_id
FROM processed_operations
WHERE user_id = $1 AND operation_id = $2`
	var op vault.ProcessedOperation
	if err := r.db.QueryRowContext(ctx, q, userID, operationID).Scan(
		&op.UserID, &op.OperationID, &op.ItemID, &op.RevisionID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &op, nil
}

var (
	_ vault.Repository                   = (*VaultRepository)(nil)
	_ vault.RevisionLogRepository       = (*RevisionLogRepository)(nil)
	_ vault.ProcessedOperationsRepository = (*ProcessedOperationsRepository)(nil)
)

