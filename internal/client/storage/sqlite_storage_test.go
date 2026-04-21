package storage

import (
	"os"
	"path/filepath"
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/stretchr/testify/require"
)

// newTestStorage создаёт тестовое хранилище во временной директории
func newTestStorage(t *testing.T) (*SQLiteStorage, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	storage, err := NewSQLiteStorage(tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		storage.Close()
	}

	return storage, cleanup
}

// ============== AuthStateStorage Tests ==============

// TestSQLiteStorage_SaveLoadAuthState проверяет сохранение и загрузку состояния аутентификации
func TestSQLiteStorage_SaveLoadAuthState(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	expected := &model.AuthState{
		UserID:            "user-123",
		AccessToken:       "access-token-abc",
		RefreshToken:      "refresh-token-xyz",
		DeviceID:          "device-001",
		PasswordSalt:      []byte("salt-bytes"),
		EncryptedDEK:      []byte("encrypted-dek"),
		EncryptedDEKNonce: []byte("nonce-bytes"),
	}

	// Сохраняем
	err := storage.Save(expected)
	require.NoError(t, err)

	// Загружаем
	actual, err := storage.Load()
	require.NoError(t, err)

	// Сравниваем
	require.Equal(t, expected.UserID, actual.UserID)
	require.Equal(t, expected.AccessToken, actual.AccessToken)
	require.Equal(t, expected.RefreshToken, actual.RefreshToken)
	require.Equal(t, expected.DeviceID, actual.DeviceID)
}

// TestSQLiteStorage_LoadEmpty проверяет загрузку при отсутствии данных
func TestSQLiteStorage_LoadEmpty(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	state, err := storage.Load()
	require.NoError(t, err)

	require.NotNil(t, state)
	require.Equal(t, "", state.UserID)
}

// TestSQLiteStorage_SaveOverwrite проверяет перезапись состояния аутентификации
func TestSQLiteStorage_SaveOverwrite(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	// Первое сохранение
	first := &model.AuthState{
		UserID:       "user-first",
		AccessToken:  "token-first",
		RefreshToken: "refresh-first",
		DeviceID:     "device-first",
	}
	err := storage.Save(first)
	require.NoError(t, err)

	// Второе сохранение (перезапись)
	second := &model.AuthState{
		UserID:       "user-second",
		AccessToken:  "token-second",
		RefreshToken: "refresh-second",
		DeviceID:     "device-second",
	}
	err = storage.Save(second)
	require.NoError(t, err)

	// Проверяем, что данные обновились
	actual, err := storage.Load()
	require.NoError(t, err)

	require.Equal(t, second.UserID, actual.UserID)
}

// ============== SyncStateStorage Tests ==============

// TestSQLiteStorage_SaveLoadSyncState проверяет сохранение и загрузку состояния синхронизации
func TestSQLiteStorage_SaveLoadSyncState(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	expected := &model.SyncState{
		LastRevision: 42,
		PendingOperations: []model.PendingOperation{
			{
				OperationID:     "op-001",
				Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				ItemID:          "item-001",
				ExpectedVersion: 0,
				Snapshot: &gophkeeperv1.VaultItemSnapshot{
					ItemType: gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
					Title: &gophkeeperv1.EncryptedField{
						Ciphertext: []byte("encrypted-title"),
						Nonce:      []byte("nonce"),
					},
				},
			},
		},
	}

	// Сохраняем
	err := storage.SaveSync(expected)
	require.NoError(t, err)

	// Загружаем
	actual, err := storage.LoadSync()
	require.NoError(t, err)

	// Сравниваем
	require.Equal(t, expected.LastRevision, actual.LastRevision)
	require.Equal(t, len(expected.PendingOperations), len(actual.PendingOperations))
}

// TestSQLiteStorage_LoadSyncEmpty проверяет загрузку пустого состояния синхронизации
func TestSQLiteStorage_LoadSyncEmpty(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	state, err := storage.LoadSync()
	require.NoError(t, err)

	require.NotNil(t, state)
	require.Zero(t, state.LastRevision)
	require.Zero(t, len(state.PendingOperations))
}

// ============== VaultStorage Tests ==============

// TestSQLiteStorage_UpsertGet проверяет добавление и получение элемента
func TestSQLiteStorage_UpsertGet(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	item := &model.Item{
		ID:      "item-001",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-title"),
			Nonce:      []byte("nonce"),
		},
		Metadata: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-meta"),
			Nonce:      []byte("nonce"),
		},
		Payload: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-payload"),
			Nonce:      []byte("nonce"),
		},
		Deleted: false,
	}

	// Добавляем
	err := storage.Upsert(item)
	require.NoError(t, err)

	// Получаем
	actual, ok := storage.Get(item.ID)
	require.True(t, ok)

	require.Equal(t, item.ID, actual.ID)
	require.Equal(t, item.Version, actual.Version)
}

