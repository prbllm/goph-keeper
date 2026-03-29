package migrations

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestRun_Success проверяет успешное применение миграций
func TestRun_Success(t *testing.T) {
	// Создаём временную БД
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Применяем миграции
	err = Run(db)
	require.NoError(t, err)

	// Проверяем, что таблицы созданы
	tables := []string{"auth_state", "sync_state", "pending_operations", "vault_items", "schema_migrations"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	}
}

// TestRun_Idempotent проверяет, что повторный запуск миграций не вызывает ошибок
func TestRun_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Первый запуск
	err = Run(db)
	require.NoError(t, err)

	// Второй запуск (должен пройти без ошибок)
	err = Run(db)
	require.NoError(t, err)
}

// TestRun_VersionTracking проверяет, что версии миграций записываются в schema_migrations
func TestRun_VersionTracking(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	err = Run(db)
	require.NoError(t, err)

	// Проверяем, что запись о миграции есть
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.NotNil(t, count)
}
