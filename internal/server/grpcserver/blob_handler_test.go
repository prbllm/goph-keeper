package grpcserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testBlobHandlerCfg() *config.Config {
	c := testVaultCfg()
	c.MaxChunkSizeBytes = 1024
	c.UploadSessionTTLHours = 24
	return c
}

type fakeUploadBlobStream struct {
	testServerStream
	msgs []*gophkeeperv1.UploadBlobRequest
	i    int
	out  *gophkeeperv1.UploadBlobResponse
}

func newFakeUploadBlobStream(ctx context.Context, msgs ...*gophkeeperv1.UploadBlobRequest) *fakeUploadBlobStream {
	return &fakeUploadBlobStream{testServerStream: testServerStream{ctx: ctx}, msgs: msgs}
}

func (s *fakeUploadBlobStream) Recv() (*gophkeeperv1.UploadBlobRequest, error) {
	if s.i >= len(s.msgs) {
		return nil, io.EOF
	}
	m := s.msgs[s.i]
	s.i++
	return m, nil
}

func (s *fakeUploadBlobStream) SendAndClose(m *gophkeeperv1.UploadBlobResponse) error {
	s.out = m
	return nil
}

var _ grpc.ClientStreamingServer[gophkeeperv1.UploadBlobRequest, gophkeeperv1.UploadBlobResponse] = (*fakeUploadBlobStream)(nil)

type fakeDownloadBlobStream struct {
	testServerStream
	sent []*gophkeeperv1.DownloadBlobResponse
}

func (s *fakeDownloadBlobStream) Send(m *gophkeeperv1.DownloadBlobResponse) error {
	s.sent = append(s.sent, m)
	return nil
}

var _ grpc.ServerStreamingServer[gophkeeperv1.DownloadBlobResponse] = (*fakeDownloadBlobStream)(nil)

type uploadBlobStreamFirstRecvErr struct {
	testServerStream
	err error
}

func (s *uploadBlobStreamFirstRecvErr) Recv() (*gophkeeperv1.UploadBlobRequest, error) {
	return nil, s.err
}

func (s *uploadBlobStreamFirstRecvErr) SendAndClose(*gophkeeperv1.UploadBlobResponse) error {
	return nil
}

var _ grpc.ClientStreamingServer[gophkeeperv1.UploadBlobRequest, gophkeeperv1.UploadBlobResponse] = (*uploadBlobStreamFirstRecvErr)(nil)

type uploadBlobStreamRecvFailsAfter struct {
	testServerStream
	msgs    []*gophkeeperv1.UploadBlobRequest
	i       int
	recvErr error
}

func (s *uploadBlobStreamRecvFailsAfter) Recv() (*gophkeeperv1.UploadBlobRequest, error) {
	if s.i >= len(s.msgs) {
		return nil, s.recvErr
	}
	m := s.msgs[s.i]
	s.i++
	return m, nil
}

func (s *uploadBlobStreamRecvFailsAfter) SendAndClose(*gophkeeperv1.UploadBlobResponse) error {
	return nil
}

var _ grpc.ClientStreamingServer[gophkeeperv1.UploadBlobRequest, gophkeeperv1.UploadBlobResponse] = (*uploadBlobStreamRecvFailsAfter)(nil)

type uploadBlobStreamSendCloseErr struct {
	*fakeUploadBlobStream
	sendCloseErr error
}

func (s *uploadBlobStreamSendCloseErr) SendAndClose(m *gophkeeperv1.UploadBlobResponse) error {
	s.out = m
	return s.sendCloseErr
}

var _ grpc.ClientStreamingServer[gophkeeperv1.UploadBlobRequest, gophkeeperv1.UploadBlobResponse] = (*uploadBlobStreamSendCloseErr)(nil)

type downloadBlobStreamSendErr struct {
	testServerStream
	failOnCall int
	calls      int
	err        error
}

