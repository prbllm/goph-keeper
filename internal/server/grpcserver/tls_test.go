package grpcserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/blob"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"github.com/prbllm/goph-keeper/internal/server/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type stubAuthService struct{}

func (stubAuthService) Register(context.Context, auth.RegisterInput) (*auth.RegisterOutput, error) {
	return &auth.RegisterOutput{}, nil
}
func (stubAuthService) Login(context.Context, auth.LoginInput) (*auth.LoginOutput, error) {
	return &auth.LoginOutput{}, nil
}
func (stubAuthService) Refresh(context.Context, auth.RefreshInput) (*auth.RefreshOutput, error) {
	return &auth.RefreshOutput{}, nil
}
func (stubAuthService) Logout(context.Context, string) error { return nil }
func (stubAuthService) Limits() *gophkeeperv1.Limits         { return &gophkeeperv1.Limits{} }

var (
	_ AuthService = stubAuthService{}
)

type stubVaultEngine struct{}

func (stubVaultEngine) Create(context.Context, string, string, vault.CreateInput) (*vault.CreateOutput, error) {
	return &vault.CreateOutput{}, nil
}
func (stubVaultEngine) Update(context.Context, string, string, string, uint64, vault.UpdateInput) (*vault.UpdateOutput, error) {
	return &vault.UpdateOutput{}, nil
}
func (stubVaultEngine) Delete(context.Context, string, string, string, uint64) (*vault.DeleteOutput, error) {
	return &vault.DeleteOutput{}, nil
}
func (stubVaultEngine) Get(context.Context, string, string) (*vault.Item, error) {
	return &vault.Item{}, nil
}
func (stubVaultEngine) List(context.Context, string, bool, int32, string) ([]*vault.Item, string, error) {
	return nil, "", nil
}

type stubBlobRepo struct{}

func (stubBlobRepo) Create(context.Context, *blob.Blob) error { return nil }
func (stubBlobRepo) GetByID(context.Context, string, string) (*blob.Blob, error) {
	return nil, blob.ErrNotFound
}
func (stubBlobRepo) MarkCommitted(context.Context, string, string, time.Time) error { return nil }
func (stubBlobRepo) MarkDeleted(context.Context, string, string, time.Time) error   { return nil }
func (stubBlobRepo) MarkFailed(context.Context, string, string, time.Time) error     { return nil }

var _ blob.Repository = stubBlobRepo{}

type stubUploadStore struct{}

func (stubUploadStore) StartSession(context.Context, *blob.Blob, *blob.UploadSession) error {
	return nil
}
func (stubUploadStore) GetSession(context.Context, string, string) (*blob.UploadSession, error) {
	return nil, blob.ErrUploadSessionNotFound
}
func (stubUploadStore) DeleteSession(context.Context, string, string) error { return nil }
func (stubUploadStore) CompleteClientUpload(context.Context, string, string, uint64) error {
	return nil
}
func (stubUploadStore) CommitUploadedBlob(context.Context, string, string, time.Time) error {
	return nil
}

var _ blob.UploadSessionStore = stubUploadStore{}

type stubBlobStorage struct{}

func (stubBlobStorage) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (stubBlobStorage) Get(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("stub blob storage")
}
func (stubBlobStorage) Stat(context.Context, string) (int64, error) { return 0, nil }
func (stubBlobStorage) Delete(context.Context, string) error        { return nil }

var _ blob.ObjectStorage = stubBlobStorage{}

func TestLoadServerTransportCredentials_missingFiles(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{
		GRPCTLSCertPath: filepath.Join(tmp, "missing.crt"),
		GRPCTLSKeyPath:  filepath.Join(tmp, "missing.key"),
	}

	_, err := LoadServerTransportCredentials(cfg)
	if err == nil {
		t.Fatal("expected error when TLS cert files are missing")
	}
}

func TestLoadServerTransportCredentials_nilConfig(t *testing.T) {
	_, err := LoadServerTransportCredentials(nil)
	if err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestNew_nilDeps(t *testing.T) {
	_, err := New(nil, nil)
	if err == nil {
		t.Fatal("expected error when deps is nil")
	}
}

func TestNew_nilTLSCreds(t *testing.T) {
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err := New(deps, nil)
	if err == nil {
		t.Fatal("expected error when tlsCreds is nil")
	}
}

func TestNew_nilCfg(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         nil,
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.Cfg is nil")
	}
}

func TestNew_nilLogger(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      nil,
		Cfg:         &config.Config{},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.Logger is nil")
	}
}

func TestNew_nilAuthService(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: nil,
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.AuthService is nil")
	}
}

func TestNew_nilVaultEngine(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: &auth.Service{},
		VaultEngine: nil,
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.VaultEngine is nil")
	}
}

func TestNew_nilBlobRepo(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    nil,
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.BlobRepo is nil")
	}
}

func TestNew_nilBlobStorage(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: nil,
		UploadStore: stubUploadStore{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.BlobStorage is nil")
	}
}

func TestNew_nilUploadStore(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: nil,
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.UploadStore is nil")
	}
}

func TestNew_success(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger:      zap.NewNop(),
		Cfg:         &config.Config{JWTSecret: strings.Repeat("a", config.MinJWTSecretLen)},
		AuthService: stubAuthService{},
		VaultEngine: stubVaultEngine{},
		BlobRepo:    stubBlobRepo{},
		BlobStorage: stubBlobStorage{},
		UploadStore: stubUploadStore{},
	}
	srv, err := New(deps, creds)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if srv == nil {
		t.Fatal("New() returned nil server")
	}
}

func TestTLSHandshake_ClientTrustsCA(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caPEM, serverCert, serverKey := writeTestServerTLSChain(t, dir)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srvCreds, err := credentials.NewServerTLSFromFile(serverCert, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer(grpc.Creds(srvCreds))
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, hs)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			t.Logf("Serve: %v", serveErr)
		}
	}()
	t.Cleanup(func() { s.Stop() })

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append CA cert to pool")
	}
	clientTLS := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	hc := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetStatus(); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status: got %v want SERVING", got)
	}
}

func TestTLSHandshake_ClientWithoutTrustedCAFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, serverCert, serverKey := writeTestServerTLSChain(t, dir)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	srvCreds, err := credentials.NewServerTLSFromFile(serverCert, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer(grpc.Creds(srvCreds))
	grpc_health_v1.RegisterHealthServer(s, health.NewServer())

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			t.Logf("Serve: %v", serveErr)
		}
	}()
	t.Cleanup(func() { s.Stop() })

	clientTLS := &tls.Config{
		RootCAs:    x509.NewCertPool(),
		MinVersion: tls.VersionTLS12,
	}
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	hc := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error when client does not trust server CA")
	}
}

// writeTestServerTLSChain writes ca.pem, server.crt, server.key under dir; returns CA PEM and server paths.
func writeTestServerTLSChain(t *testing.T, dir string) (caPEM []byte, serverCertPath, serverKeyPath string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"test-ca"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), caPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	serverCertPath = filepath.Join(dir, "server.crt")
	serverKeyPath = filepath.Join(dir, "server.key")
	if err := os.WriteFile(serverCertPath, serverCertPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverKeyPath, serverKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return caPEM, serverCertPath, serverKeyPath
}
