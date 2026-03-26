package app

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/prbllm/goph-keeper/internal/mocks"
	"github.com/prbllm/goph-keeper/internal/model"
	"github.com/prbllm/goph-keeper/internal/storage"
	"github.com/prbllm/goph-keeper/internal/transport"
	"github.com/stretchr/testify/assert"
)

func TestNew_Success(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	expectedAuth := &model.AuthState{
		UserID:      "test-user",
		AccessToken: "test-token",
	}

	expectedSync := &model.SyncState{
		LastRevision: 42,
	}

	mockStorage.EXPECT().Load().Return(expectedAuth, nil)
	mockStorage.EXPECT().LoadSync().Return(expectedSync, nil)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.Equal(t, mockClient, app.Client)
	assert.Equal(t, mockStorage, app.LocalStorage)
	assert.Equal(t, expectedAuth, app.AuthState)
	assert.Equal(t, expectedSync, app.SyncState)
}

func TestNew_LoadAuthStateError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockStorage.EXPECT().Load().Return(nil, assert.AnError)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, app)
}

func TestNew_LoadSyncStateError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockStorage.EXPECT().Load().Return(&model.AuthState{}, nil)
	mockStorage.EXPECT().LoadSync().Return(nil, assert.AnError)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, app)
}

func TestNew_EmptyStates(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockStorage.EXPECT().Load().Return(&model.AuthState{}, nil)
	mockStorage.EXPECT().LoadSync().Return(&model.SyncState{}, nil)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotNil(t, app.AuthState)
	assert.NotNil(t, app.SyncState)
	assert.Empty(t, app.AuthState.UserID)
	assert.Empty(t, app.AuthState.AccessToken)
	assert.Equal(t, uint64(0), app.SyncState.LastRevision)
}

func TestNew_WithExistingAuthState(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	expectedAuth := &model.AuthState{
		UserID:       "user-123",
		AccessToken:  "access-token-abc",
		RefreshToken: "refresh-token-xyz",
		DeviceID:     "device-cli-1",
	}

	mockStorage.EXPECT().Load().Return(expectedAuth, nil)
	mockStorage.EXPECT().LoadSync().Return(&model.SyncState{}, nil)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.Equal(t, "user-123", app.AuthState.UserID)
	assert.Equal(t, "access-token-abc", app.AuthState.AccessToken)
	assert.Equal(t, "refresh-token-xyz", app.AuthState.RefreshToken)
	assert.Equal(t, "device-cli-1", app.AuthState.DeviceID)
}

func TestNew_WithExistingSyncState(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	expectedSync := &model.SyncState{
		LastRevision: 100,
		PendingOperations: []model.PendingOperation{
			{
				OperationID:     "op-1",
				Type:            1, // PENDING_OPERATION_TYPE_CREATE
				ItemID:          "item-1",
				ExpectedVersion: 0,
			},
			{
				OperationID:     "op-2",
				Type:            2, // PENDING_OPERATION_TYPE_UPDATE
				ItemID:          "item-2",
				ExpectedVersion: 5,
			},
		},
	}

	mockStorage.EXPECT().Load().Return(&model.AuthState{}, nil)
	mockStorage.EXPECT().LoadSync().Return(expectedSync, nil)

	// Act
	app, err := New(mockClient, mockStorage)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.Equal(t, uint64(100), app.SyncState.LastRevision)
	assert.Len(t, app.SyncState.PendingOperations, 2)
	assert.Equal(t, "op-1", app.SyncState.PendingOperations[0].OperationID)
	assert.Equal(t, "op-2", app.SyncState.PendingOperations[1].OperationID)
}

// Table-driven тесты
func TestNew_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(ctrl *gomock.Controller) (transport.Client, storage.LocalStorage)
		wantErr    bool
		checkApp   func(t *testing.T, app *App)
	}{
		{
			name: "valid_states",
			setupMocks: func(ctrl *gomock.Controller) (transport.Client, storage.LocalStorage) {
				client := mocks.NewMockClient(ctrl)
				storage := mocks.NewMockLocalStorage(ctrl)

				storage.EXPECT().Load().Return(&model.AuthState{
					UserID: "test-user",
				}, nil)
				storage.EXPECT().LoadSync().Return(&model.SyncState{
					LastRevision: 10,
				}, nil)

				return client, storage
			},
			wantErr: false,
			checkApp: func(t *testing.T, app *App) {
				assert.NotNil(t, app.LocalStorage)
				assert.NotNil(t, app.AuthState)
				assert.NotNil(t, app.SyncState)
				assert.Equal(t, "test-user", app.AuthState.UserID)
				assert.Equal(t, uint64(10), app.SyncState.LastRevision)
			},
		},
		{
			name: "load_auth_fails",
			setupMocks: func(ctrl *gomock.Controller) (transport.Client, storage.LocalStorage) {
				client := mocks.NewMockClient(ctrl)
				storage := mocks.NewMockLocalStorage(ctrl)

				storage.EXPECT().Load().Return(nil, assert.AnError)

				return client, storage
			},
			wantErr: true,
		},
		{
			name: "load_sync_fails",
			setupMocks: func(ctrl *gomock.Controller) (transport.Client, storage.LocalStorage) {
				client := mocks.NewMockClient(ctrl)
				storage := mocks.NewMockLocalStorage(ctrl)

				storage.EXPECT().Load().Return(&model.AuthState{}, nil)
				storage.EXPECT().LoadSync().Return(nil, assert.AnError)

				return client, storage
			},
			wantErr: true,
		},
		{
			name: "both_empty",
			setupMocks: func(ctrl *gomock.Controller) (transport.Client, storage.LocalStorage) {
				client := mocks.NewMockClient(ctrl)
				storage := mocks.NewMockLocalStorage(ctrl)

				storage.EXPECT().Load().Return(&model.AuthState{}, nil)
				storage.EXPECT().LoadSync().Return(&model.SyncState{}, nil)

				return client, storage
			},
			wantErr: false,
			checkApp: func(t *testing.T, app *App) {
				assert.NotNil(t, app)
				assert.Empty(t, app.AuthState.UserID)
				assert.Equal(t, uint64(0), app.SyncState.LastRevision)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client, storage := tt.setupMocks(ctrl)

			app, err := New(client, storage)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, app)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, app)
				if tt.checkApp != nil {
					tt.checkApp(t, app)
				}
			}
		})
	}
}

// BenchmarkNew тестирует производительность инициализации приложения
func BenchmarkNew(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockStorage.EXPECT().Load().Return(&model.AuthState{}, nil).AnyTimes()
	mockStorage.EXPECT().LoadSync().Return(&model.SyncState{}, nil).AnyTimes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := New(mockClient, mockStorage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ExampleNew демонстрирует использование функции New
func ExampleNew() {
	ctrl := gomock.NewController(nil)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockStorage.EXPECT().Load().Return(&model.AuthState{}, nil)
	mockStorage.EXPECT().LoadSync().Return(&model.SyncState{}, nil)

	app, err := New(mockClient, mockStorage)
	if err != nil {
		panic(err)
	}

	_ = app
	// Output:
}
