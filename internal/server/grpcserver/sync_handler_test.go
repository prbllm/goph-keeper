package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/config"
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
