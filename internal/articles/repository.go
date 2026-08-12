package articles

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-backend-template/internal/db"
	"go-backend-template/internal/db/dberr"
	"go-backend-template/internal/db/sqlc"
)

// ErrNotFound is re-exported from internal/db/dberr so callers of this
// package can keep writing errors.Is(err, articles.ErrNotFound); the single
// definition lives in dberr and is shared with every other app.
var ErrNotFound = dberr.ErrNotFound

// ErrAlreadyArchived means the article_events row for this archive already
// exists -- see ArchiveWithEvent.
var ErrAlreadyArchived = errors.New("article already archived")

type Repository struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewRepository takes the pool rather than a *sqlc.Queries because
// ArchiveWithEvent needs to open its own transaction (pool.Begin), not just
// run queries against whatever DBTX the caller already wrapped.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: sqlc.New(pool)}
}

func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, title, body, summary string) (sqlc.Article, error) {
	a, err := r.q.CreateArticle(ctx, sqlc.CreateArticleParams{UserID: userID, Title: title, Body: body, Summary: summary})
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

func (r *Repository) Update(ctx context.Context, id, userID uuid.UUID, title, body, summary *string) (sqlc.Article, error) {
	a, err := r.q.UpdateArticle(ctx, sqlc.UpdateArticleParams{
		ID:      id,
		UserID:  userID,
		Title:   textFromPtr(title),
		Body:    textFromPtr(body),
		Summary: textFromPtr(summary),
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

// ArchiveWithEvent demonstrates the Django-`transaction.atomic()` pattern:
// two writes to the SAME database, wrapped in one transaction via the shared
// db.WithinTx helper so they commit or roll back together.
//
// Contrast with articles.Service.Publish (docs/backend-standards.md, "Dual
// writes"): that one straddles Postgres AND Redis/asynq, which a database
// transaction cannot span -- there the dual-write gap is accepted and
// documented instead. Here both writes are Postgres, so there is no excuse
// not to make them atomic.
func (r *Repository) ArchiveWithEvent(ctx context.Context, id, userID uuid.UUID) error {
	return db.WithinTx(ctx, r.pool, func(qtx *sqlc.Queries) error {
		rows, err := qtx.ArchiveArticle(ctx, sqlc.ArchiveArticleParams{ID: id, UserID: userID})
		if err != nil {
			return err
		}
		if rows == 0 {
			// Not found, not owned, or already archived -- all indistinguishable
			// from outside, same as every other method in this file.
			return ErrNotFound
		}

		if err := qtx.CreateArticleEvent(ctx, sqlc.CreateArticleEventParams{ArticleID: id, EventType: "archived"}); err != nil {
			if dberr.IsUniqueViolation(err) {
				// A duplicate archive event for this article. Returning here
				// rolls back the ArchiveArticle UPDATE above too -- the
				// article is left exactly as it was, not half-archived.
				return ErrAlreadyArchived
			}
			return err
		}
		return nil
	})
}