// TestSQLiteStorage_GetNotFound проверяет получение несуществующего элемента
func TestSQLiteStorage_GetNotFound(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	_, ok := storage.Get("nonexistent-id")
	require.False(t, ok)
}

// TestSQLiteStorage_List проверяет получение списка элементов
func TestSQLiteStorage_List(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	// Добавляем 3 элемента
	items := []*model.Item{
		{ID: "item-001", Version: 1, Type: gophkeeperv1.ItemType_ITEM_TYPE_TEXT},
		{ID: "item-002", Version: 1, Type: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL},
		{ID: "item-003", Version: 1, Type: gophkeeperv1.ItemType_ITEM_TYPE_CARD},
	}

	for _, item := range items {
		err := storage.Upsert(item)
		require.NoError(t, err)
	}

	// Получаем список
	list, err := storage.List()
	require.NoError(t, err)

	require.Equal(t, len(items), len(list))
}

// TestSQLiteStorage_Delete проверяет удаление элемента
func TestSQLiteStorage_Delete(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	item := &model.Item{
		ID:      "item-to-delete",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
	}

	// Добавляем
	err := storage.Upsert(item)
	require.NoError(t, err)

	// Проверяем, что элемент есть
	_, ok := storage.Get(item.ID)
	require.True(t, ok)

	// Удаляем
	err = storage.Delete(item.ID)
	require.NoError(t, err)

	// Проверяем, что элемента нет
	_, ok = storage.Get(item.ID)
	require.False(t, ok)
}

// TestSQLiteStorage_UpsertUpdate проверяет обновление существующего элемента
func TestSQLiteStorage_UpsertUpdate(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	// Создаём
	item := &model.Item{
		ID:      "item-update",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("old-title"),
			Nonce:      []byte("nonce"),
		},
	}
	err := storage.Upsert(item)
	require.NoError(t, err)

	// Обновляем
	item.Version = 2
	item.Title = &gophkeeperv1.EncryptedField{
		Ciphertext: []byte("new-title"),
		Nonce:      []byte("nonce"),
	}
	err = storage.Upsert(item)
	require.NoError(t, err)

	// Проверяем обновление
	actual, ok := storage.Get(item.ID)
	require.True(t, ok)
	require.Equal(t, uint64(2), actual.Version)
}

// TestSQLiteStorage_DeleteNonexistent проверяет удаление несуществующего элемента
func TestSQLiteStorage_DeleteNonexistent(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	// Удаление несуществующего элемента не должно вызывать ошибку
	err := storage.Delete("nonexistent-id")
	require.NoError(t, err)
}

// ============== Integration Tests ==============

// TestSQLiteStorage_FullWorkflow проверяет полный рабочий цикл
func TestSQLiteStorage_FullWorkflow(t *testing.T) {
	storage, cleanup := newTestStorage(t)
	defer cleanup()

	// 1. Регистрация (сохранение auth state)
	authState := &model.AuthState{
		UserID:       "user-123",
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		DeviceID:     "device-001",
	}
	err := storage.Save(authState)
	require.NoError(t, err)

	// 2. Добавление элемента в хранилище
	item := &model.Item{
		ID:      "item-001",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
		Deleted: false,
	}
	err = storage.Upsert(item)
	require.NoError(t, err)

	// 3. Сохранение состояния синхронизации
	syncState := &model.SyncState{
		LastRevision: 1,
		PendingOperations: []model.PendingOperation{
			{
				OperationID:     "op-001",
				Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				ItemID:          "item-001",
				ExpectedVersion: 0,
			},
		},
	}
	err = storage.SaveSync(syncState)
	require.NoError(t, err)

	// 4. Проверяем все данные
	loadedAuth, err := storage.Load()
	require.NoError(t, err)
	require.Equal(t, authState.UserID, loadedAuth.UserID)

	loadedItem, ok := storage.Get(item.ID)
	require.True(t, ok)
	require.Equal(t, item.ID, loadedItem.ID)

	loadedSync, err := storage.LoadSync()
	require.NoError(t, err)
	require.Equal(t, syncState.LastRevision, loadedSync.LastRevision)
}

// TestSQLiteStorage_Close проверяет корректное закрытие соединения
func TestSQLiteStorage_Close(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewSQLiteStorage(tmpDir)
	require.NoError(t, err)

	// Закрываем
	err = storage.Close()
	require.NoError(t, err)

	// Проверяем, что файл БД существует
	dbPath := filepath.Join(tmpDir, "gk.db")
	_, err = os.Stat(dbPath)
	require.False(t, os.IsNotExist(err))
}
