// Package postgres implements SQL repositories, transactional vault engine helpers, and connection pooling for the server.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/prbllm/goph-keeper/internal/server/config"
)

var _ Pool = (*sql.DB)(nil)

// DefaultConnector opens Postgres via database/sql and the pgx driver.
var DefaultConnector Connector = sqlConnector{}

type sqlConnector struct{}

// OpenPing implements Connector using pgx and verifies connectivity.
func (sqlConnector) OpenPing(ctx context.Context, databaseURL string) (Pool, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	db.SetMaxOpenConns(config.PostgresMaxOpenConns)
	db.SetMaxIdleConns(config.PostgresMaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(config.PostgresConnMaxLifetimeMin) * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(config.PostgresPingTimeoutSeconds)*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

// OpenPing is equivalent to DefaultConnector.OpenPing.
func OpenPing(ctx context.Context, databaseURL string) (Pool, error) {
	return DefaultConnector.OpenPing(ctx, databaseURL)
}
