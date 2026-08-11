package accounts

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"go-backend-template/internal/db/dberr"
	"go-backend-template/internal/db/sqlc"
)

// ErrNotFound is re-exported from internal/db/dberr so callers of this
// package can keep writing errors.Is(err, accounts.ErrNotFound); the single
// definition lives in dberr and is shared with every other app.
var ErrNotFound = dberr.ErrNotFound

type Repository struct {
	q *sqlc.Queries
}

func NewRepository(q *sqlc.Queries) *Repository {
	return &Repository{q: q}
}

// CreateUser inserts a user, translating the users_email_key unique
// violation into ErrEmailTaken.
//
// This is deliberately the ONLY place email uniqueness is enforced: a
// SELECT-then-INSERT pre-check in the service would race two concurrent
// registrations of the same address, and the losing request would surface the
// raw constraint error as a 500 instead of a 409. Let the database be the
// arbiter and map its error.
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (sqlc.User, error) {
	u, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if dberr.IsUniqueViolation(err) {
		return sqlc.User{}, ErrEmailTaken
	}
	return u, dberr.WrapNotFound(err)
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (sqlc.User, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	return u, dberr.WrapNotFound(err)
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (sqlc.User, error) {
	u, err := r.q.GetUserByID(ctx, id)
	return u, dberr.WrapNotFound(err)
}

func (r *Repository) StoreRefreshToken(ctx context.Context, userID uuid.UUID, digest string, expiresAt time.Time) (sqlc.RefreshToken, error) {
	rt, err := r.q.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		UserID:      userID,
		TokenDigest: digest,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	return rt, dberr.WrapNotFound(err)
}

func (r *Repository) GetRefreshTokenByDigest(ctx context.Context, digest string) (sqlc.RefreshToken, error) {
	rt, err := r.q.GetRefreshTokenByDigest(ctx, digest)
	return rt, dberr.WrapNotFound(err)
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return dberr.WrapNotFound(r.q.RevokeRefreshToken(ctx, id))
}

func (r *Repository) RevokeAllRefreshTokensForUser(ctx context.Context, userID uuid.UUID) error {
	return dberr.WrapNotFound(r.q.RevokeAllRefreshTokensForUser(ctx, userID))
}
