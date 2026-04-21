package grpcserver

import (
	"context"
	"database/sql"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/migrations"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"go.uber.org/zap"
)

type integrationNoopObjectStorage struct{}

func (integrationNoopObjectStorage) Put(context.Context, string, io.Reader, int64, string) error {
	panic("integrationNoopObjectStorage.Put: unexpected call (use inline payload)")
}

func (integrationNoopObjectStorage) Get(context.Context, string) (io.ReadCloser, int64, error) {
	panic("integrationNoopObjectStorage.Get: unexpected call")
}

func (integrationNoopObjectStorage) Stat(context.Context, string) (int64, error) {
	panic("integrationNoopObjectStorage.Stat: unexpected call")
}

func (integrationNoopObjectStorage) Delete(context.Context, string) error {
	panic("integrationNoopObjectStorage.Delete: unexpected call")
}

func openSyncIntegrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("GOPHKEEPER_DATABASE_URL")
	if dsn == "" {
		t.Skip("GOPHKEEPER_DATABASE_URL not set, skipping sync integration tests")
	}
	ctx := context.Background()
	pool, err := postgres.OpenPing(ctx, dsn)
	if err != nil {
		t.Fatalf("OpenPing: %v", err)
	}
	db, ok := pool.(*sql.DB)
	if !ok {
		_ = pool.Close()
		t.Fatalf("pool type %T, want *sql.DB", pool)
	}
	if err := migrations.Run(zap.NewNop(), dsn); err != nil {
		_ = pool.Close()
		t.Fatalf("migrations: %v", err)
	}
	return db, func() { _ = pool.Close() }
}

func newSyncIntegrationHandler(db *sql.DB) syncHandler {
	return syncHandler{
		logger:      zap.NewNop(),
		cfg:         testVaultCfg(),
		pool:        db,
		now:         time.Now,
		blobRepo:    postgres.NewBlobRepository(db),
		blobStorage: integrationNoopObjectStorage{},
	}
}

func testPendingCreate(opID string) *gophkeeperv1.PendingOperation {
	return &gophkeeperv1.PendingOperation{
		OperationId:   opID,
		OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
		Snapshot: &gophkeeperv1.VaultItemSnapshot{
			ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("title-ct"), Nonce: []byte("n1")},
			Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("meta-ct"), Nonce: []byte("n2")},
			Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("payload-ct"), Nonce: []byte("n3")},
		},
	}
}

