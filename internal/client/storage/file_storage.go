// Package storage предоставляет реализацию файлового хранилища для локальных данных.
// Сохраняет состояния аутентификации, синхронизации и элементы хранилища в JSON файлах.
package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/prbllm/goph-keeper/internal/client/model"
)

// FileStorage реализует интерфейс LocalStorage с использованием файловой системы.
// Хранит данные в JSON форматах с синхронизацией доступа через мьютекс.
type FileStorage struct {
	mu            sync.RWMutex
	items         map[string]*model.Item
	authStatePath string
	syncStatePath string
	vaultPath     string
}

// New создаёт новое файловое хранилище по указанному пути.
// Инициализирует директорию, создаёт необходимые файлы и загружает существующие данные.
// Возвращает указатель на хранилище или ошибку инициализации.
func New(dataDirPath string) (*FileStorage, error) {
	if err := os.MkdirAll(dataDirPath, 0700); err != nil {
		return nil, err
	}

	storage := &FileStorage{
		items:         make(map[string]*model.Item),
		authStatePath: filepath.Join(dataDirPath, "state.json"),
		syncStatePath: filepath.Join(dataDirPath, "sync.json"),
		vaultPath:     filepath.Join(dataDirPath, "vault.json"),
	}

	if _, err := os.Stat(storage.vaultPath); os.IsNotExist(err) {
		// Создаем пустой файл, если его нет
		if err = os.WriteFile(storage.vaultPath, []byte("{}"), 0600); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(storage.vaultPath)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(data, &storage.items); err != nil {
		return nil, err
	}

	return storage, nil
}

// AuthStateStorage

// Save сохраняет состояние аутентификации в файл state.json.
// Принимает указатель на структуру AuthState.
// Сериализует данные в JSON и записывает в файл с правами 0600.
// Возвращает ошибку в случае неудачи записи или сериализации.
func (s *FileStorage) Save(authState *model.AuthState) error {
	data, err := json.MarshalIndent(authState, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.authStatePath, data, 0600)
}

// Load загружает состояние аутентификации из файла state.json.
// Возвращает указатель на структуру AuthState.
// Если файл не существует, возвращает пустую структуру без ошибки.
// Возвращает ошибку при проблемах чтения или десериализации.
func (s *FileStorage) Load() (*model.AuthState, error) {
	data, err := os.ReadFile(s.authStatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.AuthState{}, nil
		}
		return nil, err
	}

	var authState model.AuthState
	if err = json.Unmarshal(data, &authState); err != nil {
		return nil, err
	}
	return &authState, nil
}

// SyncStorage

// LoadSync загружает состояние синхронизации из файла sync.json.
// Возвращает указатель на структуру SyncState.
// Если файл не существует, возвращает пустую структуру.
// Возвращает ошибку при проблемах чтения или десериализации.
func (s *FileStorage) LoadSync() (*model.SyncState, error) {
	data, err := os.ReadFile(s.syncStatePath)
	if err != nil {
		return &model.SyncState{}, nil
	}

	var syncState model.SyncState
	if err := json.Unmarshal(data, &syncState); err != nil {
		return nil, err
	}

	return &syncState, nil
}

// SaveSync сохраняет состояние синхронизации в файл sync.json.
// Принимает указатель на структуру SyncState.
// Сериализует данные в JSON и записывает в файл с правами 0600.
// Возвращает ошибку в случае неудачи записи или сериализации.
func (s *FileStorage) SaveSync(syncState *model.SyncState) error {
	data, err := json.MarshalIndent(syncState, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.syncStatePath, data, 0600)
}

// VaultStorage

// Upsert добавляет или обновляет элемент в локальном хранилище.
// Принимает указатель на элемент. Использует мьютекс для синхронизации доступа.
// Сохраняет изменения в файл после обновления внутреннего кэша.
// Возвращает ошибку при неудачной записи файла.
func (s *FileStorage) Upsert(item *model.Item) error {
	s.mu.Lock()
	s.items[item.ID] = item
	s.mu.Unlock()
	return s.save()
}

// List возвращает список всех элементов из локального хранилища.
// Блокирует чтение через RLock для обеспечения консистентности.
// Возвращает слайс указателей на все элементы.
// Возвращает ошибку nil в случае успеха.
func (s *FileStorage) List() ([]*model.Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*model.Item
	for _, v := range s.items {
		out = append(out, v)
	}

	return out, nil
}

// Get получает элемент по идентификатору из локального хранилища.
// Принимает строковый идентификатор элемента.
// Блокирует чтение через RLock для обеспечения консистентности.
// Возвращает указатель на элемент и флаг существования.
func (s *FileStorage) Get(id string) (*model.Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	it, ok := s.items[id]
	return it, ok
}

// Delete удаляет элемент из локального хранилища по идентификатору.
// Принимает строковый идентификатор элемента.
// Блокирует запись через Lock для обеспечения эксклюзивного доступа.
// Сохраняет изменения в файл после удаления из внутреннего кэша.
// Возвращает ошибку при неудачной записи файла.
func (s *FileStorage) Delete(id string) error {
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
	return s.save()
}

// save сохраняет все элементы хранилища в файл vault.json.
// Использует RLock так как изменяет только файл, а не внутренний кэш.
// Сериализует данные в JSON и записывает с правами 0600.
// Возвращает ошибку при неудачной записи или сериализации.
func (s *FileStorage) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.vaultPath, data, 0600)
}
