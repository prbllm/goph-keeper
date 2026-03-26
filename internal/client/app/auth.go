// Package app предоставляет функции аутентификации пользователя.
// Реализует регистрацию новых пользователей и вход существующих,
// включая генерацию и расшифровку ключей шифрования.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"runtime"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

	"github.com/prbllm/goph-keeper/internal/client/crypto"
	"github.com/prbllm/goph-keeper/internal/client/model"
)

// Register регистрирует нового пользователя в системе.
// Принимает логин и пароль, генерирует соль и ключи шифрования.
// Отправляет запрос на сервер и сохраняет состояние аутентификации локально.
// Возвращает ошибку в случае неудачи.
func (a *App) Register(login, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	salt, err := crypto.GenerateSalt()
	if err != nil {
		return err
	}

	masterKey := crypto.DeriveKey([]byte(password), salt)

	// DEK (Vault Key)
	dek := make([]byte, 32)
	if _, err = rand.Read(dek); err != nil {
		return err
	}

	nonce, encDEK, err := crypto.Encrypt(masterKey, dek)
	if err != nil {
		return err
	}

	platform := gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_UNSPECIFIED
	switch runtime.GOOS {
	case "linux":
		platform = gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX
	case "windows":
		platform = gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_WINDOWS
	case "darwin":
		platform = gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_MACOS
	}

	resp, err := a.Client.AuthClient().Register(ctx, &gophkeeperv1.RegisterRequest{
		Login:    login,
		Password: password,

		PasswordSalt: salt,
		KdfParams: &gophkeeperv1.KdfParams{
			Algorithm:   "argon2id",
			MemoryKib:   64 * 1024,
			Iterations:  3,
			Parallelism: 4,
			KeyLength:   32,
		},

		EncryptedVaultKey:      encDEK,
		EncryptedVaultKeyNonce: nonce,
		Device: &gophkeeperv1.DeviceInfo{
			DeviceName:    "cli",
			Platform:      platform,
			ClientVersion: "dev",
		},
	})
	if err != nil {
		return err
	}

	if resp.User == nil {
		return errors.New("invalid response: user is nil")
	}

	a.DEK = dek

	a.AuthState = &model.AuthState{
		UserID:       resp.User.UserId,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		DeviceID:     resp.DeviceId,
	}

	return a.LocalStorage.Save(a.AuthState)
}

// Login выполняет аутентификацию существующего пользователя.
// Принимает логин и пароль, отправляет запрос на сервер,
// расшифровывает ключ шифрования данных и сохраняет состояние.
// Возвращает ошибку при неверных учётных данных или ошибках сети.
func (a *App) Login(login, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := a.Client.AuthClient().Login(ctx, &gophkeeperv1.LoginRequest{
		Login:    login,
		Password: password,
		Device: &gophkeeperv1.DeviceInfo{
			DeviceName:    "cli",
			Platform:      gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX,
			ClientVersion: "dev",
		},
	})
	if err != nil {
		return err
	}

	masterKey := crypto.DeriveKey([]byte(password), resp.PasswordSalt)

	dek, err := crypto.Decrypt(
		masterKey,
		resp.EncryptedVaultKeyNonce,
		resp.EncryptedVaultKey,
	)
	if err != nil {
		return errors.New("invalid password")
	}

	if resp.User == nil {
		return errors.New("invalid response: user is nil")
	}

	a.DEK = dek

	a.AuthState = &model.AuthState{
		UserID:       resp.User.UserId,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		DeviceID:     resp.DeviceId,

		PasswordSalt: resp.PasswordSalt,

		EncryptedDEK:      resp.EncryptedVaultKey,
		EncryptedDEKNonce: resp.EncryptedVaultKeyNonce,
	}

	return a.LocalStorage.Save(a.AuthState)
}
