// Package crypto предоставляет криптографические функции для шифрования данных.
// Использует алгоритм XChaCha20-Poly1305 для симметричного шифрования.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

const NonceSize = chacha20poly1305.NonceSizeX

// Encrypt шифрует данные с использованием алгоритма XChaCha20-Poly1305.
// Принимает ключ шифрования (32 байта) и открытый текст.
// Генерирует случайный nonce и возвращает его вместе с шифротекстом.
// Возвращает ошибку в случае неудачи генерации ключа или nonce.
func Encrypt(key, plaintext []byte) ([]byte, []byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

// Decrypt расшифровывает данные, зашифрованные алгоритмом XChaCha20-Poly1305.
// Принимает ключ шифрования, nonce и шифротекст.
// Проверяет целостность данных и возвращает расшифрованный текст.
// Возвращает ошибку при неверном ключе, nonce или повреждённых данных.
func Decrypt(key, nonce, ciphertext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	if len(nonce) != NonceSize {
		return nil, errors.New("invalid nonce size")
	}

	return aead.Open(nil, nonce, ciphertext, nil)
}

// SHA256 вычисляет хеш данных
func SHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}
