package postgres

import "context"

// Pool is a verified SQL pool used by the server.
type Pool interface {
	PingContext(ctx context.Context) error
	Close() error
}

// Connector opens a Postgres pool and checks connectivity.
type Connector interface {
	OpenPing(ctx context.Context, databaseURL string) (Pool, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -destination=../mocks/mock_postgres.go -package=mocks . Pool,Connector
