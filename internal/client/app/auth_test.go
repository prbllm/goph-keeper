package app

import (
	"crypto/rand"
	"errors"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/crypto"
	"github.com/prbllm/goph-keeper/internal/client/mocks"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/stretchr/testify/assert"
)

// TestRegister_Success тестирует успешную регистрацию пользователя
func TestRegister_Success(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	// Настройка моков
	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.RegisterResponse{
			User: &gophkeeperv1.UserInfo{
				UserId: "user-123",
			},
			AccessToken:  "access-token-abc",
			RefreshToken: "refresh-token-xyz",
			DeviceId:     "device-cli-1",
		}, nil).
		Times(1)

	mockStorage.EXPECT().Save(gomock.Any()).
		DoAndReturn(func(authState *model.AuthState) error {
			assert.Equal(t, "user-123", authState.UserID)
			assert.Equal(t, "access-token-abc", authState.AccessToken)
			assert.Equal(t, "refresh-token-xyz", authState.RefreshToken)
			assert.Equal(t, "device-cli-1", authState.DeviceID)
			return nil
		}).
		Times(1)

	// Создание приложения
	app := &App{
		Client:       mockClient,
		DEK:          nil,
		AuthState:    nil,
		SyncState:    nil,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Register("testuser", "password123")

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app.DEK)
	assert.Len(t, app.DEK, 32)
	assert.NotNil(t, app.AuthState)
	assert.Equal(t, "user-123", app.AuthState.UserID)
}

// TestRegister_ServerError тестирует ошибку от сервера
func TestRegister_ServerError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("network error")).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Register("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "network error", err.Error())
	assert.Nil(t, app.AuthState)
	assert.Nil(t, app.DEK)
}

// TestRegister_InvalidResponse тестирует невалидный ответ сервера (отсутствует пользователь)
func TestRegister_InvalidResponse(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.RegisterResponse{
			User: nil, // Невалидный ответ - нет пользователя
		}, nil).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Register("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "invalid response: user is nil", err.Error())
}

// TestRegister_SaveError тестирует ошибку сохранения состояния
func TestRegister_SaveError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.RegisterResponse{
			User: &gophkeeperv1.UserInfo{
				UserId: "user-123",
			},
			AccessToken:  "token",
			RefreshToken: "refresh",
			DeviceId:     "device-1",
		}, nil).
		Times(1)

	mockStorage.EXPECT().Save(gomock.Any()).
		Return(errors.New("save error")).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Register("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "save error", err.Error())
	// DEK всё равно должен быть установлен
	assert.NotNil(t, app.DEK)
}

// TestLogin_Success тестирует успешный вход пользователя
func TestLogin_Success(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	// Генерируем ВАЛИДНЫЕ зашифрованные данные
	password := "password123"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(password), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek) // Генерируем случайный DEK

	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	assert.NoError(t, err)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.LoginResponse{
			User: &gophkeeperv1.UserInfo{
				UserId: "user-123",
			},
			AccessToken:            "access-token-abc",
			RefreshToken:           "refresh-token-xyz",
			DeviceId:               "device-cli-1",
			PasswordSalt:           passwordSalt,
			EncryptedVaultKey:      encryptedDEK,
			EncryptedVaultKeyNonce: nonce,
		}, nil).
		Times(1)

	mockStorage.EXPECT().Save(gomock.Any()).
		DoAndReturn(func(authState *model.AuthState) error {
			assert.Equal(t, "user-123", authState.UserID)
			assert.Equal(t, "access-token-abc", authState.AccessToken)
			assert.Equal(t, passwordSalt, authState.PasswordSalt)
			assert.Equal(t, encryptedDEK, authState.EncryptedDEK)
			assert.Equal(t, nonce, authState.EncryptedDEKNonce)
			return nil
		}).
		Times(1)

	app := &App{
		Client:       mockClient,
		DEK:          nil,
		AuthState:    nil,
		SyncState:    nil,
		LocalStorage: mockStorage,
	}

	// Act
	err = app.Login("testuser", password)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, app.DEK)
	assert.NotNil(t, app.AuthState)
	assert.Equal(t, "user-123", app.AuthState.UserID)
	assert.Equal(t, passwordSalt, app.AuthState.PasswordSalt)
	// Проверяем, что расшифрованный DEK совпадает с исходным
	assert.Equal(t, dek, app.DEK)
}

// TestLogin_ServerError тестирует ошибку от сервера при входе
func TestLogin_ServerError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused")).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Login("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "connection refused", err.Error())
	assert.Nil(t, app.AuthState)
	assert.Nil(t, app.DEK)
}

// TestLogin_InvalidPassword тестирует неверный пароль
func TestLogin_InvalidPassword(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	// Возвращаем данные, которые не могут быть расшифрованы с текущим паролем
	encryptedDEK := []byte{1, 2, 3, 4}
	nonce := []byte{5, 6, 7, 8}
	passwordSalt := []byte{9, 10, 11, 12} // Соль для другого пароля

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.LoginResponse{
			User: &gophkeeperv1.UserInfo{
				UserId: "user-123",
			},
			PasswordSalt:           passwordSalt,
			EncryptedVaultKey:      encryptedDEK,
			EncryptedVaultKeyNonce: nonce,
		}, nil).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err := app.Login("testuser", "wrongpassword")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "invalid password", err.Error())
	assert.Nil(t, app.DEK)
}

