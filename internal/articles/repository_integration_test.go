//go:build integration

package articles

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"go-backend-template/internal/db"
)

func setupArticlesRepo(t *testing.T) (*Repository, uuid.UUID) {
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

	var userID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('a@example.com', 'x') RETURNING id`).Scan(&userID))

	return NewRepository(pool), userID
}

func TestRepository_CreateGetListUpdateDelete(t *testing.T) {
	repo, userID := setupArticlesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, userID, "Title", "Body", "")
	require.NoError(t, err)
	assert.Equal(t, "draft", created.Status)

	fetched, err := repo.GetOwned(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, "Title", fetched.Title)

	otherUser := uuid.New()
	_, err = repo.GetOwned(ctx, created.ID, otherUser)
	assert.ErrorIs(t, err, ErrNotFound)

	items, total, err := repo.ListOwned(ctx, userID, "", "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, int64(1), total)

	newTitle := "Updated"
	updated, err := repo.Update(ctx, created.ID, userID, &newTitle, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Title)

	require.NoError(t, repo.Delete(ctx, created.ID, userID))
	_, err = repo.GetOwned(ctx, created.ID, userID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_PublishIfDraft_OnlyPublishesOnce(t *testing.T) {
	repo, userID := setupArticlesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, userID, "Title", "Body", "")
	require.NoError(t, err)

	published, err := repo.PublishIfDraft(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.True(t, published)

	publishedAgain, err := repo.PublishIfDraft(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.False(t, publishedAgain)
}

func TestRepository_ArchiveWithEvent_CommitsBothWrites(t *testing.T) {
	repo, userID := setupArticlesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, userID, "Title", "Body", "")
	require.NoError(t, err)

	require.NoError(t, repo.ArchiveWithEvent(ctx, created.ID, userID))

	got, err := repo.GetOwned(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, "archived", got.Status)
}

// TestRepository_ArchiveWithEvent_RollsBackStatusWhenEventInsertFails is the
// atomicity demo: a pre-existing article_events row (simulating whatever
// business condition should have blocked this archive) makes the INSERT
// inside ArchiveWithEvent fail on the UNIQUE constraint *after* the UPDATE
// has already run against the transaction. Without a real transaction the
// UPDATE would still be committed, leaving the article archived with no
// audit row. Because both statements run through the same pgx.Tx, the failed
// INSERT rolls the UPDATE back too -- the article is left exactly as it was.
func TestRepository_ArchiveWithEvent_RollsBackStatusWhenEventInsertFails(t *testing.T) {
	repo, userID := setupArticlesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, userID, "Title", "Body", "")
	require.NoError(t, err)
	require.Equal(t, "draft", created.Status)

	_, err = repo.pool.Exec(ctx,
		`INSERT INTO article_events (article_id, event_type) VALUES ($1, 'archived')`, created.ID)
	require.NoError(t, err)

	err = repo.ArchiveWithEvent(ctx, created.ID, userID)
	require.ErrorIs(t, err, ErrAlreadyArchived)

	got, err := repo.GetOwned(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, "draft", got.Status, "the UPDATE must have rolled back along with the failed INSERT")
}

func TestRepository_SummaryRoundTrips(t *testing.T) {
	repo, userID := setupArticlesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, userID, "Title", "Body", "a short summary")
	require.NoError(t, err)
	assert.Equal(t, "a short summary", created.Summary)

	// Updating only the title must leave summary alone — that is what the
	// coalesce(sqlc.narg(...)) form in the UPDATE buys us.
	newTitle := "Changed"
	updated, err := repo.Update(ctx, created.ID, userID, &newTitle, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "Changed", updated.Title)
	assert.Equal(t, "a short summary", updated.Summary)
}
