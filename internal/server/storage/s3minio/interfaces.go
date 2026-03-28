package s3minio

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
)

// Client is the MinIO surface used by the server: bucket setup at connect time
// and object I/O for blobs. Implementations are typically *minio.Client.
type Client interface {
	ListBuckets(ctx context.Context) ([]minio.BucketInfo, error)
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error

	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

// Connector dials MinIO and ensures the application bucket exists.
type Connector interface {
	Connect(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (Client, error)
}

//go:generate go run go.uber.org/mock/mockgen -typed -destination=../mocks/mock_s3minio.go -package=mocks . Client