// TestLogin_InvalidResponse тестирует невалидный ответ сервера при входе
func TestLogin_InvalidResponse(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	password := "password123"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(password), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	assert.NoError(t, err)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.LoginResponse{
			User:                   nil, // Невалидный ответ
			AccessToken:            "token",
			RefreshToken:           "refresh",
			DeviceId:               "device-1",
			PasswordSalt:           passwordSalt,
			EncryptedVaultKey:      encryptedDEK,
			EncryptedVaultKeyNonce: nonce,
		}, nil).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err = app.Login("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "invalid response: user is nil", err.Error())
}

// TestLogin_SaveError тестирует ошибку сохранения при входе
func TestLogin_SaveError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	password := "password123"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(password), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	assert.NoError(t, err)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.LoginResponse{
			User: &gophkeeperv1.UserInfo{
				UserId: "user-123",
			},
			AccessToken:            "token",
			RefreshToken:           "refresh",
			DeviceId:               "device-1",
			PasswordSalt:           passwordSalt,
			EncryptedVaultKey:      encryptedDEK,
			EncryptedVaultKeyNonce: nonce,
		}, nil).
		Times(1)

	mockStorage.EXPECT().Save(gomock.Any()).
		Return(errors.New("disk full")).
		Times(1)

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	// Act
	err = app.Login("testuser", "password123")

	// Assert
	assert.Error(t, err)
	assert.Equal(t, "disk full", err.Error())
	// DEK всё равно должен быть установлен
	assert.NotNil(t, app.DEK)
}

// Table-driven тесты для регистрации
func TestRegister_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(ctrl *gomock.Controller) (*mocks.MockClient, *mocks.MockLocalStorage)
		login          string
		password       string
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "valid_credentials",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockClient, *mocks.MockLocalStorage) {
				mockClient := mocks.NewMockClient(ctrl)
				mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
				mockStorage := mocks.NewMockLocalStorage(ctrl)

				mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
				mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
					Return(&gophkeeperv1.RegisterResponse{
						User: &gophkeeperv1.UserInfo{UserId: "user-1"},
					}, nil).
					Times(1)
				mockStorage.EXPECT().Save(gomock.Any()).Return(nil).Times(1)

				return mockClient, mockStorage
			},
			login:    "user1",
			password: "pass123",
			wantErr:  false,
		},
		{
			name: "empty_password",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockClient, *mocks.MockLocalStorage) {
				mockClient := mocks.NewMockClient(ctrl)
				mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
				mockStorage := mocks.NewMockLocalStorage(ctrl)

				mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
				mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
					Return(&gophkeeperv1.RegisterResponse{
						User: &gophkeeperv1.UserInfo{UserId: "user-2"},
					}, nil).
					Times(1)
				mockStorage.EXPECT().Save(gomock.Any()).Return(nil).Times(1)

				return mockClient, mockStorage
			},
			login:    "user2",
			password: "",
			wantErr:  false, // Пустой пароль допустим на клиенте, сервер должен проверить
		},
		{
			name: "server_returns_error",
			setupMocks: func(ctrl *gomock.Controller) (*mocks.MockClient, *mocks.MockLocalStorage) {
				mockClient := mocks.NewMockClient(ctrl)
				mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
				mockStorage := mocks.NewMockLocalStorage(ctrl)

				mockClient.EXPECT().AuthClient().Return(mockAuthClient).Times(1)
				mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("server error")).
					Times(1)

				return mockClient, mockStorage
			},
			login:          "user3",
			password:       "pass123",
			wantErr:        true,
			wantErrMessage: "server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient, mockStorage := tt.setupMocks(ctrl)

			app := &App{
				Client:       mockClient,
				LocalStorage: mockStorage,
			}

			err := app.Register(tt.login, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMessage != "" {
					assert.Contains(t, err.Error(), tt.wantErrMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, app.DEK)
				assert.NotNil(t, app.AuthState)
			}
		})
	}
}

// BenchmarkRegister тестирует производительность регистрации
func BenchmarkRegister(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).AnyTimes()
	mockAuthClient.EXPECT().Register(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.RegisterResponse{
			User: &gophkeeperv1.UserInfo{UserId: "user-123"},
		}, nil).
		AnyTimes()
	mockStorage.EXPECT().Save(gomock.Any()).Return(nil).AnyTimes()

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	for i := 0; b.Loop(); i++ {
		suffix := strconv.Itoa(i)
		err := app.Register("user"+suffix, "password"+suffix)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLogin тестирует производительность входа
func BenchmarkLogin(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockAuthClient := mocks.NewMockAuthServiceClient(ctrl)
	mockStorage := mocks.NewMockLocalStorage(ctrl)

	password := "password123"
	passwordSalt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	masterKey := crypto.DeriveKey([]byte(password), passwordSalt)

	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce, encryptedDEK, err := crypto.Encrypt(masterKey, dek)
	if err != nil {
		b.Fatal(err)
	}

	mockClient.EXPECT().AuthClient().Return(mockAuthClient).AnyTimes()
	mockAuthClient.EXPECT().Login(gomock.Any(), gomock.Any()).
		Return(&gophkeeperv1.LoginResponse{
			User:                   &gophkeeperv1.UserInfo{UserId: "user-123"},
			PasswordSalt:           passwordSalt,
			EncryptedVaultKey:      encryptedDEK,
			EncryptedVaultKeyNonce: nonce,
		}, nil).
		AnyTimes()
	mockStorage.EXPECT().Save(gomock.Any()).Return(nil).AnyTimes()

	app := &App{
		Client:       mockClient,
		LocalStorage: mockStorage,
	}

	for i := 0; b.Loop(); i++ {
		err := app.Login("user", password)
		if err != nil {
			b.Fatal(err)
		}
	}
}
