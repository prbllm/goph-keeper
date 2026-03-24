package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prbllm/goph-keeper/internal/server/auth"
)

// sqlExec matches *sql.DB, *sql.Tx, and Pool for ExecContext.
type sqlExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type AuthRepositories struct {
	db Pool
}

// NewAuthRepositories builds auth storage on top of a Pool (typically *sql.DB from OpenPing).
func NewAuthRepositories(db Pool) *AuthRepositories {
	return &AuthRepositories{db: db}
}

func (r *AuthRepositories) insertUser(ctx context.Context, db sqlExec, user *auth.User) error {
	const query = `
INSERT INTO users (
	user_id, login, password_hash, password_salt, kdf_algorithm, kdf_memory_kib,
	kdf_iterations, kdf_parallelism, kdf_key_length, encrypted_vault_key, encrypted_vault_key_nonce
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)`
	_, err := db.ExecContext(ctx, query,
		user.UserID, user.Login, user.PasswordHash, user.PasswordSalt, user.KDFAlgorithm, user.KDFMemoryKiB,
		user.KDFIterations, user.KDFParallelism, user.KDFKeyLength, user.EncryptedVaultKey, user.EncryptedVaultKeyNonce,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *AuthRepositories) GetByLogin(ctx context.Context, login string) (*auth.User, error) {
	const query = `
SELECT user_id, login, password_hash, password_salt, kdf_algorithm, kdf_memory_kib,
       kdf_iterations, kdf_parallelism, kdf_key_length, encrypted_vault_key, encrypted_vault_key_nonce
FROM users
WHERE login = $1`
	var user auth.User
	err := r.db.QueryRowContext(ctx, query, login).Scan(
		&user.UserID, &user.Login, &user.PasswordHash, &user.PasswordSalt, &user.KDFAlgorithm, &user.KDFMemoryKiB,
		&user.KDFIterations, &user.KDFParallelism, &user.KDFKeyLength, &user.EncryptedVaultKey, &user.EncryptedVaultKeyNonce,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrUnauthorized
		}
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepositories) upsertDevice(ctx context.Context, db sqlExec, device *auth.Device) error {
	const query = `
INSERT INTO devices (device_id, user_id, device_name, platform, client_version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (device_id) DO UPDATE SET
	device_name = EXCLUDED.device_name,
	platform = EXCLUDED.platform,
	client_version = EXCLUDED.client_version,
	last_seen_at = NOW(),
	revoked_at = NULL
WHERE devices.user_id = EXCLUDED.user_id`
	res, err := db.ExecContext(ctx, query, device.DeviceID, device.UserID, device.DeviceName, device.Platform, device.ClientVersion)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return auth.ErrUnauthorized
	}
	return nil
}

func (r *AuthRepositories) insertSession(ctx context.Context, db sqlExec, session *auth.Session) error {
	const query = `
INSERT INTO refresh_tokens (token_id, user_id, device_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)`
	_, err := db.ExecContext(ctx, query, session.TokenID, session.UserID, session.DeviceID, session.TokenHash, session.ExpiresAt)
	return err
}

func (r *AuthRepositories) RevokeByHash(ctx context.Context, tokenHash []byte) error {
	const query = `UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`
	res, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return auth.ErrUnauthorized
	}
	return nil
}

func (r *AuthRepositories) RegisterWithSession(ctx context.Context, user *auth.User, device *auth.Device, session *auth.Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.insertUser(ctx, tx, user); err != nil {
		return err
	}
	if err := r.upsertDevice(ctx, tx, device); err != nil {
		return err
	}
	if err := r.insertSession(ctx, tx, session); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *AuthRepositories) LoginWithDeviceAndSession(ctx context.Context, device *auth.Device, session *auth.Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.upsertDevice(ctx, tx, device); err != nil {
		return err
	}
	if err := r.insertSession(ctx, tx, session); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *AuthRepositories) RotateRefresh(ctx context.Context, oldHash []byte, deviceID string, now time.Time, newSession *auth.Session) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const sel = `
SELECT user_id, device_id
FROM refresh_tokens
WHERE token_hash = $1 AND device_id = $2 AND revoked_at IS NULL AND expires_at > $3
FOR UPDATE`
	var userID, devID string
	err = tx.QueryRowContext(ctx, sel, oldHash, deviceID, now).Scan(&userID, &devID)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrUnauthorized
	}
	if err != nil {
		return err
	}

	const revoke = `UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`
	revRes, err := tx.ExecContext(ctx, revoke, oldHash)
	if err != nil {
		return err
	}
	revoked, err := revRes.RowsAffected()
	if err != nil {
		return err
	}
	if revoked != 1 {
		return auth.ErrUnauthorized
	}

	const ins = `
INSERT INTO refresh_tokens (token_id, user_id, device_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, ins, newSession.TokenID, userID, devID, newSession.TokenHash, newSession.ExpiresAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	newSession.UserID = userID
	newSession.DeviceID = devID
	return nil
}

var (
	_ auth.UserRepository    = (*AuthRepositories)(nil)
	_ auth.AuthTx            = (*AuthRepositories)(nil)
	_ auth.SessionRepository = (*sessionRepoAdapter)(nil)
)

type sessionRepoAdapter struct {
	parent *AuthRepositories
}

func (a *sessionRepoAdapter) RevokeByHash(ctx context.Context, tokenHash []byte) error {
	return a.parent.RevokeByHash(ctx, tokenHash)
}

func (a *sessionRepoAdapter) RotateRefresh(ctx context.Context, oldTokenHash []byte, deviceID string, now time.Time, newSession *auth.Session) error {
	return a.parent.RotateRefresh(ctx, oldTokenHash, deviceID, now, newSession)
}

func (r *AuthRepositories) SessionRepository() auth.SessionRepository {
	return &sessionRepoAdapter{parent: r}
}