func (s *downloadBlobStreamSendErr) Send(m *gophkeeperv1.DownloadBlobResponse) error {
	s.calls++
	if s.calls == s.failOnCall {
		return s.err
	}
	return nil
}

var _ grpc.ServerStreamingServer[gophkeeperv1.DownloadBlobResponse] = (*downloadBlobStreamSendErr)(nil)

type blobUploadStoreFake struct {
	startErr    error
	sess        *blob.UploadSession
	getErr      error
	completeErr error
	commitErr   error
}

func (f *blobUploadStoreFake) StartSession(context.Context, *blob.Blob, *blob.UploadSession) error {
	return f.startErr
}

func (f *blobUploadStoreFake) GetSession(context.Context, string, string) (*blob.UploadSession, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.sess, nil
}

func (f *blobUploadStoreFake) DeleteSession(context.Context, string, string) error { return nil }

func (f *blobUploadStoreFake) CompleteClientUpload(context.Context, string, string, uint64) error {
	return f.completeErr
}

func (f *blobUploadStoreFake) CommitUploadedBlob(context.Context, string, string, time.Time) error {
	return f.commitErr
}

var _ blob.UploadSessionStore = (*blobUploadStoreFake)(nil)

type blobRepoFake struct {
	b       *blob.Blob
	getErr  error
	getByID func(context.Context, string, string) (*blob.Blob, error)
}

func (r *blobRepoFake) Create(context.Context, *blob.Blob) error { return nil }

func (r *blobRepoFake) GetByID(ctx context.Context, uid, bid string) (*blob.Blob, error) {
	if r.getByID != nil {
		return r.getByID(ctx, uid, bid)
	}
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.b, nil
}

func (r *blobRepoFake) MarkCommitted(context.Context, string, string, time.Time) error { return nil }
func (r *blobRepoFake) MarkDeleted(context.Context, string, string, time.Time) error   { return nil }
func (r *blobRepoFake) MarkFailed(context.Context, string, string, time.Time) error    { return nil }

var _ blob.Repository = (*blobRepoFake)(nil)

type blobObjectStorageFake struct {
	putErr   error
	deleted  []string
	putKeys  []string
	getRC    io.ReadCloser
	getSize  int64
	getErr   error
	statSize int64
	statErr  error
}

func (o *blobObjectStorageFake) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	_ = ctx
	_ = r
	_ = size
	_ = ct
	o.putKeys = append(o.putKeys, key)
	return o.putErr
}

func (o *blobObjectStorageFake) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	_ = ctx
	_ = key
	if o.getErr != nil {
		return nil, 0, o.getErr
	}
	return o.getRC, o.getSize, nil
}

func (o *blobObjectStorageFake) Stat(ctx context.Context, key string) (int64, error) {
	_ = ctx
	_ = key
	return o.statSize, o.statErr
}

func (o *blobObjectStorageFake) Delete(ctx context.Context, key string) error {
	_ = ctx
	o.deleted = append(o.deleted, key)
	return nil
}

var _ blob.ObjectStorage = (*blobObjectStorageFake)(nil)

func testBlobHandler(uploads blob.UploadSessionStore, repo *blobRepoFake, storage *blobObjectStorageFake) blobHandler {
	return blobHandler{
		logger:      zap.NewNop(),
		cfg:         testBlobHandlerCfg(),
		uploadLocks: &uploadSessionLocks{},
		uploads:     uploads,
		blobRepo:    repo,
		blobStorage: storage,
	}
}

func openUploadFixture(t *testing.T) (ctx context.Context, payload []byte, sess *blob.UploadSession, bmeta *blob.Blob) {
	t.Helper()
	ctx = WithAuthContext(context.Background(), "u1", "d1")
	payload = []byte("hello")
	sum := sha256.Sum256(payload)
	now := time.Now().UTC()
	exp := now.Add(time.Hour)
	sess = &blob.UploadSession{
		UploadSessionID:  "sid",
		BlobID:           "bid",
		UserID:           "u1",
		ExpectedSize:     uint64(len(payload)),
		ExpectedChecksum: sum[:],
		Status:           blob.UploadSessionOpen,
		ExpiresAt:        exp,
	}
	bmeta = &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "users/u1/blobs/bid",
		SizeBytes: uint64(len(payload)), Checksum: sum[:], ContentKind: "k",
		Status: blob.StatusPending, CreatedAt: now,
	}
	return ctx, payload, sess, bmeta
}

