package articles

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/db/sqlc"
)

type fakeArticlesRepo struct {
	items map[uuid.UUID]sqlc.Article
}

func newFakeArticlesRepo() *fakeArticlesRepo {
	return &fakeArticlesRepo{items: map[uuid.UUID]sqlc.Article{}}
}

func (f *fakeArticlesRepo) Create(ctx context.Context, userID uuid.UUID, title, body string) (sqlc.Article, error) {
	a := sqlc.Article{ID: uuid.New(), UserID: userID, Title: title, Body: body, Status: "draft"}
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
	var out []sqlc.Article
	for _, a := range f.items {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeArticlesRepo) Update(ctx context.Context, id, userID uuid.UUID, title, body *string) (sqlc.Article, error) {
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
	created, err := svc.Create(ctx, userID, "T", "B")
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
	created, err := svc.Create(ctx, owner, "T", "B")
	require.NoError(t, err)

	_, err = svc.Get(ctx, created.ID, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
}
