package grpcserver

import (
	"context"
	"testing"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/vault/mocks"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVaultHandler_CreateItem_missingEncryptedFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	engine := mocks.NewMockEngine(ctrl)
	h := vaultHandler{engine: engine}

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
