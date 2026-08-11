package accounts

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAndParseAccessToken(t *testing.T) {
	userID := uuid.New()
	token, err := NewAccessToken("secret", userID, 15*time.Minute)
	require.NoError(t, err)

	gotID, err := ParseAccessToken("secret", token)
	require.NoError(t, err)
	assert.Equal(t, userID, gotID)
}

func TestParseAccessToken_WrongSecret_Fails(t *testing.T) {
	userID := uuid.New()
	token, err := NewAccessToken("secret", userID, 15*time.Minute)
	require.NoError(t, err)

	_, err = ParseAccessToken("other-secret", token)
	assert.Error(t, err)
}

func TestParseAccessToken_Expired_Fails(t *testing.T) {
	userID := uuid.New()
	token, err := NewAccessToken("secret", userID, -1*time.Minute)
	require.NoError(t, err)

	_, err = ParseAccessToken("secret", token)
	assert.Error(t, err)
}

func TestNewRefreshTokenPlain_ProducesDistinctPlainAndDigest(t *testing.T) {
	plain, digest, err := NewRefreshTokenPlain()
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.NotEmpty(t, digest)
	assert.NotEqual(t, plain, digest)

	plain2, digest2, err := NewRefreshTokenPlain()
	require.NoError(t, err)
	assert.NotEqual(t, plain, plain2)
	assert.NotEqual(t, digest, digest2)
}
