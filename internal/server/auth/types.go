package auth

//go:generate go tool mockgen -typed -destination=./mocks/mock_auth.go -package=mocks . UserRepository,AuthTx,SessionRepository,PasswordHasher,TokenIssuer

import (
	"context"
	"time"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
)

type User struct {
	UserID                 string
	Login                  string
	PasswordHash           []byte
	PasswordSalt           []byte
	KDFAlgorithm           string
	KDFMemoryKiB           int32
	KDFIterations          int32
	KDFParallelism         int32
	KDFKeyLength           int32
	EncryptedVaultKey      []byte
	EncryptedVaultKeyNonce []byte
}

type Device struct {
	DeviceID      string
	UserID        string
	DeviceName    string
	Platform      int16
	ClientVersion string
}

type Session struct {
	TokenID   string
	UserID    string
	DeviceID  string
	TokenHash []byte
	ExpiresAt time.Time
}

type UserRepository interface {
	GetByLogin(ctx context.Context, login string) (*User, error)
}

// AuthTx performs multi-step auth persistence atomically (single DB transaction).
type AuthTx interface {
	RegisterWithSession(ctx context.Context, user *User, device *Device, session *Session) error
	LoginWithDeviceAndSession(ctx context.Context, device *Device, session *Session) error
}

type SessionRepository interface {
	RevokeByHash(ctx context.Context, tokenHash []byte) error
	// RotateRefresh revokes the session for oldTokenHash and inserts newSession in one transaction.
	// On success it sets newSession.UserID (and DeviceID from the stored row).
	RotateRefresh(ctx context.Context, oldTokenHash []byte, deviceID string, now time.Time, newSession *Session) error
}

type PasswordHasher interface {
	Hash(password string) ([]byte, error)
	Compare(hash []byte, password string) error
}

type AccessTokenClaims struct {
	UserID   string
	DeviceID string
}

type TokenIssuer interface {
	IssueAccessToken(claims AccessTokenClaims, ttl time.Duration) (string, error)
}

type Clock func() time.Time

type Service struct {
	users        UserRepository
	sessions     SessionRepository
	authTx       AuthTx
	hasher       PasswordHasher
	tokenIssuer  TokenIssuer
	now          Clock
	accessTTL    time.Duration
	refreshTTL   time.Duration
	inlineLimit  uint64
	maxBlobSize  uint64
	maxChunkSize uint64
}

func NewService(
	users UserRepository,
	sessions SessionRepository,
	authTx AuthTx,
	hasher PasswordHasher,
	tokenIssuer TokenIssuer,
	now Clock,
	accessTTL time.Duration,
	refreshTTL time.Duration,
	inlineLimit, maxBlobSize, maxChunkSize uint64,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		users:        users,
		sessions:     sessions,
		authTx:       authTx,
		hasher:       hasher,
		tokenIssuer:  tokenIssuer,
		now:          now,
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
		inlineLimit:  inlineLimit,
		maxBlobSize:  maxBlobSize,
		maxChunkSize: maxChunkSize,
	}
}

func (s *Service) Limits() *gophkeeperv1.Limits {
	return &gophkeeperv1.Limits{
		InlineThresholdBytes: s.inlineLimit,
		MaxBlobSizeBytes:     s.maxBlobSize,
		MaxChunkSizeBytes:    s.maxChunkSize,
	}
}
