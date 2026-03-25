package grpcserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/prbllm/goph-keeper/internal/server/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type stubPool struct{}

func (stubPool) PingContext(context.Context) error { return nil }
func (stubPool) Close() error                      { return nil }
func (stubPool) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}
func (stubPool) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }
func (stubPool) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}

type stubMinIO struct{}

func (stubMinIO) ListBuckets(context.Context) ([]minio.BucketInfo, error) { return nil, nil }
func (stubMinIO) BucketExists(context.Context, string) (bool, error)      { return true, nil }
func (stubMinIO) MakeBucket(context.Context, string, minio.MakeBucketOptions) error {
	return nil
}

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
		Logger: zap.NewNop(),
		Cfg:    &config.Config{},
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
	deps := &Deps{Logger: zap.NewNop(), Cfg: nil}
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
	deps := &Deps{Logger: nil, Cfg: &config.Config{}}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.Logger is nil")
	}
}

func TestNew_nilDB(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger: zap.NewNop(),
		Cfg: &config.Config{
			JWTSecret: strings.Repeat("a", config.MinJWTSecretLen),
		},
		MinIO: stubMinIO{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.DB is nil")
	}
}

func TestNew_nilMinIO(t *testing.T) {
	dir := t.TempDir()
	_, cert, key := writeTestServerTLSChain(t, dir)
	creds, err := credentials.NewServerTLSFromFile(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		Logger: zap.NewNop(),
		Cfg: &config.Config{
			JWTSecret: strings.Repeat("a", config.MinJWTSecretLen),
		},
		DB: stubPool{},
	}
	_, err = New(deps, creds)
	if err == nil {
		t.Fatal("expected error when deps.MinIO is nil")
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
