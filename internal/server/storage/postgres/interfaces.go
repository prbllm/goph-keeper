package postgres

import (
	"context"
	"database/sql"
)

// Pool is a verified SQL pool used by the server.
type Pool interface {
	PingContext(ctx context.Context) error
	Close() error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Connector opens a Postgres pool and checks connectivity.
type Connector interface {
	OpenPing(ctx context.Context, databaseURL string) (Pool, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -destination=../mocks/mock_postgres.go -package=mocks . Pool,Connector
