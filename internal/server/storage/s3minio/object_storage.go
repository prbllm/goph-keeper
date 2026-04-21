// Package s3minio connects to MinIO/S3 and implements blob.ObjectStorage for encrypted vault and blob payloads.
package s3minio

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/prbllm/goph-keeper/internal/server/blob"
)

// ObjectStorage implements blob.ObjectStorage on top of Client.
type ObjectStorage struct {
	client Client
	bucket string
}

// NewObjectStorage wraps a MinIO Client for object operations in one bucket.
func NewObjectStorage(client Client, bucket string) *ObjectStorage {
	return &ObjectStorage{
		client: client,
		bucket: bucket,
	}
}

var _ blob.ObjectStorage = (*ObjectStorage)(nil)

func (s *ObjectStorage) Put(ctx context.Context, objectKey string, r io.Reader, sizeBytes int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, r, sizeBytes, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *ObjectStorage) Get(ctx context.Context, objectKey string) (io.ReadCloser, int64, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, 0, err
	}
	return obj, info.Size, nil
}

func (s *ObjectStorage) Stat(ctx context.Context, objectKey string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return 0, blob.ErrObjectNotFound
		}
		return 0, err
	}
	return info.Size, nil
}

func (s *ObjectStorage) Delete(ctx context.Context, objectKey string) error {
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}
