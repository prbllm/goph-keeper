package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/blob"
)

// UploadStore implements blob.UploadSessionStore.
type UploadStore struct {
	db Pool
}

func NewUploadStore(db Pool) *UploadStore {
	return &UploadStore{db: db}
}

var _ blob.UploadSessionStore = (*UploadStore)(nil)

func uploadSessionRowExists(ctx context.Context, tx *sql.Tx, sessionID, userID string) (bool, error) {
	const q = `SELECT 1 FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2 LIMIT 1`
	var one int
	err := tx.QueryRowContext(ctx, q, sessionID, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *UploadStore) StartSession(ctx context.Context, b *blob.Blob, us *blob.UploadSession) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := execInsertBlob(ctx, tx, b); err != nil {
		return err
	}

	const insertSession = `
INSERT INTO upload_sessions (
	upload_session_id, blob_id, user_id,
	expected_size, expected_checksum,
	received_size, status, expires_at,
	created_at, updated_at
) VALUES (
	$1, $2, $3,
	$4, $5,
	$6, $7, $8,
	$9, $10
)`
	if _, err := tx.ExecContext(ctx, insertSession,
		us.UploadSessionID, us.BlobID, us.UserID,
		us.ExpectedSize, us.ExpectedChecksum,
		us.ReceivedSize, us.Status, us.ExpiresAt,
		us.CreatedAt, us.UpdatedAt,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *UploadStore) GetSession(ctx context.Context, userID, sessionID string) (*blob.UploadSession, error) {
	const q = `
SELECT upload_session_id, blob_id, user_id,
       expected_size, expected_checksum,
       received_size, status, expires_at,
       created_at, updated_at
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2`
	var row blob.UploadSession
	err := s.db.QueryRowContext(ctx, q, sessionID, userID).Scan(
		&row.UploadSessionID, &row.BlobID, &row.UserID,
		&row.ExpectedSize, &row.ExpectedChecksum,
		&row.ReceivedSize, &row.Status, &row.ExpiresAt,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, blob.ErrUploadSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *UploadStore) DeleteSession(ctx context.Context, userID, sessionID string) error {
	const q = `DELETE FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2`
	res, err := s.db.ExecContext(ctx, q, sessionID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return blob.ErrUploadSessionNotFound
	}
	return nil
}

func (s *UploadStore) CompleteClientUpload(ctx context.Context, userID, sessionID string, receivedSize uint64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const updSession = `
UPDATE upload_sessions
SET received_size = $1,
    status = $2,
    updated_at = $3
WHERE upload_session_id = $4
  AND user_id = $5
  AND status = $6
  AND expected_size = $1
RETURNING blob_id`
	var blobID string
	err = tx.QueryRowContext(ctx, updSession,
		receivedSize,
		blob.UploadSessionAwaitingCommit,
		time.Now().UTC(),
		sessionID,
		userID,
		blob.UploadSessionOpen,
	).Scan(&blobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			exists, qerr := uploadSessionRowExists(ctx, tx, sessionID, userID)
			if qerr != nil {
				return qerr
			}
			if !exists {
				return blob.ErrUploadSessionNotFound
			}
			return blob.ErrUploadSessionNotOpen
		}
		return err
	}

	const updBlob = `
UPDATE blobs
SET status = $1
WHERE user_id = $2 AND blob_id = $3 AND status = $4`
	bres, err := tx.ExecContext(ctx, updBlob,
		blob.StatusUploading,
		userID,
		blobID,
		blob.StatusPending,
	)
	if err != nil {
		return err
	}
	bn, err := bres.RowsAffected()
	if err != nil {
		return err
	}
	if bn == 0 {
		return blob.ErrNotFound
	}

	return tx.Commit()
}

func (s *UploadStore) CommitUploadedBlob(ctx context.Context, userID, sessionID string, committedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const sel = `
SELECT blob_id, expires_at, status
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2
FOR UPDATE`
	var blobID string
	var expiresAt time.Time
	var st int16
	if err := tx.QueryRowContext(ctx, sel, sessionID, userID).Scan(&blobID, &expiresAt, &st); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blob.ErrUploadSessionNotFound
		}
		return err
	}
	if time.Now().UTC().After(expiresAt) {
		return blob.ErrUploadSessionExpired
	}
	if blob.UploadSessionStatus(st) != blob.UploadSessionAwaitingCommit {
		return blob.ErrUploadSessionNotAwaitingCommit
	}

	const del = `DELETE FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2`
	if _, err := tx.ExecContext(ctx, del, sessionID, userID); err != nil {
		return err
	}

	const mark = `
UPDATE blobs
SET status = $1,
    committed_at = $2,
    deleted_at = NULL
WHERE user_id = $3 AND blob_id = $4 AND status = $5`
	res, err := tx.ExecContext(ctx, mark,
		blob.StatusCommitted,
		committedAt,
		userID,
		blobID,
		blob.StatusUploading,
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

	return tx.Commit()
}
