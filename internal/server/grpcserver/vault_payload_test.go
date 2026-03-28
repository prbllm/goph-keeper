package grpcserver

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testVaultCfg() *config.Config {
	return &config.Config{
		InlineThresholdBytes: 1024,
		MaxBlobSizeBytes:     10 << 20,
		GRPCMaxMessageBytes:  100 << 20,
	}
}

type vaultPayloadRepo struct {
	getByID func(context.Context, string, string) (*blob.Blob, error)
	create  func(context.Context, *blob.Blob) error
}

func (r vaultPayloadRepo) Create(ctx context.Context, b *blob.Blob) error {
	if r.create != nil {
		return r.create(ctx, b)
	}
	return nil
}

func (r vaultPayloadRepo) GetByID(ctx context.Context, uid, bid string) (*blob.Blob, error) {
	if r.getByID != nil {
		return r.getByID(ctx, uid, bid)
	}
	return nil, blob.ErrNotFound
}

func (vaultPayloadRepo) MarkCommitted(context.Context, string, string, time.Time) error { return nil }
func (vaultPayloadRepo) MarkDeleted(context.Context, string, string, time.Time) error   { return nil }
func (vaultPayloadRepo) MarkFailed(context.Context, string, string, time.Time) error    { return nil }

type vaultPayloadStorage struct {
	put         func(context.Context, string, io.Reader, int64, string) error
	deleteCalls []string
	lastPutKey  string
	statHook    func(objectKey string) (int64, error)
}

func (s *vaultPayloadStorage) Put(ctx context.Context, key string, r io.Reader, sz int64, ct string) error {
	if s.put != nil {
		return s.put(ctx, key, r, sz, ct)
	}
	s.lastPutKey = key
	return nil
}

func (s *vaultPayloadStorage) Delete(ctx context.Context, key string) error {
	s.deleteCalls = append(s.deleteCalls, key)
	return nil
}