func uploadFixture(payload []byte, sessStatus blob.UploadSessionStatus, blobSt blob.Status) (context.Context, *blob.UploadSession, *blob.Blob) {
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	sum := sha256.Sum256(payload)
	now := time.Now().UTC()
	exp := now.Add(time.Hour)
	sess := &blob.UploadSession{
		UploadSessionID:  "sid",
		BlobID:           "bid",
		UserID:           "u1",
		ExpectedSize:     uint64(len(payload)),
		ExpectedChecksum: sum[:],
		Status:           sessStatus,
		ExpiresAt:        exp,
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "users/u1/blobs/bid",
		SizeBytes: uint64(len(payload)), Checksum: sum[:], ContentKind: "k",
		Status: blobSt, CreatedAt: now,
	}
	return ctx, sess, bmeta
}

func TestBlobHandler_StartBlobUpload_unauthenticated(t *testing.T) {
	t.Parallel()
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.StartBlobUpload(context.Background(), &gophkeeperv1.StartBlobUploadRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_StartBlobUpload_validation(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})

	_, err := h.StartBlobUpload(ctx, nil)
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("nil req: %v", err)
	}

	hBad := blobHandler{logger: zap.NewNop(), cfg: testBlobHandlerCfg(), uploadLocks: &uploadSessionLocks{}}
	_, err = hBad.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 1, ExpectedChecksum: []byte{1}, ContentKind: "k"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("not configured: %v", err)
	}

	cfg := testBlobHandlerCfg()
	cfg.UploadSessionTTLHours = 0
	h = blobHandler{
		logger: zap.NewNop(), cfg: cfg, uploadLocks: &uploadSessionLocks{},
		uploads: &blobUploadStoreFake{}, blobRepo: &blobRepoFake{}, blobStorage: &blobObjectStorageFake{},
	}
	_, err = h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 1, ExpectedChecksum: []byte{1}, ContentKind: "k"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("ttl: %v", err)
	}

	cfg = testBlobHandlerCfg()
	cfg.MaxBlobSizeBytes = 10
	h = testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	h.cfg = cfg
	_, err = h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 99, ExpectedChecksum: []byte{1}, ContentKind: "k"})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("max size: %v", err)
	}

	h = testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err = h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 1, ContentKind: "k"})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("checksum: %v", err)
	}

	_, err = h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 1, ExpectedChecksum: []byte{1}, ContentKind: "  "})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("content kind: %v", err)
	}

	cfg = testBlobHandlerCfg()
	cfg.MaxChunkSizeBytes = 0
	h = testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	h.cfg = cfg
	_, err = h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{ExpectedSize: 1, ExpectedChecksum: []byte{1}, ContentKind: "k"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("max chunk: %v", err)
	}
}

