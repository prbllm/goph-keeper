package app

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/crypto"
	"github.com/prbllm/goph-keeper/internal/client/mocks"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/stretchr/testify/assert"
)

func TestAddItem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:    nil,
		DEK:       make([]byte, 32), // Валидный DEK
		AuthState: &model.AuthState{},
		SyncState: &model.SyncState{
			LastRevision:      10,
			PendingOperations: []model.PendingOperation{},
		},
		LocalStorage: mockStorage,
	}

	title := []byte("My Secret")
	metadata := []byte("important")
	payload := []byte("super-secret-data")

	// Мокаем Upsert для сохранения элемента
	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_TEXT, item.Type)
			assert.Equal(t, uint64(1), item.Version)
			assert.False(t, item.Deleted)
			// Проверяем, что данные зашифрованы
			assert.NotNil(t, item.Title)
			assert.NotNil(t, item.Metadata)
			assert.NotNil(t, item.Payload)
			return nil
		}).
		Times(1)

	// Мокаем SaveSync для сохранения состояния синхронизации
	mockStorage.EXPECT().SaveSync(gomock.Any()).
		DoAndReturn(func(syncState *model.SyncState) error {
			assert.Len(t, syncState.PendingOperations, 1)
			op := syncState.PendingOperations[0]
			assert.Equal(t, gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE, op.Type)
			assert.Equal(t, uint64(0), op.ExpectedVersion) // Новая версия = 0
			return nil
		}).
		Times(1)

	// Act
	err := app.AddItem(gophkeeperv1.ItemType_ITEM_TYPE_TEXT, title, metadata, payload, "")

	// Assert
	assert.NoError(t, err)
	assert.Len(t, app.SyncState.PendingOperations, 1)
	assert.Equal(t, uint64(10), app.SyncState.LastRevision) // Ревизия не меняется при добавлении
}

func TestAddItem_NoDEK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          nil, // Отсутствует DEK
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// Upsert не должен быть вызван
	mockStorage.EXPECT().Upsert(gomock.Any()).Times(0)
	mockStorage.EXPECT().SaveSync(gomock.Any()).Times(0)

	// Act
	err := app.AddItem(gophkeeperv1.ItemType_ITEM_TYPE_TEXT, []byte("title"), nil, []byte("data"), "")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "login required (DEK missing)", err.Error())
	assert.Empty(t, app.SyncState.PendingOperations)
}

func TestAddItem_EncryptError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          []byte("invalid-key-too-short"), // Невалидный ключ (менее 32 байт)
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// Act
	err := app.AddItem(gophkeeperv1.ItemType_ITEM_TYPE_TEXT, []byte("title"), nil, []byte("data"), "")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad key length")
	assert.Empty(t, app.SyncState.PendingOperations)
}

func TestAddItem_UpsertError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          make([]byte, 32),
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		Return(errors.New("disk full")).
		Times(1)

	// SaveSync не должен быть вызван при ошибке Upsert
	mockStorage.EXPECT().SaveSync(gomock.Any()).Times(0)

	// Act
	err := app.AddItem(gophkeeperv1.ItemType_ITEM_TYPE_TEXT, []byte("title"), nil, []byte("data"), "")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "disk full", err.Error())
	assert.Empty(t, app.SyncState.PendingOperations) // Операция не добавлена
}

func TestListItems_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	expectedItems := []*model.Item{
		{ID: "item-1", Version: 1, Type: gophkeeperv1.ItemType_ITEM_TYPE_TEXT},
		{ID: "item-2", Version: 2, Type: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL},
	}

	mockStorage.EXPECT().List().
		Return(expectedItems, nil).
		Times(1)

	// Act
	items, err := app.ListItems()

	// Assert
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, expectedItems, items)
}

func TestListItems_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().List().
		Return(nil, errors.New("storage error")).
		Times(1)

	// Act
	items, err := app.ListItems()

	// Assert
	assert.Error(t, err)
	assert.Nil(t, items)
	assert.Equal(t, "storage error", err.Error())
}

