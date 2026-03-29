package grpcserver

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type syncHandler struct {
	gophkeeperv1.UnimplementedSyncServiceServer
	logger      *zap.Logger
	cfg         *config.Config
	pool        postgres.Pool
	now         func() time.Time
	blobRepo    blob.Repository
	blobStorage blob.ObjectStorage
}

func (h syncHandler) Sync(ctx context.Context, req *gophkeeperv1.SyncRequest) (*gophkeeperv1.SyncResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid Sync request: request is required")
	}

	accepted, conflicts, err := h.runPushOperations(ctx, userID, req.GetPendingOperations())
	if err != nil {
		return nil, err
	}

	after := req.GetClientRevision()
	limit := int(config.MaxSyncBundleRevisionEvents)
	var rows []postgres.RevisionEventRow
	var maxRev uint64
	err = postgres.WithRevisionSnapshotRead(ctx, h.pool, func(exec postgres.Executor) error {
		var inner error
		rows, inner = postgres.ListRevisionEventsAfter(ctx, exec, userID, after, limit)
		if inner != nil {
			if h.logger != nil {
				h.logger.Error("sync: list revision events failed", zap.Error(inner))
			}
			return inner
		}
		maxRev, inner = postgres.UserMaxRevision(ctx, exec, userID)
		if inner != nil && h.logger != nil {
			h.logger.Error("sync: user max revision failed", zap.Error(inner))
		}
		return inner
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "sync: revision read failed")
	}

	return &gophkeeperv1.SyncResponse{
		AcceptedOperations: accepted,
		Conflicts:          conflicts,
		RemoteChanges:      revisionRowsToProto(rows),
		NewServerRevision:  maxRev,
	}, nil
}

func (h syncHandler) PullChanges(ctx context.Context, req *gophkeeperv1.PullChangesRequest) (*gophkeeperv1.PullChangesResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid PullChanges request: request is required")
	}

	after, err := pullAfterRevision(req.GetSinceRevision(), req.GetPageToken())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	limit := normalizePullPageSize(req.GetPageSize())
	fetch := limit + 1
	var rows []postgres.RevisionEventRow
	var maxRev uint64
	err = postgres.WithRevisionSnapshotRead(ctx, h.pool, func(exec postgres.Executor) error {
		var inner error
		rows, inner = postgres.ListRevisionEventsAfter(ctx, exec, userID, after, fetch)
		if inner != nil {
			if h.logger != nil {
				h.logger.Error("sync: pull list revision events failed", zap.Error(inner))
			}
			return inner
		}
		maxRev, inner = postgres.UserMaxRevision(ctx, exec, userID)
		if inner != nil && h.logger != nil {
			h.logger.Error("sync: pull user max revision failed", zap.Error(inner))
		}
		return inner
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "sync: revision read failed")
	}

	var nextToken string
	if len(rows) > limit {
		last := rows[limit-1]
		nextToken = strconv.FormatUint(last.RevisionID, 10)
		rows = rows[:limit]
	}

	return &gophkeeperv1.PullChangesResponse{
		Changes:           revisionRowsToProto(rows),
		NewServerRevision: maxRev,
		NextPageToken:     nextToken,
	}, nil
}

func (h syncHandler) PushChanges(ctx context.Context, req *gophkeeperv1.PushChangesRequest) (*gophkeeperv1.PushChangesResponse, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing auth context")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid PushChanges request: request is required")
	}

	accepted, conflicts, err := h.runPushOperations(ctx, userID, req.GetPendingOperations())
	if err != nil {
		return nil, err
	}
	var maxRev uint64
	err = postgres.WithRevisionSnapshotRead(ctx, h.pool, func(exec postgres.Executor) error {
		var inner error
		maxRev, inner = postgres.UserMaxRevision(ctx, exec, userID)
		if inner != nil && h.logger != nil {
			h.logger.Error("sync: push user max revision failed", zap.Error(inner))
		}
		return inner
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "sync: revision read failed")
	}

	return &gophkeeperv1.PushChangesResponse{
		AcceptedOperations: accepted,
		Conflicts:          conflicts,
		ServerRevision:     maxRev,
	}, nil
}

func (h syncHandler) runPushOperations(ctx context.Context, userID string, ops []*gophkeeperv1.PendingOperation) (
	accepted []*gophkeeperv1.OperationResult,
	conflicts []*gophkeeperv1.Conflict,
	err error,
) {
	var clk vault.Clock
	if h.now != nil {
		clk = h.now
	}
	for _, op := range ops {
		if op == nil {
			return nil, nil, status.Error(codes.InvalidArgument, "pending_operations: nil entry")
		}
		a, c, e := h.applyPendingOperation(ctx, userID, op, clk)
		if e != nil {
			return nil, nil, e
		}
		if a != nil {
			accepted = append(accepted, a)
		}
		if c != nil {
			conflicts = append(conflicts, c)
		}
	}
	return accepted, conflicts, nil
}

