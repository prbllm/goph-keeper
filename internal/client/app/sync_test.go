package app

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/prbllm/goph-keeper/api"
	"github.com/prbllm/goph-keeper/internal/client/mocks"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestSync_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	// Подготовка состояния с ожидающими операциями
	pendingOps := []model.PendingOperation{
		{
			OperationID:     "op-1",
			Type:            api.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
			ItemID:          "item-1",
			ExpectedVersion: 0,
			Snapshot: &api.VaultItemSnapshot{
				ItemType: api.ItemType_ITEM_TYPE_TEXT,
				Title: &api.EncryptedField{
					Ciphertext: []byte("encrypted-title"),
					Nonce:      []byte("nonce-1"),
				},
			},
		},
		{
			OperationID:     "op-2",
			Type:            api.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
			ItemID:          "item-2",
			ExpectedVersion: 5,
			Snapshot: &api.VaultItemSnapshot{
				ItemType: api.ItemType_ITEM_TYPE_CREDENTIAL,
				Title: &api.EncryptedField{
					Ciphertext: []byte("encrypted-title-2"),
					Nonce:      []byte("nonce-2"),
				},
			},
		},
	}

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      42,
			PendingOperations: pendingOps,
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	// Настройка моков
	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx interface{}, req *api.SyncRequest, opts ...grpc.CallOption) (*api.SyncResponse, error) {
			// Проверяем корректность запроса
			assert.Equal(t, uint64(42), req.ClientRevision)
			assert.Len(t, req.PendingOperations, 2)
			assert.Equal(t, "op-1", req.PendingOperations[0].OperationId)
			assert.Equal(t, "op-2", req.PendingOperations[1].OperationId)
			return &api.SyncResponse{
				NewServerRevision: 100,
				RemoteChanges:     []*api.RevisionEvent{},
			}, nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		DoAndReturn(func(syncState *model.SyncState) error {
			assert.Equal(t, uint64(100), syncState.LastRevision)
			assert.Empty(t, syncState.PendingOperations) // Операции очищены
			return nil
		}).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), app.SyncState.LastRevision)
	assert.Empty(t, app.SyncState.PendingOperations)
}

func TestSync_ServerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      10,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection timeout")).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "connection timeout", err.Error())
	// Состояние не должно измениться
	assert.Equal(t, uint64(10), app.SyncState.LastRevision)
}

func TestSync_ApplyRemoteChanges_Created(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      50,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	remoteEvent := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_CREATED,
		Item: &api.VaultItem{
			ItemId:   "remote-item-1",
			Version:  1,
			ItemType: api.ItemType_ITEM_TYPE_TEXT,
			Title: &api.EncryptedField{
				Ciphertext: []byte("title-cipher"),
				Nonce:      []byte("title-nonce"),
			},
			Metadata: &api.EncryptedField{
				Ciphertext: []byte("meta-cipher"),
				Nonce:      []byte("meta-nonce"),
			},
			Payload: &api.EncryptedField{
				Ciphertext: []byte("payload-cipher"),
				Nonce:      []byte("payload-nonce"),
			},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 51,
			RemoteChanges:     []*api.RevisionEvent{remoteEvent},
		}, nil).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "remote-item-1", item.ID)
			assert.Equal(t, uint64(1), item.Version)
			assert.Equal(t, api.ItemType_ITEM_TYPE_TEXT, item.Type)
			assert.False(t, item.Deleted)
			return nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		Return(nil).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(51), app.SyncState.LastRevision)
}

func TestSync_ApplyRemoteChanges_Updated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      75,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	remoteEvent := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_UPDATED,
		Item: &api.VaultItem{
			ItemId:   "existing-item",
			Version:  10,
			ItemType: api.ItemType_ITEM_TYPE_CREDENTIAL,
			Title: &api.EncryptedField{
				Ciphertext: []byte("new-title"),
				Nonce:      []byte("new-nonce"),
			},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 76,
			RemoteChanges:     []*api.RevisionEvent{remoteEvent},
		}, nil).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "existing-item", item.ID)
			assert.Equal(t, uint64(10), item.Version)
			assert.Equal(t, api.ItemType_ITEM_TYPE_CREDENTIAL, item.Type)
			assert.False(t, item.Deleted)
			return nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		Return(nil).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(76), app.SyncState.LastRevision)
}