func TestGetItem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	expectedItem := &model.Item{ID: "test-id", Version: 1}

	mockStorage.EXPECT().Get("test-id").
		Return(expectedItem, true).
		Times(1)

	// Act
	item, ok := app.GetItem("test-id")

	// Assert
	assert.True(t, ok)
	assert.Equal(t, expectedItem, item)
}

func TestGetItem_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().Get("non-existent").
		Return(nil, false).
		Times(1)

	// Act
	item, ok := app.GetItem("non-existent")

	// Assert
	assert.False(t, ok)
	assert.Nil(t, item)
}

func TestUpdateItem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:    nil,
		DEK:       make([]byte, 32),
		AuthState: &model.AuthState{},
		SyncState: &model.SyncState{
			LastRevision:      20,
			PendingOperations: []model.PendingOperation{},
		},
		LocalStorage: mockStorage,
	}

	existingItem := &model.Item{
		ID:      "item-123",
		Version: 5,
		Type:    gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("old-title"),
			Nonce:      []byte("old-nonce"),
		},
	}

	mockStorage.EXPECT().Get("item-123").
		Return(existingItem, true).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "item-123", item.ID)
			assert.Equal(t, uint64(6), item.Version) // Версия увеличена
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL, item.Type)
			return nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		DoAndReturn(func(syncState *model.SyncState) error {
			assert.Len(t, syncState.PendingOperations, 1)
			op := syncState.PendingOperations[0]
			assert.Equal(t, gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE, op.Type)
			assert.Equal(t, uint64(5), op.ExpectedVersion) // Ожидаемая старая версия
			return nil
		}).
		Times(1)

	// Act
	err := app.UpdateItem("item-123", gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL, []byte("new-title"), nil, []byte("new-data"))

	// Assert
	assert.NoError(t, err)
	assert.Len(t, app.SyncState.PendingOperations, 1)
	assert.Equal(t, uint64(6), existingItem.Version) // Локальный объект обновлён
}

func TestUpdateItem_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          make([]byte, 32),
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().Get("non-existent").
		Return(nil, false).
		Times(1)

	// Upsert и SaveSync не должны быть вызваны
	mockStorage.EXPECT().Upsert(gomock.Any()).Times(0)
	mockStorage.EXPECT().SaveSync(gomock.Any()).Times(0)

	// Act
	err := app.UpdateItem("non-existent", gophkeeperv1.ItemType_ITEM_TYPE_TEXT, []byte("title"), nil, []byte("data"))

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "item not found", err.Error())
}

func TestUpdateItem_NoDEK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          nil,
		LocalStorage: mockStorage,
	}

	// Get не должен быть вызван (проверка DEK происходит раньше)
	mockStorage.EXPECT().Get(gomock.Any()).Times(0)

	// Act
	err := app.UpdateItem("item-1", gophkeeperv1.ItemType_ITEM_TYPE_TEXT, []byte("title"), nil, []byte("data"))

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "login required (DEK missing)", err.Error())
}

func TestDeleteItem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:    nil,
		DEK:       make([]byte, 32),
		AuthState: &model.AuthState{},
		SyncState: &model.SyncState{
			LastRevision:      30,
			PendingOperations: []model.PendingOperation{},
		},
		LocalStorage: mockStorage,
	}

	existingItem := &model.Item{
		ID:      "item-to-delete",
		Version: 10,
	}

	mockStorage.EXPECT().Get("item-to-delete").
		Return(existingItem, true).
		Times(1)

	mockStorage.EXPECT().Delete("item-to-delete").
		Return(nil).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		DoAndReturn(func(syncState *model.SyncState) error {
			assert.Len(t, syncState.PendingOperations, 1)
			op := syncState.PendingOperations[0]
			assert.Equal(t, gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_DELETE, op.Type)
			assert.Equal(t, uint64(10), op.ExpectedVersion)
			return nil
		}).
		Times(1)

	// Act
	err := app.DeleteItem("item-to-delete")

	// Assert
	assert.NoError(t, err)
	assert.Len(t, app.SyncState.PendingOperations, 1)
}

