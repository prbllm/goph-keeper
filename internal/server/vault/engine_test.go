package vault_test

import (
	"context"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/vault"
	"github.com/prbllm/goph-keeper/internal/server/vault/mocks"
	"go.uber.org/mock/gomock"
)

func TestEngineService_Create_Idempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	revisions := mocks.NewMockRevisionLogRepository(ctrl)
	processed := mocks.NewMockProcessedOperationsRepository(ctrl)

	const userID = "user-1"
	const opID = "op-1"

	now := time.Unix(1700000000, 0)
	clock := func() time.Time { return now }

	engine := vault.NewEngine(repo, revisions, processed, clock)

	in := vault.CreateInput{
		ItemType:        1,
		TitleCiphertext: []byte("t"),
		TitleNonce:      []byte("n"),
	}

	processed.EXPECT().
		Lookup(ctx, userID, opID).
		Return(nil, nil)

	repo.EXPECT().
		CreateItem(ctx, gomock.Any()).
		Return(nil)

	revisions.EXPECT().
		AppendRevision(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, entry *vault.RevisionLogEntry) error {
			entry.RevisionID = 1
			return nil
		})

	processed.EXPECT().
		MarkProcessed(ctx, userID, opID, gomock.Any(), gomock.Any()).
		Return(nil)

	out1, err := engine.Create(ctx, userID, opID, in)
	if err != nil {
		t.Fatalf("Create first call: %v", err)
	}
	if out1.ItemID == "" || out1.Version == 0 || out1.ServerRevision == 0 {
		t.Fatalf("unexpected output: %#v", out1)
	}

	processed.EXPECT().
		Lookup(ctx, userID, opID).
		Return(&vault.ProcessedOperation{
			UserID:      userID,
			OperationID: opID,
			ItemID:      out1.ItemID,
			RevisionID:  out1.ServerRevision,
		}, nil)

	repo.EXPECT().
		GetItem(ctx, userID, out1.ItemID).
		Return(&vault.Item{
			ItemID:  out1.ItemID,
			UserID:  userID,
			Version: out1.Version,
		}, nil)

	out2, err := engine.Create(ctx, userID, opID, in)
	if err != nil {
		t.Fatalf("Create second call: %v", err)
	}
	if *out1 != *out2 {
		t.Fatalf("idempotent output mismatch:\nfirst:  %#v\nsecond: %#v", out1, out2)
	}
}
