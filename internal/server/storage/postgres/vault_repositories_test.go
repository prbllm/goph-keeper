package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

type stubPool struct{}

func (stubPool) PingContext(context.Context) error                                   { return nil }
func (stubPool) Close() error                                                       { return nil }
func (stubPool) ExecContext(context.Context, string, ...any) (sql.Result, error)   { return nil, nil }
func (stubPool) QueryRowContext(context.Context, string, ...any) *sql.Row          { return &sql.Row{} }
func (stubPool) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("stubPool: QueryContext not supported")
}
func (stubPool) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)          { return nil, nil }

var _ Pool = stubPool{}

func TestVaultRepositories_LazyInitAndSingleton(t *testing.T) {
	v := NewVaultRepositories(stubPool{})

	if got := v.Vault(); got == nil {
		t.Fatal("Vault() returned nil")
	} else if again := v.Vault(); got != again {
		t.Fatal("Vault() did not return the same instance on subsequent calls")
	}

	if got := v.Revisions(); got == nil {
		t.Fatal("Revisions() returned nil")
	} else if again := v.Revisions(); got != again {
		t.Fatal("Revisions() did not return the same instance on subsequent calls")
	}

	if got := v.ProcessedOperations(); got == nil {
		t.Fatal("ProcessedOperations() returned nil")
	} else if again := v.ProcessedOperations(); got != again {
		t.Fatal("ProcessedOperations() did not return the same instance on subsequent calls")
	}
}

