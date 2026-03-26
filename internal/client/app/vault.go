// Package app предоставляет функции управления секретами в хранилище.
// Реализует операции добавления, получения, обновления и удаления элементов,
// а также шифрование и расшифровку данных.
package app

import (
	"crypto/rand"
	"errors"
	"fmt"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

	"github.com/prbllm/goph-keeper/internal/client/crypto"
	"github.com/prbllm/goph-keeper/internal/client/model"
)

// AddItem добавляет новый элемент в хранилище.
// Принимает тип элемента, заголовок, метаданные и полезную нагрузку.
// Шифрует данные, сохраняет локально и добавляет операцию в очередь синхронизации.
// Возвращает ошибку при отсутствии ключа шифрования или ошибках сохранения.
func (a *App) AddItem(itemType gophkeeperv1.ItemType, title, metadata, payload []byte) error {
	if len(a.DEK) == 0 {
		return errors.New("login required (DEK missing)")
	}

	encTitle, err := encryptField(a.DEK, title)
	if err != nil {
		return err
	}

	encMeta, err := encryptField(a.DEK, metadata)
	if err != nil {
		return err
	}

	encPayload, err := encryptField(a.DEK, payload)
	if err != nil {
		return err
	}

	itemID := generateID("item")
	opID := generateID("op")

	item := &model.Item{
		ID:       itemID,
		Version:  1,
		Type:     itemType,
		Title:    encTitle,
		Metadata: encMeta,
		Payload:  encPayload,
	}

	// local cache
	if err := a.LocalStorage.Upsert(item); err != nil {
		return err
	}

	// pending op
	a.SyncState.PendingOperations = append(a.SyncState.PendingOperations, model.PendingOperation{
		OperationID:     opID,
		Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		ItemID:          itemID,
		ExpectedVersion: 0,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: item.Type,
			Title:    item.Title,
			Metadata: item.Metadata,
			Payload:  item.Payload,
		},
	})

	return a.LocalStorage.SaveSync(a.SyncState)
}

// ListItems возвращает список всех элементов из локального хранилища.
// Возвращает слайс указателей на элементы и ошибку в случае неудачи.
func (a *App) ListItems() ([]*model.Item, error) {
	return a.LocalStorage.List()
}

// GetItem получает элемент по идентификатору из локального хранилища.
// Принимает идентификатор элемента.
// Возвращает указатель на элемент и флаг существования.
func (a *App) GetItem(itemID string) (*model.Item, bool) {
	return a.LocalStorage.Get(itemID)
}

// UpdateItem обновляет существующий элемент в хранилище.
// Принимает идентификатор элемента, новый тип, заголовок, метаданные и полезную нагрузку.
// Шифрует новые данные, обновляет локально и добавляет операцию в очередь синхронизации.
// Возвращает ошибку при отсутствии элемента или ключа шифрования.
func (a *App) UpdateItem(itemID string, itemType gophkeeperv1.ItemType, title, metadata, payload []byte) error {
	if len(a.DEK) == 0 {
		return errors.New("login required (DEK missing)")
	}

	item, ok := a.LocalStorage.Get(itemID)
	if !ok {
		return fmt.Errorf("item not found")
	}

	// шифруем новые данные
	encTitle, err := encryptField(a.DEK, title)
	if err != nil {
		return err
	}

	encMeta, err := encryptField(a.DEK, metadata)
	if err != nil {
		return err
	}

	encPayload, err := encryptField(a.DEK, payload)
	if err != nil {
		return err
	}

	opID := generateID("op")

	newVersion := item.Version + 1

	// обновляем локально
	item.Type = itemType
	item.Title = encTitle
	item.Metadata = encMeta
	item.Payload = encPayload
	item.Version = newVersion

	if err := a.LocalStorage.Upsert(item); err != nil {
		return err
	}

	// добавляем pending операцию
	a.SyncState.PendingOperations = append(a.SyncState.PendingOperations, model.PendingOperation{
		OperationID:     opID,
		Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
		ItemID:          itemID,
		ExpectedVersion: item.Version - 1, // старая версия!
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: item.Type,
			Title:    item.Title,
			Metadata: item.Metadata,
			Payload:  item.Payload,
		},
	})

	return a.LocalStorage.SaveSync(a.SyncState)
}

// DeleteItem удаляет элемент из хранилища.
// Принимает идентификатор элемента.
// Удаляет локально и добавляет операцию удаления в очередь синхронизации.
// Возвращает ошибку при отсутствии элемента.
func (a *App) DeleteItem(itemID string) error {
	if len(a.DEK) == 0 {
		return errors.New("login required (DEK missing)")
	}

	// проверим, что item существует локально
	item, ok := a.LocalStorage.Get(itemID)
	if !ok {
		return fmt.Errorf("item not found")
	}

	opID := generateID("op")

	// удаляем локально
	if err := a.LocalStorage.Delete(itemID); err != nil {
		return err
	}

	// добавляем pending операцию
	a.SyncState.PendingOperations = append(a.SyncState.PendingOperations, model.PendingOperation{
		OperationID:     opID,
		Type:            gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_DELETE,
		ItemID:          itemID,
		ExpectedVersion: item.Version,
	})

	return a.LocalStorage.SaveSync(a.SyncState)
}

// DecryptItem расшифровывает все поля элемента.
// Принимает указатель на элемент.
// Возвращает расшифрованные заголовок, метаданные и полезную нагрузку в виде строк.
// Возвращает ошибку при отсутствии ключа шифрования или ошибках расшифровки.
func (a *App) DecryptItem(it *model.Item) (string, string, string, error) {
	if len(a.DEK) == 0 {
		return "", "", "", errors.New("login required (DEK missing)")
	}

	title, err := crypto.Decrypt(a.DEK, it.Title.Nonce, it.Title.Ciphertext)
	if err != nil {
		return "", "", "", err
	}

	meta, err := crypto.Decrypt(a.DEK, it.Metadata.Nonce, it.Metadata.Ciphertext)
	if err != nil {
		return "", "", "", err
	}

	payload, err := crypto.Decrypt(a.DEK, it.Payload.Nonce, it.Payload.Ciphertext)
	if err != nil {
		return "", "", "", err
	}

	return string(title), string(meta), string(payload), nil
}

// Unlock расшифровывает ключ шифрования данных с помощью пароля.
// Принимает пароль пользователя.
// Загружает состояние аутентификации, производит расшифровку DEK.
// Возвращает ошибку при неверном пароле или ошибках загрузки.
func (a *App) Unlock(password string) error {
	authState, err := a.LocalStorage.Load()
	if err != nil {
		return err
	}

	masterKey := crypto.DeriveKey([]byte(password), authState.PasswordSalt)

	dek, err := crypto.Decrypt(
		masterKey,
		authState.EncryptedDEKNonce,
		authState.EncryptedDEK,
	)
	if err != nil {
		return errors.New("invalid password")
	}

	a.DEK = dek
	a.AuthState = authState

	return nil
}

// encryptField шифрует поле данных с использованием ключа шифрования.
// Принимает ключ и данные для шифрования.
// Возвращает структуру с шифротекстом и nonce или ошибку.
func encryptField(dek, data []byte) (*gophkeeperv1.EncryptedField, error) {
	nonce, cipher, err := crypto.Encrypt(dek, data)
	if err != nil {
		return nil, err
	}

	return &gophkeeperv1.EncryptedField{
		Ciphertext: cipher,
		Nonce:      nonce,
	}, nil
}

// generateID генерирует уникальный идентификатор с префиксом.
// Принимает префикс для идентификатора.
// Возвращает строку в формате "префикс-случайные_байты".
func generateID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}
