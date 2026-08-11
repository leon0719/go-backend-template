package jwtutil_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/jwtutil"
)

func TestParseAccessToken_RoundTrip(t *testing.T) {
	id := uuid.New()
	tok, err := jwtutil.NewAccessToken("secret", id, time.Minute)
	require.NoError(t, err)

	got, err := jwtutil.ParseAccessToken("secret", tok)
	require.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestParseAccessToken_RejectsWrongSecret(t *testing.T) {
	tok, err := jwtutil.NewAccessToken("secret", uuid.New(), time.Minute)
	require.NoError(t, err)

	_, err = jwtutil.ParseAccessToken("other-secret", tok)
	assert.Error(t, err)
}

func TestParseAccessToken_RejectsExpired(t *testing.T) {
	tok, err := jwtutil.NewAccessToken("secret", uuid.New(), -time.Minute)
	require.NoError(t, err)

	_, err = jwtutil.ParseAccessToken("secret", tok)
	assert.Error(t, err)
}

// The signing algorithm is pinned to HS256; a validly-signed token using any
// other algorithm must be rejected rather than accepted on the strength of
// the shared secret.
func TestParseAccessToken_RejectsUnpinnedAlgorithm(t *testing.T) {
	claims := jwt.RegisteredClaims{
		Subject:   uuid.New().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte("secret"))
	require.NoError(t, err)

	_, err = jwtutil.ParseAccessToken("secret", tok)
	assert.Error(t, err)
}
