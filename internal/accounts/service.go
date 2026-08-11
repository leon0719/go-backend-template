package accounts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"go-backend-template/internal/db/sqlc"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrEmailTaken          = errors.New("email already registered")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

type accountsRepository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (sqlc.User, error)
	GetUserByEmail(ctx context.Context, email string) (sqlc.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (sqlc.User, error)
	StoreRefreshToken(ctx context.Context, userID uuid.UUID, digest string, expiresAt time.Time) (sqlc.RefreshToken, error)
	GetRefreshTokenByDigest(ctx context.Context, digest string) (sqlc.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
	RevokeAllRefreshTokensForUser(ctx context.Context, userID uuid.UUID) error
}

var _ accountsRepository = (*Repository)(nil)

type Service struct {
	repo      accountsRepository
	jwtSecret string
}

func NewService(repo accountsRepository, jwtSecret string) *Service {
	return &Service{repo: repo, jwtSecret: jwtSecret}
}

func (s *Service) issueTokens(ctx context.Context, userID uuid.UUID) (access, refresh string, err error) {
	access, err = NewAccessToken(s.jwtSecret, userID, accessTokenTTL)
	if err != nil {
		return "", "", err
	}
	plain, digest, err := NewRefreshTokenPlain()
	if err != nil {
		return "", "", err
	}
	if _, err = s.repo.StoreRefreshToken(ctx, userID, digest, time.Now().Add(refreshTokenTTL)); err != nil {
		return "", "", err
	}
	return access, plain, nil
}

// normalizeEmail canonicalizes an address so "A@X.com" and "a@x.com" are the
// same account. The domain part of an email is case-insensitive by spec and
// effectively every provider treats the local part that way too, so folding
// the whole address is the pragmatic choice -- and it must be applied on both
// register and login or users cannot log back in.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Register creates a user and issues a token pair.
//
// There is deliberately NO "does this email already exist?" pre-check: it
// would be a TOCTOU race (two concurrent registrations both pass, and the
// loser gets a raw constraint error as a 500). Uniqueness is enforced by the
// users_email_key UNIQUE constraint, and Repository.CreateUser translates
// SQLSTATE 23505 into ErrEmailTaken -> 409.
func (s *Service) Register(ctx context.Context, email, password string) (access, refresh string, err error) {
	email = normalizeEmail(email)

	hash, err := HashPassword(password)
	if err != nil {
		return "", "", err
	}
	user, err := s.repo.CreateUser(ctx, email, hash)
	if err != nil {
		return "", "", err
	}
	return s.issueTokens(ctx, user.ID)
}

func (s *Service) Login(ctx context.Context, email, password string) (access, refresh string, err error) {
	user, err := s.repo.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return "", "", ErrInvalidCredentials
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return "", "", ErrInvalidCredentials
	}
	return s.issueTokens(ctx, user.ID)
}

func (s *Service) Refresh(ctx context.Context, refreshPlain string) (access, refresh string, err error) {
	digest := digestOf(refreshPlain)
	rt, err := s.repo.GetRefreshTokenByDigest(ctx, digest)
	if err != nil || rt.RevokedAt.Valid || rt.ExpiresAt.Time.Before(time.Now()) {
		return "", "", ErrInvalidRefreshToken
	}
	if err := s.repo.RevokeRefreshToken(ctx, rt.ID); err != nil {
		return "", "", err
	}
	return s.issueTokens(ctx, rt.UserID)
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.repo.RevokeAllRefreshTokensForUser(ctx, userID)
}

func (s *Service) Me(ctx context.Context, userID uuid.UUID) (sqlc.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}
