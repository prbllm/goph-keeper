package blob

import (
	"context"
	"io"
)

// ObjectStorage abstracts blob content storage in S3/MinIO.
// It operates on opaque object keys; association with users and blob IDs
// is handled at the Repository/domain level.
type ObjectStorage interface {
	// Put stores object content under the given key.
	// sizeBytes may be -1 if the size is unknown in advance; the MinIO client then uses a multipart upload path
	// (see minio-go PutObject). Prefer a non-negative size when you know it.
	Put(ctx context.Context, objectKey string, r io.Reader, sizeBytes int64, contentType string) error

	// Get returns a reader for the object content and its size in bytes, if known.
	// The caller is responsible for closing the reader.
	Get(ctx context.Context, objectKey string) (io.ReadCloser, int64, error)

	// Delete removes object content by key. It should be idempotent:
	// deleting a non-existent object must not return an error.
	Delete(ctx context.Context, objectKey string) error
}

