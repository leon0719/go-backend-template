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
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := db.NewPool(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Run migrations via goose programmatically is out of scope here;
	// tests assume `goose up` has been run against connStr, or use a
	// migrate helper — see Task 5 pattern. For this task, apply schema directly:
	_, err = pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE refresh_tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_digest TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	require.NoError(t, err)

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
