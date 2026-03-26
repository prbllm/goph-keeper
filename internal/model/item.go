// Package model определяет структуру элемента хранилища секретов.
// Представляет зашифрованный элемент с метаданными и версионированием.
package model

import (
	"github.com/prbllm/goph-keeper/api"
)

// Item представляет локальную модель элемента хранилища (кэш).
// Содержит зашифрованные данные и метаинформацию для синхронизации.
type Item struct {
	// ID — уникальный идентификатор элемента
	ID string

	// Version — номер версии элемента для отслеживания изменений
	Version uint64
	// Type — тип элемента (текст, учётные данные, банковская карта и т.д.)
	Type api.ItemType

	// Title — зашифрованный заголовок элемента
	Title *api.EncryptedField
	// Metadata — зашифрованные метаданные элемента
	Metadata *api.EncryptedField
	// Payload — зашифрованная полезная нагрузка (основные данные)
	Payload *api.EncryptedField

	// Deleted — флаг удаления элемента (мягкое удаление)
	Deleted bool
}
