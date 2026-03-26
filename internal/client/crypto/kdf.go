// Package crypto предоставляет функции для работы с ключами производной функции (KDF).
// Использует алгоритм Argon2id для безопасного получения ключей из паролей.
package crypto

import (
	"crypto/rand"

	"golang.org/x/crypto/argon2"
)

const (
	// saltSize определяет размер криптографической соли в байтах.
	// Используется для генерации уникальных ключей из одинаковых паролей.
	saltSize = 16
)

// GenerateSalt генерирует криптографически стойкую случайную соль.
// Возвращает соль размером 16 байт или ошибку в случае неудачи.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	_, err := rand.Read(salt)
	return salt, err
}

// DeriveKey получает криптографический ключ из пароля с использованием Argon2id.
// Принимает пароль и соль, применяет параметры: 3 итерации, 64 МБ памяти, 4 потока.
// Возвращает 32-байтный ключ для использования в шифровании.
func DeriveKey(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, 3, 64*1024, 4, 32)
}
