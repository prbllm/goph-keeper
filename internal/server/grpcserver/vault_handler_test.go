package grpcserver

import (
	"context"
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"github.com/prbllm/goph-keeper/internal/server/vault/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVaultHandler_CreateItem_invalidItemType(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{
		logger:      zap.NewNop(),
		engine:      engine,
		blobRepo:    stubBlobRepo{},
		blobStorage: stubBlobStorage{},
		cfg: &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     10 << 20,
			GRPCMaxMessageBytes:  100 << 20,
		},
	}

	ctx := WithAuthContext(context.Background(), "user-1", "dev-1")
	req := &gophkeeperv1.CreateItemRequest{
		ItemType:    0,
		OperationId: "op-1",
		Title:       &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n")},
		Metadata:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n")},
		Payload:     &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n")},
	}
	engine.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := h.CreateItem(ctx, req)
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("code=%v, want InvalidArgument", err)
	}
}

func TestVaultHandler_CreateItem_missingEncryptedFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{
		logger:      zap.NewNop(),
		engine:      engine,
		blobRepo:    stubBlobRepo{},
		blobStorage: stubBlobStorage{},
		cfg: &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     10 << 20,
			GRPCMaxMessageBytes:  100 << 20,
		},
	}

	ctx := WithAuthContext(context.Background(), "user-1", "dev-1")

	req := &gophkeeperv1.CreateItemRequest{}
	engine.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := h.CreateItem(ctx, req)
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("code=%v, want InvalidArgument", err)
	}
}

func TestVaultHandler_CreateItem_emptyPayloadEncryptedField(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{
		logger:      zap.NewNop(),
		engine:      engine,
		blobRepo:    stubBlobRepo{},
		blobStorage: stubBlobStorage{},
		cfg: &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     10 << 20,
			GRPCMaxMessageBytes:  100 << 20,
		},
	}

	ctx := WithAuthContext(context.Background(), "user-1", "dev-1")
	req := &gophkeeperv1.CreateItemRequest{
		ItemType:    1,
		OperationId: "op-empty-payload",
		Title:       &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("nt")},
		Metadata:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("nm")},
		Payload:     &gophkeeperv1.EncryptedField{},
	}
	engine.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := h.CreateItem(ctx, req)
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("code=%v, want InvalidArgument", err)
	}
}

func TestVaultHandler_GetItem_nilItemWithoutError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{
		logger:      zap.NewNop(),
		engine:      engine,
		blobRepo:    stubBlobRepo{},
		blobStorage: stubBlobStorage{},
		cfg: &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     10 << 20,
			GRPCMaxMessageBytes:  100 << 20,
		},
	}

	ctx := WithAuthContext(context.Background(), "user-1", "dev-1")
	engine.EXPECT().Get(gomock.Any(), "user-1", "item-1").Return(nil, nil)

	_, err := h.GetItem(ctx, &gophkeeperv1.GetItemRequest{ItemId: "item-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Fatalf("code=%v, want Internal", err)
	}
}

func TestVaultHandler_ListItems_nilElement(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{
		logger:      zap.NewNop(),
		engine:      engine,
		blobRepo:    stubBlobRepo{},
		blobStorage: stubBlobStorage{},
		cfg: &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     10 << 20,
			GRPCMaxMessageBytes:  100 << 20,
		},
	}

	ctx := WithAuthContext(context.Background(), "user-1", "dev-1")
	engine.EXPECT().List(gomock.Any(), "user-1", gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]*vault.Item{{ItemID: "ok"}, nil}, "", nil)

	_, err := h.ListItems(ctx, &gophkeeperv1.ListItemsRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Fatalf("code=%v, want Internal", err)
	}
}
