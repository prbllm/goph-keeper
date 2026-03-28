package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWithRevisionSnapshotRead_success(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(revision_id\), 0\)`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(uint64(42)))
	mock.ExpectCommit()

	var saw uint64
	err = WithRevisionSnapshotRead(context.Background(), db, func(exec Executor) error {
		m, e := UserMaxRevision(context.Background(), exec, "user-1")
		saw = m
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if saw != 42 {
		t.Fatalf("max=%d", saw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithRevisionSnapshotRead_fnErrorRollsBack(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = WithRevisionSnapshotRead(context.Background(), db, func(Executor) error {
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithRevisionSnapshotRead_panicRollsBackAndRepanics(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}()

	WithRevisionSnapshotRead(context.Background(), db, func(Executor) error {
		panic("test panic")
	})
}