func TestDeleteItem_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		DEK:          make([]byte, 32),
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().Get("non-existent").
		Return(nil, false).
		Times(1)

	// Delete и SaveSync не должны быть вызваны
	mockStorage.EXPECT().Delete(gomock.Any()).Times(0)
	mockStorage.EXPECT().SaveSync(gomock.Any()).Times(0)

	// Act
	err := app.DeleteItem("non-existent")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "item not found", err.Error())
}

func TestDecryptItem_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Генерируем валидные зашифрованные данные
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	titlePlain := []byte("My Secret Title")
	metaPlain := []byte("metadata")
	payloadPlain := []byte("super secret payload")

	titleEnc, _ := encryptField(dek, titlePlain)
	metaEnc, _ := encryptField(dek, metaPlain)
	payloadEnc, _ := encryptField(dek, payloadPlain)

	app := &App{
		Client: nil,
		DEK:    dek,
	}

	item := &model.Item{
		ID:       "test-item",
		Version:  1,
		Type:     gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
		Title:    titleEnc,
		Metadata: metaEnc,
		Payload:  payloadEnc,
	}

	// Act
	title, meta, payload, err := app.DecryptItem(item)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, string(titlePlain), title)
	assert.Equal(t, string(metaPlain), meta)
	assert.Equal(t, string(payloadPlain), payload)
}

func TestDecryptItem_NoDEK(t *testing.T) {
	app := &App{
		Client: nil,
		DEK:    nil, // Отсутствует DEK
	}

	item := &model.Item{
		ID: "test",
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("cipher"),
			Nonce:      []byte("nonce"),
		},
	}

	// Act
	title, meta, payload, err := app.DecryptItem(item)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "login required (DEK missing)", err.Error())
	assert.Empty(t, title)
	assert.Empty(t, meta)
	assert.Empty(t, payload)
}

func TestDecryptItem_InvalidData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client: nil,
		DEK:    dek,
	}

	// Невалидные зашифрованные данные (невозможно расшифровать)
	invalidItem := &model.Item{
		ID: "test",
		Title: &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("invalid-cipher"),
			Nonce:      make([]byte, 24), // Валидный размер nonce для XChaCha20
		},
	}

	// Act
	_, _, _, err := app.DecryptItem(invalidItem)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message authentication failed")
}

func TestUnlock_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	// Подготовка валидных данных для расшифровки
	password := "my-password"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(password), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	assert.NoError(t, err)

	authState := &model.AuthState{
		UserID:            "user-123",
		PasswordSalt:      passwordSalt,
		EncryptedDEK:      encryptedDEK,
		EncryptedDEKNonce: nonce,
	}

	mockStorage.EXPECT().Load().
		Return(authState, nil).
		Times(1)

	// Act
	err = app.Unlock(password)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app.DEK)
	assert.Len(t, app.DEK, 32)
	assert.Equal(t, dek, app.DEK)
	assert.Equal(t, authState, app.AuthState)
}

func TestUnlock_InvalidPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	// Данные зашифрованы с другим паролем
	correctPassword := "correct-password"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(correctPassword), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	assert.NoError(t, err)

	authState := &model.AuthState{
		PasswordSalt:      passwordSalt,
		EncryptedDEK:      encryptedDEK,
		EncryptedDEKNonce: nonce,
	}

	mockStorage.EXPECT().Load().
		Return(authState, nil).
		Times(1)

	// Act - пытаемся расшифровать с НЕПРАВИЛЬНЫМ паролем
	err = app.Unlock("wrong-password")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "invalid password", err.Error())
	assert.Nil(t, app.DEK)
}

func TestUnlock_LoadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		Client:       nil,
		LocalStorage: mockStorage,
	}

	mockStorage.EXPECT().Load().
		Return(nil, errors.New("storage unavailable")).
		Times(1)

	// Act
	err := app.Unlock("password")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "storage unavailable", err.Error())
	assert.Nil(t, app.DEK)
}

