package s3minio_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/storage/mocks"
	"github.com/prbllm/goph-keeper/internal/server/storage/s3minio"
	"go.uber.org/mock/gomock"
)

func TestObjectStorage_Put(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().PutObject(ctx, "bucket", "object-key", gomock.AssignableToTypeOf((*strings.Reader)(nil)), int64(2),
		minio.PutObjectOptions{ContentType: "text/plain"}).Return(minio.UploadInfo{}, nil)

	s := s3minio.NewObjectStorage(mc, "bucket")
	err := s.Put(ctx, "object-key", strings.NewReader("ab"), 2, "text/plain")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestObjectStorage_Put_unknownSize(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().PutObject(ctx, "b", "k", gomock.AssignableToTypeOf((*strings.Reader)(nil)), int64(-1),
		minio.PutObjectOptions{ContentType: "application/octet-stream"}).Return(minio.UploadInfo{}, nil)

	s := s3minio.NewObjectStorage(mc, "b")
	if err := s.Put(ctx, "k", strings.NewReader("x"), -1, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
}

func TestObjectStorage_Get_clientError(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	want := errors.New("boom")
	mc.EXPECT().GetObject(ctx, "bucket", "k", minio.GetObjectOptions{}).Return(nil, want)

	s := s3minio.NewObjectStorage(mc, "bucket")
	_, _, err := s.Get(ctx, "k")
	if !errors.Is(err, want) {
		t.Fatalf("Get err: got %v want %v", err, want)
	}
}

func TestObjectStorage_Get_nilObject(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().GetObject(ctx, "bucket", "k", minio.GetObjectOptions{}).Return(nil, nil)

	s := s3minio.NewObjectStorage(mc, "bucket")
	_, _, err := s.Get(ctx, "k")
	if err == nil {
		t.Fatal("expected error when GetObject returns nil object")
	}
}

func TestObjectStorage_Delete(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().RemoveObject(ctx, "bucket", "k", minio.RemoveObjectOptions{}).Return(nil)

	s := s3minio.NewObjectStorage(mc, "bucket")
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
}

func TestObjectStorage_Stat(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().StatObject(ctx, "bucket", "k", minio.StatObjectOptions{}).Return(minio.ObjectInfo{Size: 42}, nil)

	s := s3minio.NewObjectStorage(mc, "bucket")
	n, err := s.Stat(ctx, "k")
	if err != nil || n != 42 {
		t.Fatalf("Stat: n=%d err=%v", n, err)
	}
}

func TestObjectStorage_Stat_noSuchKey(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockClient(ctrl)
	mc.EXPECT().StatObject(ctx, "bucket", "k", minio.StatObjectOptions{}).Return(minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey"})

	s := s3minio.NewObjectStorage(mc, "bucket")
	_, err := s.Stat(ctx, "k")
	if !errors.Is(err, blob.ErrObjectNotFound) {
		t.Fatalf("Stat err=%v want ErrObjectNotFound", err)
	}
}
