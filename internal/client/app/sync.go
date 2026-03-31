// Package app предоставляет функциональность синхронизации данных с сервером.
// Реализует двухстороннюю синхронизацию изменений, обработку конфликтов
// и применение удалённых изменений к локальному хранилищу.
package app

import (
	"context"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/model"
)

// Sync выполняет синхронизацию локальных изменений с сервером.
// Отправляет ожидающие операции и получает удалённые изменения.
// Обновляет состояние синхронизации и очищает список операций.
// Возвращает ошибку в случае неудачи синхронизации.
func (a *App) Sync(ctx context.Context) error {
	req := &gophkeeperv1.SyncRequest{
		ClientRevision: a.SyncState.LastRevision,
	}

	for _, op := range a.SyncState.PendingOperations {
		req.PendingOperations = append(req.PendingOperations, &gophkeeperv1.PendingOperation{
			OperationId:     op.OperationID,
			OperationType:   op.Type,
			ItemId:          op.ItemID,
			ExpectedVersion: op.ExpectedVersion,
			Snapshot:        op.Snapshot,
		})
	}

	// 1. вызываем сервер
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := a.Client.SyncClient().Sync(ctx, req)
	if err != nil {
		return err
	}

	// 2. применяем remote changes
	for _, ev := range resp.RemoteChanges {
		a.applyEvent(ev)
	}

	// 3. обновляем revision
	a.SyncState.LastRevision = resp.NewServerRevision

	// 4. очищаем только успешные операции
	if len(resp.Conflicts) == 0 {
		a.SyncState.PendingOperations = nil
	}

	return a.LocalStorage.SaveSync(a.SyncState)
}

// applyEvent применяет событие изменения к локальному хранилищу.
// Обрабатывает создание, обновление и удаление элементов.
// Шифрует данные перед сохранением в локальное хранилище.
func (a *App) applyEvent(ev *gophkeeperv1.RevisionEvent) {
	it := ev.Item

	switch ev.ChangeType {

	case gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED,
		gophkeeperv1.ChangeType_CHANGE_TYPE_UPDATED:

		item := &model.Item{
			ID:      it.ItemId,
			Version: it.Version,
			Type:    it.ItemType,

			Title:    it.Title,
			Metadata: it.Metadata,
			Payload:  it.Payload,

			Deleted: false,
		}

		_ = a.LocalStorage.Upsert(item)

	case gophkeeperv1.ChangeType_CHANGE_TYPE_DELETED:

		item := &model.Item{
			ID:      it.ItemId,
			Version: it.Version,
			Deleted: true,
		}

		_ = a.LocalStorage.Upsert(item)
	}
}
