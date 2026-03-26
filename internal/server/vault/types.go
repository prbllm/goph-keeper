package vault

import (
	"context"
	"time"
)

//go:generate go tool mockgen -typed -destination=./mocks/mock_vault.go -package=mocks . Repository,RevisionLogRepository,ProcessedOperationsRepository,Engine

// Item represents a decrypted view of a vault item on the server.
// It operates on already-encrypted fields; the server treats them as opaque bytes.
type Item struct {
	ItemID   string
	UserID   string
	ItemType int16

	TitleCiphertext    []byte
	TitleNonce         []byte
	MetadataCiphertext []byte
	MetadataNonce      []byte

	PayloadCiphertext []byte
	PayloadNonce      []byte

	BlobID   *string
	Version  uint64
	Checksum []byte

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// ChangeType mirrors api/proto ChangeType enum but stays decoupled from protobuf.
type ChangeType int16

const (
	ChangeTypeUnspecified ChangeType = 0
	ChangeTypeCreated     ChangeType = 1
	ChangeTypeUpdated     ChangeType = 2
	ChangeTypeDeleted     ChangeType = 3
)

// RevisionLogEntry represents a single row in revision_log.
type RevisionLogEntry struct {
	RevisionID  uint64
	UserID      string
	OperationID *string
	ItemID      string
	ChangeType  ChangeType
	ItemVersion uint64
	ChangedAt   time.Time
}

// Repository provides CRUD access to vault_items with optimistic concurrency.
type Repository interface {
	CreateItem(ctx context.Context, it *Item) error
	GetItem(ctx context.Context, userID, itemID string) (*Item, error)
	ListItems(ctx context.Context, userID string, includeDeleted bool, limit int32, pageToken string) ([]*Item, string, error)
	UpdateItem(ctx context.Context, it *Item, expectedVersion uint64) error
	DeleteItem(ctx context.Context, userID, itemID string, expectedVersion uint64, deletedAt time.Time) (uint64, error)
}

// RevisionLogRepository persists revision events for each mutation.
type RevisionLogRepository interface {
	AppendRevision(ctx context.Context, entry *RevisionLogEntry) error
}

// ProcessedOperationsRepository tracks idempotent operations.
type ProcessedOperationsRepository interface {
	MarkProcessed(ctx context.Context, userID, operationID, itemID string, revisionID uint64) error
	// Lookup returns the last processed operation for a given user and operationID, if any.
	Lookup(ctx context.Context, userID, operationID string) (*ProcessedOperation, error)
}

// ProcessedOperation represents a previously processed idempotent operation.
type ProcessedOperation struct {
	UserID      string
	OperationID string
	ItemID      string
	RevisionID  uint64
}

// Engine encapsulates vault business rules and persistence.
type Engine interface {
	Create(ctx context.Context, userID, operationID string, in CreateInput) (*CreateOutput, error)
	Update(ctx context.Context, userID, operationID, itemID string, expectedVersion uint64, in UpdateInput) (*UpdateOutput, error)
	Delete(ctx context.Context, userID, operationID, itemID string, expectedVersion uint64) (*DeleteOutput, error)
	Get(ctx context.Context, userID, itemID string) (*Item, error)
	List(ctx context.Context, userID string, includeDeleted bool, pageSize int32, pageToken string) ([]*Item, string, error)
}

// CreateInput contains data required to create a new item.
type CreateInput struct {
	ItemType           int16
	TitleCiphertext    []byte
	TitleNonce         []byte
	MetadataCiphertext []byte
	MetadataNonce      []byte
	PayloadCiphertext  []byte
	PayloadNonce       []byte
	BlobID             *string
	Checksum           []byte
}

type CreateOutput struct {
	ItemID        string
	Version       uint64
	ServerRevision uint64
}

type UpdateInput struct {
	TitleCiphertext    []byte
	TitleNonce         []byte
	MetadataCiphertext []byte
	MetadataNonce      []byte
	PayloadCiphertext  []byte
	PayloadNonce       []byte
	BlobID             *string
	Checksum           []byte
}

type UpdateOutput struct {
	NewVersion     uint64
	ServerRevision uint64
}

type DeleteOutput struct {
	NewVersion     uint64
	ServerRevision uint64
}

