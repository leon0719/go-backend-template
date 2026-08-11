package articles

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/db/sqlc"
)

type fakeArticlesRepo struct {
	items map[uuid.UUID]sqlc.Article
	// lastOffset records what Service.List computed, so tests can assert the
	// offset never goes negative (int32 overflow on huge page numbers).
	lastOffset int32
	lastLimit  int32
}

func newFakeArticlesRepo() *fakeArticlesRepo {
	return &fakeArticlesRepo{items: map[uuid.UUID]sqlc.Article{}}
}

func (f *fakeArticlesRepo) Create(ctx context.Context, userID uuid.UUID, title, body, summary string) (sqlc.Article, error) {
	a := sqlc.Article{ID: uuid.New(), UserID: userID, Title: title, Body: body, Summary: summary, Status: "draft"}
	f.items[a.ID] = a
	return a, nil
}

func (f *fakeArticlesRepo) GetOwned(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	a, ok := f.items[id]
	if !ok || a.UserID != userID {
		return sqlc.Article{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeArticlesRepo) ListOwned(ctx context.Context, userID uuid.UUID, status, q string, limit, offset int32) ([]sqlc.Article, int64, error) {
	f.lastLimit, f.lastOffset = limit, offset

	var all []sqlc.Article
	for _, a := range f.items {
		if a.UserID == userID {
			all = append(all, a)
		}
	}
	// Deterministic order so limit/offset slicing is reproducible.
	sort.Slice(all, func(i, j int) bool { return all[i].ID.String() < all[j].ID.String() })

	total := int64(len(all))
	if int(offset) >= len(all) {
		return nil, total, nil
	}
	page := all[offset:]
	if int(limit) < len(page) {
		page = page[:limit]
	}
	return page, total, nil
}

func (f *fakeArticlesRepo) Update(ctx context.Context, id, userID uuid.UUID, title, body, summary *string) (sqlc.Article, error) {
	a, ok := f.items[id]
	if !ok || a.UserID != userID {
		return sqlc.Article{}, ErrNotFound
	}
	if title != nil {
		a.Title = *title
	}
	if body != nil {
		a.Body = *body
	}
	if summary != nil {
		a.Summary = *summary
	}
	f.items[id] = a
	return a, nil
}

func (f *fakeArticlesRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	a, ok := f.items[id]
	if !ok || a.UserID != userID {
		return ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func (f *fakeArticlesRepo) PublishIfDraft(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	a, ok := f.items[id]
	if !ok || a.UserID != userID || a.Status != "draft" {
		return false, nil
	}
	a.Status = "published"
	f.items[id] = a
	return true, nil
}

func TestService_Publish_EnqueuesTaskOnlyOnTransition(t *testing.T) {
	repo := newFakeArticlesRepo()
	var enqueued []*asynq.Task
	svc := NewService(repo, func(t *asynq.Task) error {
		enqueued = append(enqueued, t)
		return nil
	})

	ctx := context.Background()
	userID := uuid.New()
	created, err := svc.Create(ctx, userID, "T", "B", "")
	require.NoError(t, err)

	_, err = svc.Publish(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Len(t, enqueued, 1)

	_, err = svc.Publish(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Len(t, enqueued, 1, "publishing an already-published article must not enqueue again")
}

func TestService_Get_OtherUsersArticle_ReturnsNotFound(t *testing.T) {
	repo := newFakeArticlesRepo()
	svc := NewService(repo, func(t *asynq.Task) error { return nil })

	ctx := context.Background()
	owner := uuid.New()
	created, err := svc.Create(ctx, owner, "T", "B", "")
	require.NoError(t, err)

	_, err = svc.Get(ctx, created.ID, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestService_Publish_EnqueueFailureStillSucceeds documents the dual-write
// gap: PublishIfDraft has already COMMITTED by the time enqueue runs. Failing
// the request would be a lie (the article is published) and would invite a
// retry that sees transitioned == false and silently succeeds without ever
// sending the webhook. We log at Error and return success instead. The real
// fix is a transactional outbox -- see docs/backend-standards.md.
func TestService_Publish_EnqueueFailureStillSucceeds(t *testing.T) {
	repo := newFakeArticlesRepo()
	svc := NewService(repo, func(*asynq.Task) error { return errors.New("redis down") })

	ctx := context.Background()
	userID := uuid.New()
	created, err := svc.Create(ctx, userID, "T", "B", "")
	require.NoError(t, err)

	got, err := svc.Publish(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, "published", got.Status)
}

func TestService_List_HugePageDoesNotOverflowOffset(t *testing.T) {
	repo := newFakeArticlesRepo()
	svc := NewService(repo, func(*asynq.Task) error { return nil })

	ctx := context.Background()
	userID := uuid.New()
	_, err := svc.Create(ctx, userID, "T", "B", "")
	require.NoError(t, err)

	// (100000000-1)*100 overflows int32; the offset must stay non-negative.
	items, _, err := svc.List(ctx, userID, "", "", 100000000, 100)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.GreaterOrEqual(t, repo.lastOffset, int32(0))
}
