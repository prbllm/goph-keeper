package jobs

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"go.uber.org/zap"
)

type seqCleanupStore struct {
	returns [][]postgres.ExpiredUploadBlobRef
	call    int
	deleted []string
	delErr  map[string]error
}

func (s *seqCleanupStore) ListExpiredUploadBlobRefs(ctx context.Context, expiresBefore time.Time, limit int) ([]postgres.ExpiredUploadBlobRef, error) {
	_ = ctx
	_ = expiresBefore
	_ = limit
	if s.call >= len(s.returns) {
		return nil, nil
	}
	r := s.returns[s.call]
	s.call++
	return r, nil
}

func (s *seqCleanupStore) DeleteBlobByID(ctx context.Context, blobID string) error {
	_ = ctx
	if err, ok := s.delErr[blobID]; ok {
		return err
	}
	s.deleted = append(s.deleted, blobID)
	return nil
}

type recordingObjectStorage struct {
	deleted []string
	failKey string
}

func (s *recordingObjectStorage) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("unexpected Put")
}

func (s *recordingObjectStorage) Get(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("unexpected Get")
}

func (s *recordingObjectStorage) Stat(context.Context, string) (int64, error) {
	return 0, errors.New("unexpected Stat")
}

func (s *recordingObjectStorage) Delete(ctx context.Context, objectKey string) error {
	_ = ctx
	if s.failKey != "" && objectKey == s.failKey {
		return errors.New("object delete failed")
	}
	s.deleted = append(s.deleted, objectKey)
	return nil
}

var _ blob.ObjectStorage = (*recordingObjectStorage)(nil)

func TestCleanupExpiredUploadSessionsOnce_removesAndCounts(t *testing.T) {
	store := &seqCleanupStore{
		returns: [][]postgres.ExpiredUploadBlobRef{
			{{BlobID: "b1", ObjectKey: "k1"}, {BlobID: "b2", ObjectKey: "k2"}},
		},
	}
	obj := &recordingObjectStorage{}
	logger := zap.NewNop()
	n, err := cleanupExpiredUploadSessionsOnce(context.Background(), logger, store, obj, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("removed = %d, want 2", n)
	}
	if len(obj.deleted) != 2 || obj.deleted[0] != "k1" || obj.deleted[1] != "k2" {
		t.Fatalf("object deletes = %#v", obj.deleted)
	}
	if len(store.deleted) != 2 || store.deleted[0] != "b1" || store.deleted[1] != "b2" {
		t.Fatalf("blob deletes = %#v", store.deleted)
	}
}

func TestCleanupExpiredUploadSessionsOnce_batchesUntilShortRead(t *testing.T) {
	store := &seqCleanupStore{
		returns: [][]postgres.ExpiredUploadBlobRef{
			{{BlobID: "a", ObjectKey: "ka"}, {BlobID: "b", ObjectKey: "kb"}},
			{{BlobID: "c", ObjectKey: "kc"}},
		},
	}
	obj := &recordingObjectStorage{}
	logger := zap.NewNop()
	n, err := cleanupExpiredUploadSessionsOnce(context.Background(), logger, store, obj, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("removed = %d, want 3", n)
	}
	if store.call != 2 {
		t.Fatalf("list calls = %d, want 2", store.call)
	}
}

func TestCleanupExpiredUploadSessionsOnce_skipsNoRowsBlob(t *testing.T) {
	store := &seqCleanupStore{
		returns: [][]postgres.ExpiredUploadBlobRef{
			{{BlobID: "gone", ObjectKey: "kg"}},
		},
		delErr: map[string]error{"gone": sql.ErrNoRows},
	}
	obj := &recordingObjectStorage{}
	logger := zap.NewNop()
	n, err := cleanupExpiredUploadSessionsOnce(context.Background(), logger, store, obj, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("removed = %d, want 0", n)
	}
}

func TestCleanupExpiredUploadSessionsOnce_objectDeleteErrorStillDeletesBlob(t *testing.T) {
	store := &seqCleanupStore{
		returns: [][]postgres.ExpiredUploadBlobRef{
			{{BlobID: "b1", ObjectKey: "bad-key"}},
		},
	}
	obj := &recordingObjectStorage{failKey: "bad-key"}
	logger := zap.NewNop()
	n, err := cleanupExpiredUploadSessionsOnce(context.Background(), logger, store, obj, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed = %d, want 1", n)
	}
	if len(obj.deleted) != 0 {
		t.Fatalf("expected no successful object deletes, got %#v", obj.deleted)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "b1" {
		t.Fatalf("blob deletes = %#v", store.deleted)
	}
}

func TestCleanupExpiredUploadSessionsOnce_nonPositiveBatchUsesDefault(t *testing.T) {
	store := &seqCleanupStore{
		returns: [][]postgres.ExpiredUploadBlobRef{
			{{BlobID: "x", ObjectKey: "kx"}},
		},
	}
	obj := &recordingObjectStorage{}
	logger := zap.NewNop()
	n, err := cleanupExpiredUploadSessionsOnce(context.Background(), logger, store, obj, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed = %d, want 1", n)
	}
}
