package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"go-backend-template/internal/db/sqlc"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	q *sqlc.Queries
}

func NewRepository(q *sqlc.Queries) *Repository {
	return &Repository{q: q}
}

func wrapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (sqlc.User, error) {
	u, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{Email: email, PasswordHash: passwordHash})
	return u, wrapNotFound(err)
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (sqlc.User, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	return u, wrapNotFound(err)
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (sqlc.User, error) {
	u, err := r.q.GetUserByID(ctx, id)
	return u, wrapNotFound(err)
}

func (r *Repository) StoreRefreshToken(ctx context.Context, userID uuid.UUID, digest string, expiresAt time.Time) (sqlc.RefreshToken, error) {
	rt, err := r.q.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		UserID:      userID,
		TokenDigest: digest,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	return rt, wrapNotFound(err)
}

func (r *Repository) GetRefreshTokenByDigest(ctx context.Context, digest string) (sqlc.RefreshToken, error) {
	rt, err := r.q.GetRefreshTokenByDigest(ctx, digest)
	return rt, wrapNotFound(err)
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return wrapNotFound(r.q.RevokeRefreshToken(ctx, id))
}

func (r *Repository) RevokeAllRefreshTokensForUser(ctx context.Context, userID uuid.UUID) error {
	return wrapNotFound(r.q.RevokeAllRefreshTokensForUser(ctx, userID))
}