func (h syncHandler) applyPendingOperation(ctx context.Context, userID string, op *gophkeeperv1.PendingOperation, clk vault.Clock) (
	accepted *gophkeeperv1.OperationResult,
	conflict *gophkeeperv1.Conflict,
	err error,
) {
	opID := strings.TrimSpace(op.GetOperationId())
	if opID == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "pending operation: operation_id is required")
	}

	switch op.GetOperationType() {
	case gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UNSPECIFIED:
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: operation_type is required", opID)
	case gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE:
		return h.pushCreate(ctx, userID, opID, op, clk)
	case gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE:
		return h.pushUpdate(ctx, userID, opID, op, clk)
	case gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_DELETE:
		return h.pushDelete(ctx, userID, opID, op, clk)
	default:
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: unknown operation_type", opID)
	}
}

func (h syncHandler) pushCreate(ctx context.Context, userID, opID string, op *gophkeeperv1.PendingOperation, clk vault.Clock) (*gophkeeperv1.OperationResult, *gophkeeperv1.Conflict, error) {
	snap := op.GetSnapshot()
	if snap == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: snapshot is required for create", opID)
	}
	if snap.GetTitle() == nil || snap.GetMetadata() == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: title and metadata are required", opID)
	}
	if strings.TrimSpace(snap.GetBlobId()) == "" && snap.GetPayload() == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: payload or blob_id is required", opID)
	}

	itemType, ok := concreteProtoItemType(snap.GetItemType())
	if !ok {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: item_type must be a concrete type", opID)
	}

	pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, h.now, userID, snap.GetPayload(), snap.GetBlobId())
	if err != nil {
		return nil, nil, err
	}

	var out *vault.CreateOutput
	err = postgres.WithVaultEngineTx(ctx, h.pool, h.logger, clk, func(eng *vault.EngineService) error {
		o, e := eng.Create(ctx, userID, opID, vault.CreateInput{
			ItemType:           itemType,
			TitleCiphertext:    snap.GetTitle().GetCiphertext(),
			TitleNonce:         snap.GetTitle().GetNonce(),
			MetadataCiphertext: snap.GetMetadata().GetCiphertext(),
			MetadataNonce:      snap.GetMetadata().GetNonce(),
			PayloadCiphertext:  pc,
			PayloadNonce:       pn,
			BlobID:             bid,
			Checksum:           sum,
		})
		if e != nil {
			return e
		}
		out = o
		return nil
	})
	if err != nil {
		return nil, nil, mapPushEngineError(h.logger, opID, err)
	}

	return &gophkeeperv1.OperationResult{
		OperationId: opID,
		Applied:     true,
		ItemId:      out.ItemID,
		NewVersion:  out.Version,
		RevisionId:  out.ServerRevision,
	}, nil, nil
}

func (h syncHandler) pushUpdate(ctx context.Context, userID, opID string, op *gophkeeperv1.PendingOperation, clk vault.Clock) (*gophkeeperv1.OperationResult, *gophkeeperv1.Conflict, error) {
	itemID := strings.TrimSpace(op.GetItemId())
	if itemID == "" {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: item_id is required for update", opID)
	}
	snap := op.GetSnapshot()
	if snap == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: snapshot is required for update", opID)
	}
	if snap.GetTitle() == nil || snap.GetMetadata() == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: title and metadata are required", opID)
	}
	if strings.TrimSpace(snap.GetBlobId()) == "" && snap.GetPayload() == nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: payload or blob_id is required", opID)
	}

	if _, ok := concreteProtoItemType(snap.GetItemType()); !ok {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: item_type must be a concrete type", opID)
	}

	pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, h.now, userID, snap.GetPayload(), snap.GetBlobId())
	if err != nil {
		return nil, nil, err
	}

	var out *vault.UpdateOutput
	err = postgres.WithVaultEngineTx(ctx, h.pool, h.logger, clk, func(eng *vault.EngineService) error {
		o, e := eng.Update(ctx, userID, opID, itemID, op.GetExpectedVersion(), vault.UpdateInput{
			TitleCiphertext:    snap.GetTitle().GetCiphertext(),
			TitleNonce:         snap.GetTitle().GetNonce(),
			MetadataCiphertext: snap.GetMetadata().GetCiphertext(),
			MetadataNonce:      snap.GetMetadata().GetNonce(),
			PayloadCiphertext:  pc,
			PayloadNonce:       pn,
			BlobID:             bid,
			Checksum:           sum,
		})
		if e != nil {
			return e
		}
		out = o
		return nil
	})
	if err != nil {
		if errors.Is(err, vault.ErrConflict) {
			return nil, h.versionConflict(ctx, userID, opID, itemID, op.GetExpectedVersion(), "version mismatch"), nil
		}
		return nil, nil, mapPushEngineError(h.logger, opID, err)
	}

	return &gophkeeperv1.OperationResult{
		OperationId: opID,
		Applied:     true,
		ItemId:      itemID,
		NewVersion:  out.NewVersion,
		RevisionId:  out.ServerRevision,
	}, nil, nil
}