func TestEncryptField_Success(t *testing.T) {
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	data := []byte("sensitive data")

	// Act
	encField, err := encryptField(dek, data)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, encField)
	assert.NotNil(t, encField.Ciphertext)
	assert.NotNil(t, encField.Nonce)
	assert.Len(t, encField.Nonce, 24) // XChaCha20 nonce size

	// Проверяем, что можно расшифровать обратно
	decrypted, err := crypto.Decrypt(dek, encField.Nonce, encField.Ciphertext)
	assert.NoError(t, err)
	assert.Equal(t, data, decrypted)
}

func TestEncryptField_EmptyData(t *testing.T) {
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	// Act
	encField, err := encryptField(dek, []byte{})

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, encField)

	// Пустые данные тоже можно расшифровать
	decrypted, err := crypto.Decrypt(dek, encField.Nonce, encField.Ciphertext)
	assert.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestGenerateID(t *testing.T) {
	// Act
	id1 := generateID("item")
	id2 := generateID("item")
	id3 := generateID("op")

	// Assert
	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEmpty(t, id3)
	assert.Contains(t, id1, "item-")
	assert.Contains(t, id2, "item-")
	assert.Contains(t, id3, "op-")
	assert.NotEqual(t, id1, id2) // Уникальность
}

func TestGenerateID_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected string
	}{
		{"item", "item", "item-"},
		{"op", "op", "op-"},
		{"test", "test", "test-"},
		{"", "", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateID(tt.prefix)
			assert.NotEmpty(t, id)
			assert.Contains(t, id, tt.expected)
		})
	}
}

// BenchmarkAddItem измеряет производительность добавления элемента
func BenchmarkAddItem(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       nil,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// Настройка моков для бенчмарка
	mockStorage.EXPECT().Upsert(gomock.Any()).Return(nil).AnyTimes()
	mockStorage.EXPECT().SaveSync(gomock.Any()).Return(nil).AnyTimes()

	title := []byte("benchmark title")
	payload := []byte("benchmark payload")

	for b.Loop() {
		err := app.AddItem(gophkeeperv1.ItemType_ITEM_TYPE_TEXT, title, nil, payload, "")
		if err != nil {
			b.Fatal(err)
		}
		// Очищаем очередь операций для следующей итерации
		app.SyncState.PendingOperations = nil
	}
}

// BenchmarkDecryptItem измеряет производительность расшифровки
func BenchmarkDecryptItem(b *testing.B) {
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	titleEnc, _ := encryptField(dek, []byte("title"))
	metaEnc, _ := encryptField(dek, []byte("meta"))
	payloadEnc, _ := encryptField(dek, []byte("payload"))

	app := &App{
		Client: nil,
		DEK:    dek,
	}

	item := &model.Item{
		Title:    titleEnc,
		Metadata: metaEnc,
		Payload:  payloadEnc,
	}

	for b.Loop() {
		_, _, _, err := app.DecryptItem(item)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// vault_test.go

// ... существующие тесты ...

func TestUploadFile_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Создаём временный файл для загрузки
	tmpFile, err := os.CreateTemp("", "test-upload-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	testData := []byte("test file content for upload")
	_, err = tmpFile.Write(testData)
	assert.NoError(t, err)
	tmpFile.Close()

	// Генерируем DEK
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// Мокаем BlobClient
	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	// Мокаем StartBlobUpload
	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.StartBlobUploadResponse{
			UploadSessionId: "session-123",
			BlobId:          "blob-456",
			MaxChunkSize:    1024 * 1024,
		}, nil).
		Times(1)

	// Мокаем UploadBlob stream
	mockUploadStream := mocks.NewMockBlobService_UploadBlobClient(ctrl)
	mockBlobClient.EXPECT().UploadBlob(gomock.Any()).
		Return(mockUploadStream, nil).
		Times(1)

	// Ожидаем Send для header
	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(1)
	// Ожидаем Send для chunks (1 чанк для маленьких данных)
	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(1)
	// Ожидаем CloseAndRecv
	mockUploadStream.EXPECT().CloseAndRecv().
		Return(&gophkeeperv1.UploadBlobResponse{
			Status: gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED,
		}, nil).
		Times(1)

	// Act
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, "blob-456", blobID)
}

