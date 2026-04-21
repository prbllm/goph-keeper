// Package model определяет структуру элемента хранилища секретов.
// Представляет зашифрованный элемент с метаданными и версионированием.
package model

import (
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
)

// Item представляет локальную модель элемента хранилища (кэш).
// Содержит зашифрованные данные и метаинформацию для синхронизации.
type Item struct {
	// ID — уникальный идентификатор элемента
	ID string

	// Version — номер версии элемента для отслеживания изменений
	Version uint64
	// Type — тип элемента (текст, учётные данные, банковская карта и т.д.)
	Type gophkeeperv1.ItemType

	// Title — зашифрованный заголовок элемента
	Title *gophkeeperv1.EncryptedField
	// Metadata — зашифрованные метаданные элемента
	Metadata *gophkeeperv1.EncryptedField
	// Payload — зашифрованная полезная нагрузка (основные данные)
	Payload *gophkeeperv1.EncryptedField
	// BlobID - идентификатор загруженного файла
	BlobID string

	// Deleted — флаг удаления элемента (мягкое удаление)
	Deleted bool
}
