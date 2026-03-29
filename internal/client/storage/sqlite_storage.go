// Package storage предоставляет реализацию локального хранилища данных клиента
// с использованием базы данных SQLite.
//
// Все данные шифруются на стороне клиента перед сохранением.
// Для построения SQL-запросов используется библиотека squirrel,
// что обеспечивает защиту от инъекций и читаемость кода.
package storage

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/Masterminds/squirrel"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/prbllm/goph-keeper/internal/client/storage/migrations"
	_ "modernc.org/sqlite"
)

// sq — глобальный билдер запросов squirrel, настроенный для SQLite.
// Использует знак вопроса (?) в качестве плейсхолдера параметров,
// что соответствует синтаксису SQLite.
var sq = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Question)

// SQLiteStorage реализует интерфейс LocalStorage с использованием
// встраиваемой базы данных SQLite.
//
// Хранилище обеспечивает:
//   - Потокобезопасность за счёт механизмов блокировок SQLite (WAL-режим).
//   - Автоматическое применение миграций при инициализации.
//   - Подготовку директории для файла БД при её отсутствии.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage создаёт и инициализирует новое экземпляр SQLiteStorage.
func NewSQLiteStorage(dataDirPath string) (*SQLiteStorage, error) {
	// Создаём директорию, если нет
	if err := os.MkdirAll(dataDirPath, 0700); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDirPath, "gk.db")

	// Открываем соединение с БД
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Настраиваем PRAGMA для SQLite
	// foreign_keys=ON — включение проверки внешних ключей
	// journal_mode=WAL — режим Write-Ahead Logging для лучшей конкурентности
	// synchronous=NORMAL — баланс между производительностью и надёжностью
	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Применяем миграции схемы
	if err = migrations.Run(db); err != nil {
		db.Close()
		return nil, err
	}

	// Проверяем доступность соединения
	// В SQLite Ping() может вернуть ошибку при проблемах с файлом БД
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStorage{db: db}, nil
}

// ============== AuthStateStorage ==============

// Save сохраняет состояние аутентификации пользователя в таблицу auth_state.
func (s *SQLiteStorage) Save(authState *model.AuthState) error {
	query, args, err := sq.
		Insert("auth_state").
		SetMap(map[string]any{
			"id":                  1,
			"user_id":             authState.UserID,
			"access_token":        authState.AccessToken,
			"refresh_token":       authState.RefreshToken,
			"device_id":           authState.DeviceID,
			"password_salt":       authState.PasswordSalt,
			"encrypted_dek":       authState.EncryptedDEK,
			"encrypted_dek_nonce": authState.EncryptedDEKNonce,
			"updated_at":          squirrel.Expr("CURRENT_TIMESTAMP"),
		}).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"user_id=EXCLUDED.user_id, " +
			"access_token=EXCLUDED.access_token, " +
			"refresh_token=EXCLUDED.refresh_token, " +
			"device_id=EXCLUDED.device_id, " +
			"password_salt=EXCLUDED.password_salt, " +
			"encrypted_dek=EXCLUDED.encrypted_dek, " +
			"encrypted_dek_nonce=EXCLUDED.encrypted_dek_nonce, " +
			"updated_at=CURRENT_TIMESTAMP").
		ToSql()
	if err != nil {
		return err
	}

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	return nil
}

