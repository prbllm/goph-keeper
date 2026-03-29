package grpcserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type blobHandler struct {
	gophkeeperv1.UnimplementedBlobServiceServer
	logger      *zap.Logger
	cfg         *config.Config
	uploadLocks *uploadSessionLocks
	uploads     blob.UploadSessionStore
	blobRepo    blob.Repository
	blobStorage blob.ObjectStorage
}

func (h blobHandler) StartBlobUpload(ctx context.Context, req *gophkeeperv1.StartBlobUploadRequest) (*gophkeeperv1.StartBlobUploadResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if h.cfg == nil || h.uploads == nil || h.blobRepo == nil || h.blobStorage == nil {
		return nil, status.Error(codes.Internal, "blob: not configured")
	}
	if h.cfg.UploadSessionTTLHours <= 0 {
		return nil, status.Error(codes.Internal, "blob: upload session ttl is not configured")
	}

	expectedSize := req.GetExpectedSize()
	if expectedSize > h.cfg.MaxBlobSizeBytes {
		return nil, status.Errorf(codes.InvalidArgument, "expected_size exceeds max_blob_size_bytes (%d)", h.cfg.MaxBlobSizeBytes)
	}
	sum := req.GetExpectedChecksum()
	if len(sum) == 0 {
		return nil, status.Error(codes.InvalidArgument, "expected_checksum is required")
	}
	contentKind := strings.TrimSpace(req.GetContentKind())
	if contentKind == "" {
		return nil, status.Error(codes.InvalidArgument, "content_kind is required")
	}

	maxChunk := h.cfg.MaxChunkSizeBytes
	if maxChunk == 0 {
		return nil, status.Error(codes.Internal, "max_chunk_size_bytes is not configured")
	}

	blobUUID := uuid.NewString()
	sessionID := uuid.NewString()
	objectKey := "users/" + userID + "/blobs/" + blobUUID
	now := time.Now().UTC()
	expires := now.Add(time.Duration(h.cfg.UploadSessionTTLHours) * time.Hour)

	var fileName *string
	if fn := strings.TrimSpace(req.GetFileName()); fn != "" {
		fileName = &fn
	}
	var mimeType *string
	if mt := strings.TrimSpace(req.GetMimeType()); mt != "" {
		mimeType = &mt
	}

	brow := &blob.Blob{
		BlobID:      blobUUID,
		UserID:      userID,
		ObjectKey:   objectKey,
		SizeBytes:   expectedSize,
		Checksum:    append([]byte(nil), sum...),
		ContentKind: contentKind,
		FileName:    fileName,
		MimeType:    mimeType,
		Status:      blob.StatusPending,
		CreatedAt:   now,
		CommittedAt: nil,
		DeletedAt:   nil,
	}

	us := &blob.UploadSession{
		UploadSessionID:  sessionID,
		BlobID:           blobUUID,
		UserID:           userID,
		ExpectedSize:     expectedSize,
		ExpectedChecksum: append([]byte(nil), sum...),
		ReceivedSize:     0,
		Status:           blob.UploadSessionOpen,
		ExpiresAt:        expires,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := h.uploads.StartSession(ctx, brow, us); err != nil {
		log := h.logger
		if log == nil {
			log = zap.NewNop()
		}
		log.Error("blob: start session failed", zap.Error(err))
		return nil, status.Error(codes.Internal, "blob: start upload failed")
	}

	return &gophkeeperv1.StartBlobUploadResponse{
		UploadSessionId: sessionID,
		BlobId:          blobUUID,
		MaxChunkSize:    maxChunk,
		ExpiresAt:       timestamppb.New(expires),
	}, nil
}

func (h blobHandler) UploadBlob(stream gophkeeperv1.BlobService_UploadBlobServer) error {
	ctx := stream.Context()
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing auth context")
	}
	if h.cfg == nil || h.uploads == nil || h.blobRepo == nil || h.blobStorage == nil {
		return status.Error(codes.Internal, "blob: not configured")
	}
	if h.uploadLocks == nil {
		return status.Error(codes.Internal, "blob: not configured")
	}

	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	header := msg.GetHeader()
	if header == nil || strings.TrimSpace(header.GetUploadSessionId()) == "" {
		return status.Error(codes.InvalidArgument, "first message must be UploadBlobHeader with upload_session_id")
	}
	sessionID := strings.TrimSpace(header.GetUploadSessionId())

	release := h.uploadLocks.acquire(uploadLockKey(userID, sessionID))
	defer release()

	sess, err := h.uploads.GetSession(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, blob.ErrUploadSessionNotFound) {
			return status.Error(codes.NotFound, "upload session not found")
		}
		return status.Error(codes.Internal, "blob: load session failed")
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, "")
		return status.Error(codes.FailedPrecondition, "upload session expired")
	}
	if sess.Status != blob.UploadSessionOpen {
		return status.Error(codes.FailedPrecondition, "upload session is not open")
	}

	bmeta, err := h.blobRepo.GetByID(ctx, userID, sess.BlobID)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return status.Error(codes.NotFound, "blob not found")
		}
		return status.Error(codes.Internal, "blob: load blob failed")
	}
	if bmeta.Status != blob.StatusPending {
		return status.Error(codes.FailedPrecondition, "blob is not pending upload")
	}

	maxChunk := h.cfg.MaxChunkSizeBytes
	// Whole object is buffered in memory up to MaxBlobSizeBytes; streaming straight to object storage
	// would avoid this footprint for large blobs.
	var buf bytes.Buffer
	if sess.ExpectedSize > 0 {
		buf.Grow(int(sess.ExpectedSize))
	}
	hasher := sha256.New()
	var total uint64
	var nextChunk uint64

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		chunk := msg.GetChunk()
		if chunk == nil {
			return status.Error(codes.InvalidArgument, "expected chunk after header")
		}
		if chunk.GetChunkIndex() != nextChunk {
			_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
			return status.Errorf(codes.InvalidArgument, "unexpected chunk_index: want %d", nextChunk)
		}
		data := chunk.GetData()
		if uint64(len(data)) > maxChunk {
			_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
			return status.Errorf(codes.InvalidArgument, "chunk exceeds max_chunk_size (%d)", maxChunk)
		}
		if total+uint64(len(data)) > sess.ExpectedSize {
			_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
			return status.Error(codes.InvalidArgument, "total upload size exceeds expected_size")
		}
		if _, err := hasher.Write(data); err != nil {
			return status.Error(codes.Internal, "blob: hash failed")
		}
		if _, err := buf.Write(data); err != nil {
			return status.Error(codes.Internal, "blob: buffer write failed")
		}
		total += uint64(len(data))
		nextChunk++
	}

	if total != sess.ExpectedSize {
		_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
		return status.Error(codes.InvalidArgument, "received size does not match expected_size")
	}
	actualSum := hasher.Sum(nil)
	if !bytes.Equal(actualSum, sess.ExpectedChecksum) {
		_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
		return status.Error(codes.InvalidArgument, "checksum mismatch")
	}

	contentType := "application/octet-stream"
	if bmeta.MimeType != nil && strings.TrimSpace(*bmeta.MimeType) != "" {
		contentType = strings.TrimSpace(*bmeta.MimeType)
	} else if strings.TrimSpace(bmeta.ContentKind) != "" {
		contentType = bmeta.ContentKind
	}

	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
		return status.Error(codes.FailedPrecondition, "upload session expired")
	}

	if err := h.blobStorage.Put(ctx, bmeta.ObjectKey, bytes.NewReader(buf.Bytes()), int64(total), contentType); err != nil {
		log := h.logger
		if log == nil {
			log = zap.NewNop()
		}
		log.Error("blob: put object failed", zap.String("object_key", bmeta.ObjectKey), zap.Error(err))
		_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, bmeta.ObjectKey)
		return status.Error(codes.Internal, "blob: storage upload failed")
	}

	if err := h.uploads.CompleteClientUpload(ctx, userID, sessionID, total); err != nil {
		log := h.logger
		if log == nil {
			log = zap.NewNop()
		}
		log.Error("blob: complete upload transaction failed", zap.Error(err))
		if !errors.Is(err, blob.ErrUploadSessionNotOpen) {
			_ = h.blobStorage.Delete(ctx, bmeta.ObjectKey)
			_ = h.abortUpload(ctx, userID, sessionID, sess.BlobID, "")
		}
		return status.Error(codes.Internal, "blob: finalize upload failed")
	}

	return stream.SendAndClose(&gophkeeperv1.UploadBlobResponse{
		UploadSessionId: sessionID,
		BlobId:          sess.BlobID,
		ReceivedSize:    total,
		Status:          gophkeeperv1.BlobStatus_BLOB_STATUS_UPLOADING,
	})
}

