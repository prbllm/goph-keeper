package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/prbllm/goph-keeper/internal/server/config"
)

type JWTIssuer struct {
	secret []byte
	now    func() time.Time
}

func NewJWTIssuer(secret string, now func() time.Time) *JWTIssuer {
	if now == nil {
		now = time.Now
	}
	return &JWTIssuer{secret: []byte(secret), now: now}
}

func (j *JWTIssuer) IssueAccessToken(claims AccessTokenClaims, ttl time.Duration) (string, error) {
	now := j.now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		config.JWTClaimSubject:  claims.UserID,
		config.JWTClaimDeviceID: claims.DeviceID,
		config.JWTClaimIssuedAt: now.Unix(),
		config.JWTClaimExpiry:   now.Add(ttl).Unix(),
	})
	return token.SignedString(j.secret)
}