func TestUploadFile_NoDEK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	app := &App{
		Client:       mockClient,
		DEK:          nil, // Отсутствует DEK
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// Создаём временный файл
	tmpFile, err := os.CreateTemp("", "test-upload-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// BlobClient не должен быть вызван
	mockClient.EXPECT().BlobClient().Times(0)

	// Act
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "login required (DEK missing)", err.Error())
	assert.Empty(t, blobID)
}

func TestUploadFile_FileNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// BlobClient не должен быть вызван (ошибка до этого)
	mockClient.EXPECT().BlobClient().Times(0)

	// Act
	blobID, err := app.UploadFile(t.Context(), "/nonexistent/path/file.txt", "")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
	assert.Empty(t, blobID)
}

func TestUploadFile_StartUploadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Создаём временный файл
	tmpFile, err := os.CreateTemp("", "test-upload-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	// Мокаем ошибку StartBlobUpload
	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("upload service unavailable")).
		Times(1)

	// Act
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upload service unavailable")
	assert.Empty(t, blobID)
}

func TestUploadFile_UploadStreamError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Создаём временный файл
	tmpFile, err := os.CreateTemp("", "test-upload-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.StartBlobUploadResponse{
			UploadSessionId: "session-123",
			BlobId:          "blob-456",
			MaxChunkSize:    1024 * 1024,
		}, nil).
		Times(1)

	// Мокаем ошибку создания stream
	mockBlobClient.EXPECT().UploadBlob(gomock.Any()).
		Return(nil, errors.New("stream creation failed")).
		Times(1)

	// Act
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream creation failed")
	assert.Empty(t, blobID)
}

func TestDownloadFile_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Генерируем DEK и тестовые данные
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	testData := []byte("test file content for download")
	nonce, encryptedData, err := crypto.Encrypt(dek, testData)
	assert.NoError(t, err)
	blobData := append(nonce, encryptedData...)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	// Мокаем DownloadBlob stream
	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		Times(1)

	// Ожидаем Recv для header
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Header{
				Header: &gophkeeperv1.DownloadBlobHeader{
					Blob: &gophkeeperv1.BlobInfo{
						BlobId: "blob-456",
					},
				},
			},
		}, nil).
		Times(1)

	// Ожидаем Recv для chunks (1 чанк для маленьких данных)
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
				Chunk: &gophkeeperv1.DownloadBlobChunk{
					ChunkIndex: 0,
					Data:       blobData,
				},
			},
		}, nil).
		Times(1)

	// Ожидаем EOF
	mockDownloadStream.EXPECT().Recv().
		Return(nil, io.EOF).
		Times(1)

	// Act
	downloadedData, err := app.DownloadFile(t.Context(), "blob-456")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, testData, downloadedData)
}

func TestDownloadFile_NoDEK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	app := &App{
		Client:       mockClient,
		DEK:          nil, // Отсутствует DEK
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	// BlobClient не должен быть вызван
	mockClient.EXPECT().BlobClient().Times(0)

	// Act
	data, err := app.DownloadFile(t.Context(), "blob-456")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "login required (DEK missing)", err.Error())
	assert.Nil(t, data)
}

func TestDownloadFile_BlobNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	// Мокаем ошибку DownloadBlob
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("blob not found")).
		Times(1)

	// Act
	data, err := app.DownloadFile(t.Context(), "nonexistent-blob")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blob not found")
	assert.Nil(t, data)
}

