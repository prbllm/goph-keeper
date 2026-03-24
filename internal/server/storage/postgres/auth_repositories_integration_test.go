package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prbllm/goph-keeper/internal/server/auth"
	"github.com/prbllm/goph-keeper/internal/server/migrations"
	"github.com/prbllm/goph-keeper/internal/server/storage/postgres"
	"go.uber.org/zap"
)

func testAuthUser(login string) *auth.User {
	return &auth.User{
		UserID:                 uuid.NewString(),
		Login:                  login,
		PasswordHash:           []byte("$2a$10$hashedplaceholderxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"),
		PasswordSalt:           []byte("salt"),
		KDFAlgorithm:           "argon2id",
		KDFMemoryKiB:           65536,
		KDFIterations:          3,
		KDFParallelism:         4,
		KDFKeyLength:           32,
		EncryptedVaultKey:      []byte("vault"),
		EncryptedVaultKeyNonce: []byte("nonce"),
	}
}

func openAuthIntegration(t *testing.T) (repos *postgres.AuthRepositories, sqlDB *sql.DB, cleanup func()) {
	t.Helper()
	dsn := os.Getenv("GOPHKEEPER_DATABASE_URL")
	if dsn == "" {
		t.Skip("GOPHKEEPER_DATABASE_URL not set, skipping auth integration tests")
	}
	ctx := context.Background()
	pool, err := postgres.OpenPing(ctx, dsn)
	if err != nil {
		t.Fatalf("OpenPing: %v", err)
	}
	sqlDB, ok := pool.(*sql.DB)
	if !ok {
		_ = pool.Close()
		t.Fatalf("pool type %T, want *sql.DB", pool)
	}
	if err := migrations.Run(zap.NewNop(), dsn); err != nil {
		_ = pool.Close()
		t.Fatalf("migrations: %v", err)
	}
	return postgres.NewAuthRepositories(pool), sqlDB, func() { _ = pool.Close() }
}

func TestAuthRepositories_RegisterWithSession_rollsBackOnInvalidSessionUser(t *testing.T) {
	repos, db, cleanup := openAuthIntegration(t)
	defer cleanup()
	ctx := context.Background()

	login := "rollback-" + uuid.NewString()
	user := testAuthUser(login)
	device := &auth.Device{
		DeviceID:      "dev-" + uuid.NewString(),
		UserID:        user.UserID,
		DeviceName:    "d",
		Platform:      1,
		ClientVersion: "1",
	}
	rt, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	session := &auth.Session{
		TokenID:   uuid.NewString(),
		UserID:    uuid.NewString(),
		DeviceID:  device.DeviceID,
		TokenHash: auth.HashToken(rt),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repos.RegisterWithSession(ctx, user, device, session); err == nil {
		t.Fatal("expected error from FK / transaction failure")
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE login = $1`, login).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected no user row after rollback, got count %d", n)
	}
}

func TestAuthRepositories_LoginWithDeviceAndSession_rejectsOtherUserDevice(t *testing.T) {
	repos, _, cleanup := openAuthIntegration(t)
	defer cleanup()
	ctx := context.Background()

	devShared := "dev-shared-" + uuid.NewString()
	u1 := testAuthUser("u1-" + uuid.NewString())
	d1 := &auth.Device{
		DeviceID:      devShared,
		UserID:        u1.UserID,
		DeviceName:    "a",
		Platform:      1,
		ClientVersion: "1",
	}
	rt1, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	s1 := &auth.Session{
		TokenID:   uuid.NewString(),
		UserID:    u1.UserID,
		DeviceID:  devShared,
		TokenHash: auth.HashToken(rt1),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repos.RegisterWithSession(ctx, u1, d1, s1); err != nil {
		t.Fatalf("register u1: %v", err)
	}

	u2 := testAuthUser("u2-" + uuid.NewString())
	d2 := &auth.Device{
		DeviceID:      "dev-u2-" + uuid.NewString(),
		UserID:        u2.UserID,
		DeviceName:    "b",
		Platform:      1,
		ClientVersion: "1",
	}
	rt2, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	s2 := &auth.Session{
		TokenID:   uuid.NewString(),
		UserID:    u2.UserID,
		DeviceID:  d2.DeviceID,
		TokenHash: auth.HashToken(rt2),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repos.RegisterWithSession(ctx, u2, d2, s2); err != nil {
		t.Fatalf("register u2: %v", err)
	}

	rt3, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	s3 := &auth.Session{
		TokenID:   uuid.NewString(),
		UserID:    u2.UserID,
		DeviceID:  devShared,
		TokenHash: auth.HashToken(rt3),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	dSteal := &auth.Device{
		DeviceID:      devShared,
		UserID:        u2.UserID,
		DeviceName:    "evil",
		Platform:      1,
		ClientVersion: "1",
	}
	err = repos.LoginWithDeviceAndSession(ctx, dSteal, s3)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("LoginWithDeviceAndSession: %v, want ErrUnauthorized", err)
	}
}

func TestAuthRepositories_RotateRefresh(t *testing.T) {
	repos, db, cleanup := openAuthIntegration(t)
	defer cleanup()
	ctx := context.Background()

	login := "rot-" + uuid.NewString()
	user := testAuthUser(login)
	devID := "dev-rot-" + uuid.NewString()
	device := &auth.Device{
		DeviceID:      devID,
		UserID:        user.UserID,
		DeviceName:    "d",
		Platform:      1,
		ClientVersion: "1",
	}
	oldPlain, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	oldSession := &auth.Session{
		TokenID:   uuid.NewString(),
		UserID:    user.UserID,
		DeviceID:  devID,
		TokenHash: auth.HashToken(oldPlain),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := repos.RegisterWithSession(ctx, user, device, oldSession); err != nil {
		t.Fatalf("register: %v", err)
	}

	newPlain, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	newSess := &auth.Session{
		TokenID:   uuid.NewString(),
		TokenHash: auth.HashToken(newPlain),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		DeviceID:  devID,
	}
	srepo := repos.SessionRepository()
	if err := srepo.RotateRefresh(ctx, auth.HashToken(oldPlain), devID, time.Now(), newSess); err != nil {
		t.Fatalf("RotateRefresh: %v", err)
	}
	if newSess.UserID != user.UserID || newSess.DeviceID != devID {
		t.Fatalf("unexpected session fields user=%q device=%q", newSess.UserID, newSess.DeviceID)
	}

	var revoked bool
	q := `SELECT revoked_at IS NOT NULL FROM refresh_tokens WHERE token_hash = $1`
	if err := db.QueryRowContext(ctx, q, auth.HashToken(oldPlain)).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("expected old refresh token row to be revoked")
	}
}