func (*vaultPayloadStorage) Get(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (s *vaultPayloadStorage) Stat(_ context.Context, objectKey string) (int64, error) {
	if s.statHook != nil {
		return s.statHook(objectKey)
	}
	return 0, nil
}

func testVaultLogger() *zap.Logger {
	return zap.NewNop()
}

func TestResolveVaultPayload(t *testing.T) {
	ctx := context.Background()
	const userID = "user-1"

	t.Run("storage not configured", func(t *testing.T) {
		h := vaultHandler{cfg: testVaultCfg()}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("x"), Nonce: []byte("y"),
		}, "")
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v, want Internal", status.Code(err))
		}
	})

	t.Run("empty ciphertext", func(t *testing.T) {
		h := vaultHandler{
			logger:      testVaultLogger(),
			cfg:         testVaultCfg(),
			blobRepo:    vaultPayloadRepo{},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{}, "")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("blob_id and payload mutually exclusive", func(t *testing.T) {
		h := vaultHandler{
			logger:      testVaultLogger(),
			cfg:         testVaultCfg(),
			blobRepo:    vaultPayloadRepo{},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("a"), Nonce: []byte("b"),
		}, "blob-1")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("blob lookup internal error sanitized", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return nil, errors.New("secret db detail")
				},
			},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v", status.Code(err))
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaks details: %v", err)
		}
	})

	t.Run("blob not found", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return nil, blob.ErrNotFound
				},
			},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "missing")
		if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "blob not found") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("blob not committed", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{Status: blob.StatusPending}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob deleted", func(t *testing.T) {
		deleted := time.Now().UTC()
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						SizeBytes: 64,
						DeletedAt: &deleted,
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("referenced blob exceeds size limit", func(t *testing.T) {
		const big = 10<<20 + 1
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "users/user-1/blobs/big",
						SizeBytes: big,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(string) (int64, error) { return int64(big), nil },
			},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "big-blob")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob missing from object storage", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "users/user-1/blobs/missing",
						SizeBytes: 10,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(string) (int64, error) { return 0, blob.ErrObjectNotFound },
			},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob size mismatch metadata vs storage", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "users/user-1/blobs/x",
						SizeBytes: 128,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(string) (int64, error) { return 127, nil },
			},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob stat internal error sanitized", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "users/user-1/blobs/x",
						SizeBytes: 10,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(string) (int64, error) { return 0, errors.New("secret storage detail") },
			},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v", status.Code(err))
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaks details: %v", err)
		}
	})

	t.Run("blob object key missing", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "",
						SizeBytes: 10,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob stat negative size", func(t *testing.T) {
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: "users/user-1/blobs/x",
						SizeBytes: 10,
						Checksum:  []byte{1},
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(string) (int64, error) { return -1, nil },
			},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "bid")
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("missing nonce", func(t *testing.T) {
		h := vaultHandler{
			logger:      testVaultLogger(),
			cfg:         testVaultCfg(),
			blobRepo:    vaultPayloadRepo{},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: []byte("x"),
		}, "")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("blob_id success", func(t *testing.T) {
		sum := []byte{1, 2, 3}
		const objKey = "users/user-1/blobs/ref"
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				getByID: func(context.Context, string, string) (*blob.Blob, error) {
					return &blob.Blob{
						Status:    blob.StatusCommitted,
						ObjectKey: objKey,
						SizeBytes: 128,
						Checksum:  sum,
					}, nil
				},
			},
			blobStorage: &vaultPayloadStorage{
				statHook: func(key string) (int64, error) {
					if key != objKey {
						t.Fatalf("stat key=%q want %q", key, objKey)
					}
					return 128, nil
				},
			},
		}
		pc, pn, bid, cs, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, nil, "my-blob")
		if err != nil {
			t.Fatal(err)
		}
		if pc != nil || pn != nil || bid == nil || *bid != "my-blob" {
			t.Fatalf("unexpected pc/pn/bid: %v %v %v", pc, pn, bid)
		}
		if len(cs) != len(sum) {
			t.Fatalf("checksum len=%d", len(cs))
		}
		for i := range sum {
			if cs[i] != sum[i] {
				t.Fatal("checksum mismatch")
			}
		}
	})

	t.Run("ciphertext exceeds limit", func(t *testing.T) {
		cfg := &config.Config{
			InlineThresholdBytes: 1024,
			MaxBlobSizeBytes:     500,
			GRPCMaxMessageBytes:  100 << 20,
		}
		ct := make([]byte, 501)
		h := vaultHandler{
			logger:      testVaultLogger(),
			cfg:         cfg,
			blobRepo:    vaultPayloadRepo{},
			blobStorage: &vaultPayloadStorage{},
		}
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: ct, Nonce: []byte("n"),
		}, "")
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code=%v", status.Code(err))
		}
	})

	t.Run("inline", func(t *testing.T) {
		h := vaultHandler{
			logger:      testVaultLogger(),
			cfg:         testVaultCfg(),
			blobRepo:    vaultPayloadRepo{},
			blobStorage: &vaultPayloadStorage{},
		}
		ct, nonce := []byte(strings.Repeat("a", 100)), []byte("nonce")
		pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: ct, Nonce: nonce,
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		if bid != nil || sum != nil {
			t.Fatalf("expected inline without blob/checksum")
		}
		if string(pc) != string(ct) || string(pn) != string(nonce) {
			t.Fatal("inline bytes mismatch")
		}
	})

	t.Run("upload then create fails deletes object", func(t *testing.T) {
		store := &vaultPayloadStorage{}
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				create: func(context.Context, *blob.Blob) error {
					return errors.New("db down")
				},
			},
			blobStorage: store,
		}
		ct := []byte(strings.Repeat("b", 2048))
		_, _, _, _, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, time.Now, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: ct, Nonce: []byte("n"),
		}, "")
		if status.Code(err) != codes.Internal {
			t.Fatalf("code=%v, want Internal", status.Code(err))
		}
		if store.lastPutKey == "" {
			t.Fatal("expected Put before Create failure")
		}
		if len(store.deleteCalls) != 1 || store.deleteCalls[0] != store.lastPutKey {
			t.Fatalf("deleteCalls=%v, putKey=%q", store.deleteCalls, store.lastPutKey)
		}
	})

	t.Run("upload success", func(t *testing.T) {
		var created *blob.Blob
		store := &vaultPayloadStorage{}
		fixed := time.Date(2024, 3, 15, 12, 30, 0, 0, time.UTC)
		clock := func() time.Time { return fixed }
		h := vaultHandler{
			logger: testVaultLogger(),
			cfg:    testVaultCfg(),
			blobRepo: vaultPayloadRepo{
				create: func(_ context.Context, b *blob.Blob) error {
					created = b
					return nil
				},
			},
			blobStorage: store,
		}
		ct := []byte(strings.Repeat("c", 2048))
		pc, pn, bid, sum, err := resolveVaultPayload(ctx, h.logger, h.cfg, h.blobRepo, h.blobStorage, clock, userID, &gophkeeperv1.EncryptedField{
			Ciphertext: ct, Nonce: []byte("n"),
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		if pc != nil || pn != nil || bid == nil || created == nil {
			t.Fatalf("unexpected result pc=%v pn=%v bid=%v created=%v", pc, pn, bid, created)
		}
		if *bid != created.BlobID || created.ObjectKey != store.lastPutKey {
			t.Fatalf("blob id/key mismatch bid=%q key=%q", *bid, store.lastPutKey)
		}
		if len(sum) != 32 {
			t.Fatalf("checksum len=%d", len(sum))
		}
		if !created.CreatedAt.Equal(fixed) || created.CommittedAt == nil || !created.CommittedAt.Equal(fixed) {
			t.Fatalf("blob times: created=%v committed=%v want %v", created.CreatedAt, created.CommittedAt, fixed)
		}
	})
}
