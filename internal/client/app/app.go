// Package app — основной пакет приложения, содержащий бизнес-логику клиента.
// Реализует функциональность аутентификации, управления хранилищем секретов,
// шифрования данных и синхронизации с сервером.
package app

import (
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/prbllm/goph-keeper/internal/client/storage"
	"github.com/prbllm/goph-keeper/internal/client/transport"
)

// App представляет основное приложение клиента с полным состоянием.
// Содержит все необходимые компоненты для работы с сервером и локальным хранилищем.
type App struct {
	// Client — интерфейс клиента для взаимодействия с сервером
	Client transport.Client

	// DEK (Data Encryption Key) — ключ для шифрования данных в хранилище
	// Генерируется при регистрации и расшифровывается при входе
	DEK []byte

	// AuthState — текущее состояние аутентификации пользователя
	AuthState *model.AuthState

	// SyncState — состояние синхронизации с сервером
	// Содержит последнюю ревизию и ожидающие операции
	SyncState *model.SyncState

	// LocalStorage — интерфейс для работы с локальным хранилищем
	LocalStorage storage.LocalStorage
}

// New создаёт и инициализирует новое приложение.
// Принимает клиент для сервера и путь к директории данных.
// Выполняет создание директории, инициализацию хранилища и загрузку состояний.
// Возвращает указатель на приложение или ошибку инициализации.
func New(client transport.Client, localStorage storage.LocalStorage) (*App, error) {
	// Загрузка состояний из хранилища
	authState, err := localStorage.Load()
	if err != nil {
		return nil, err
	}

	syncState, err := localStorage.LoadSync()
	if err != nil {
		return nil, err
	}

	return &App{
		Client:       client,
		AuthState:    authState,
		SyncState:    syncState,
		LocalStorage: localStorage,
	}, nil
}