// Load загружает состояние аутентификации из таблицы auth_state.
func (s *SQLiteStorage) Load() (*model.AuthState, error) {
	query, args, err := sq.
		Select("user_id", "access_token", "refresh_token", "device_id",
			"password_salt", "encrypted_dek", "encrypted_dek_nonce").
		From("auth_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var authState model.AuthState
	err = s.db.QueryRow(query, args...).Scan(
		&authState.UserID,
		&authState.AccessToken,
		&authState.RefreshToken,
		&authState.DeviceID,
		&authState.PasswordSalt,
		&authState.EncryptedDEK,
		&authState.EncryptedDEKNonce,
	)
	if err == sql.ErrNoRows {
		// Нет сохранённого состояния — возвращаем пустую структуру
		return &model.AuthState{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &authState, nil
}

// ============== SyncStateStorage ==============

// SaveSync сохраняет состояние синхронизации и список отложенных операций.
//
// Все операции выполняются в одной транзакции для обеспечения целостности:
// либо применяются все изменения, либо ни одно.
func (s *SQLiteStorage) SaveSync(syncState *model.SyncState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // откат при любой ошибке до Commit()

	// Сохраняем состояние синхронизации
	{
		query, args, err := sq.
			Insert("sync_state").
			SetMap(map[string]any{
				"id":            1,
				"last_revision": syncState.LastRevision,
				"updated_at":    squirrel.Expr("CURRENT_TIMESTAMP"),
			}).
			Suffix("ON CONFLICT(id) DO UPDATE SET " +
				"last_revision=EXCLUDED.last_revision, " +
				"updated_at=CURRENT_TIMESTAMP").
			ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}

	// Очищаем старые pending операции
	{
		query, args, err := sq.Delete("pending_operations").ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}

	// Вставляем новые pending операции
	for _, op := range syncState.PendingOperations {
		var snapshotType int32
		var titleCipher, titleNonce, metaCipher, metaNonce, payloadCipher, payloadNonce []byte
		var blobID string

		if op.Snapshot != nil {
			snapshotType = int32(op.Snapshot.ItemType)
			if op.Snapshot.Title != nil {
				titleCipher = op.Snapshot.Title.Ciphertext
				titleNonce = op.Snapshot.Title.Nonce
			}
			if op.Snapshot.Metadata != nil {
				metaCipher = op.Snapshot.Metadata.Ciphertext
				metaNonce = op.Snapshot.Metadata.Nonce
			}
			if op.Snapshot.Payload != nil {
				payloadCipher = op.Snapshot.Payload.Ciphertext
				payloadNonce = op.Snapshot.Payload.Nonce
			}
			blobID = op.Snapshot.BlobId
		}

		query, args, err := sq.
			Insert("pending_operations").
			SetMap(map[string]any{
				"operation_id":             op.OperationID,
				"operation_type":           int32(op.Type),
				"item_id":                  op.ItemID,
				"expected_version":         op.ExpectedVersion,
				"snapshot_type":            snapshotType,
				"snapshot_title_cipher":    titleCipher,
				"snapshot_title_nonce":     titleNonce,
				"snapshot_metadata_cipher": metaCipher,
				"snapshot_metadata_nonce":  metaNonce,
				"snapshot_payload_cipher":  payloadCipher,
				"snapshot_payload_nonce":   payloadNonce,
				"snapshot_blob_id":         blobID,
			}).
			ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// LoadSync загружает состояние синхронизации и список отложенных операций.
//
// Если таблица sync_state пуста, возвращается состояние с нулевой ревизией
// и пустым списком операций — это корректно для нового клиента.
func (s *SQLiteStorage) LoadSync() (*model.SyncState, error) {
	syncState := &model.SyncState{PendingOperations: []model.PendingOperation{}}

	// Загружаем состояние синхронизации
	{
		query, args, err := sq.
			Select("last_revision").
			From("sync_state").
			Where("id = ?", 1).
			ToSql()
		if err != nil {
			return nil, err
		}
		err = s.db.QueryRow(query, args...).Scan(&syncState.LastRevision)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		// ErrNoRows — нормальная ситуация, оставляем LastRevision = 0
	}

	// Загружаем pending операции
	{
		query, args, err := sq.
			Select("operation_id", "operation_type", "item_id", "expected_version",
				"snapshot_type", "snapshot_title_cipher", "snapshot_title_nonce",
				"snapshot_metadata_cipher", "snapshot_metadata_nonce",
				"snapshot_payload_cipher", "snapshot_payload_nonce",
				"snapshot_blob_id").
			From("pending_operations").
			OrderBy("id").
			ToSql()
		if err != nil {
			return nil, err
		}

		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var op model.PendingOperation
			var opType, snapshotType int32
			var titleCipher, titleNonce, metaCipher, metaNonce, payloadCipher, payloadNonce []byte
			var blobID sql.NullString

			err := rows.Scan(
				&op.OperationID, &opType, &op.ItemID, &op.ExpectedVersion,
				&snapshotType, &titleCipher, &titleNonce,
				&metaCipher, &metaNonce, &payloadCipher, &payloadNonce, &blobID,
			)
			if err != nil {
				return nil, err
			}

			op.Type = gophkeeperv1.PendingOperationType(opType)

			// Восстанавливаем snapshot, если данные присутствуют
			if snapshotType != 0 {
				op.Snapshot = &gophkeeperv1.VaultItemSnapshot{ItemType: gophkeeperv1.ItemType(snapshotType)}
				if len(titleCipher) > 0 {
					op.Snapshot.Title = &gophkeeperv1.EncryptedField{Ciphertext: titleCipher, Nonce: titleNonce}
				}
				if len(metaCipher) > 0 {
					op.Snapshot.Metadata = &gophkeeperv1.EncryptedField{Ciphertext: metaCipher, Nonce: metaNonce}
				}
				if len(payloadCipher) > 0 {
					op.Snapshot.Payload = &gophkeeperv1.EncryptedField{Ciphertext: payloadCipher, Nonce: payloadNonce}
				}
				if blobID.Valid {
					op.Snapshot.BlobId = blobID.String
				}
			}
			syncState.PendingOperations = append(syncState.PendingOperations, op)
		}
	}

	return syncState, nil
}

// ============== VaultStorage ==============

// Upsert добавляет новый элемент хранилища или обновляет существующий.
func (s *SQLiteStorage) Upsert(item *model.Item) error {
	var titleCipher, titleNonce, metaCipher, metaNonce, payloadCipher, payloadNonce []byte
	if item.Title != nil {
		titleCipher = item.Title.Ciphertext
		titleNonce = item.Title.Nonce
	}
	if item.Metadata != nil {
		metaCipher = item.Metadata.Ciphertext
		metaNonce = item.Metadata.Nonce
	}
	if item.Payload != nil {
		payloadCipher = item.Payload.Ciphertext
		payloadNonce = item.Payload.Nonce
	}

	query, args, err := sq.
		Insert("vault_items").
		SetMap(map[string]any{
			"id":              item.ID,
			"version":         item.Version,
			"item_type":       int32(item.Type),
			"title_cipher":    titleCipher,
			"title_nonce":     titleNonce,
			"metadata_cipher": metaCipher,
			"metadata_nonce":  metaNonce,
			"payload_cipher":  payloadCipher,
			"payload_nonce":   payloadNonce,
			"blob_id":         item.BlobID,
			"deleted":         item.Deleted,
			"updated_at":      squirrel.Expr("CURRENT_TIMESTAMP"),
		}).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"version=EXCLUDED.version, " +
			"item_type=EXCLUDED.item_type, " +
			"title_cipher=EXCLUDED.title_cipher, " +
			"title_nonce=EXCLUDED.title_nonce, " +
			"metadata_cipher=EXCLUDED.metadata_cipher, " +
			"metadata_nonce=EXCLUDED.metadata_nonce, " +
			"payload_cipher=EXCLUDED.payload_cipher, " +
			"payload_nonce=EXCLUDED.payload_nonce, " +
			"blob_id=EXCLUDED.blob_id, " +
			"deleted=EXCLUDED.deleted, " +
			"updated_at=CURRENT_TIMESTAMP").
		ToSql()
	if err != nil {
		return err
	}

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	return nil
}

// List возвращает срез указателей на все элементы хранилища.
func (s *SQLiteStorage) List() ([]*model.Item, error) {
	query, args, err := sq.
		Select("id", "version", "item_type", "title_cipher", "title_nonce",
			"metadata_cipher", "metadata_nonce", "payload_cipher", "payload_nonce",
			"blob_id", "deleted").
		From("vault_items").
		OrderBy("updated_at DESC").
		ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}

// Get возвращает элемент хранилища по указанному идентификатору.
func (s *SQLiteStorage) Get(id string) (*model.Item, bool) {
	query, args, err := sq.
		Select("id", "version", "item_type", "title_cipher", "title_nonce",
			"metadata_cipher", "metadata_nonce", "payload_cipher", "payload_nonce",
			"blob_id", "deleted").
		From("vault_items").
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return nil, false
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	items, err := scanItems(rows)
	if err != nil || len(items) == 0 {
		return nil, false
	}
	return items[0], true
}

// Delete удаляет элемент хранилища по указанному идентификатору.
func (s *SQLiteStorage) Delete(id string) error {
	query, args, err := sq.Delete("vault_items").Where("id = ?", id).ToSql()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	return nil
}

// scanItems — вспомогательная функция для сканирования строк в []*model.Item
func scanItems(rows *sql.Rows) ([]*model.Item, error) {
	var items []*model.Item
	for rows.Next() {
		var item model.Item
		var itemType int32
		var deleted int
		var titleCipher, titleNonce, metaCipher, metaNonce, payloadCipher, payloadNonce []byte
		var blobID sql.NullString

		err := rows.Scan(
			&item.ID, &item.Version, &itemType,
			&titleCipher, &titleNonce,
			&metaCipher, &metaNonce,
			&payloadCipher, &payloadNonce,
			&blobID, &deleted,
		)
		if err != nil {
			return nil, err
		}

		item.Type = gophkeeperv1.ItemType(itemType)
		item.Deleted = deleted == 1

		if len(titleCipher) > 0 {
			item.Title = &gophkeeperv1.EncryptedField{Ciphertext: titleCipher, Nonce: titleNonce}
		}
		if len(metaCipher) > 0 {
			item.Metadata = &gophkeeperv1.EncryptedField{Ciphertext: metaCipher, Nonce: metaNonce}
		}
		if len(payloadCipher) > 0 {
			item.Payload = &gophkeeperv1.EncryptedField{Ciphertext: payloadCipher, Nonce: payloadNonce}
		}
		if blobID.Valid {
			item.BlobID = blobID.String
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

// Close закрывает соединение с базой данных
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// Проверка на уровне компиляции: SQLiteStorage реализует интерфейс LocalStorage.
// Если сигнатуры методов не совпадут, компилятор выдаст ошибку.
// Переменная _ не используется в рантайме, только для статической проверки типов.
var _ LocalStorage = (*SQLiteStorage)(nil)
