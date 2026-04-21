package postgres

import (
	"context"
	"database/sql"

	"github.com/prbllm/goph-keeper/internal/server/blob"
)

// blobExec matches *sql.DB and *sql.Tx for inserting blob rows.
type blobExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

const blobInsertSQL = `
INSERT INTO blobs (
	blob_id, user_id, object_key,
	size_bytes, checksum,
	content_kind, file_name, mime_type,
	status, created_at, committed_at, deleted_at, failed_at
) VALUES (
	$1, $2, $3,
	$4, $5,
	$6, $7, $8,
	$9, $10, $11, $12, $13
)`

func execInsertBlob(ctx context.Context, exec blobExec, b *blob.Blob) error {
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
	_, err := exec.ExecContext(ctx, blobInsertSQL,
		b.BlobID, b.UserID, b.ObjectKey,
		b.SizeBytes, b.Checksum,
		b.ContentKind, fileName, mimeType,
		b.Status, b.CreatedAt, b.CommittedAt, b.DeletedAt, nil,
	)
	return err
}