func (h syncHandler) pushDelete(ctx context.Context, userID, opID string, op *gophkeeperv1.PendingOperation, clk vault.Clock) (*gophkeeperv1.OperationResult, *gophkeeperv1.Conflict, error) {
	itemID := strings.TrimSpace(op.GetItemId())
	if itemID == "" {
		return nil, nil, status.Errorf(codes.InvalidArgument, "pending operation %q: item_id is required for delete", opID)
	}

	var out *vault.DeleteOutput
	err := postgres.WithVaultEngineTx(ctx, h.pool, h.logger, clk, func(eng *vault.EngineService) error {
		o, e := eng.Delete(ctx, userID, opID, itemID, op.GetExpectedVersion())
		if e != nil {
			return e
		}
		out = o
		return nil
	})
	if err != nil {
		if errors.Is(err, vault.ErrConflict) {
			return nil, h.versionConflict(ctx, userID, opID, itemID, op.GetExpectedVersion(), "version mismatch"), nil
		}
		return nil, nil, mapPushEngineError(h.logger, opID, err)
	}

	return &gophkeeperv1.OperationResult{
		OperationId: opID,
		Applied:     true,
		ItemId:      itemID,
		NewVersion:  out.NewVersion,
		RevisionId:  out.ServerRevision,
	}, nil, nil
}

// versionConflict builds a version-mismatch Conflict. current_item and actual_version are loaded
// in a separate pool read (not inside the vault engine transaction that rejected the update),
// so they are a best-effort view and may already differ from another concurrent writer.
func (h syncHandler) versionConflict(ctx context.Context, userID, opID, itemID string, expectedVersion uint64, reason string) *gophkeeperv1.Conflict {
	repo := postgres.NewVaultRepository(h.pool)
	it, err := repo.GetItem(ctx, userID, itemID)
	c := &gophkeeperv1.Conflict{
		OperationId:     opID,
		ItemId:          itemID,
		ExpectedVersion: expectedVersion,
		Reason:          reason,
	}
	if err == nil && it != nil {
		c.ActualVersion = it.Version
		c.CurrentItem = toProtoItem(it)
	}
	return c
}

func mapPushEngineError(log *zap.Logger, opID string, err error) error {
	switch {
	case errors.Is(err, vault.ErrInvalidArgument):
		return status.Errorf(codes.InvalidArgument, "pending operation %q: invalid argument", opID)
	case errors.Is(err, vault.ErrNotFound):
		return status.Errorf(codes.NotFound, "pending operation %q: not found", opID)
	default:
		if log != nil {
			log.Error("sync: push engine error", zap.String("operation_id", opID), zap.Error(err))
		}
		return status.Error(codes.Internal, "sync: operation failed")
	}
}

func pullAfterRevision(since uint64, pageToken string) (uint64, error) {
	after := since
	tok := strings.TrimSpace(pageToken)
	if tok == "" {
		return after, nil
	}
	u, err := strconv.ParseUint(tok, 10, 64)
	if err != nil {
		return 0, errors.New("invalid page_token")
	}
	if u > after {
		after = u
	}
	return after, nil
}

func normalizePullPageSize(n uint32) int {
	if n == 0 {
		return int(config.DefaultSyncPullPageSize)
	}
	if int(n) > config.MaxSyncPullPageSize {
		return int(config.MaxSyncPullPageSize)
	}
	return int(n)
}

// revisionRowsToProto maps DB rows to API events. Item may be nil when vault_items is missing or incomplete.
func revisionRowsToProto(rows []postgres.RevisionEventRow) []*gophkeeperv1.RevisionEvent {
	out := make([]*gophkeeperv1.RevisionEvent, 0, len(rows))
	for _, r := range rows {
		ev := &gophkeeperv1.RevisionEvent{
			RevisionId:  r.RevisionID,
			ItemId:      r.ItemID,
			ChangeType:  gophkeeperv1.ChangeType(int32(r.ChangeType)),
			ItemVersion: r.ItemVersion,
			ChangedAt:   timestamppb.New(r.ChangedAt),
			Item:        toProtoItem(r.Item),
		}
		out = append(out, ev)
	}
	return out
}