func (h blobHandler) abortUpload(ctx context.Context, userID, sessionID, blobID, objectKey string) error {
	_ = h.uploads.DeleteSession(ctx, userID, sessionID)
	if blobID != "" {
		_ = h.blobRepo.MarkFailed(ctx, userID, blobID, time.Now().UTC())
	}
	if objectKey != "" {
		_ = h.blobStorage.Delete(ctx, objectKey)
	}
	return nil
}

func (h blobHandler) CommitBlobUpload(ctx context.Context, req *gophkeeperv1.CommitBlobUploadRequest) (*gophkeeperv1.CommitBlobUploadResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil || strings.TrimSpace(req.GetUploadSessionId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "upload_session_id is required")
	}
	if h.uploads == nil || h.blobRepo == nil || h.blobStorage == nil {
		return nil, status.Error(codes.Internal, "blob: not configured")
	}

	sessionID := strings.TrimSpace(req.GetUploadSessionId())
	sess, err := h.uploads.GetSession(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, blob.ErrUploadSessionNotFound) {
			return nil, status.Error(codes.NotFound, "upload session not found")
		}
		return nil, status.Error(codes.Internal, "blob: load session failed")
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, status.Error(codes.FailedPrecondition, "upload session expired")
	}
	if sess.Status != blob.UploadSessionAwaitingCommit {
		return nil, status.Error(codes.FailedPrecondition, "upload is not ready to commit")
	}

	bmeta, err := h.blobRepo.GetByID(ctx, userID, sess.BlobID)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "blob not found")
		}
		return nil, status.Error(codes.Internal, "blob: load blob failed")
	}
	if bmeta.Status != blob.StatusUploading {
		return nil, status.Error(codes.FailedPrecondition, "blob is not ready to commit")
	}

	stSize, err := h.blobStorage.Stat(ctx, bmeta.ObjectKey)
	if err != nil {
		if errors.Is(err, blob.ErrObjectNotFound) {
			return nil, status.Error(codes.FailedPrecondition, "blob object missing from storage")
		}
		return nil, status.Error(codes.Internal, "blob: stat object failed")
	}
	if stSize < 0 || uint64(stSize) != bmeta.SizeBytes {
		return nil, status.Error(codes.FailedPrecondition, "stored object size does not match metadata")
	}

	now := time.Now().UTC()
	if err := h.uploads.CommitUploadedBlob(ctx, userID, sessionID, now); err != nil {
		switch {
		case errors.Is(err, blob.ErrUploadSessionNotFound):
			return nil, status.Error(codes.NotFound, "upload session not found")
		case errors.Is(err, blob.ErrUploadSessionExpired):
			return nil, status.Error(codes.FailedPrecondition, "upload session expired")
		case errors.Is(err, blob.ErrUploadSessionNotAwaitingCommit):
			return nil, status.Error(codes.FailedPrecondition, "upload is not ready to commit")
		case errors.Is(err, blob.ErrNotFound):
			return nil, status.Error(codes.FailedPrecondition, "blob commit rejected")
		default:
			return nil, status.Error(codes.Internal, "blob: commit failed")
		}
	}

	bmeta, err = h.blobRepo.GetByID(ctx, userID, sess.BlobID)
	if err != nil {
		return nil, status.Error(codes.Internal, "blob: reload after commit failed")
	}

	return &gophkeeperv1.CommitBlobUploadResponse{
		Blob: toProtoBlobInfo(bmeta),
	}, nil
}

