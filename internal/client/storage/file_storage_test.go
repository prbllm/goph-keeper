package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	// Act
	storage, err := New(tempDir)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, storage)
	assert.NotNil(t, storage.items)
	assert.Equal(t, filepath.Join(tempDir, "state.json"), storage.authStatePath)
	assert.Equal(t, filepath.Join(tempDir, "sync.json"), storage.syncStatePath)
	assert.Equal(t, filepath.Join(tempDir, "vault.json"), storage.vaultPath)
}

func TestNew_CreateDir(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	nestedDir := filepath.Join(tempDir, "nested", "subdir")

	// Act
	storage, err := New(nestedDir)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, storage)

	// Проверяем, что директория создана
	_, err = os.Stat(nestedDir)
	assert.NoError(t, err)
}

func TestNew_InvalidPath(t *testing.T) {
	// Arrange
	invalidPath := "/root/protected/dir" // Путь, куда нет прав на запись

	// Act
	storage, err := New(invalidPath)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, storage)
}

func TestSave_Load_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	authState := &model.AuthState{
		UserID:            "test-user",
		AccessToken:       "access-token-abc",
		RefreshToken:      "refresh-token-xyz",
		DeviceID:          "device-1",
		PasswordSalt:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		EncryptedDEK:      []byte{17, 18, 19, 20},
		EncryptedDEKNonce: []byte{21, 22, 23, 24},
	}

	// Act
	err = storage.Save(authState)
	require.NoError(t, err)

	loaded, err := storage.Load()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "test-user", loaded.UserID)
	assert.Equal(t, "access-token-abc", loaded.AccessToken)
	assert.Equal(t, "refresh-token-xyz", loaded.RefreshToken)
	assert.Equal(t, "device-1", loaded.DeviceID)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, loaded.PasswordSalt)
	assert.Equal(t, []byte{17, 18, 19, 20}, loaded.EncryptedDEK)
	assert.Equal(t, []byte{21, 22, 23, 24}, loaded.EncryptedDEKNonce)
}

func TestSave_Load_FilePermissions(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	authState := &model.AuthState{UserID: "test"}
	err = storage.Save(authState)
	require.NoError(t, err)

	// Act
	info, err := os.Stat(storage.authStatePath)
	require.NoError(t, err)

	// Assert
	// Проверяем права доступа (0600 = -rw-------)
	expectedMode := os.FileMode(0600)
	assert.Equal(t, expectedMode, info.Mode().Perm())
}

func TestLoad_FileNotFound(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Удаляем файл состояния, если он существует
	os.Remove(storage.authStatePath)

	// Act
	loaded, err := storage.Load()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Empty(t, loaded.UserID)
	assert.Empty(t, loaded.AccessToken)
}

func TestLoad_InvalidJSON(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Создаём файл с невалидным JSON
	invalidJSON := []byte("{invalid json}")
	err = os.WriteFile(storage.authStatePath, invalidJSON, 0600)
	require.NoError(t, err)

	// Act
	loaded, err := storage.Load()

	// Assert
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestSaveSync_LoadSync_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	syncState := &model.SyncState{
		LastRevision: 100,
		PendingOperations: []model.PendingOperation{
			{
				OperationID:     "op-1",
				Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				ItemID:          "item-1",
				ExpectedVersion: 0,
				Snapshot: &gophkeeperv1.VaultItemSnapshot{
					ItemType: gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
					Title: &gophkeeperv1.EncryptedField{
						Ciphertext: []byte("title"),
						Nonce:      []byte("nonce"),
					},
				},
			},
		},
	}

	// Act
	err = storage.SaveSync(syncState)
	require.NoError(t, err)

	loaded, err := storage.LoadSync()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, uint64(100), loaded.LastRevision)
	assert.Len(t, loaded.PendingOperations, 1)
	assert.Equal(t, "op-1", loaded.PendingOperations[0].OperationID)
	assert.Equal(t, "item-1", loaded.PendingOperations[0].ItemID)
}

func TestLoadSync_FileNotFound(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Удаляем файл синхронизации
	os.Remove(storage.syncStatePath)

	// Act
	loaded, err := storage.LoadSync()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, uint64(0), loaded.LastRevision)
	assert.Empty(t, loaded.PendingOperations)
}

