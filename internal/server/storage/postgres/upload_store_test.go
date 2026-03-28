package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prbllm/goph-keeper/internal/server/blob"
)

func TestUploadStore_GetSession_notFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT upload_session_id, blob_id, user_id,
       expected_size, expected_checksum,
       received_size, status, expires_at,
       created_at, updated_at
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2`)).
		WithArgs("sess-1", "user-1").
		WillReturnError(sql.ErrNoRows)

	s := NewUploadStore(db)
	_, err = s.GetSession(context.Background(), "user-1", "sess-1")
	if !errors.Is(err, blob.ErrUploadSessionNotFound) {
		t.Fatalf("GetSession: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CompleteClientUpload_sessionNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
UPDATE upload_sessions
SET received_size = $1,
    status = $2,
    updated_at = $3
WHERE upload_session_id = $4
  AND user_id = $5
  AND status = $6
  AND expected_size = $1
RETURNING blob_id`)).
		WithArgs(uint64(10), blob.UploadSessionAwaitingCommit, sqlmock.AnyArg(), "sess-1", "user-1", blob.UploadSessionOpen).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2 LIMIT 1`)).
		WithArgs("sess-1", "user-1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	s := NewUploadStore(db)
	err = s.CompleteClientUpload(context.Background(), "user-1", "sess-1", 10)
	if !errors.Is(err, blob.ErrUploadSessionNotFound) {
		t.Fatalf("CompleteClientUpload: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CompleteClientUpload_sessionNotOpen(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
UPDATE upload_sessions
SET received_size = $1,
    status = $2,
    updated_at = $3
WHERE upload_session_id = $4
  AND user_id = $5
  AND status = $6
  AND expected_size = $1
RETURNING blob_id`)).
		WithArgs(uint64(10), blob.UploadSessionAwaitingCommit, sqlmock.AnyArg(), "sess-1", "user-1", blob.UploadSessionOpen).
		WillReturnError(sql.ErrNoRows)
	exists := sqlmock.NewRows([]string{"one"}).AddRow(1)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2 LIMIT 1`)).
		WithArgs("sess-1", "user-1").
		WillReturnRows(exists)
	mock.ExpectRollback()

	s := NewUploadStore(db)
	err = s.CompleteClientUpload(context.Background(), "user-1", "sess-1", 10)
	if !errors.Is(err, blob.ErrUploadSessionNotOpen) {
		t.Fatalf("CompleteClientUpload: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CompleteClientUpload_ok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"blob_id"}).AddRow("blob-1")
	mock.ExpectQuery(regexp.QuoteMeta(`
UPDATE upload_sessions
SET received_size = $1,
    status = $2,
    updated_at = $3
WHERE upload_session_id = $4
  AND user_id = $5
  AND status = $6
  AND expected_size = $1
RETURNING blob_id`)).
		WithArgs(uint64(10), blob.UploadSessionAwaitingCommit, sqlmock.AnyArg(), "sess-1", "user-1", blob.UploadSessionOpen).
		WillReturnRows(rows)
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE blobs
SET status = $1
WHERE user_id = $2 AND blob_id = $3 AND status = $4`)).
		WithArgs(blob.StatusUploading, "user-1", "blob-1", blob.StatusPending).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	s := NewUploadStore(db)
	if err := s.CompleteClientUpload(context.Background(), "user-1", "sess-1", 10); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CommitUploadedBlob_expired(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	selRows := sqlmock.NewRows([]string{"blob_id", "expires_at", "status"}).
		AddRow("blob-1", time.Now().UTC().Add(-time.Hour), int16(blob.UploadSessionAwaitingCommit))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT blob_id, expires_at, status
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2
FOR UPDATE`)).
		WithArgs("sess-1", "user-1").
		WillReturnRows(selRows)
	mock.ExpectRollback()

	s := NewUploadStore(db)
	err = s.CommitUploadedBlob(context.Background(), "user-1", "sess-1", time.Now().UTC())
	if !errors.Is(err, blob.ErrUploadSessionExpired) {
		t.Fatalf("CommitUploadedBlob: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CommitUploadedBlob_notAwaitingCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	selRows := sqlmock.NewRows([]string{"blob_id", "expires_at", "status"}).
		AddRow("blob-1", time.Now().UTC().Add(time.Hour), int16(blob.UploadSessionOpen))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT blob_id, expires_at, status
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2
FOR UPDATE`)).
		WithArgs("sess-1", "user-1").
		WillReturnRows(selRows)
	mock.ExpectRollback()

	s := NewUploadStore(db)
	err = s.CommitUploadedBlob(context.Background(), "user-1", "sess-1", time.Now().UTC())
	if !errors.Is(err, blob.ErrUploadSessionNotAwaitingCommit) {
		t.Fatalf("CommitUploadedBlob: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadStore_CommitUploadedBlob_ok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	at := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	selRows := sqlmock.NewRows([]string{"blob_id", "expires_at", "status"}).
		AddRow("blob-1", time.Now().UTC().Add(time.Hour), int16(blob.UploadSessionAwaitingCommit))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT blob_id, expires_at, status
FROM upload_sessions
WHERE upload_session_id = $1 AND user_id = $2
FOR UPDATE`)).
		WithArgs("sess-1", "user-1").
		WillReturnRows(selRows)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM upload_sessions WHERE upload_session_id = $1 AND user_id = $2`)).
		WithArgs("sess-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE blobs
SET status = $1,
    committed_at = $2,
    deleted_at = NULL
WHERE user_id = $3 AND blob_id = $4 AND status = $5`)).
		WithArgs(blob.StatusCommitted, at, "user-1", "blob-1", blob.StatusUploading).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	s := NewUploadStore(db)
	if err := s.CommitUploadedBlob(context.Background(), "user-1", "sess-1", at); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
