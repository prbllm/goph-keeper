//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/transport"
	"github.com/prbllm/goph-keeper/internal/server/app"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestE2E_FullStack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres dsn: %v", err)
	}

	minioContainer, err := minio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	if err != nil {
		t.Fatalf("minio: %v", err)
	}
	t.Cleanup(func() { _ = minioContainer.Terminate(context.Background()) })

	minioHostPort, err := minioContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("minio endpoint: %v", err)
	}

	tlsDir := t.TempDir()
	_, certPath, keyPath := WriteTestServerTLSChain(t, tlsDir)
	caPath := tlsDir + "/ca.pem"

	jwtSecret := strings.Repeat("e", config.MinJWTSecretLen)
	t.Setenv(config.EnvDatabaseURL, dsn)
	t.Setenv(config.EnvMinioEndpoint, minioHostPort)
	t.Setenv(config.EnvMinioAccessKey, minioContainer.Username)
	t.Setenv(config.EnvMinioSecretKey, minioContainer.Password)
	t.Setenv(config.EnvMinioBucket, "gophkeeper-e2e")
	t.Setenv(config.EnvMinioUseSSL, "false")
	t.Setenv(config.EnvJWTSecret, jwtSecret)
	t.Setenv(config.EnvGRPCAddr, "127.0.0.1:0")
	t.Setenv(config.EnvGRPCTLSCertPath, certPath)
	t.Setenv(config.EnvGRPCTLSKeyPath, keyPath)
	t.Setenv(config.EnvLogLevel, config.LogLevelError)
	t.Setenv(config.EnvAccessTTLSec, "900")
	t.Setenv(config.EnvRefreshTTLSec, "86400")

	addrCh := make(chan string, 1)
	srvCtx, srvCancel := context.WithCancel(context.Background())
	t.Cleanup(srvCancel)
	go func() {
		_ = app.RunContext(srvCtx, func(addr net.Addr) {
			addrCh <- addr.String()
		})
	}()

	var grpcAddr string
	select {
	case grpcAddr = <-addrCh:
	case <-time.After(120 * time.Second):
		t.Fatal("timeout waiting for server listen address")
	}

	waitForGRPCReady(t, grpcAddr, caPath)

	t.Run("Health", func(t *testing.T) {
		t.Parallel()
		cli, err := transport.New(grpcAddr, "", transport.DialOptions{CAPath: caPath})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cli.Close() })
		hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		conn := cli.Conn
		hc := grpc_health_v1.NewHealthClient(conn)
		resp, err := hc.Check(hctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("health: %v", resp.GetStatus())
		}
		gh := gophkeeperv1.NewHealthServiceClient(conn)
		r2, err := gh.Check(hctx, &gophkeeperv1.HealthCheckRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if r2.GetStatus() != "SERVING" {
			t.Fatalf("custom health: %q", r2.GetStatus())
		}
	})

	login := "e2e-" + uuid.NewString()
	password := "e2e-password-9"
	deviceID := "e2e-dev-" + uuid.NewString()

	reg, cli := registerUser(t, grpcAddr, caPath, login, password, deviceID)
	access := reg.GetAccessToken()
	refresh := reg.GetRefreshToken()
	t.Cleanup(func() { _ = cli.Close() })

	t.Run("Auth_refreshAndLogout", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		ref1, err := cli.Auth.Refresh(cctx, &gophkeeperv1.RefreshRequest{
			RefreshToken: refresh,
			DeviceId:     deviceID,
		})
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if ref1.GetAccessToken() == "" || ref1.GetRefreshToken() == "" {
			t.Fatal("expected rotated tokens")
		}
		_, err = cli.Auth.Refresh(cctx, &gophkeeperv1.RefreshRequest{
			RefreshToken: refresh,
			DeviceId:     deviceID,
		})
		if err == nil {
			t.Fatal("expected error reusing old refresh token")
		}
		if st, ok := status.FromError(err); !ok || st.Code() != codes.Unauthenticated {
			t.Fatalf("old refresh: %v", err)
		}
		logCli, err := transport.New(grpcAddr, ref1.GetAccessToken(), transport.DialOptions{CAPath: caPath})
		if err != nil {
			t.Fatal(err)
		}
		defer logCli.Close()
		_, err = logCli.Auth.Logout(cctx, &gophkeeperv1.LogoutRequest{RefreshToken: ref1.GetRefreshToken()})
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
	})

	cliVault, err := transport.New(grpcAddr, access, transport.DialOptions{CAPath: caPath})
	if err != nil {
		t.Fatalf("vault client: %v", err)
	}
	t.Cleanup(func() { _ = cliVault.Close() })

	var itemID string
	var itemVersion uint64

	t.Run("Vault_CRUD", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		opCreate := "op-v-" + uuid.NewString()
		cr, err := cliVault.Vault.CreateItem(cctx, &gophkeeperv1.CreateItemRequest{
			OperationId: opCreate,
			ItemType:    gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
			Title:       &gophkeeperv1.EncryptedField{Ciphertext: []byte("t"), Nonce: []byte("n1")},
			Metadata:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("m"), Nonce: []byte("n2")},
			Payload:     &gophkeeperv1.EncryptedField{Ciphertext: []byte("p"), Nonce: []byte("n3")},
		})
		if err != nil {
			t.Fatalf("CreateItem: %v", err)
		}
		itemID = cr.GetItemId()
		itemVersion = cr.GetVersion()
		if itemID == "" || itemVersion != 1 {
			t.Fatalf("create: id=%q ver=%d", itemID, itemVersion)
		}
		getr, err := cliVault.Vault.GetItem(cctx, &gophkeeperv1.GetItemRequest{ItemId: itemID})
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if getr.GetItem().GetItemId() != itemID {
			t.Fatalf("get item mismatch")
		}
		opUp := "op-vu-" + uuid.NewString()
		up, err := cliVault.Vault.UpdateItem(cctx, &gophkeeperv1.UpdateItemRequest{
			OperationId:     opUp,
			ItemId:          itemID,
			ExpectedVersion: itemVersion,
			Title:           &gophkeeperv1.EncryptedField{Ciphertext: []byte("t2"), Nonce: []byte("n1")},
			Metadata:        &gophkeeperv1.EncryptedField{Ciphertext: []byte("m2"), Nonce: []byte("n2")},
			Payload:         &gophkeeperv1.EncryptedField{Ciphertext: []byte("p2"), Nonce: []byte("n3")},
		})
		if err != nil {
			t.Fatalf("UpdateItem: %v", err)
		}
		itemVersion = up.GetNewVersion()
		opDel := "op-vd-" + uuid.NewString()
		_, err = cliVault.Vault.DeleteItem(cctx, &gophkeeperv1.DeleteItemRequest{
			OperationId:     opDel,
			ItemId:          itemID,
			ExpectedVersion: itemVersion,
		})
		if err != nil {
			t.Fatalf("DeleteItem: %v", err)
		}
	})

	cliSync, err := transport.New(grpcAddr, access, transport.DialOptions{CAPath: caPath})
	if err != nil {
		t.Fatalf("sync client: %v", err)
	}
	t.Cleanup(func() { _ = cliSync.Close() })

	t.Run("Sync_pushPullIdempotent", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		opID := "op-sync-" + uuid.NewString()
		push1, err := cliSync.Sync.PushChanges(cctx, &gophkeeperv1.PushChangesRequest{
			PendingOperations: []*gophkeeperv1.PendingOperation{{
				OperationId:   opID,
				OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				Snapshot: &gophkeeperv1.VaultItemSnapshot{
					ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
					Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("st"), Nonce: []byte("sn")},
					Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("sm"), Nonce: []byte("sn")},
					Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("sp"), Nonce: []byte("sn")},
				},
			}},
		})
		if err != nil {
			t.Fatalf("PushChanges: %v", err)
		}
		if len(push1.GetAcceptedOperations()) != 1 {
			t.Fatalf("accepted: %d", len(push1.GetAcceptedOperations()))
		}
		acc1 := push1.GetAcceptedOperations()[0]
		push2, err := cliSync.Sync.PushChanges(cctx, &gophkeeperv1.PushChangesRequest{
			PendingOperations: []*gophkeeperv1.PendingOperation{{
				OperationId:   opID,
				OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				Snapshot: &gophkeeperv1.VaultItemSnapshot{
					ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
					Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("st"), Nonce: []byte("sn")},
					Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("sm"), Nonce: []byte("sn")},
					Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("sp"), Nonce: []byte("sn")},
				},
			}},
		})
		if err != nil {
			t.Fatalf("PushChanges idempotent: %v", err)
		}
		acc2 := push2.GetAcceptedOperations()[0]
		if acc1.GetItemId() != acc2.GetItemId() {
			t.Fatalf("idempotent item mismatch")
		}
		pull, err := cliSync.Sync.PullChanges(cctx, &gophkeeperv1.PullChangesRequest{SinceRevision: 0, PageSize: 50})
		if err != nil {
			t.Fatalf("PullChanges: %v", err)
		}
		if len(pull.GetChanges()) == 0 {
			t.Fatal("expected pull changes")
		}
		syncResp, err := cliSync.Sync.Sync(cctx, &gophkeeperv1.SyncRequest{
			ClientRevision: 0,
			PendingOperations: []*gophkeeperv1.PendingOperation{{
				OperationId:   "op-sync2-" + uuid.NewString(),
				OperationType: gophkeeperv1.PendingOperationType_PENDING_OPERATION_TYPE_CREATE,
				Snapshot: &gophkeeperv1.VaultItemSnapshot{
					ItemType: gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL,
					Title:    &gophkeeperv1.EncryptedField{Ciphertext: []byte("x"), Nonce: []byte("n")},
					Metadata: &gophkeeperv1.EncryptedField{Ciphertext: []byte("y"), Nonce: []byte("n")},
					Payload:  &gophkeeperv1.EncryptedField{Ciphertext: []byte("z"), Nonce: []byte("n")},
				},
			}},
		})
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if len(syncResp.GetRemoteChanges()) == 0 {
			t.Fatal("expected RemoteChanges on Sync")
		}
	})

	cliBlob, err := transport.New(grpcAddr, access, transport.DialOptions{CAPath: caPath})
	if err != nil {
		t.Fatalf("blob client: %v", err)
	}
	t.Cleanup(func() { _ = cliBlob.Close() })

	t.Run("Blob_uploadDownload", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		payload := []byte("blob-e2e-payload-" + uuid.NewString())
		sum := sha256.Sum256(payload)
		start, err := cliBlob.Blob.StartBlobUpload(cctx, &gophkeeperv1.StartBlobUploadRequest{
			ExpectedSize:     uint64(len(payload)),
			ExpectedChecksum: sum[:],
			ContentKind:    "application/octet-stream",
			FileName:       "f.bin",
			MimeType:       "application/octet-stream",
		})
		if err != nil {
			t.Fatalf("StartBlobUpload: %v", err)
		}
		sess := start.GetUploadSessionId()
		if sess == "" {
			t.Fatal("empty session")
		}
		upStream, err := cliBlob.Blob.UploadBlob(cctx)
		if err != nil {
			t.Fatalf("UploadBlob: %v", err)
		}
		if err := upStream.Send(&gophkeeperv1.UploadBlobRequest{
			Body: &gophkeeperv1.UploadBlobRequest_Header{
				Header: &gophkeeperv1.UploadBlobHeader{UploadSessionId: sess},
			},
		}); err != nil {
			t.Fatal(err)
		}
		if err := upStream.Send(&gophkeeperv1.UploadBlobRequest{
			Body: &gophkeeperv1.UploadBlobRequest_Chunk{
				Chunk: &gophkeeperv1.UploadBlobChunk{ChunkIndex: 0, Data: payload},
			},
		}); err != nil {
			t.Fatal(err)
		}
		if err := upStream.CloseSend(); err != nil {
			t.Fatal(err)
		}
		if _, err := upStream.CloseAndRecv(); err != nil {
			t.Fatalf("upload close: %v", err)
		}
		blobID := start.GetBlobId()
		_, err = cliBlob.Blob.CommitBlobUpload(cctx, &gophkeeperv1.CommitBlobUploadRequest{
			UploadSessionId: sess,
		})
		if err != nil {
			t.Fatalf("CommitBlobUpload: %v", err)
		}
		dlStream, err := cliBlob.Blob.DownloadBlob(cctx, &gophkeeperv1.DownloadBlobRequest{BlobId: blobID})
		if err != nil {
			t.Fatalf("DownloadBlob: %v", err)
		}
		var got []byte
		for {
			msg, err := dlStream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("recv: %v", err)
			}
			if ch := msg.GetChunk(); ch != nil {
				got = append(got, ch.GetData()...)
			}
		}
		if string(got) != string(payload) {
			t.Fatalf("download payload mismatch")
		}
	})

	t.Run("Device_revokeBlocksRefresh", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		login2 := "e2e2-" + uuid.NewString()
		dev2 := "e2e-dev2-" + uuid.NewString()
		reg2, cli2 := registerUser(t, grpcAddr, caPath, login2, password, dev2)
		defer cli2.Close()
		refB := reg2.GetRefreshToken()
		accB := reg2.GetAccessToken()

		devCli, err := transport.New(grpcAddr, accB, transport.DialOptions{CAPath: caPath})
		if err != nil {
			t.Fatal(err)
		}
		defer devCli.Close()
		dc := gophkeeperv1.NewDeviceServiceClient(devCli.Conn)
		list, err := dc.ListDevices(cctx, &gophkeeperv1.ListDevicesRequest{})
		if err != nil {
			t.Fatalf("ListDevices: %v", err)
		}
		if len(list.GetDevices()) != 1 {
			t.Fatalf("devices: %d", len(list.GetDevices()))
		}
		_, err = dc.RevokeDevice(cctx, &gophkeeperv1.RevokeDeviceRequest{DeviceId: dev2})
		if err != nil {
			t.Fatalf("RevokeDevice: %v", err)
		}
		_, err = cli2.Auth.Refresh(cctx, &gophkeeperv1.RefreshRequest{
			RefreshToken: refB,
			DeviceId:     dev2,
		})
		if err == nil {
			t.Fatal("expected Refresh to fail after revoke")
		}
		if st, ok := status.FromError(err); !ok || st.Code() != codes.Unauthenticated {
			t.Fatalf("Refresh after revoke: %v", err)
		}
	})
}