func TestLoadSync_InvalidJSON(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Создаём файл с невалидным JSON
	invalidJSON := []byte("{invalid sync json}")
	err = os.WriteFile(storage.syncStatePath, invalidJSON, 0600)
	require.NoError(t, err)

	// Act
	loaded, err := storage.LoadSync()

	// Assert
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestUpsert_Get_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	item := &model.Item{
		ID:      "item-123",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-title"),
			Nonce:      []byte("nonce-1"),
		},
		Metadata: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-meta"),
			Nonce:      []byte("nonce-2"),
		},
		Payload: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("encrypted-payload"),
			Nonce:      []byte("nonce-3"),
		},
		Deleted: false,
	}

	// Act
	err = storage.Upsert(item)
	require.NoError(t, err)

	retrieved, ok := storage.Get("item-123")

	// Assert
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "item-123", retrieved.ID)
	assert.Equal(t, uint64(1), retrieved.Version)
	assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_TEXT, retrieved.Type)
	assert.Equal(t, []byte("encrypted-title"), retrieved.Title.Ciphertext)
	assert.Equal(t, []byte("nonce-1"), retrieved.Title.Nonce)
	assert.False(t, retrieved.Deleted)
}

func TestUpsert_Update_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	item := &model.Item{
		ID:      "item-1",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
	}

	err = storage.Upsert(item)
	require.NoError(t, err)

	// Обновляем элемент
	updatedItem := &model.Item{
		ID:      "item-1",
		Version: 2,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
	}

	err = storage.Upsert(updatedItem)
	require.NoError(t, err)

	// Act
	retrieved, ok := storage.Get("item-1")

	// Assert
	assert.True(t, ok)
	assert.Equal(t, uint64(2), retrieved.Version)
	assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL, retrieved.Type)
}

func TestGet_NotFound(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Act
	item, ok := storage.Get("non-existent")

	// Assert
	assert.False(t, ok)
	assert.Nil(t, item)
}

func TestList_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	items := []*model.Item{
		{ID: "item-1", Version: 1},
		{ID: "item-2", Version: 2},
		{ID: "item-3", Version: 3},
	}

	for _, item := range items {
		err = storage.Upsert(item)
		require.NoError(t, err)
	}

	// Act
	listed, err := storage.List()
	require.NoError(t, err)

	// Assert
	assert.Len(t, listed, 3)

	// Проверяем, что все элементы присутствуют
	ids := make(map[string]bool)
	for _, item := range listed {
		ids[item.ID] = true
	}
	assert.True(t, ids["item-1"])
	assert.True(t, ids["item-2"])
	assert.True(t, ids["item-3"])
}

func TestList_Empty(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Act
	listed, err := storage.List()
	require.NoError(t, err)

	// Assert
	assert.Empty(t, listed)
}

func TestDelete_Success(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	item := &model.Item{ID: "item-to-delete", Version: 1}
	err = storage.Upsert(item)
	require.NoError(t, err)

	// Act
	err = storage.Delete("item-to-delete")
	require.NoError(t, err)

	_, ok := storage.Get("item-to-delete")

	// Assert
	assert.False(t, ok)
}

func TestDelete_NotFound(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Act
	err = storage.Delete("non-existent")

	// Assert
	require.NoError(t, err) // Удаление несуществующего элемента не должно вызывать ошибку
}

func TestSave_Persistence(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	// Создаём первое хранилище и сохраняем данные
	storage1, err := New(tempDir)
	require.NoError(t, err)

	authState := &model.AuthState{
		UserID:      "user-1",
		AccessToken: "token-1",
	}
	err = storage1.Save(authState)
	require.NoError(t, err)

	// Создаём второе хранилище и загружаем те же данные
	storage2, err := New(tempDir)
	require.NoError(t, err)

	// Act
	loaded, err := storage2.Load()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "user-1", loaded.UserID)
	assert.Equal(t, "token-1", loaded.AccessToken)
}

func TestSaveSync_Persistence(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	storage1, err := New(tempDir)
	require.NoError(t, err)

	syncState := &model.SyncState{
		LastRevision: 42,
	}
	err = storage1.SaveSync(syncState)
	require.NoError(t, err)

	storage2, err := New(tempDir)
	require.NoError(t, err)

	// Act
	loaded, err := storage2.LoadSync()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, uint64(42), loaded.LastRevision)
}

func TestUpsert_Persistence(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	storage1, err := New(tempDir)
	require.NoError(t, err)

	item := &model.Item{
		ID:      "persistent-item",
		Version: 1,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
	}
	err = storage1.Upsert(item)
	require.NoError(t, err)

	// Пересоздаём хранилище
	storage2, err := New(tempDir)
	require.NoError(t, err)

	// Act
	retrieved, ok := storage2.Get("persistent-item")

	// Assert
	assert.True(t, ok)
	assert.Equal(t, "persistent-item", retrieved.ID)
	assert.Equal(t, uint64(1), retrieved.Version)
}

func TestConcurrentAccess(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	// Act
	done := make(chan bool, 100)
	for i := range 100 {
		go func(id int) {
			// Обновляем версию
			updated := &model.Item{
				ID:      "concurrent-item",
				Version: uint64(id + 1),
			}
			_ = storage.Upsert(updated)
			done <- true
		}(i)
	}

	for range 100 {
		<-done
	}

	// Assert
	// Проверяем, что не произошло паники и данные доступны
	retrieved, ok := storage.Get("concurrent-item")
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
}

