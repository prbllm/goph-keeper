package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"go.uber.org/zap"
)

// uploadSessionCleanupStore is implemented by *postgres.UploadStore.
type uploadSessionCleanupStore interface {
	ListExpiredUploadBlobRefs(ctx context.Context, expiresBefore time.Time, limit int) ([]postgres.ExpiredUploadBlobRef, error)
	DeleteBlobByID(ctx context.Context, blobID string) error
}

// RunUploadSessionCleanup runs until ctx is cancelled. It performs an immediate pass,
// then waits interval between subsequent passes. Each pass uses runTimeout for SQL and storage calls.
func RunUploadSessionCleanup(
	ctx context.Context,
	logger *zap.Logger,
	uploadStore uploadSessionCleanupStore,
	objectStorage blob.ObjectStorage,
	interval time.Duration,
	runTimeout time.Duration,
) {
	if interval <= 0 {
		logger.Debug("upload session cleanup disabled (non-positive interval)")
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		runCtx, cancel := context.WithTimeout(ctx, runTimeout)
		n, err := cleanupExpiredUploadSessionsOnce(
			runCtx, logger, uploadStore, objectStorage, config.UploadSessionCleanupListBatchSize)
		cancel()
		if err != nil {
			logger.Error("upload session cleanup pass failed", zap.Error(err))
		} else if n > 0 {
			logger.Info("upload session cleanup removed expired sessions", zap.Int("blobs_removed", n))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func cleanupExpiredUploadSessionsOnce(
	ctx context.Context,
	logger *zap.Logger,
	store uploadSessionCleanupStore,
	objectStorage blob.ObjectStorage,
	listBatchSize int,
) (removed int, err error) {
	if listBatchSize <= 0 {
		listBatchSize = config.UploadSessionCleanupListBatchSize
	}
	cutoff := time.Now().UTC()
	for {
		refs, err := store.ListExpiredUploadBlobRefs(ctx, cutoff, listBatchSize)
		if err != nil {
			return removed, err
		}
		if len(refs) == 0 {
			break
		}
		for _, ref := range refs {
			if err := objectStorage.Delete(ctx, ref.ObjectKey); err != nil {
				logger.Warn("upload session cleanup: object delete failed (continuing with blob row removal)",
					zap.String("blob_id", ref.BlobID),
					zap.String("object_key", ref.ObjectKey),
					zap.Error(err))
			}
			if err := store.DeleteBlobByID(ctx, ref.BlobID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				logger.Warn("upload session cleanup: blob delete failed",
					zap.String("blob_id", ref.BlobID),
					zap.Error(err))
				continue
			}
			removed++
		}
		if len(refs) < listBatchSize {
			break
		}
	}
	return removed, nil
}
