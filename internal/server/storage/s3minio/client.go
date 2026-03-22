package s3minio

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/prbllm/goph-keeper/internal/server/config"
)

var _ Client = (*minio.Client)(nil)

// DefaultConnector builds a MinIO client and ensures the bucket exists.
var DefaultConnector Connector = minioConnector{}

type minioConnector struct{}

// Connect implements Connector.
func (minioConnector) Connect(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(config.MinIOConnectTimeoutSeconds)*time.Second)
	defer cancel()
	if _, err := client.ListBuckets(checkCtx); err != nil {
		return nil, fmt.Errorf("minio list buckets: %w", err)
	}

	exists, err := client.BucketExists(checkCtx, bucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket exists: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(checkCtx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio make bucket: %w", err)
		}
	}
	return client, nil
}

// Connect is equivalent to DefaultConnector.Connect.
func Connect(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (Client, error) {
	return DefaultConnector.Connect(ctx, endpoint, accessKey, secretKey, bucket, useSSL)
}