func registerUser(t *testing.T, grpcAddr, caPath, login, password, deviceID string) (*gophkeeperv1.RegisterResponse, *transport.GRPCClient) {
	t.Helper()
	cli, err := transport.New(grpcAddr, "", transport.DialOptions{CAPath: caPath})
	if err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reg, err := cli.Auth.Register(cctx, &gophkeeperv1.RegisterRequest{
		Login:                  login,
		Password:               password,
		PasswordSalt:           []byte("salt"),
		KdfParams:              &gophkeeperv1.KdfParams{Algorithm: "argon2id", MemoryKib: 65536, Iterations: 3, Parallelism: 4, KeyLength: 32},
		EncryptedVaultKey:      []byte("vk"),
		EncryptedVaultKeyNonce: []byte("nv"),
		Device: &gophkeeperv1.DeviceInfo{
			DeviceId:      deviceID,
			DeviceName:    "e2e",
			Platform:      gophkeeperv1.DevicePlatform_DEVICE_PLATFORM_LINUX,
			ClientVersion: "1.0",
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.GetAccessToken() == "" || reg.GetRefreshToken() == "" {
		t.Fatal("missing tokens")
	}
	_ = cli.Close()
	out, err := transport.New(grpcAddr, reg.GetAccessToken(), transport.DialOptions{CAPath: caPath})
	if err != nil {
		t.Fatal(err)
	}
	return reg, out
}

func waitForGRPCReady(t *testing.T, addr, caPath string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	ca, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		t.Fatal("invalid CA")
	}
	for time.Now().Before(deadline) {
		dctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			})),
		)
		if err != nil {
			cancel()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		hc := grpc_health_v1.NewHealthClient(conn)
		_, err = hc.Check(dctx, &grpc_health_v1.HealthCheckRequest{})
		_ = conn.Close()
		cancel()
		if err == nil {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal("gRPC server did not become ready in time")
}
