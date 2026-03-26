package app

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
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
			Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
			ItemID:          "item-1",
			ExpectedVersion: 0,
			Snapshot: &gophkeeperv1.VaultItemSnapshot{
				ItemType: gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
				Title: &gophkeeperv1.EncryptedField{
					Ciphertext: []byte("encrypted-title"),
					Nonce:      []byte("nonce-1"),
				},
			},
		},
		{
			OperationID:     "op-2",
			Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
			ItemID:          "item-2",
			ExpectedVersion: 5,
			Snapshot: &gophkeeperv1.VaultItemSnapshot{
				ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
				Title: &gophkeeperv1.EncryptedField{
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
		DoAndReturn(func(ctx interface{}, req *gophkeeperv1.SyncRequest, opts ...grpc.CallOption) (*gophkeeperv1.SyncResponse, error) {
			// Проверяем корректность запроса
			assert.Equal(t, uint64(42), req.ClientRevision)
			assert.Len(t, req.PendingOperations, 2)
			assert.Equal(t, "op-1", req.PendingOperations[0].OperationId)
			assert.Equal(t, "op-2", req.PendingOperations[1].OperationId)
			return &gophkeeperv1.SyncResponse{
				NewServerRevision: 100,
				RemoteChanges:     []*gophkeeperv1.RevisionEvent{},
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

	remoteEvent := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED,
		Item: &gophkeeperv1.VaultItem{
			ItemId:   "remote-item-1",
			Version:  1,
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
			Title: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("title-cipher"),
				Nonce:      []byte("title-nonce"),
			},
			Metadata: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("meta-cipher"),
				Nonce:      []byte("meta-nonce"),
			},
			Payload: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("payload-cipher"),
				Nonce:      []byte("payload-nonce"),
			},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.SyncResponse{
			NewServerRevision: 51,
			RemoteChanges:     []*gophkeeperv1.RevisionEvent{remoteEvent},
		}, nil).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "remote-item-1", item.ID)
			assert.Equal(t, uint64(1), item.Version)
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_TEXT, item.Type)
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

	remoteEvent := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_UPDATED,
		Item: &gophkeeperv1.VaultItem{
			ItemId:   "existing-item",
			Version:  10,
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("new-title"),
				Nonce:      []byte("new-nonce"),
			},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.SyncResponse{
			NewServerRevision: 76,
			RemoteChanges:     []*gophkeeperv1.RevisionEvent{remoteEvent},
		}, nil).
		Times(1)

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "existing-item", item.ID)
			assert.Equal(t, uint64(10), item.Version)
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL, item.Type)
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

	remoteEvent := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_DELETED,
		Item: &gophkeeperv1.VaultItem{
			ItemId:  "deleted-item",
			Version: 15,
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.SyncResponse{
			NewServerRevision: 91,
			RemoteChanges:     []*gophkeeperv1.RevisionEvent{remoteEvent},
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

	events := []*gophkeeperv1.RevisionEvent{
		{
			ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED,
			Item:       &gophkeeperv1.VaultItem{ItemId: "item-1", Version: 1},
		},
		{
			ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_UPDATED,
			Item:       &gophkeeperv1.VaultItem{ItemId: "item-2", Version: 5},
		},
		{
			ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_DELETED,
			Item:       &gophkeeperv1.VaultItem{ItemId: "item-3", Version: 3},
		},
	}

	mockClient.EXPECT().SyncClient().Return(mockSyncClient).Times(1)
	mockSyncClient.EXPECT().Sync(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.SyncResponse{
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
		DoAndReturn(func(ctx interface{}, req *gophkeeperv1.SyncRequest, opts ...grpc.CallOption) (*gophkeeperv1.SyncResponse, error) {
			assert.Empty(t, req.PendingOperations) // Нет операций в запросе
			assert.Equal(t, uint64(200), req.ClientRevision)
			return &gophkeeperv1.SyncResponse{
				NewServerRevision: 201,
				RemoteChanges:     []*gophkeeperv1.RevisionEvent{},
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
		Return(&gophkeeperv1.SyncResponse{
			NewServerRevision: 301,
			RemoteChanges:     []*gophkeeperv1.RevisionEvent{},
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

	event := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED,
		Item: &gophkeeperv1.VaultItem{
			ItemId:   "test-item",
			Version:  1,
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_TEXT,
			Title: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("cipher"),
				Nonce:      []byte("nonce"),
			},
			Metadata: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("meta"),
				Nonce:      []byte("meta-nonce"),
			},
			Payload: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("payload"),
				Nonce:      []byte("payload-nonce"),
			},
		},
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "test-item", item.ID)
			assert.Equal(t, uint64(1), item.Version)
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_TEXT, item.Type)
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

	event := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_UPDATED,
		Item: &gophkeeperv1.VaultItem{
			ItemId:   "updated-item",
			Version:  10,
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CARD,
			Title: &gophkeeperv1.EncryptedField{
				Ciphertext: []byte("new-title"),
				Nonce:      []byte("new-nonce"),
			},
		},
	}

	mockStorage.EXPECT().Upsert(gomock.Any()).
		DoAndReturn(func(item *model.Item) error {
			assert.Equal(t, "updated-item", item.ID)
			assert.Equal(t, uint64(10), item.Version)
			assert.Equal(t, gophkeeperv1.ItemType_ITEM_TYPE_CARD, item.Type)
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

	event := &gophkeeperv1.RevisionEvent{
		ChangeType: gophkeeperv1.ChangeType_CHANGE_TYPE_DELETED,
		Item: &gophkeeperv1.VaultItem{
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
	event := &gophkeeperv1.RevisionEvent{
		ChangeType: 999, // Неизвестный тип
		Item:       &gophkeeperv1.VaultItem{ItemId: "test", Version: 1},
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
		changeType   gophkeeperv1.ChangeType
		expectUpsert bool
		checkItem    func(t *testing.T, item *model.Item)
	}{
		{
			name:         "created",
			changeType:   gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.False(t, item.Deleted)
			},
		},
		{
			name:         "updated",
			changeType:   gophkeeperv1.ChangeType_CHANGE_TYPE_UPDATED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.False(t, item.Deleted)
			},
		},
		{
			name:         "deleted",
			changeType:   gophkeeperv1.ChangeType_CHANGE_TYPE_DELETED,
			expectUpsert: true,
			checkItem: func(t *testing.T, item *model.Item) {
				assert.True(t, item.Deleted)
			},
		},
		{
			name:         "unknown_type",
			changeType:   gophkeeperv1.ChangeType(999),
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

			event := &gophkeeperv1.RevisionEvent{
				ChangeType: tt.changeType,
				Item: &gophkeeperv1.VaultItem{
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
		Return(&gophkeeperv1.SyncResponse{
			NewServerRevision: 1001,
			RemoteChanges:     []*gophkeeperv1.RevisionEvent{},
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