func TestBlobHandler_StartBlobUpload_startSessionFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	store := &blobUploadStoreFake{startErr: errors.New("db down")}
	h := testBlobHandler(store, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.StartBlobUpload(ctx, &gophkeeperv1.StartBlobUploadRequest{
		ExpectedSize: 1, ExpectedChecksum: []byte{1}, ContentKind: "app",
	})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_unauthenticated(t *testing.T) {
	t.Parallel()
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	err := h.UploadBlob(newFakeUploadBlobStream(context.Background()))
	if err == nil {
		t.Fatal("expected error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_notConfigured(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := blobHandler{logger: zap.NewNop(), cfg: testBlobHandlerCfg()}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, &gophkeeperv1.UploadBlobRequest{
		Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "s"}},
	}))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_missingLocks(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := blobHandler{
		logger:   zap.NewNop(),
		cfg:      testBlobHandlerCfg(),
		uploads:  &blobUploadStoreFake{},
		blobRepo: &blobRepoFake{}, blobStorage: &blobObjectStorageFake{},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, &gophkeeperv1.UploadBlobRequest{
		Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "s"}},
	}))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_missingHeader(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	stream := newFakeUploadBlobStream(ctx, &gophkeeperv1.UploadBlobRequest{
		Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0}},
	})
	err := h.UploadBlob(stream)
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_sessionNotFound(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	store := &blobUploadStoreFake{getErr: blob.ErrUploadSessionNotFound}
	h := testBlobHandler(store, &blobRepoFake{}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_expiredSession(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	sess.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	store := &blobUploadStoreFake{sess: sess}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_wrongChunkIndex_aborts(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 1, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 || stor.deleted[0] != bmeta.ObjectKey {
		t.Fatalf("expected object delete on abort, deleted=%v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_checksumMismatch_aborts(t *testing.T) {
	t.Parallel()
	ctx, _, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	wrong := []byte("hallo")
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: wrong}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 {
		t.Fatalf("want storage delete on checksum abort")
	}
}

func TestBlobHandler_UploadBlob_putFails_deletesObject(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{putErr: errors.New("s3 down")}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 || stor.deleted[0] != bmeta.ObjectKey {
		t.Fatalf("want delete after failed put: %v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_success(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	stream := newFakeUploadBlobStream(ctx, msgs...)
	if err := h.UploadBlob(stream); err != nil {
		t.Fatal(err)
	}
	if stream.out == nil || stream.out.GetBlobId() != "bid" || stream.out.GetReceivedSize() != uint64(len(payload)) {
		t.Fatalf("response: %+v", stream.out)
	}
	if len(stor.putKeys) != 1 || stor.putKeys[0] != bmeta.ObjectKey {
		t.Fatalf("put keys: %v", stor.putKeys)
	}
}

func TestBlobHandler_CommitBlobUpload_validation(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})

	_, err := h.CommitBlobUpload(ctx, nil)
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("nil req: %v", err)
	}
	_, err = h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("empty session: %v", err)
	}

	hBad := blobHandler{logger: zap.NewNop(), cfg: testBlobHandlerCfg()}
	_, err = hBad.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("not configured: %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_success(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	exp := now.Add(time.Hour)
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: exp,
	}
	uploading := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "key1", SizeBytes: 5,
		Checksum: []byte{1}, ContentKind: "k", Status: blob.StatusUploading, CreatedAt: now,
	}
	committed := *uploading
	committedAt := now.Add(time.Minute)
	committed.Status = blob.StatusCommitted
	committed.CommittedAt = &committedAt
	var calls int
	repo := &blobRepoFake{
		getByID: func(context.Context, string, string) (*blob.Blob, error) {
			calls++
			if calls == 1 {
				return uploading, nil
			}
			return &committed, nil
		},
	}
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{statSize: 5}
	h := testBlobHandler(store, repo, stor)
	resp, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetBlob().GetBlobId() != "bid" || resp.GetBlob().GetStatus() != gophkeeperv1.BlobStatus_BLOB_STATUS_COMMITTED {
		t.Fatalf("blob: %+v", resp.GetBlob())
	}
}

func TestBlobHandler_DownloadBlob_unauthenticated(t *testing.T) {
	t.Parallel()
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "b"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: context.Background()}})
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_notCommitted(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 1,
		Status: blob.StatusPending, CreatedAt: now,
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	stream := &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}}
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, stream)
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_success(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	payload := []byte("abc")
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: uint64(len(payload)),
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(bytes.NewReader(payload)),
		getSize: int64(len(payload)),
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	stream := &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}}
	if err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.sent) < 2 {
		t.Fatalf("want header + chunk, got %d msgs", len(stream.sent))
	}
	if stream.sent[0].GetHeader() == nil {
		t.Fatal("expected header first")
	}
	var got []byte
	for _, m := range stream.sent[1:] {
		got = append(got, m.GetChunk().GetData()...)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload: %q want %q", got, payload)
	}
}