func TestSync_ApplyRemoteChanges_Deleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      90,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	remoteEvent := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_DELETED,
		Item: &api.VaultItem{
			ItemId:  "deleted-item",
			Version: 15,
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 91,
			RemoteChanges:     []*api.RevisionEvent{remoteEvent},
		}, nil).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "deleted-item", item.ID)
			assert.Equal(t, uint64(15), item.Version)
			assert.True(t, item.Deleted) // Мягкое удаление
			return nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		Return(nil).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(91), app.SyncState.LastRevision)
}

func TestSync_MultipleRemoteChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      100,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	events := []*api.RevisionEvent{
		{
			ChangeType: api.ChangeType_CHANGE_TYPE_CREATED,
			Item:       &api.VaultItem{ItemId: "item-1", Version: 1},
		},
		{
			ChangeType: api.ChangeType_CHANGE_TYPE_UPDATED,
			Item:       &api.VaultItem{ItemId: "item-2", Version: 5},
		},
		{
			ChangeType: api.ChangeType_CHANGE_TYPE_DELETED,
			Item:       &api.VaultItem{ItemId: "item-3", Version: 3},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 103,
			RemoteChanges:     events,
		}, nil).
		Times(1)

	// Upsert вызывается 3 раза для каждого события
	mockStorage.EXPECT().Upsert(gomock.Any()).Return(nil).Times(3)
	mockStorage.EXPECT().SaveSync(gomock.Any()).
		DoAndReturn(func(syncState *model.SyncState) error {
			assert.Equal(t, uint64(103), syncState.LastRevision)
			return nil
		}).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(103), app.SyncState.LastRevision)
}

func TestSync_NoPendingOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      200,
			PendingOperations: []model.PendingOperation{}, // Нет операций
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx interface{}, req *api.SyncRequest, opts ...grpc.CallOption) (*api.SyncResponse, error) {
			assert.Empty(t, req.PendingOperations) // Нет операций в запросе
			assert.Equal(t, uint64(200), req.ClientRevision)
			return &api.SyncResponse{
				NewServerRevision: 201,
				RemoteChanges:     []*api.RevisionEvent{},
			}, nil
		}).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		Return(nil).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, uint64(201), app.SyncState.LastRevision)
}

func TestSync_SaveSyncError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      300,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 301,
			RemoteChanges:     []*api.RevisionEvent{},
		}, nil).
		Times(1)

	mockStorage.EXPECT().SaveSync(gomock.Any()).
		Return(errors.New("disk full")).
		Times(1)

	// Act
	err := app.Sync()

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "disk full", err.Error())
	// Ревизия всё равно обновлена в памяти
	assert.Equal(t, uint64(301), app.SyncState.LastRevision)
	assert.Empty(t, app.SyncState.PendingOperations)
}

func TestApplyEvent_Created(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		LocalStorage: mockStorage,
	}

	event := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_CREATED,
		Item: &api.VaultItem{
			ItemId:   "test-item",
			Version:  1,
			ItemType: api.ItemType_ITEM_TYPE_TEXT,
			Title: &api.EncryptedField{
				Ciphertext: []byte("cipher"),
				Nonce:      []byte("nonce"),
			},
			Metadata: &api.EncryptedField{
				Ciphertext: []byte("meta"),
				Nonce:      []byte("meta-nonce"),
			},
			Payload: &api.EncryptedField{
				Ciphertext: []byte("payload"),
				Nonce:      []byte("payload-nonce"),
			},
		},
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "test-item", item.ID)
			assert.Equal(t, uint64(1), item.Version)
			assert.Equal(t, api.ItemType_ITEM_TYPE_TEXT, item.Type)
			assert.Equal(t, []byte("cipher"), item.Title.Ciphertext)
			assert.Equal(t, []byte("nonce"), item.Title.Nonce)
			assert.False(t, item.Deleted)
			return nil
		}).
		Times(1)

	// Act
	app.applyEvent(event)

	// Assert: проверка вызова мока выше
}

