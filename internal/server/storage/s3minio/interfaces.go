package s3minio

import (
	"context"

	"github.com/minio/minio-go/v7"
)

// Client is the subset of MinIO operations the server relies on.
type Client interface {
	ListBuckets(ctx context.Context) ([]minio.BucketInfo, error)
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error
}

// Connector dials MinIO and ensures the application bucket exists.
type Connector interface {
	Connect(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (Client, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -destination=../mocks/mock_s3minio.go -package=mocks . Client
