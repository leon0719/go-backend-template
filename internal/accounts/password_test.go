package accounts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	require.NoError(t, err)
	assert.NotEqual(t, "s3cret!", hash)
	assert.True(t, VerifyPassword(hash, "s3cret!"))
	assert.False(t, VerifyPassword(hash, "wrong"))
}
