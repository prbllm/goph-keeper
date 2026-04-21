package s3minio_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
)

func TestConnect_unreachable(t *testing.T) {
	ctx := context.Background()
	_, err := s3minio.Connect(ctx, "127.0.0.1:1", "dummy", "dummy", "any-bucket", false)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "minio") {
		t.Errorf("error should mention minio, got: %v", err)
	}
}

func TestConnect_integration(t *testing.T) {
	endpoint := osGetenvDefault("GOPHKEEPER_MINIO_ENDPOINT", "127.0.0.1:9000")
	access := os.Getenv("GOPHKEEPER_MINIO_ACCESS_KEY")
	secret := os.Getenv("GOPHKEEPER_MINIO_SECRET_KEY")
	bucket := osGetenvDefault("GOPHKEEPER_MINIO_BUCKET", "gophkeeper")
	if access == "" || secret == "" {
		t.Skip("GOPHKEEPER_MINIO_ACCESS_KEY / GOPHKEEPER_MINIO_SECRET_KEY not set, skipping integration test")
	}

	useSSL := os.Getenv("GOPHKEEPER_MINIO_USE_SSL") == "true"

	ctx := context.Background()
	start := time.Now()
	client, err := s3minio.Connect(ctx, endpoint, access, secret, bucket, useSSL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if time.Since(start) > 2*time.Minute {
		t.Logf("slow connect: %s", time.Since(start))
	}
	_ = client
}

func osGetenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
