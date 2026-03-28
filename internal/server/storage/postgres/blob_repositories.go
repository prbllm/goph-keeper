package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/blob"
)

// BlobRepository implements blob.Repository on top of a Pool.
type BlobRepository struct {
	db Pool
}

func NewBlobRepository(db Pool) *BlobRepository {
	return &BlobRepository{db: db}
}

var _ blob.Repository = (*BlobRepository)(nil)

const blobGetByIDSQL = `
SELECT blob_id, user_id, object_key,
       size_bytes, checksum,
       content_kind, file_name, mime_type,
       status, created_at, committed_at, deleted_at
FROM blobs
WHERE user_id = $1 AND blob_id = $2`

const blobMarkCommittedSQL = `
UPDATE blobs
SET status = $1,
    committed_at = $2,
    deleted_at = NULL
WHERE user_id = $3 AND blob_id = $4`

const blobMarkDeletedSQL = `
UPDATE blobs
SET status = $1,
    deleted_at = $2
WHERE user_id = $3 AND blob_id = $4`

func (r *BlobRepository) Create(ctx context.Context, b *blob.Blob) error {
	const q = `
INSERT INTO blobs (
	blob_id, user_id, object_key,
	size_bytes, checksum,
	content_kind, file_name, mime_type,
	status, created_at, committed_at, deleted_at
) VALUES (
	$1, $2, $3,
	$4, $5,
	$6, $7, $8,
	$9, $10, $11, $12
)`

	var (
		fileName sql.NullString
		mimeType sql.NullString
	)
	if b.FileName != nil {
		fileName = sql.NullString{String: *b.FileName, Valid: true}
	}
	if b.MimeType != nil {
		mimeType = sql.NullString{String: *b.MimeType, Valid: true}
	}

	_, err := r.db.ExecContext(ctx, q,
		b.BlobID, b.UserID, b.ObjectKey,
		b.SizeBytes, b.Checksum,
		b.ContentKind, fileName, mimeType,
		b.Status, b.CreatedAt, b.CommittedAt, b.DeletedAt,
	)
	return err
}

func (r *BlobRepository) GetByID(ctx context.Context, userID, blobID string) (*blob.Blob, error) {
	var (
		row         blob.Blob
		fileName    sql.NullString
		mimeType    sql.NullString
		committedAt sql.NullTime
		deletedAt   sql.NullTime
	)

	err := r.db.QueryRowContext(ctx, blobGetByIDSQL, userID, blobID).Scan(
		&row.BlobID, &row.UserID, &row.ObjectKey,
		&row.SizeBytes, &row.Checksum,
		&row.ContentKind, &fileName, &mimeType,
		&row.Status, &row.CreatedAt, &committedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, blob.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if fileName.Valid {
		s := fileName.String
		row.FileName = &s
	}
	if mimeType.Valid {
		s := mimeType.String
		row.MimeType = &s
	}
	if committedAt.Valid {
		t := committedAt.Time
		row.CommittedAt = &t
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		row.DeletedAt = &t
	}
	return &row, nil
}

func (r *BlobRepository) MarkCommitted(ctx context.Context, userID, blobID string, committedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, blobMarkCommittedSQL,
		blob.StatusCommitted,
		committedAt,
		userID, blobID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return blob.ErrNotFound
	}
	return nil
}

func (r *BlobRepository) MarkDeleted(ctx context.Context, userID, blobID string, deletedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, blobMarkDeletedSQL,
		blob.StatusDeleted,
		deletedAt,
		userID, blobID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return blob.ErrNotFound
	}
	return nil
}
