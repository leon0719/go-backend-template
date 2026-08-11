//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestNewPool_ConnectsSuccessfully(t *testing.T) {
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		// Required, not optional. The official Postgres image runs initdb
		// against a temporary server, logs "ready to accept connections",
		// then shuts it down and starts the real one. Connecting in that
		// window fails with "connection reset by peer". BasicWaitStrategies
		// waits for that log line twice and for the port to actually serve.
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	defer func() { _ = pgContainer.Terminate(ctx) }()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := NewPool(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	assert.NoError(t, pool.Ping(ctx))
}