func TestDownloadFile_InvalidHeader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		Times(1)

	// Возвращаем ответ без header
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{}, nil).
		Times(1)

	// Act
	data, err := app.DownloadFile(t.Context(), "blob-456")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no header")
	assert.Nil(t, data)
}

func TestDownloadFile_DataTooShort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		Times(1)

	// Header
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Header{
				Header: &gophkeeperv1.DownloadBlobHeader{
					Blob: &gophkeeperv1.BlobInfo{BlobId: "blob-456"},
				},
			},
		}, nil).
		Times(1)

	// Chunk с недостаточными данными (меньше nonce)
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
				Chunk: &gophkeeperv1.DownloadBlobChunk{
					Data: []byte("too-short"),
				},
			},
		}, nil).
		Times(1)

	// EOF
	mockDownloadStream.EXPECT().Recv().
		Return(nil, io.EOF).
		Times(1)

	// Act
	data, err := app.DownloadFile(t.Context(), "blob-456")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
	assert.Nil(t, data)
}

func TestDownloadFile_DecryptionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		Times(1)

	// Header
	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Header{
				Header: &gophkeeperv1.DownloadBlobHeader{
					Blob: &gophkeeperv1.BlobInfo{BlobId: "blob-456"},
				},
			},
		}, nil).
		Times(1)

	// Chunk с невалидными зашифрованными данными
	invalidNonce := make([]byte, 24)
	invalidCiphertext := []byte("invalid-ciphertext-data")
	invalidBlobData := append(invalidNonce, invalidCiphertext...)

	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
				Chunk: &gophkeeperv1.DownloadBlobChunk{
					Data: invalidBlobData,
				},
			},
		}, nil).
		Times(1)

	// EOF
	mockDownloadStream.EXPECT().Recv().
		Return(nil, io.EOF).
		Times(1)

	// Act
	data, err := app.DownloadFile(t.Context(), "blob-456")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message authentication failed")
	assert.Nil(t, data)
}

func TestUploadDownloadFile_RoundTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Создаём временный файл
	tmpFile, err := os.CreateTemp("", "test-roundtrip-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	originalData := []byte("round trip test data for upload and download")
	_, err = tmpFile.Write(originalData)
	assert.NoError(t, err)
	tmpFile.Close()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	// === UPLOAD ===
	nonce, encryptedData, err := crypto.Encrypt(dek, originalData)
	assert.NoError(t, err)
	blobData := append(nonce, encryptedData...)

	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.StartBlobUploadResponse{
			UploadSessionId: "session-123",
			BlobId:          "blob-456",
			MaxChunkSize:    1024 * 1024,
		}, nil).
		Times(1)

	mockUploadStream := mocks.NewMockBlobService_UploadBlobClient(ctrl)
	mockBlobClient.EXPECT().UploadBlob(gomock.Any()).
		Return(mockUploadStream, nil).
		Times(1)

	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(1) // header
	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(1) // chunk
	mockUploadStream.EXPECT().CloseAndRecv().
		Return(&gophkeeperv1.UploadBlobResponse{
			Status: gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED,
		}, nil).
		Times(1)

	// Act - Upload
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")
	assert.NoError(t, err)
	assert.Equal(t, "blob-456", blobID)

	// === DOWNLOAD ===
	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		Times(1)

	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Header{
				Header: &gophkeeperv1.DownloadBlobHeader{
					Blob: &gophkeeperv1.BlobInfo{BlobId: "blob-456"},
				},
			},
		}, nil).
		Times(1)

	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
				Chunk: &gophkeeperv1.DownloadBlobChunk{
					Data: blobData,
				},
			},
		}, nil).
		Times(1)

	mockDownloadStream.EXPECT().Recv().
		Return(nil, io.EOF).
		Times(1)

	// Act - Download
	downloadedData, err := app.DownloadFile(t.Context(), blobID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, originalData, downloadedData)
}

