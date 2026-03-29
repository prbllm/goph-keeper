package postgres

import (
	"context"
	"database/sql"
)

// Executor is the subset of *sql.DB / *sql.Tx used by repositories inside a transaction or pool.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Pool is a verified SQL pool used by the server.
type Pool interface {
	Executor
	PingContext(ctx context.Context) error
	Close() error
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Connector opens a Postgres pool and checks connectivity.
type Connector interface {
	OpenPing(ctx context.Context, databaseURL string) (Pool, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -destination=../mocks/mock_postgres.go -package=mocks . Pool,Connector
