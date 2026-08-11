package articles

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"go-backend-template/internal/db/dberr"
	"go-backend-template/internal/db/sqlc"
)

// ErrNotFound is re-exported from internal/db/dberr so callers of this
// package can keep writing errors.Is(err, articles.ErrNotFound); the single
// definition lives in dberr and is shared with every other app.
var ErrNotFound = dberr.ErrNotFound

type Repository struct {
	q *sqlc.Queries
}

func NewRepository(q *sqlc.Queries) *Repository {
	return &Repository{q: q}
}

func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, title, body string) (sqlc.Article, error) {
	a, err := r.q.CreateArticle(ctx, sqlc.CreateArticleParams{UserID: userID, Title: title, Body: body})
	return a, dberr.WrapNotFound(err)
}

func (r *Repository) GetOwned(ctx context.Context, id, userID uuid.UUID) (sqlc.Article, error) {
	a, err := r.q.GetOwnedArticle(ctx, sqlc.GetOwnedArticleParams{ID: id, UserID: userID})
	return a, dberr.WrapNotFound(err)
}

func (r *Repository) ListOwned(ctx context.Context, userID uuid.UUID, status, q string, limit, offset int32) ([]sqlc.Article, int64, error) {
	items, err := r.q.ListOwnedArticles(ctx, sqlc.ListOwnedArticlesParams{
		UserID: userID, Column2: status, Column3: q, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, 0, dberr.WrapNotFound(err)
	}
	total, err := r.q.CountOwnedArticles(ctx, sqlc.CountOwnedArticlesParams{UserID: userID, Column2: status, Column3: q})
	if err != nil {
		return nil, 0, dberr.WrapNotFound(err)
	}
	return items, total, nil
}

func (r *Repository) Update(ctx context.Context, id, userID uuid.UUID, title, body *string) (sqlc.Article, error) {
	a, err := r.q.UpdateArticle(ctx, sqlc.UpdateArticleParams{
		ID:     id,
		UserID: userID,
		Title:  textFromPtr(title),
		Body:   textFromPtr(body),
	})
	return a, dberr.WrapNotFound(err)
}

func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	rows, err := r.q.DeleteArticle(ctx, sqlc.DeleteArticleParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) PublishIfDraft(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	rows, err := r.q.PublishArticleIfDraft(ctx, sqlc.PublishArticleIfDraftParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
