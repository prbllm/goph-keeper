package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPullAfterRevision(t *testing.T) {
	t.Parallel()
	after, err := pullAfterRevision(3, "")
	if err != nil || after != 3 {
		t.Fatalf("after=%v err=%v", after, err)
	}
	after, err = pullAfterRevision(3, "7")
	if err != nil || after != 7 {
		t.Fatalf("token raises after: got %v err=%v", after, err)
	}
	after, err = pullAfterRevision(10, "5")
	if err != nil || after != 10 {
		t.Fatalf("since wins when higher: got %v", after)
	}
	_, err = pullAfterRevision(0, "nope")
	if err == nil {
		t.Fatal("expected invalid page_token")
	}
}

func TestNormalizePullPageSize(t *testing.T) {
	t.Parallel()
	if n := normalizePullPageSize(0); n != int(config.DefaultSyncPullPageSize) {
		t.Fatalf("default: %d", n)
	}
	if n := normalizePullPageSize(50); n != 50 {
		t.Fatalf("50: %d", n)
	}
	if n := normalizePullPageSize(uint32(config.MaxSyncPullPageSize + 10)); n != int(config.MaxSyncPullPageSize) {
		t.Fatalf("cap: %d", n)
	}
}

func TestMapPushEngineError(t *testing.T) {
	t.Parallel()
	err := mapPushEngineError(zap.NewNop(), "op-1", vault.ErrInvalidArgument)
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("invalid: %v", err)
	}
	err = mapPushEngineError(zap.NewNop(), "op-1", vault.ErrNotFound)
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Fatalf("notfound: %v", err)
	}
	err = mapPushEngineError(zap.NewNop(), "op-1", errors.New("weird"))
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Fatalf("internal: %v", err)
	}
	err = mapPushEngineError(zap.NewNop(), "op-1", vault.ErrConflict)
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Fatalf("conflict maps to internal (not handled in mapPushEngineError): %v", err)
	}
}

func TestRevisionRowsToProto(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	item := &vault.Item{
		ItemID: "it-1", UserID: "u1", ItemType: 1,
		TitleCiphertext: []byte("t"), TitleNonce: []byte("tn"),
		MetadataCiphertext: []byte("m"), MetadataNonce: []byte("mn"),
		PayloadCiphertext: []byte("p"), PayloadNonce: []byte("pn"),
		Version: 2, CreatedAt: ts, UpdatedAt: ts,
	}
	rows := []postgres.RevisionEventRow{
		{RevisionID: 9, ItemID: "it-1", ChangeType: vault.ChangeTypeUpdated, ItemVersion: 2, ChangedAt: ts, Item: nil},
		{RevisionID: 10, ItemID: "it-1", ChangeType: vault.ChangeTypeUpdated, ItemVersion: 2, ChangedAt: ts, Item: item},
	}
	out := revisionRowsToProto(rows)
	if len(out) != 2 {
		t.Fatalf("len: %d", len(out))
	}
	if out[0].GetRevisionId() != 9 || out[0].GetItem() != nil {
		t.Fatalf("first event: %+v", out[0])
	}
	if out[1].GetRevisionId() != 10 || out[1].GetItem() == nil {
		t.Fatalf("second event: %+v", out[1])
	}
	if !out[1].GetChangedAt().AsTime().Equal(ts) {
		t.Fatalf("changed_at: %v", out[1].GetChangedAt())
	}
}