func (h blobHandler) DownloadBlob(req *gophkeeperv1.DownloadBlobRequest, stream gophkeeperv1.BlobService_DownloadBlobServer) error {
	ctx := stream.Context()
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil || strings.TrimSpace(req.GetBlobId()) == "" {
		return status.Error(codes.InvalidArgument, "blob_id is required")
	}
	if h.cfg == nil || h.blobRepo == nil || h.blobStorage == nil {
		return status.Error(codes.Internal, "blob: not configured")
	}

	bmeta, err := h.blobRepo.GetByID(ctx, userID, strings.TrimSpace(req.GetBlobId()))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return status.Error(codes.NotFound, "blob not found")
		}
		return status.Error(codes.Internal, "blob: load blob failed")
	}
	if bmeta.Status != blob.StatusCommitted {
		return status.Error(codes.FailedPrecondition, "blob is not committed")
	}
	if bmeta.DeletedAt != nil {
		return status.Error(codes.NotFound, "blob not found")
	}

	rc, sz, err := h.blobStorage.Get(ctx, bmeta.ObjectKey)
	if err != nil {
		return status.Error(codes.Internal, "blob: open object failed")
	}
	defer func() { _ = rc.Close() }()
	if sz >= 0 && uint64(sz) != bmeta.SizeBytes {
		return status.Error(codes.Internal, "blob: object size mismatch")
	}

	if err := stream.Send(&gophkeeperv1.DownloadBlobResponse{
		Body: &gophkeeperv1.DownloadBlobResponse_Header{
			Header: &gophkeeperv1.DownloadBlobHeader{
				Blob: toProtoBlobInfo(bmeta),
			},
		},
	}); err != nil {
		return err
	}

	maxChunk := h.cfg.MaxChunkSizeBytes
	if maxChunk == 0 {
		return status.Error(codes.Internal, "max_chunk_size_bytes is not configured")
	}

	var sent uint64
	var idx uint64
	buf := make([]byte, maxChunk)
	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			payload := slices.Clone(buf[:n])
			if err := stream.Send(&gophkeeperv1.DownloadBlobResponse{
				Body: &gophkeeperv1.DownloadBlobResponse_Chunk{
					Chunk: &gophkeeperv1.DownloadBlobChunk{
						ChunkIndex: idx,
						Data:       payload,
					},
				},
			}); err != nil {
				return err
			}
			sent += uint64(n)
			idx++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return status.Error(codes.Internal, "blob: read object failed")
		}
	}
	if sent != bmeta.SizeBytes {
		return status.Error(codes.Internal, "blob: short read from storage")
	}
	return nil
}

func toProtoBlobInfo(b *blob.Blob) *gophkeeperv1.BlobInfo {
	if b == nil {
		return nil
	}
	out := &gophkeeperv1.BlobInfo{
		BlobId:      b.BlobID,
		UserId:      b.UserID,
		ObjectKey:   b.ObjectKey,
		SizeBytes:   b.SizeBytes,
		Checksum:    append([]byte(nil), b.Checksum...),
		ContentKind: b.ContentKind,
		Status:      gophkeeperv1.BlobStatus(b.Status),
		CreatedAt:   timestamppb.New(b.CreatedAt),
	}
	if b.FileName != nil {
		out.FileName = *b.FileName
	}
	if b.MimeType != nil {
		out.MimeType = *b.MimeType
	}
	if b.CommittedAt != nil {
		out.CommittedAt = timestamppb.New(*b.CommittedAt)
	}
	if b.DeletedAt != nil {
		out.DeletedAt = timestamppb.New(*b.DeletedAt)
	}
	return out
}
