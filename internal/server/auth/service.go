package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type RegisterInput struct {
	Login                  string
	Password               string
	PasswordSalt           []byte
	KDFAlgorithm           string
	KDFMemoryKiB           int32
	KDFIterations          int32
	KDFParallelism         int32
	KDFKeyLength           int32
	EncryptedVaultKey      []byte
	EncryptedVaultKeyNonce []byte
	DeviceID               string
	DeviceName             string
	Platform               int16
	ClientVersion          string
}

type RegisterOutput struct {
	UserID       string
	Login        string
	DeviceID     string
	AccessToken  string
	RefreshToken string
}

type LoginInput struct {
	Login         string
	Password      string
	DeviceID      string
	DeviceName    string
	Platform      int16
	ClientVersion string
}

type LoginOutput struct {
	UserID                 string
	Login                  string
	DeviceID               string
	AccessToken            string
	RefreshToken           string
	PasswordSalt           []byte
	KDFAlgorithm           string
	KDFMemoryKiB           int32
	KDFIterations          int32
	KDFParallelism         int32
	KDFKeyLength           int32
	EncryptedVaultKey      []byte
	EncryptedVaultKeyNonce []byte
}

type RefreshInput struct {
	RefreshToken string
	DeviceID     string
}

type RefreshOutput struct {
	AccessToken  string
	RefreshToken string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	login := strings.TrimSpace(in.Login)
	if login == "" || in.Password == "" {
		return nil, ErrInvalidArgument
	}
	passwordHash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}
	userID := uuid.NewString()
	deviceID := strings.TrimSpace(in.DeviceID)
	if deviceID == "" {
		deviceID = uuid.NewString()
	}

	refreshPlain, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session := &Session{
		TokenID:   uuid.NewString(),
		UserID:    userID,
		DeviceID:  deviceID,
		TokenHash: HashToken(refreshPlain),
		ExpiresAt: s.now().Add(s.refreshTTL),
	}
	if err := s.authTx.RegisterWithSession(ctx, &User{
		UserID:                 userID,
		Login:                  login,
		PasswordHash:           passwordHash,
		PasswordSalt:           in.PasswordSalt,
		KDFAlgorithm:           in.KDFAlgorithm,
		KDFMemoryKiB:           in.KDFMemoryKiB,
		KDFIterations:          in.KDFIterations,
		KDFParallelism:         in.KDFParallelism,
		KDFKeyLength:           in.KDFKeyLength,
		EncryptedVaultKey:      in.EncryptedVaultKey,
		EncryptedVaultKeyNonce: in.EncryptedVaultKeyNonce,
	}, &Device{
		DeviceID:      deviceID,
		UserID:        userID,
		DeviceName:    in.DeviceName,
		Platform:      in.Platform,
		ClientVersion: in.ClientVersion,
	}, session); err != nil {
		return nil, err
	}
	// Access JWT after DB commit; rare signing failure leaves account + refresh row without returning tokens to the client.
	accessToken, err := s.tokenIssuer.IssueAccessToken(AccessTokenClaims{
		UserID:   userID,
		DeviceID: deviceID,
	}, s.accessTTL)
	if err != nil {
		return nil, err
	}
	return &RegisterOutput{
		UserID:       userID,
		Login:        login,
		DeviceID:     deviceID,
		AccessToken:  accessToken,
		RefreshToken: refreshPlain,
	}, nil
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	login := strings.TrimSpace(in.Login)
	if login == "" || in.Password == "" {
		return nil, ErrInvalidArgument
	}
	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if err := s.hasher.Compare(user.PasswordHash, in.Password); err != nil {
		return nil, ErrUnauthorized
	}
	deviceID := strings.TrimSpace(in.DeviceID)
	if deviceID == "" {
		deviceID = uuid.NewString()
	}
	refreshPlain, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session := &Session{
		TokenID:   uuid.NewString(),
		UserID:    user.UserID,
		DeviceID:  deviceID,
		TokenHash: HashToken(refreshPlain),
		ExpiresAt: s.now().Add(s.refreshTTL),
	}
	if err := s.authTx.LoginWithDeviceAndSession(ctx, &Device{
		DeviceID:      deviceID,
		UserID:        user.UserID,
		DeviceName:    in.DeviceName,
		Platform:      in.Platform,
		ClientVersion: in.ClientVersion,
	}, session); err != nil {
		return nil, err
	}
	// Access JWT after DB commit; rare signing failure leaves device + refresh row without returning tokens to the client.
	accessToken, err := s.tokenIssuer.IssueAccessToken(AccessTokenClaims{
		UserID:   user.UserID,
		DeviceID: deviceID,
	}, s.accessTTL)
	if err != nil {
		return nil, err
	}
	return &LoginOutput{
		UserID:                 user.UserID,
		Login:                  user.Login,
		DeviceID:               deviceID,
		AccessToken:            accessToken,
		RefreshToken:           refreshPlain,
		PasswordSalt:           user.PasswordSalt,
		KDFAlgorithm:           user.KDFAlgorithm,
		KDFMemoryKiB:           user.KDFMemoryKiB,
		KDFIterations:          user.KDFIterations,
		KDFParallelism:         user.KDFParallelism,
		KDFKeyLength:           user.KDFKeyLength,
		EncryptedVaultKey:      user.EncryptedVaultKey,
		EncryptedVaultKeyNonce: user.EncryptedVaultKeyNonce,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, in RefreshInput) (*RefreshOutput, error) {
	if strings.TrimSpace(in.RefreshToken) == "" || strings.TrimSpace(in.DeviceID) == "" {
		return nil, ErrInvalidArgument
	}
	now := s.now()
	oldHash := HashToken(in.RefreshToken)
	refreshPlain, err := NewRefreshToken()
	if err != nil {
		return nil, err
	}
	newSession := &Session{
		TokenID:   uuid.NewString(),
		TokenHash: HashToken(refreshPlain),
		ExpiresAt: now.Add(s.refreshTTL),
		DeviceID:  strings.TrimSpace(in.DeviceID),
	}
	if err := s.sessions.RotateRefresh(ctx, oldHash, newSession.DeviceID, now, newSession); err != nil {
		return nil, err
	}
	// Access JWT is minted only after DB commit; if signing fails here, the client must sign in again.
	accessToken, err := s.tokenIssuer.IssueAccessToken(AccessTokenClaims{
		UserID:   newSession.UserID,
		DeviceID: newSession.DeviceID,
	}, s.accessTTL)
	if err != nil {
		return nil, err
	}
	return &RefreshOutput{
		AccessToken:  accessToken,
		RefreshToken: refreshPlain,
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return ErrInvalidArgument
	}
	if err := s.sessions.RevokeByHash(ctx, HashToken(refreshToken)); err != nil && !errors.Is(err, ErrUnauthorized) {
		return err
	}
	return nil
}
