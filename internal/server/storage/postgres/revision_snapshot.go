package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// WithRevisionSnapshotRead runs fn with an Executor backed by a read-only transaction
// using REPEATABLE READ isolation so multiple queries observe one consistent snapshot
// of committed rows (PostgreSQL semantics).
func WithRevisionSnapshotRead(ctx context.Context, pool Pool, fn func(Executor) error) (err error) {
	if pool == nil {
		return fmt.Errorf("postgres: pool is nil")
	}
	tx, err := pool.BeginTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: sql.LevelRepeatableRead,
	})
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
