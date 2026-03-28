package grpcserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func vaultPayloadToProto(it *vault.Item) *gophkeeperv1.EncryptedField {
	if it == nil {
		return nil
	}
	return &gophkeeperv1.EncryptedField{
		Ciphertext: it.PayloadCiphertext,
		Nonce:      it.PayloadNonce,
	}
}

func effectiveMaxPayloadCiphertextBytes(cfg *config.Config) uint64 {
	if cfg.GRPCMaxMessageBytes <= config.VaultGRPCProtoReserveBytes {
		return 0
	}
	grpcBudget := cfg.GRPCMaxMessageBytes - config.VaultGRPCProtoReserveBytes
	if cfg.MaxBlobSizeBytes < grpcBudget {
		return cfg.MaxBlobSizeBytes
	}
	return grpcBudget
}

func encryptedFieldEmpty(f *gophkeeperv1.EncryptedField) bool {
	if f == nil {
		return true
	}
	return len(f.GetCiphertext()) == 0 && len(f.GetNonce()) == 0
}

// resolveVaultPayload maps request payload/blob_id to vault storage: small inline in PostgreSQL,
// large ciphertext uploaded to S3 as a single object, or reference to an existing committed blob.
// now sets blob CreatedAt / CommittedAt; if nil, time.Now is used.
func resolveVaultPayload(ctx context.Context, log *zap.Logger, cfg *config.Config, blobRepo blob.Repository, blobStorage blob.ObjectStorage, now func() time.Time, userID string, payload *gophkeeperv1.EncryptedField, blobIDReq string) (
	payloadCiphertext []byte,
	payloadNonce []byte,
	blobID *string,
	checksum []byte,
	err error,
) {
	if log == nil {
		log = zap.NewNop()
	}
	if now == nil {
		now = time.Now
	}

	if cfg == nil || blobRepo == nil || blobStorage == nil {
		return nil, nil, nil, nil, status.Error(codes.Internal, "vault: storage not configured")
	}

	maxCipher := effectiveMaxPayloadCiphertextBytes(cfg)

	blobIDReq = strings.TrimSpace(blobIDReq)
	if blobIDReq != "" {
		if !encryptedFieldEmpty(payload) {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "blob_id and payload are mutually exclusive")
		}
		b, err := blobRepo.GetByID(ctx, userID, blobIDReq)
		if errors.Is(err, blob.ErrNotFound) {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "blob not found")
		}
		if err != nil {
			log.Error("vault: blob lookup failed", zap.Error(err))
			return nil, nil, nil, nil, status.Error(codes.Internal, "vault: blob lookup failed")
		}
		if b.Status != blob.StatusCommitted {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "blob is not committed")
		}
		if b.DeletedAt != nil {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "blob is deleted")
		}
		if strings.TrimSpace(b.ObjectKey) == "" {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "vault: blob object key missing")
		}
		stSize, err := blobStorage.Stat(ctx, b.ObjectKey)
		if err != nil {
			if errors.Is(err, blob.ErrObjectNotFound) {
				return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "vault: blob missing from object storage")
			}
			log.Error("vault: blob object stat failed", zap.String("object_key", b.ObjectKey), zap.Error(err))
			return nil, nil, nil, nil, status.Error(codes.Internal, "vault: blob object stat failed")
		}
		if stSize < 0 {
			log.Error("vault: blob object stat returned negative size",
				zap.String("object_key", b.ObjectKey),
				zap.Int64("size", stSize))
			return nil, nil, nil, nil, status.Error(codes.Internal, "vault: blob object size invalid")
		}
		if uint64(stSize) != b.SizeBytes {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "vault: blob size does not match metadata")
		}
		if uint64(stSize) > maxCipher {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "referenced blob exceeds configured size limit")
		}
		sum := make([]byte, len(b.Checksum))
		copy(sum, b.Checksum)
		return nil, nil, &blobIDReq, sum, nil
	}

	if payload == nil {
		return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "payload is required when blob_id is empty")
	}

	ct := payload.GetCiphertext()
	nonce := payload.GetNonce()
	if len(ct) == 0 {
		if len(nonce) > 0 {
			return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "invalid payload: empty ciphertext with non-empty nonce")
		}
		return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "invalid payload: empty ciphertext")
	}
	if len(nonce) == 0 {
		return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "invalid payload: missing nonce")
	}

	if uint64(len(ct)) > maxCipher {
		return nil, nil, nil, nil, status.Error(codes.InvalidArgument, "payload ciphertext exceeds configured limit")
	}

	if uint64(len(ct)) <= cfg.InlineThresholdBytes {
		return ct, nonce, nil, nil, nil
	}

	blobUUID := uuid.NewString()
	objectKey := "users/" + userID + "/blobs/" + blobUUID
	sum := sha256.Sum256(ct)
	checksum = sum[:]

	if err := blobStorage.Put(ctx, objectKey, bytes.NewReader(ct), int64(len(ct)), "application/octet-stream"); err != nil {
		log.Error("vault: blob upload failed", zap.String("object_key", objectKey), zap.Error(err))
		return nil, nil, nil, nil, status.Error(codes.Internal, "vault: blob upload failed")
	}

	ts := now().UTC()
	brow := &blob.Blob{
		BlobID:      blobUUID,
		UserID:      userID,
		ObjectKey:   objectKey,
		SizeBytes:   uint64(len(ct)),
		Checksum:    checksum,
		ContentKind: "application/octet-stream",
		Status:      blob.StatusCommitted,
		CreatedAt:   ts,
		CommittedAt: &ts,
	}
	if err := blobRepo.Create(ctx, brow); err != nil {
		if delErr := blobStorage.Delete(ctx, objectKey); delErr != nil {
			log.Warn("vault: compensating blob delete failed",
				zap.String("object_key", objectKey),
				zap.Error(delErr))
		}
		log.Error("vault: blob metadata persist failed", zap.String("object_key", objectKey), zap.Error(err))
		return nil, nil, nil, nil, status.Error(codes.Internal, "vault: blob metadata persist failed")
	}

	bid := blobUUID
	return nil, nil, &bid, checksum, nil
}
