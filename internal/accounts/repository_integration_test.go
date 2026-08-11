//go:build integration

package accounts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"go-backend-template/internal/db"
	"go-backend-template/internal/db/sqlc"
)

func setupTestRepo(t *testing.T) *Repository {
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
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := db.NewPool(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Apply the REAL migrations rather than a hand-written copy of the schema.
	// The copy that used to live here had already drifted: it predated the
	// case-insensitive email index, so a test could pass against a table shape
	// production no longer has.
	require.NoError(t, db.MigrateUp(connStr))

	return NewRepository(sqlc.New(pool))
}

func TestRepository_CreateAndGetUser(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	created, err := repo.CreateUser(ctx, "a@example.com", "hash")
	require.NoError(t, err)

	byEmail, err := repo.GetUserByEmail(ctx, "a@example.com")
	require.NoError(t, err)
	assert.Equal(t, created.ID, byEmail.ID)

	byID, err := repo.GetUserByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", byID.Email)
}

func TestRepository_GetUserByEmail_NotFound(t *testing.T) {
	repo := setupTestRepo(t)
	_, err := repo.GetUserByEmail(context.Background(), "missing@example.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_RefreshTokenLifecycle(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, "b@example.com", "hash")
	require.NoError(t, err)

	rt, err := repo.StoreRefreshToken(ctx, user.ID, "digest-1", time.Now().Add(24*time.Hour))
	require.NoError(t, err)

	found, err := repo.GetRefreshTokenByDigest(ctx, "digest-1")
	require.NoError(t, err)
	assert.Equal(t, rt.ID, found.ID)
	assert.False(t, found.RevokedAt.Valid)

	require.NoError(t, repo.RevokeAllRefreshTokensForUser(ctx, user.ID))

	revoked, err := repo.GetRefreshTokenByDigest(ctx, "digest-1")
	require.NoError(t, err)
	assert.True(t, revoked.RevokedAt.Valid)
}

// The service lowercases addresses before they reach the repository, so this
// goes straight to the repository to prove the guarantee holds even when it
// doesn't. The plain UNIQUE on email cannot catch this — the two strings
// differ — so a pass here means the functional index is doing the work.
func TestRepository_CreateUser_EmailIsCaseInsensitive(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, err := repo.CreateUser(ctx, "Someone@Example.com", "hash")
	require.NoError(t, err)

	_, err = repo.CreateUser(ctx, "someone@example.com", "hash")
	assert.ErrorIs(t, err, ErrEmailTaken)
}
