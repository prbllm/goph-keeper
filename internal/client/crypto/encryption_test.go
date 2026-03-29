package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncrypt_Success(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("secret message")

	// Act
	nonce, ciphertext, err := Encrypt(key, plaintext)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, nonce)
	assert.NotNil(t, ciphertext)
	assert.Len(t, nonce, 24) // XChaCha20 nonce size
	assert.NotEmpty(t, ciphertext)
	assert.NotEqual(t, plaintext, ciphertext) // Данные должны быть зашифрованы
}

func TestDecrypt_Success(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("secret message")

	nonce, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	// Act
	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, decrypted)
	assert.Equal(t, plaintext, decrypted) // Расшифрованные данные должны совпадать с оригиналом
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	original := []byte("test data")

	// Act
	nonce, ciphertext, err := Encrypt(key, original)
	assert.NoError(t, err)

	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, original, decrypted)
}

func TestEncrypt_InvalidKey(t *testing.T) {
	// Arrange
	invalidKey := []byte("short-key") // Менее 32 байт
	plaintext := []byte("data")

	// Act
	nonce, ciphertext, err := Encrypt(invalidKey, plaintext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, nonce)
	assert.Nil(t, ciphertext)
}

func TestDecrypt_InvalidKey(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("secret")

	nonce, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	wrongKey := make([]byte, 32)
	wrongKey[0] = 1 // Изменяем ключ

	// Act
	decrypted, err := Decrypt(wrongKey, nonce, ciphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Contains(t, err.Error(), "message authentication failed")
}

func TestDecrypt_InvalidNonce(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("secret")

	_, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	invalidNonce := make([]byte, 10) // Неверный размер nonce

	// Act
	decrypted, err := Decrypt(key, invalidNonce, ciphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Equal(t, "invalid nonce size", err.Error())
}

func TestDecrypt_EmptyNonce(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("secret")

	_, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	// Act
	decrypted, err := Decrypt(key, []byte{}, ciphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Equal(t, "invalid nonce size", err.Error())
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	nonce := make([]byte, 24)

	invalidCiphertext := []byte("invalid-ciphertext")

	// Act
	decrypted, err := Decrypt(key, nonce, invalidCiphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Contains(t, err.Error(), "message authentication failed")
}

func TestEncryptDecrypt_EmptyData(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	emptyData := []byte{}

	// Act
	nonce, ciphertext, err := Encrypt(key, emptyData)
	assert.NoError(t, err)

	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestEncryptDecrypt_LargeData(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	largeData := make([]byte, 1024*1024) // 1 MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Act
	nonce, ciphertext, err := Encrypt(key, largeData)
	assert.NoError(t, err)

	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, largeData, decrypted)
	assert.Len(t, ciphertext, len(largeData)+16) // +16 байт для тега аутентификации
}

func TestEncryptDecrypt_UnicodeData(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	unicodeData := []byte("Привет, мир! 🌍 Hello, world! 你好，世界!")

	// Act
	nonce, ciphertext, err := Encrypt(key, unicodeData)
	assert.NoError(t, err)

	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, unicodeData, decrypted)
}

func TestEncryptDecrypt_SpecialCharacters(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	specialChars := []byte("!@#$%^&*()_+-=[]{}|;':\",./<>?\\`~\n\t\r")

	// Act
	nonce, ciphertext, err := Encrypt(key, specialChars)
	assert.NoError(t, err)

	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, specialChars, decrypted)
}

func TestEncrypt_NonDeterministic(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("same data")

	// Act
	nonce1, _, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	nonce2, _, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	// Assert
	assert.NotEqual(t, nonce1, nonce2) // Nonce должен быть разным
	// Ciphertext может быть разным из-за разных nonce, но это нормально
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("important data")

	nonce, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	// Tamper with ciphertext
	ciphertext[0] ^= 0xFF // Инвертируем первый байт

	// Act
	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Contains(t, err.Error(), "message authentication failed")
}

func TestDecrypt_TamperedNonce(t *testing.T) {
	// Arrange
	key := make([]byte, 32)
	plaintext := []byte("important data")

	nonce, ciphertext, err := Encrypt(key, plaintext)
	assert.NoError(t, err)

	// Tamper with nonce
	nonce[0] ^= 0xFF

	// Act
	decrypted, err := Decrypt(key, nonce, ciphertext)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Contains(t, err.Error(), "message authentication failed")
}

func TestEncrypt_ZeroKey(t *testing.T) {
	// Arrange
	zeroKey := make([]byte, 32) // Все нули
	plaintext := []byte("data")

	// Act
	nonce, ciphertext, err := Encrypt(zeroKey, plaintext)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, nonce)
	assert.NotNil(t, ciphertext)

	// Проверяем, что можно расшифровать
	decrypted, err := Decrypt(zeroKey, nonce, ciphertext)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecrypt_NilInputs(t *testing.T) {
	// Arrange
	key := make([]byte, 32)

	// Act & Assert
	// Nil ciphertext
	decrypted, err := Decrypt(key, make([]byte, 24), nil)
	assert.Error(t, err)
	assert.Nil(t, decrypted)

	// Nil key
	decrypted, err = Decrypt(nil, make([]byte, 24), []byte("cipher"))
	assert.Error(t, err)
	assert.Nil(t, decrypted)
}

func TestSHA256(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantHex string
	}{
		{
			name:    "Empty string",
			input:   []byte(""),
			wantHex: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:    "Hello World",
			input:   []byte("hello world"),
			wantHex: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SHA256(tt.input)

			want, _ := hex.DecodeString(tt.wantHex)
			assert.Equal(t, want, got)
		})
	}
}

// BenchmarkEncrypt измеряет производительность шифрования
func BenchmarkEncrypt(b *testing.B) {
	key := make([]byte, 32)
	data := []byte("benchmark data")

	for b.Loop() {
		_, _, err := Encrypt(key, data)
		require.NoError(b, err)
	}
}

// BenchmarkDecrypt измеряет производительность расшифровки
func BenchmarkDecrypt(b *testing.B) {
	key := make([]byte, 32)
	data := []byte("benchmark data")

	nonce, ciphertext, err := Encrypt(key, data)
	require.NoError(b, err)

	for b.Loop() {
		_, err := Decrypt(key, nonce, ciphertext)
		require.NoError(b, err)
	}
}

// BenchmarkEncryptDecrypt_RoundTrip измеряет полный цикл шифрования/расшифровки
func BenchmarkEncryptDecrypt_RoundTrip(b *testing.B) {
	key := make([]byte, 32)
	data := []byte("benchmark data")

	for b.Loop() {
		nonce, ciphertext, err := Encrypt(key, data)
		require.NoError(b, err)

		_, err = Decrypt(key, nonce, ciphertext)
		require.NoError(b, err)
	}
}

// ExampleEncrypt демонстрирует использование функции шифрования
func ExampleEncrypt() {
	key := make([]byte, 32)
	plaintext := []byte("secret message")

	nonce, ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		panic(err)
	}

	_ = nonce
	_ = ciphertext
	// Output:
}
