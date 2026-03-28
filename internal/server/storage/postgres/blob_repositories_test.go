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

func TestBlobRepository_GetByID_notFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(blobGetByIDSQL)).
		WithArgs("user-1", "blob-1").
		WillReturnError(sql.ErrNoRows)

	repo := NewBlobRepository(db)
	_, err = repo.GetByID(context.Background(), "user-1", "blob-1")
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("GetByID: got %v want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlobRepository_GetByID_found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"blob_id", "user_id", "object_key",
		"size_bytes", "checksum",
		"content_kind", "file_name", "mime_type",
		"status", "created_at", "committed_at", "deleted_at",
	}).AddRow(
		"bid", "uid", "obj-key",
		uint64(10), []byte{1, 2, 3},
		"file", "fname.txt", "text/plain",
		int16(blob.StatusCommitted), created, nil, nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(blobGetByIDSQL)).
		WithArgs("uid", "bid").
		WillReturnRows(rows)

	repo := NewBlobRepository(db)
	got, err := repo.GetByID(context.Background(), "uid", "bid")
	if err != nil {
		t.Fatal(err)
	}
	if got.BlobID != "bid" || got.UserID != "uid" || got.ObjectKey != "obj-key" {
		t.Fatalf("unexpected row: %+v", got)
	}
	if got.SizeBytes != 10 || got.Status != blob.StatusCommitted {
		t.Fatalf("size/status: %+v", got)
	}
	if got.FileName == nil || *got.FileName != "fname.txt" {
		t.Fatalf("FileName: %+v", got.FileName)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlobRepository_MarkCommitted_notFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	at := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(blobMarkCommittedSQL)).
		WithArgs(blob.StatusCommitted, at, "u", "b").
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := NewBlobRepository(db)
	err = repo.MarkCommitted(context.Background(), "u", "b", at)
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("MarkCommitted: got %v want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlobRepository_MarkCommitted_ok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	at := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(blobMarkCommittedSQL)).
		WithArgs(blob.StatusCommitted, at, "u", "b").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewBlobRepository(db)
	if err := repo.MarkCommitted(context.Background(), "u", "b", at); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlobRepository_MarkDeleted_notFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	at := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(blobMarkDeletedSQL)).
		WithArgs(blob.StatusDeleted, at, "u", "b").
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := NewBlobRepository(db)
	err = repo.MarkDeleted(context.Background(), "u", "b", at)
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("MarkDeleted: got %v want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlobRepository_MarkDeleted_ok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	at := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(blobMarkDeletedSQL)).
		WithArgs(blob.StatusDeleted, at, "u", "b").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewBlobRepository(db)
	if err := repo.MarkDeleted(context.Background(), "u", "b", at); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
