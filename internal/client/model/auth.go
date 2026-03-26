// Package model определяет структуры данных для хранения состояния аутентификации.
// Используется для сериализации и десериализации данных в локальном хранилище.
package model

// AuthState представляет локальное состояние аутентификации клиента.
// Содержит информацию о пользователе, токенах доступа и ключах шифрования.
type AuthState struct {
	// UserID — уникальный идентификатор пользователя на сервере
	UserID string `json:"user_id"`
	// AccessToken — JWT токен для аутентификации запросов к серверу
	AccessToken string `json:"access_token"`
	// RefreshToken — токен для обновления access token
	RefreshToken string `json:"refresh_token"`
	// DeviceID — уникальный идентификатор устройства
	DeviceID string `json:"device_id"`

	// PasswordSalt — соль, используемая для получения мастер-ключа из пароля
	PasswordSalt []byte `json:"password_salt"`

	// EncryptedDEK — зашифрованный ключ шифрования данных (DEK)
	EncryptedDEK []byte `json:"encrypted_dek"`
	// EncryptedDEKNonce — nonce, использованный при шифровании DEK
	EncryptedDEKNonce []byte `json:"encrypted_dek_nonce"`
}