func TestBlobHandler_UploadBlob_chunkExceedsMaxSize_aborts(t *testing.T) {
	t.Parallel()
	payload := []byte("hello") // 5 bytes
	ctx, sess, bmeta := uploadFixture(payload, blob.UploadSessionOpen, blob.StatusPending)
	cfg := testBlobHandlerCfg()
	cfg.MaxChunkSizeBytes = 4
	h := blobHandler{
		logger: zap.NewNop(), cfg: cfg, uploadLocks: &uploadSessionLocks{},
		uploads: &blobUploadStoreFake{sess: sess}, blobRepo: &blobRepoFake{b: bmeta},
		blobStorage: &blobObjectStorageFake{},
	}
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	stor := h.blobStorage.(*blobObjectStorageFake)
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 || stor.deleted[0] != bmeta.ObjectKey {
		t.Fatalf("want delete on abort: %v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_totalExceedsExpected_aborts(t *testing.T) {
	t.Parallel()
	payload := []byte("hello")
	ctx, sess, bmeta := uploadFixture(payload, blob.UploadSessionOpen, blob.StatusPending)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	stor := h.blobStorage.(*blobObjectStorageFake)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: []byte("hel")}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 1, Data: []byte("loz")}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 {
		t.Fatalf("want storage delete: %v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_receivedSizeMismatch_aborts(t *testing.T) {
	t.Parallel()
	ctx, _, sess, bmeta := openUploadFixture(t)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	stor := h.blobStorage.(*blobObjectStorageFake)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: []byte("hi")}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 {
		t.Fatalf("want delete: %v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_sessionNotOpen(t *testing.T) {
	t.Parallel()
	payload := []byte("hello")
	ctx, sess, bmeta := uploadFixture(payload, blob.UploadSessionAwaitingCommit, blob.StatusPending)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_blobNotPending(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	bmeta.Status = blob.StatusUploading
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_blobNotFound(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, _ := openUploadFixture(t)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{getErr: blob.ErrNotFound}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_secondMessageNotChunk(t *testing.T) {
	t.Parallel()
	ctx, _, sess, bmeta := openUploadFixture(t)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_completeClientUpload_skipsDeleteWhenNotOpen(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess, completeErr: blob.ErrUploadSessionNotOpen}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 0 {
		t.Fatalf("ErrUploadSessionNotOpen should not trigger storage delete, got %v", stor.deleted)
	}
}

func TestBlobHandler_CommitBlobUpload_unauthenticated(t *testing.T) {
	t.Parallel()
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(context.Background(), &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_sessionNotFound(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{getErr: blob.ErrUploadSessionNotFound}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_getSessionInternal(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{getErr: errors.New("db")}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_expiredSession(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(-time.Minute),
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_sessionNotAwaitingCommit(t *testing.T) {
	t.Parallel()
	ctx, sess, _ := uploadFixture([]byte("x"), blob.UploadSessionOpen, blob.StatusPending)
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_blobNotFound(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{getErr: blob.ErrNotFound}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_blobNotUploading(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 1,
		Status: blob.StatusPending, CreatedAt: now,
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_statObjectNotFound(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{statErr: blob.ErrObjectNotFound}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, stor)
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_statSizeMismatch(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{statSize: 99})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_statGenericError(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{statErr: errors.New("stat boom")})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_CommitBlobUpload_commitTxErrors(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"session_not_found", blob.ErrUploadSessionNotFound, codes.NotFound},
		{"session_expired", blob.ErrUploadSessionExpired, codes.FailedPrecondition},
		{"not_awaiting_commit", blob.ErrUploadSessionNotAwaitingCommit, codes.FailedPrecondition},
		{"blob_not_found", blob.ErrNotFound, codes.FailedPrecondition},
		{"generic", errors.New("tx failed"), codes.Internal},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &blobUploadStoreFake{sess: sess, commitErr: tc.err}
			h := testBlobHandler(store, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{statSize: 5})
			_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
			st, _ := status.FromError(err)
			if st.Code() != tc.code {
				t.Fatalf("got %v want %v", st.Code(), tc.code)
			}
		})
	}
}

func TestBlobHandler_CommitBlobUpload_reloadAfterCommitFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	uploading := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	var calls int
	repo := &blobRepoFake{
		getByID: func(context.Context, string, string) (*blob.Blob, error) {
			calls++
			if calls == 1 {
				return uploading, nil
			}
			return nil, errors.New("reload failed")
		},
	}
	store := &blobUploadStoreFake{sess: sess}
	h := testBlobHandler(store, repo, &blobObjectStorageFake{statSize: 5})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_nilRequest(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	err := h.DownloadBlob(nil, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_emptyBlobID(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_notConfigured(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := blobHandler{logger: zap.NewNop(), blobRepo: &blobRepoFake{}, blobStorage: &blobObjectStorageFake{}}
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_blobNotFound(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{getErr: blob.ErrNotFound}, &blobObjectStorageFake{})
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_getRepoError(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{getErr: errors.New("db")}, &blobObjectStorageFake{})
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_deletedAt(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	del := now.Add(time.Minute)
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 1,
		Status: blob.StatusCommitted, CreatedAt: now, DeletedAt: &del,
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_getFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 3,
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{getErr: errors.New("no object")}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_getReportedSizeMismatch(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(bytes.NewReader([]byte("hello"))),
		getSize: 99,
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_maxChunkNotConfigured(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	payload := []byte("a")
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: uint64(len(payload)),
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	cfg := testBlobHandlerCfg()
	cfg.MaxChunkSizeBytes = 0
	h := blobHandler{
		logger: zap.NewNop(), cfg: cfg, uploadLocks: &uploadSessionLocks{},
		uploads: &blobUploadStoreFake{}, blobRepo: &blobRepoFake{b: bmeta},
		blobStorage: &blobObjectStorageFake{
			getRC:   io.NopCloser(bytes.NewReader(payload)),
			getSize: int64(len(payload)),
		},
	}
	stream := &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}}
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, stream)
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].GetHeader() == nil {
		t.Fatalf("expected header sent before failure, sent=%d", len(stream.sent))
	}
}

func TestBlobHandler_DownloadBlob_shortReadFromStorage(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 100,
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(bytes.NewReader([]byte("ab"))),
		getSize: 100,
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

type errReader struct {
	data []byte
	err  error
	off  int
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.err != nil && r.off >= len(r.data) {
		return 0, r.err
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func TestBlobHandler_DownloadBlob_readFromBodyFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 100,
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(&errReader{data: []byte("x"), err: errors.New("read boom")}),
		getSize: -1,
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, &fakeDownloadBlobStream{testServerStream: testServerStream{ctx: ctx}})
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_getSessionInternal(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	store := &blobUploadStoreFake{getErr: errors.New("db")}
	h := testBlobHandler(store, &blobRepoFake{}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_getBlobInternal(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, _ := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	h := testBlobHandler(store, &blobRepoFake{getErr: errors.New("db")}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_UploadBlob_firstRecvError(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{}, &blobObjectStorageFake{})
	want := errors.New("stream closed")
	err := h.UploadBlob(&uploadBlobStreamFirstRecvErr{testServerStream: testServerStream{ctx: ctx}, err: want})
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestBlobHandler_UploadBlob_recvErrorMidStream(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{})
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	want := errors.New("rpc canceled")
	err := h.UploadBlob(&uploadBlobStreamRecvFailsAfter{
		testServerStream: testServerStream{ctx: ctx},
		msgs:             msgs,
		recvErr:          want,
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestBlobHandler_UploadBlob_completeClientUpload_genericErrorDeletesObject(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess, completeErr: errors.New("tx failed")}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	err := h.UploadBlob(newFakeUploadBlobStream(ctx, msgs...))
	if st, _ := status.FromError(err); st.Code() != codes.Internal {
		t.Fatalf("got %v", err)
	}
	if len(stor.deleted) != 1 || stor.deleted[0] != bmeta.ObjectKey {
		t.Fatalf("want storage delete after complete failure: %v", stor.deleted)
	}
}

func TestBlobHandler_UploadBlob_sendAndCloseFails(t *testing.T) {
	t.Parallel()
	ctx, payload, sess, bmeta := openUploadFixture(t)
	store := &blobUploadStoreFake{sess: sess}
	stor := &blobObjectStorageFake{}
	h := testBlobHandler(store, &blobRepoFake{b: bmeta}, stor)
	msgs := []*gophkeeperv1.UploadBlobRequest{
		{Body: &gophkeeperv1.UploadBlobRequest_Header{Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: "sid"}}},
		{Body: &gophkeeperv1.UploadBlobRequest_Chunk{Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload}}},
	}
	want := errors.New("client reset")
	stream := &uploadBlobStreamSendCloseErr{
		fakeUploadBlobStream: newFakeUploadBlobStream(ctx, msgs...),
		sendCloseErr:         want,
	}
	err := h.UploadBlob(stream)
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestBlobHandler_CommitBlobUpload_statNegativeSize(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	sess := &blob.UploadSession{
		UploadSessionID: "sid", BlobID: "bid", UserID: "u1",
		Status: blob.UploadSessionAwaitingCommit, ExpiresAt: now.Add(time.Hour),
	}
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: 5,
		Status: blob.StatusUploading, CreatedAt: now,
	}
	h := testBlobHandler(&blobUploadStoreFake{sess: sess}, &blobRepoFake{b: bmeta}, &blobObjectStorageFake{statSize: -1})
	_, err := h.CommitBlobUpload(ctx, &gophkeeperv1.CommitBlobUploadRequest{UploadSessionId: "sid"})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Fatalf("got %v", err)
	}
}

func TestBlobHandler_DownloadBlob_sendHeaderFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	payload := []byte("x")
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: uint64(len(payload)),
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(bytes.NewReader(payload)),
		getSize: int64(len(payload)),
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	want := errors.New("backpressure")
	stream := &downloadBlobStreamSendErr{
		testServerStream: testServerStream{ctx: ctx},
		failOnCall:       1,
		err:              want,
	}
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, stream)
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestBlobHandler_DownloadBlob_sendChunkFails(t *testing.T) {
	t.Parallel()
	ctx := WithAuthContext(context.Background(), "u1", "d1")
	now := time.Now().UTC()
	payload := []byte("xy")
	bmeta := &blob.Blob{
		BlobID: "bid", UserID: "u1", ObjectKey: "k", SizeBytes: uint64(len(payload)),
		Status: blob.StatusCommitted, CreatedAt: now,
	}
	stor := &blobObjectStorageFake{
		getRC:   io.NopCloser(bytes.NewReader(payload)),
		getSize: int64(len(payload)),
	}
	h := testBlobHandler(&blobUploadStoreFake{}, &blobRepoFake{b: bmeta}, stor)
	want := errors.New("reset")
	stream := &downloadBlobStreamSendErr{
		testServerStream: testServerStream{ctx: ctx},
		failOnCall:       2,
		err:              want,
	}
	err := h.DownloadBlob(&gophkeeperv1.DownloadBlobRequest{BlobId: "bid"}, stream)
	if !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestToProtoBlobInfo_nil(t *testing.T) {
	t.Parallel()
	if toProtoBlobInfo(nil) != nil {
		t.Fatal("expected nil")
	}
}
