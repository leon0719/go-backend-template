//go:build integration

package middleware

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

func setupTestRedis(t *testing.T) *goredis.Client {
	ctx := context.Background()
	rc, err := redis.Run(ctx, "redis:8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Terminate(ctx) })

	uri, err := rc.ConnectionString(ctx)
	require.NoError(t, err)

	opt, err := goredis.ParseURL(uri)
	require.NoError(t, err)
	return goredis.NewClient(opt)
}

func TestRateLimiter_AllowsUpToLimitThenBlocks(t *testing.T) {
	rdb := setupTestRedis(t)
	rl := NewRateLimiter(rdb, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, err := rl.Allow(ctx, "key1")
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	allowed, err := rl.Allow(ctx, "key1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rdb := setupTestRedis(t)
	rl := NewRateLimiter(rdb, 1, time.Minute)
	ctx := context.Background()

	allowed, err := rl.Allow(ctx, "a")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = rl.Allow(ctx, "b")
	require.NoError(t, err)
	assert.True(t, allowed)
}
