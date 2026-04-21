// Package migrations предоставляет функционал для применения миграций базы данных
// с использованием библиотеки golang-migrate.
//
// Миграции хранятся в embedded файловой системе и применяются автоматически
// при инициализации хранилища. Поддерживается версионирование миграций,
// откат изменений и проверка применённых миграций.
package migrations

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/prbllm/goph-keeper/migrations"
)

// Run применяет все неприменённые клиентские миграции к указанной базе данных.
func Run(db *sql.DB) error {
	// 1. Источник миграций из embed
	sourceDriver, err := iofs.New(migrations.ClientFS, "client")
	if err != nil {
		return err
	}

	// 2. Драйвер БД — используем generic sqlite, он совместим с modernc.org/sqlite
	dbDriver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return nil
	}

	// 3. Инициализируем мигратор
	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"sqlite",
		dbDriver,
	)
	if err != nil {
		return err
	}

	// 4. Запускаем миграции
	// ErrNoChange означает, что база уже актуальна — это не ошибка
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