func TestApplyEvent_Updated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		LocalStorage: mockStorage,
	}

	event := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_UPDATED,
		Item: &api.VaultItem{
			ItemId:   "updated-item",
			Version:  10,
			ItemType: api.ItemType_ITEM_TYPE_CARD,
			Title: &api.EncryptedField{
				Ciphertext: []byte("new-title"),
				Nonce:      []byte("new-nonce"),
			},
		},
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "updated-item", item.ID)
			assert.Equal(t, uint64(10), item.Version)
			assert.Equal(t, api.ItemType_ITEM_TYPE_CARD, item.Type)
			assert.False(t, item.Deleted)
			return nil
		}).
		Times(1)

	// Act
	app.applyEvent(event)
}

func TestApplyEvent_Deleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		LocalStorage: mockStorage,
	}

	event := &api.RevisionEvent{
		ChangeType: api.ChangeType_CHANGE_TYPE_DELETED,
		Item: &api.VaultItem{
			ItemId:  "deleted-item",
			Version: 5,
		},
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "deleted-item", item.ID)
			assert.Equal(t, uint64(5), item.Version)
			assert.True(t, item.Deleted) // Флаг удаления установлен
			// Поля данных могут быть nil для удалённых элементов
			return nil
		}).
		Times(1)

	// Act
	app.applyEvent(event)
}

func TestApplyEvent_UnknownChangeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockLocalStorage(ctrl)
	app := &App{
		LocalStorage: mockStorage,
	}

	// Неизвестный тип изменения (не должен вызывать Upsert)
	event := &api.RevisionEvent{
		ChangeType: 999, // Неизвестный тип
		Item:       &api.VaultItem{ItemId: "test", Version: 1},
	}

	// Upsert не должен быть вызван
	mockStorage.EXPECT().Upsert(gomock.Any()).Times(0)

	// Act
	app.applyEvent(event)

	// Assert: нет паники, нет вызова Upsert
}

// Table-driven тест для различных типов изменений
func TestApplyEvent_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		changeType   api.ChangeType
		expectUpsert bool
		checkItem    func(t *testing.T, item *model.Item)
	}{
		{
			name:         "created",
			changeType:   api.ChangeType_CHANGE_TYPE_CREATED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.False(t, item.Deleted)
			},
		},
		{
			name:         "updated",
			changeType:   api.ChangeType_CHANGE_TYPE_UPDATED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.False(t, item.Deleted)
			},
		},
		{
			name:         "deleted",
			changeType:   api.ChangeType_CHANGE_TYPE_DELETED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.True(t, item.Deleted)
			},
		},
		{
			name:         "unknown_type",
			changeType:   api.ChangeType(999),
			expectUpsert: false,
			checkItem:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := mocks.NewMockLocalStorage(ctrl)
			app := &App{LocalStorage: mockStorage}

			event := &api.RevisionEvent{
				ChangeType: tt.changeType,
				Item: &api.VaultItem{
					ItemId:  "test-item",
					Version: 1,
				},
			}

			if tt.expectUpsert {
				mockStorage.EXPECT().Upsert(gomock.Any()).
					DoAndReturn(func(item *model.Item) error {
						if tt.checkItem != nil {
							tt.checkItem(t, item)
						}
						return nil
					}).
					Times(1)
			} else {
				mockStorage.EXPECT().Upsert(gomock.Any()).Times(0)
			}

			app.applyEvent(event)
		})
	}
}

// BenchmarkSync измеряет производительность синхронизации
func BenchmarkSync(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockSyncClient := mocks.NewMockSyncServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	// Подготовка состояния
	app := &App{
		Client: mockClient,
		SyncState: &model.SyncState{
			LastRevision:      1000,
			PendingOperations: []model.PendingOperation{},
		},
		AuthState:    &model.AuthState{},
		LocalStorage: mockStorage,
	}

	// Настройка моков для бенчмарка
	mockClient.EXPECT().SyncClient().Return(mockSyncClient).AnyTimes()
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&api.SyncResponse{
			NewServerRevision: 1001,
			RemoteChanges:     []*api.RevisionEvent{},
		}, nil).
		AnyTimes()
	mockStorage.EXPECT().SaveSync(gomock.Any()).Return(nil).AnyTimes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := app.Sync()
		if err != nil {
			b.Fatal(err)
		}
		// Восстанавливаем ревизию для следующей итерации
		app.SyncState.LastRevision = 1000 + uint64(i)
	}
}
