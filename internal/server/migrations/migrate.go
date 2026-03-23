package migrations

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	projectmigrations "github.com/prbllm/goph-keeper/migrations"
	"go.uber.org/zap"
)

const sourcePath = "server"

// Run applies embedded migrations and validates resulting state.
func Run(logger *zap.Logger, databaseURL string) error {
	sourceDriver, err := iofs.New(projectmigrations.ServerFS, sourcePath)
	if err != nil {
		logger.Error("migrations failed: source init", zap.Error(err))
		return fmt.Errorf("migrate source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		logger.Error("migrations failed: runner init", zap.Error(err))
		return fmt.Errorf("migrate init: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			logger.Warn("migrate close source failed", zap.Error(srcErr))
		}
		if dbErr != nil {
			logger.Warn("migrate close database failed", zap.Error(dbErr))
		}
	}()

	noChange, err := handleUpResult(logger, m.Up())
	if err != nil {
		return err
	}
	if noChange {
		return nil
	}

	version, dirty, err := m.Version()
	switch {
	case errors.Is(err, migrate.ErrNilVersion):
		logger.Info("migrations: no versions applied")
		return nil
	case err != nil:
		logger.Error("migrations failed: read version", zap.Error(err))
		return fmt.Errorf("migrate version: %w", err)
	}

	logger.Info("migrations applied", zap.Uint("version", version), zap.Bool("dirty", dirty))
	if dirty {
		logger.Error("migrations failed: dirty after apply", zap.Uint("version", version))
		return fmt.Errorf("migrate completed but database is dirty at version %d", version)
	}

	return nil
}

func handleUpResult(logger *zap.Logger, err error) (bool, error) {
	if err == nil {
		return false, nil
	}

	if errors.Is(err, migrate.ErrNoChange) {
		logger.Info("migrations: no changes required")
		return true, nil
	}

	var dirtyErr migrate.ErrDirty
	if errors.As(err, &dirtyErr) {
		logger.Error("migrations failed: dirty state", zap.Int("version", dirtyErr.Version), zap.Error(err))
		return false, fmt.Errorf("migrate dirty state at version %d: %w", dirtyErr.Version, err)
	}

	logger.Error("migrations failed: up", zap.Error(err))
	return false, fmt.Errorf("migrate up: %w", err)
}