func TestSyncHandler_pushCreate_rejectsNilSnapshot(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "op-c",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
	}
	_, _, err := h.pushCreate(ctx, "u1", "op-c", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushCreate_rejectsMissingTitleOrMetadata(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "op-c",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushCreate(ctx, "u1", "op-c", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushCreate_rejectsMissingPayloadAndBlobId(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "op-c",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushCreate(ctx, "u1", "op-c", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushUpdate_rejectsEmptyItemId(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "op-u",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushUpdate(ctx, "u1", "op-u", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushUpdate_rejectsNilSnapshot(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:     "op-u",
		OperationType:   gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
		ItemId:          "item-1",
		ExpectedVersion: 1,
	}
	_, _, err := h.pushUpdate(ctx, "u1", "op-u", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushUpdate_rejectsMissingPayloadAndBlobId(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:     "op-u",
		OperationType:   gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
		ItemId:          "item-1",
		ExpectedVersion: 1,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushUpdate(ctx, "u1", "op-u", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_pushDelete_rejectsEmptyItemId(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}, now: time.Now}
	op := &gophkeeperv1.PendingOperation{
		OperationId:     "op-d",
		OperationType:   gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_DELETE,
		ItemId:          "  ",
		ExpectedVersion: 1,
	}
	_, _, err := h.pushDelete(ctx, "u1", "op-d", op, time.Now)
	assertInvalidArgument(t, err)
}

func TestSyncHandler_runPushOperations_deleteRejectsEmptyItemId(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	op := &gophkeeperv1.PendingOperation{
		OperationId:     "op-d",
		OperationType:   gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_DELETE,
		ExpectedVersion: 1,
	}
	_, _, err := h.runPushOperations(ctx, "u1", []*gophkeeperv1.PendingOperation{op})
	assertInvalidArgument(t, err)
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestSyncHandler_pushCreate_rejectsUnspecifiedItemType(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "user-1", "d1")
	h := syncHandler{
		logger: zap.NewNop(),
		cfg:    testVaultCfg(),
		pool:   tlsTestPool{},
		now:    time.Now,
	}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "op-create",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_UNSPECIFIED,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushCreate(ctx, "user-1", "op-create", op, time.Now)
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestSyncHandler_pushUpdate_rejectsBadItemType(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "user-1", "d1")
	h := syncHandler{
		logger: zap.NewNop(),
		cfg:    testVaultCfg(),
		pool:   tlsTestPool{},
		now:    time.Now,
	}
	op := &gophkeeperv1.PendingOperation{
		OperationId:     "op-upd",
		OperationType:   gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_UPDATE,
		ItemId:          "item-1",
		ExpectedVersion: 1,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_UNSPECIFIED,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.pushUpdate(ctx, "user-1", "op-upd", op, time.Now)
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestSyncHandler_Sync_unauthenticated(t *testing.T) {
	t.Parallel()
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.Sync(context.Background(), &gophkeeperv1.SyncRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("Sync: %v", err)
	}
}

func TestSyncHandler_Sync_nilRequest(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.Sync(ctx, nil)
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("Sync: %v", err)
	}
}

func TestSyncHandler_PushChanges_nilRequest(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.PushChanges(ctx, nil)
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("PushChanges: %v", err)
	}
}

func TestSyncHandler_PullChanges_unauthenticated(t *testing.T) {
	t.Parallel()
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.PullChanges(context.Background(), &gophkeeperv1.PullChangesRequest{})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("PullChanges: %v", err)
	}
}

func TestSyncHandler_PullChanges_nilRequest(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.PullChanges(ctx, nil)
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("PullChanges: %v", err)
	}
}

func TestSyncHandler_PullChanges_invalidPageToken(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, err := h.PullChanges(ctx, &gophkeeperv1.PullChangesRequest{SinceRevision: 1, PageToken: "not-a-number"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("PullChanges: %v", err)
	}
}

func TestSyncHandler_runPushOperations_nilEntry(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	_, _, err := h.runPushOperations(ctx, "u1", []*gophkeeperv1.PendingOperation{nil})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("runPushOperations: %v", err)
	}
}

func TestSyncHandler_runPushOperations_emptyOperationID(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	op := &gophkeeperv1.PendingOperation{
		OperationId:   "  ",
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
		},
	}
	_, _, err := h.runPushOperations(ctx, "u1", []*gophkeeperv1.PendingOperation{op})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("runPushOperations: %v", err)
	}
}

func TestSyncHandler_runPushOperations_unspecifiedOperationType(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := syncHandler{logger: zap.NewNop(), cfg: testVaultCfg(), pool: tlsTestPool{}}
	op := &gophkeeperv1.PendingOperation{OperationId: "op-1"}
	_, _, err := h.runPushOperations(ctx, "u1", []*gophkeeperv1.PendingOperation{op})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("runPushOperations: %v", err)
	}
}
