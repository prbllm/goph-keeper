package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/blob"
)

// ExpiredUploadBlobRef identifies a blob row tied to an upload session past expires_at.
type ExpiredUploadBlobRef struct {
	BlobID    string
	ObjectKey string
}

const listExpiredUploadBlobRefsSQL = `
SELECT b.blob_id, b.object_key
FROM blobs b
INNER JOIN upload_sessions u ON b.blob_id = u.blob_id
WHERE u.expires_at < $1
  AND b.status IN ($2, $3)
ORDER BY b.blob_id
LIMIT $4`

// ListExpiredUploadBlobRefs returns up to limit blob rows that still have an expired upload session
// and are in an in-flight upload state (pending or uploading). limit must be positive.
func (s *UploadStore) ListExpiredUploadBlobRefs(ctx context.Context, expiresBefore time.Time, limit int) ([]ExpiredUploadBlobRef, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, listExpiredUploadBlobRefsSQL,
		expiresBefore, int16(blob.StatusPending), int16(blob.StatusUploading), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []ExpiredUploadBlobRef
	for rows.Next() {
		var ref ExpiredUploadBlobRef
		if err := rows.Scan(&ref.BlobID, &ref.ObjectKey); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteBlobByID removes a blob row by primary key. Dependent upload_sessions rows
// are removed via ON DELETE CASCADE.
func (s *UploadStore) DeleteBlobByID(ctx context.Context, blobID string) error {
	const q = `DELETE FROM blobs WHERE blob_id = $1`
	res, err := s.db.ExecContext(ctx, q, blobID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
