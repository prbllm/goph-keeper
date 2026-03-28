package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prbllm/goph-keeper/internal/server/vault"
)

func TestWithVaultEngineTx_commitEmptyFn(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	err = WithVaultEngineTx(context.Background(), db, time.Now, func(*vault.EngineService) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithVaultEngineTx_fnErrorRollsBack(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = WithVaultEngineTx(context.Background(), db, time.Now, func(*vault.EngineService) error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithVaultEngineTx_panicRollsBackAndRepanics(t *testing.T) {
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

	WithVaultEngineTx(context.Background(), db, time.Now, func(*vault.EngineService) error {
		panic("test panic")
	})
}
