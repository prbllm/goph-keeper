package vault_test

import (
	"context"
	"testing"
	"time"

	"github.com/prbllm/goph-keeper/internal/server/vault"
	"github.com/prbllm/goph-keeper/internal/server/vault/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
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

	engine := vault.NewEngine(repo, revisions, processed, clock, zap.NewNop())

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

func TestEngineService_Update_VersionConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	revisions := mocks.NewMockRevisionLogRepository(ctrl)
	processed := mocks.NewMockProcessedOperationsRepository(ctrl)

	const userID = "user-1"
	const opID = "op-upd"
	const itemID = "item-1"

	now := time.Unix(1700000000, 0)
	engine := vault.NewEngine(repo, revisions, processed, func() time.Time { return now }, zap.NewNop())

	processed.EXPECT().Lookup(ctx, userID, opID).Return(nil, nil)
	repo.EXPECT().GetItem(ctx, userID, itemID).Return(&vault.Item{
		ItemID:  itemID,
		UserID:  userID,
		Version: 5,
	}, nil)

	repo.EXPECT().UpdateItem(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	revisions.EXPECT().AppendRevision(gomock.Any(), gomock.Any()).Times(0)
	processed.EXPECT().MarkProcessed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := engine.Update(ctx, userID, opID, itemID, 4, vault.UpdateInput{
		TitleCiphertext:    []byte("t"),
		TitleNonce:         []byte("n"),
		MetadataCiphertext: []byte("m"),
		MetadataNonce:      []byte("nm"),
	})
	if err != vault.ErrConflict {
		t.Fatalf("Update: err=%v, want ErrConflict", err)
	}
}

func TestEngineService_Update_SuccessIncrementsVersionAndRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	revisions := mocks.NewMockRevisionLogRepository(ctrl)
	processed := mocks.NewMockProcessedOperationsRepository(ctrl)

	const userID = "user-1"
	const opID = "op-upd-ok"
	const itemID = "item-1"

	now := time.Unix(1700000000, 0)
	engine := vault.NewEngine(repo, revisions, processed, func() time.Time { return now }, zap.NewNop())

	processed.EXPECT().Lookup(ctx, userID, opID).Return(nil, nil)
	repo.EXPECT().GetItem(ctx, userID, itemID).Return(&vault.Item{
		ItemID:  itemID,
		UserID:  userID,
		Version: 2,
	}, nil)

	repo.EXPECT().
		UpdateItem(ctx, gomock.AssignableToTypeOf(&vault.Item{}), uint64(2)).
		DoAndReturn(func(_ context.Context, it *vault.Item, _ uint64) error {
			if it.Version != 3 {
				t.Fatalf("UpdateItem item.Version=%d, want 3", it.Version)
			}
			return nil
		})

	revisions.EXPECT().
		AppendRevision(ctx, gomock.AssignableToTypeOf(&vault.RevisionLogEntry{})).
		DoAndReturn(func(_ context.Context, entry *vault.RevisionLogEntry) error {
			if entry.ItemVersion != 3 {
				t.Fatalf("revision item_version=%d, want 3", entry.ItemVersion)
			}
			entry.RevisionID = 42
			return nil
		})

	processed.EXPECT().
		MarkProcessed(ctx, userID, opID, itemID, uint64(42)).
		Return(nil)

	out, err := engine.Update(ctx, userID, opID, itemID, 2, vault.UpdateInput{
		TitleCiphertext:    []byte("t"),
		TitleNonce:         []byte("n"),
		MetadataCiphertext: []byte("m"),
		MetadataNonce:      []byte("nm"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.NewVersion != 3 || out.ServerRevision != 42 {
		t.Fatalf("output: %#v", out)
	}
}
