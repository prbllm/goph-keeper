package postgres

import (
	"context"
	"fmt"

	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
)

// WithVaultEngineTx runs fn with a vault engine whose repositories share one SQL transaction.
func WithVaultEngineTx(ctx context.Context, pool Pool, logger *zap.Logger, now vault.Clock, fn func(*vault.EngineService) error) (err error) {
	if pool == nil {
		return fmt.Errorf("postgres: pool is nil")
	}
	tx, err := pool.BeginTx(ctx, nil)
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

	eng := vault.NewEngine(
		NewVaultRepository(tx),
		NewRevisionLogRepository(tx),
		NewProcessedOperationsRepository(tx),
		now,
		logger,
	)
	if err = fn(eng); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
