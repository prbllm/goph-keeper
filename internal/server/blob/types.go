package blob

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Repository when no blob row exists for the user and blob id
// (GetByID, or MarkCommitted/MarkDeleted when no row was updated).
var ErrNotFound = errors.New("blob not found")

// Status mirrors api/proto BlobStatus enum but stays decoupled from protobuf.
type Status int16

const (
	StatusUnspecified Status = 0
	StatusPending     Status = 1
	StatusUploading   Status = 2
	StatusCommitted   Status = 3
	StatusFailed      Status = 4
	StatusDeleted     Status = 5
)

// Blob represents a single blob row stored in PostgreSQL.
// The actual encrypted content is stored in S3/MinIO and addressed by ObjectKey.
type Blob struct {
	BlobID    string
	UserID    string
	ObjectKey string

	SizeBytes uint64
	Checksum  []byte

	ContentKind string
	FileName    *string
	MimeType    *string

	Status Status

	CreatedAt   time.Time
	CommittedAt *time.Time
	DeletedAt   *time.Time
}

// Repository provides access to blob metadata stored in PostgreSQL.
type Repository interface {
	// Create a new blob row with all fields.
	Create(ctx context.Context, b *Blob) error
	// GetByID returns ErrNotFound if there is no blob for this user and id.
	GetByID(ctx context.Context, userID, blobID string) (*Blob, error)
	// MarkCommitted returns ErrNotFound when no row matches user_id and blob_id.
	MarkCommitted(ctx context.Context, userID, blobID string, committedAt time.Time) error
	// MarkDeleted returns ErrNotFound when no row matches user_id and blob_id.
	MarkDeleted(ctx context.Context, userID, blobID string, deletedAt time.Time) error
}
