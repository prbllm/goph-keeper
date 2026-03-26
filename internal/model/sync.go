// Package model определяет структуры для управления состоянием синхронизации.
// Отслеживает локальные изменения и ревизии сервера для двусторонней синхронизации.
package model

import "github.com/prbllm/goph-keeper/api"

// PendingOperation представляет операцию, ожидающую синхронизации с сервером.
// Используется для отслеживания локальных изменений до их отправки на сервер.
type PendingOperation struct {
	// OperationID — уникальный идентификатор операции
	OperationID string
	// Type — тип операции (создание, обновление, удаление)
	Type api.PendingOperationType
	// ItemID — идентификатор элемента, к которому применяется операция
	ItemID string
	// ExpectedVersion — ожидаемая версия элемента на сервере (для обнаружения конфликтов)
	ExpectedVersion uint64
	// Snapshot — снимок данных элемента для отправки на сервер
	Snapshot *api.VaultItemSnapshot
}

// SyncState представляет локальное состояние синхронизации с сервером.
// Содержит информацию о последней синхронизированной ревизии и ожидающих операциях.
type SyncState struct {
	// LastRevision — номер последней успешно синхронизированной ревизии сервера
	LastRevision uint64 `json:"last_revision"`
	// PendingOperations — список операций, ожидающих отправки на сервер
	PendingOperations []PendingOperation `json:"pending_operations"`
}
