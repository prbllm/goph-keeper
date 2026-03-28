// Package app предоставляет функции управления секретами в хранилище.
// Реализует операции добавления, получения, обновления и удаления элементов,
// а также шифрование и расшифровку данных.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

	"github.com/prbllm/goph-keeper/internal/client/crypto"
	"github.com/prbllm/goph-keeper/internal/client/model"
)

// AddItem добавляет новый элемент в хранилище.
// Принимает тип элемента, заголовок, метаданные и полезную нагрузку.
// Шифрует данные, сохраняет локально и добавляет операцию в очередь синхронизации.
// Возвращает ошибку при отсутствии ключа шифрования или ошибках сохранения.
func (a *App) AddItem(itemType gophkeeperv1.ItemType, title, metadata, payload []byte, blobId string) error {
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
		BlobID:   blobId,
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

// UploadFile загружает файл в blob-хранилище.
// Принимает путь к загружаемому файлу.
// Возвращает blobID загруженного файла.
func (a *App) UploadFile(ctx context.Context, filePath, fileName string) (string, error) {
	if len(a.DEK) == 0 {
		return "", errors.New("login required (DEK missing)")
	}

	// Читаем файл
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Шифруем данные
	nonce, encryptedData, err := crypto.Encrypt(a.DEK, data)
	if err != nil {
		return "", err
	}

	// Добавляем nonce перед зашифрованными данными
	blobData := append(nonce, encryptedData...)

	// Вычисляем хэш-сумму
	blobChecksum := crypto.SHA256(blobData)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Начинаем загрузку
	startResp, err := a.Client.BlobClient().StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{
		ExpectedSize:     uint64(len(blobData)),
		ExpectedChecksum: blobChecksum,
		ContentKind:      "application/octet-stream",
		FileName:         fileName,
		MimeType:         "application/octet-stream",
	})
	if err != nil {
		return "", err
	}

	// Загружаем чанками
	stream, err := a.Client.BlobClient().UploadBlob(ctx)
	if err != nil {
		return "", err
	}

	// Отправляем header
	err = stream.Send(&gophkeeperv1.UploadBlobRequest{
		Body: &gophkeeperv1.UploadBlobRequest_Header{
			Header: &gophkeeperv1.UploadBlobHeader{
				UploadSessionId: startResp.UploadSessionId,
			},
		},
	})
	if err != nil {
		return "", err
	}

	// Отправляем данные чанками
	maxChunkSize := int(startResp.MaxChunkSize)
	for i := 0; i < len(blobData); i += maxChunkSize {
		end := min(i+maxChunkSize, len(blobData))

		chunk := &gophkeeperv1.UploadBlobChunk{
			ChunkIndex: uint64(i / maxChunkSize),
			Data:       blobData[i:end],
		}

		err = stream.Send(&gophkeeperv1.UploadBlobRequest{
			Body: &gophkeeperv1.UploadBlobRequest_Chunk{
				Chunk: chunk,
			},
		})
		if err != nil {
			return "", err
		}
	}

	// Завершаем загрузку
	uploadResp, err := stream.CloseAndRecv()
	if err != nil {
		return "", err
	}

	if uploadResp.Status != gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED {
		// Фиксируем загрузку
		commitResp, err := a.Client.BlobClient().CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{
			UploadSessionId: startResp.UploadSessionId,
		})
		if err != nil {
			return "", err
		}

		if commitResp.Blob == nil {
			return "", errors.New("commit failed: no blob info")
		}
	}

	return startResp.BlobId, nil
}

// DownloadFile скачивает файл из blob-хранилища.
// Принимает blobID и путь, куда требуется сохранить файл.
func (a *App) DownloadFile(ctx context.Context, blobID string) ([]byte, error) {
	if len(a.DEK) == 0 {
		return nil, errors.New("login required (DEK missing)")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Инициируем потоковую загрузку
	stream, err := a.Client.BlobClient().DownloadBlob(ctx, &gophkeeperv1.DownloadBlobRequest{
		BlobId: blobID,
	})
	if err != nil {
		return nil, err
	}

	// Читаем заголовок
	headerMsg, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	header := headerMsg.GetHeader()
	if header == nil || header.Blob == nil {
		return nil, errors.New("invalid download response: no header")
	}

	// Читаем чанки данных
	var encryptedData []byte
	for {
		chunkMsg, errStream := stream.Recv()
		if errStream != nil {
			if errStream == io.EOF {
				break
			}
			return nil, errStream
		}

		chunk := chunkMsg.GetChunk()
		if chunk != nil {
			encryptedData = append(encryptedData, chunk.Data...)
		}
	}

	// Извлекаем Nonce и шифрованные данные
	if len(encryptedData) < crypto.NonceSize {
		return nil, errors.New("invalid blob data: too short")
	}

	nonce := encryptedData[:crypto.NonceSize]
	ciphertext := encryptedData[crypto.NonceSize:]

	// Расшифровываем данные
	decryptedData, err := crypto.Decrypt(a.DEK, nonce, ciphertext)
	if err != nil {
		return nil, err
	}

	return decryptedData, nil
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