func TestUploadFile_LargeFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	// Создаём временный файл с большими данными
	tmpFile, err := os.CreateTemp("", "test-large-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// 2 MB данных (больше чем maxChunkSize 1 MB)
	largeData := make([]byte, 2*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	_, err = tmpFile.Write(largeData)
	assert.NoError(t, err)
	tmpFile.Close()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		AuthState:    &model.AuthState{},
		SyncState:    &model.SyncState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.StartBlobUploadResponse{
			UploadSessionId: "session-123",
			BlobId:          "blob-456",
			MaxChunkSize:    1024 * 1024, // 1 MB chunks
		}, nil).
		Times(1)

	mockUploadStream := mocks.NewMockBlobService_UploadBlobClient(ctrl)
	mockBlobClient.EXPECT().UploadBlob(gomock.Any()).
		Return(mockUploadStream, nil).
		Times(1)

	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(1) // header
	// Ожидаем 3 чанка (nonce + 2 MB encrypted data)
	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).Times(3)
	mockUploadStream.EXPECT().CloseAndRecv().
		Return(&gophkeeperv1.UploadBlobResponse{
			Status: gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED,
		}, nil).
		Times(1)

	// Act
	blobID, err := app.UploadFile(t.Context(), tmpFile.Name(), "")

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, "blob-456", blobID)
}

// BenchmarkUploadFile измеряет производительность загрузки файла
func BenchmarkUploadFile(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	tmpFile, err := os.CreateTemp("", "benchmark-*.txt")
	assert.NoError(b, err)
	defer os.Remove(tmpFile.Name())

	testData := make([]byte, 1024) // 1 KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	_, err = tmpFile.Write(testData)
	assert.NoError(b, err)
	tmpFile.Close()

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()
	mockBlobClient.EXPECT().StartBlobUpload(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.StartBlobUploadResponse{
			UploadSessionId: "session-123",
			BlobId:          "blob-456",
			MaxChunkSize:    1024 * 1024,
		}, nil).
		AnyTimes()

	mockUploadStream := mocks.NewMockBlobService_UploadBlobClient(ctrl)
	mockBlobClient.EXPECT().UploadBlob(gomock.Any()).
		Return(mockUploadStream, nil).
		AnyTimes()

	mockUploadStream.EXPECT().Send(gomock.Any()).Return(nil).AnyTimes()
	mockUploadStream.EXPECT().CloseAndRecv().
		Return(&gophkeeperv1.UploadBlobResponse{
			Status: gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED,
		}, nil).
		AnyTimes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := app.UploadFile(b.Context(), tmpFile.Name(), "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDownloadFile измеряет производительность выгрузки файла
func BenchmarkDownloadFile(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	mockBlobClient := mocks.NewMockBlobServiceClient(ctrl)
	mockClient := mocks.NewMockClient(ctrl)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)

	testData := make([]byte, 1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	nonce, encryptedData, _ := crypto.Encrypt(dek, testData)
	blobData := append(nonce, encryptedData...)

	app := &App{
		Client:       mockClient,
		DEK:          dek,
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().BlobClient().Return(mockBlobClient).AnyTimes()

	mockDownloadStream := mocks.NewMockBlobService_DownloadBlobClient(ctrl)
	mockBlobClient.EXPECT().DownloadBlob(gomock.Any(), gomock.Any()).
		Return(mockDownloadStream, nil).
		AnyTimes()

	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Header{
				Header: &gophkeeperv1.DownloadBlobHeader{
					Blob: &gophkeeperv1.BlobInfo{BlobId: "blob-456"},
				},
			},
		}, nil).
		AnyTimes()

	mockDownloadStream.EXPECT().Recv().
		Return(&gophkeeperv1.DownloadBlobResponse{
			Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
				Chunk: &gophkeeperv1.DownloadBlobChunk{
					Data: blobData,
				},
			},
		}, nil).
		AnyTimes()

	mockDownloadStream.EXPECT().Recv().
		Return(nil, io.EOF).
		AnyTimes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := app.DownloadFile(b.Context(), "blob-456")
		if err != nil {
			b.Fatal(err)
		}
	}
}