func TestSyncIntegration_pushCreate_pullChanges(t *testing.T) {
	db, cleanup := openSyncIntegrationDB(t)
	defer cleanup()
	userID := uuid.NewString()
	deviceID := uuid.NewString()
	ctx := WithAuthContext(context.Background(), userID, deviceID)
	h := newSyncIntegrationHandler(db)
	opID := "op-create-" + uuid.NewString()

	pushResp, err := h.PushChanges(ctx, &gophkeeperv1.PushChangesRequest{
		PendingOperations: []*gophkeeperv1.PendingOperation{testPendingCreate(opID)},
	})
	if err != nil {
		t.Fatalf("PushChanges: %v", err)
	}
	if len(pushResp.GetAcceptedOperations()) != 1 {
		t.Fatalf("accepted: got %d", len(pushResp.GetAcceptedOperations()))
	}
	acc := pushResp.GetAcceptedOperations()[0]
	if !acc.GetApplied() || acc.GetItemId() == "" || acc.GetRevisionId() == 0 {
		t.Fatalf("unexpected accepted op: %+v", acc)
	}
	if pushResp.GetServerRevision() == 0 {
		t.Fatalf("ServerRevision: %d", pushResp.GetServerRevision())
	}

	pullResp, err := h.PullChanges(ctx, &gophkeeperv1.PullChangesRequest{
		SinceRevision: 0,
		PageSize:      50,
	})
	if err != nil {
		t.Fatalf("PullChanges: %v", err)
	}
	if pullResp.GetNewServerRevision() == 0 {
		t.Fatalf("Pull NewServerRevision: %d", pullResp.GetNewServerRevision())
	}
	changes := pullResp.GetChanges()
	if len(changes) == 0 {
		t.Fatal("expected at least one revision event")
	}
	var found bool
	for _, ch := range changes {
		if ch.GetItemId() == acc.GetItemId() && ch.GetChangeType() == gophkeeperv1.ChangeType_CHANGE_TYPE_CREATED {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no CREATED event for item %q in %#v", acc.GetItemId(), changes)
	}
}

func TestSyncIntegration_pushCreate_idempotentReplay(t *testing.T) {
	db, cleanup := openSyncIntegrationDB(t)
	defer cleanup()
	userID := uuid.NewString()
	deviceID := uuid.NewString()
	ctx := WithAuthContext(context.Background(), userID, deviceID)
	h := newSyncIntegrationHandler(db)
	opID := "op-idem-" + uuid.NewString()
	op := testPendingCreate(opID)

	r1, err := h.PushChanges(ctx, &gophkeeperv1.PushChangesRequest{
		PendingOperations: []*gophkeeperv1.PendingOperation{op},
	})
	if err != nil {
		t.Fatalf("PushChanges first: %v", err)
	}
	if len(r1.GetAcceptedOperations()) != 1 {
		t.Fatalf("first accepted len: %d", len(r1.GetAcceptedOperations()))
	}
	a1 := r1.GetAcceptedOperations()[0]

	r2, err := h.PushChanges(ctx, &gophkeeperv1.PushChangesRequest{
		PendingOperations: []*gophkeeperv1.PendingOperation{testPendingCreate(opID)},
	})
	if err != nil {
		t.Fatalf("PushChanges second: %v", err)
	}
	if len(r2.GetAcceptedOperations()) != 1 {
		t.Fatalf("second accepted len: %d", len(r2.GetAcceptedOperations()))
	}
	a2 := r2.GetAcceptedOperations()[0]

	if a1.GetItemId() != a2.GetItemId() {
		t.Fatalf("item id mismatch: %q vs %q", a1.GetItemId(), a2.GetItemId())
	}
	if a1.GetNewVersion() != a2.GetNewVersion() || a1.GetRevisionId() != a2.GetRevisionId() {
		t.Fatalf("version/revision mismatch: %+v vs %+v", a1, a2)
	}
	if r2.GetServerRevision() != r1.GetServerRevision() {
		t.Fatalf("server revision changed on replay: %d vs %d", r1.GetServerRevision(), r2.GetServerRevision())
	}
}

func TestSyncIntegration_syncPushAndRevisionRead(t *testing.T) {
	db, cleanup := openSyncIntegrationDB(t)
	defer cleanup()
	userID := uuid.NewString()
	deviceID := uuid.NewString()
	ctx := WithAuthContext(context.Background(), userID, deviceID)
	h := newSyncIntegrationHandler(db)
	opID := "op-sync-" + uuid.NewString()

	resp, err := h.Sync(ctx, &gophkeeperv1.SyncRequest{
		ClientRevision:    0,
		PendingOperations: []*gophkeeperv1.PendingOperation{testPendingCreate(opID)},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(resp.GetAcceptedOperations()) != 1 {
		t.Fatalf("accepted: got %d", len(resp.GetAcceptedOperations()))
	}
	acc := resp.GetAcceptedOperations()[0]
	if !acc.GetApplied() || acc.GetItemId() == "" {
		t.Fatalf("accepted op: %+v", acc)
	}
	if resp.GetNewServerRevision() == 0 {
		t.Fatalf("NewServerRevision: %d", resp.GetNewServerRevision())
	}
	remote := resp.GetRemoteChanges()
	if len(remote) == 0 {
		t.Fatal("expected RemoteChanges after push with ClientRevision=0")
	}
	var match bool
	for _, ev := range remote {
		if ev.GetRevisionId() == acc.GetRevisionId() && ev.GetItemId() == acc.GetItemId() {
			match = true
			break
		}
	}
	if !match {
		t.Fatalf("RemoteChanges missing revision %d for item %q: %#v", acc.GetRevisionId(), acc.GetItemId(), remote)
	}
}
