package blob

import (
	"context"
	"errors"
	"time"
)

// ErrUploadSessionNotFound is returned when no upload session exists for the user and id.
var ErrUploadSessionNotFound = errors.New("upload session not found")

// ErrUploadSessionNotOpen means a session row exists but it is not open for completing the streaming upload
// (wrong status or received size does not match expected_size).
var ErrUploadSessionNotOpen = errors.New("upload session is not open for completing client upload")

// ErrUploadSessionExpired is returned when an upload session is past expires_at (e.g. during commit).
var ErrUploadSessionExpired = errors.New("upload session expired")

// ErrUploadSessionNotAwaitingCommit means the session exists but is not in awaiting-commit state.
var ErrUploadSessionNotAwaitingCommit = errors.New("upload session is not awaiting commit")

// UploadSessionStatus is stored in upload_sessions.status (SMALLINT).
type UploadSessionStatus int16

const (
	UploadSessionOpen           UploadSessionStatus = 1 // accepting stream chunks
	UploadSessionAwaitingCommit UploadSessionStatus = 2 // bytes in object storage, waiting for CommitBlobUpload
)

// UploadSession mirrors the upload_sessions table.
type UploadSession struct {
	UploadSessionID  string
	BlobID           string
	UserID           string
	ExpectedSize     uint64
	ExpectedChecksum []byte
	ReceivedSize     uint64
	Status           UploadSessionStatus
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// UploadSessionStore manages upload_sessions rows and related blob state for BlobService RPCs.
type UploadSessionStore interface {
	// StartSession inserts the blob row and upload session in one transaction.
	StartSession(ctx context.Context, b *Blob, s *UploadSession) error
	// GetSession returns ErrUploadSessionNotFound if there is no session for this user and id.
	GetSession(ctx context.Context, userID, sessionID string) (*UploadSession, error)
	// DeleteSession returns ErrUploadSessionNotFound if there is no session for this user and id.
	DeleteSession(ctx context.Context, userID, sessionID string) error
	// CompleteClientUpload marks the session awaiting commit and sets blob status to uploading.
	CompleteClientUpload(ctx context.Context, userID, sessionID string, receivedSize uint64) error
	// CommitUploadedBlob deletes the session row and marks the blob committed (expects awaiting-commit session).
	CommitUploadedBlob(ctx context.Context, userID, sessionID string, committedAt time.Time) error
}
