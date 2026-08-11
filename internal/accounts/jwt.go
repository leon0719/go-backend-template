package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"go-backend-template/internal/jwtutil"
)

// NewAccessToken and ParseAccessToken delegate to internal/jwtutil, which
// holds the actual JWT logic. They are kept here as thin wrappers so
// existing callers/tests in this package don't need to change, while
// internal/httpserver/middleware depends on jwtutil directly (avoiding an
// accounts <-> middleware import cycle, since middleware also needs to
// parse tokens and accounts needs middleware for route wiring).
func NewAccessToken(secret string, userID uuid.UUID, ttl time.Duration) (string, error) {
	return jwtutil.NewAccessToken(secret, userID, ttl)
}

func ParseAccessToken(secret, tokenStr string) (uuid.UUID, error) {
	return jwtutil.ParseAccessToken(secret, tokenStr)
}

func digestOf(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func NewRefreshTokenPlain() (plain string, digest string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = hex.EncodeToString(b)
	digest = digestOf(plain)
	return plain, digest, nil
}