func TestConcurrentRead(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	item := &model.Item{ID: "read-item", Version: 1}
	err = storage.Upsert(item)
	require.NoError(t, err)

	// Act
	done := make(chan bool, 100)
	for range 100 {
		go func() {
			_, _ = storage.Get("read-item")
			_, _ = storage.List()
			done <- true
		}()
	}

	for range 100 {
		<-done
	}

	// Assert
	// Проверяем, что не произошло паники
	_, ok := storage.Get("read-item")
	assert.True(t, ok)
}

func TestExistingVaultFile(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	// Создаём файл хранилища с существующими данными
	vaultPath := filepath.Join(tempDir, "vault.json")
	existingData := `{
		"item-1": {
			"ID": "item-1",
			"Version": 1,
			"Type": 1,
			"Deleted": false
		},
		"item-2": {
			"ID": "item-2",
			"Version": 2,
			"Type": 2,
			"Deleted": false
		}
	}`
	err := os.WriteFile(vaultPath, []byte(existingData), 0600)
	require.NoError(t, err)

	// Act
	storage, err := New(tempDir)
	require.NoError(t, err)

	listed, err := storage.List()
	require.NoError(t, err)

	// Assert
	assert.Len(t, listed, 2)
	ids := make(map[string]bool)
	for _, item := range listed {
		ids[item.ID] = true
	}
	assert.True(t, ids["item-1"])
	assert.True(t, ids["item-2"])
}

func TestInvalidVaultFile(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	vaultPath := filepath.Join(tempDir, "vault.json")
	invalidData := []byte("{invalid json")
	err := os.WriteFile(vaultPath, invalidData, 0600)
	require.NoError(t, err)

	// Act
	storage, err := New(tempDir)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, storage)
}

func TestSave_Overwrite(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	state1 := &model.AuthState{UserID: "user-1"}
	err = storage.Save(state1)
	require.NoError(t, err)

	state2 := &model.AuthState{UserID: "user-2"}
	err = storage.Save(state2)
	require.NoError(t, err)

	// Act
	loaded, err := storage.Load()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "user-2", loaded.UserID) // Должно быть перезаписано
}

func TestSaveSync_Overwrite(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	storage, err := New(tempDir)
	require.NoError(t, err)

	state1 := &model.SyncState{LastRevision: 10}
	err = storage.SaveSync(state1)
	require.NoError(t, err)

	state2 := &model.SyncState{LastRevision: 20}
	err = storage.SaveSync(state2)
	require.NoError(t, err)

	// Act
	loaded, err := storage.LoadSync()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, uint64(20), loaded.LastRevision) // Должно быть перезаписано
}

// Table-driven тесты для различных типов элементов
func TestUpsert_List_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		itemType gophkeeperv1.ItemType
	}{
		{"text", gophkeeperv1.ItemType_ITEM_TYPE_TEXT},
		{"credential", gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL},
		{"card", gophkeeperv1.ItemType_ITEM_TYPE_CARD},
		{"binary", gophkeeperv1.ItemType_ITEM_TYPE_BINARY},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			storage, err := New(tempDir)
			require.NoError(t, err)

			item := &model.Item{
				ID:      "test-" + tt.name,
				Version: 1,
				Type:    tt.itemType,
			}

			err = storage.Upsert(item)
			require.NoError(t, err)

			listed, err := storage.List()
			require.NoError(t, err)

			assert.Len(t, listed, 1)
			assert.Equal(t, tt.itemType, listed[0].Type)
		})
	}
}

// BenchmarkSave измеряет производительность сохранения состояния
func BenchmarkSave(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := New(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	authState := &model.AuthState{
		UserID:      "benchmark-user",
		AccessToken: "benchmark-token",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := storage.Save(authState)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoad измеряет производительность загрузки состояния
func BenchmarkLoad(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := New(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	authState := &model.AuthState{UserID: "user"}
	err = storage.Save(authState)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := storage.Load()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpsert измеряет производительность добавления элемента
func BenchmarkUpsert(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := New(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	item := &model.Item{ID: "benchmark-item", Version: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := storage.Upsert(item)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkList измеряет производительность получения списка
func BenchmarkList(b *testing.B) {
	tempDir := b.TempDir()
	storage, err := New(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	// Предварительно добавляем элементы
	for i := range 100 {
		suffix := strconv.Itoa(i)
		item := &model.Item{ID: "item-" + suffix, Version: uint64(i)}
		err := storage.Upsert(item)
		if err != nil {
			b.Fatal(err)
		}
	}

	for b.Loop() {
		_, err := storage.List()
		if err != nil {
			b.Fatal(err)
		}
	}
}
