package accounts

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/db/sqlc"
)

type fakeRepo struct {
	usersByEmail map[string]sqlc.User
	usersByID    map[uuid.UUID]sqlc.User
	tokens       map[string]sqlc.RefreshToken
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		usersByEmail: map[string]sqlc.User{},
		usersByID:    map[uuid.UUID]sqlc.User{},
		tokens:       map[string]sqlc.RefreshToken{},
	}
}

func (f *fakeRepo) CreateUser(ctx context.Context, email, passwordHash string) (sqlc.User, error) {
	if _, ok := f.usersByEmail[email]; ok {
		return sqlc.User{}, ErrEmailTaken
	}
	u := sqlc.User{ID: uuid.New(), Email: email, PasswordHash: passwordHash}
	f.usersByEmail[email] = u
	f.usersByID[u.ID] = u
	return u, nil
}

func (f *fakeRepo) GetUserByEmail(ctx context.Context, email string) (sqlc.User, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return sqlc.User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) GetUserByID(ctx context.Context, id uuid.UUID) (sqlc.User, error) {
	u, ok := f.usersByID[id]
	if !ok {
		return sqlc.User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) StoreRefreshToken(ctx context.Context, userID uuid.UUID, digest string, expiresAt time.Time) (sqlc.RefreshToken, error) {
	rt := sqlc.RefreshToken{
		ID:          uuid.New(),
		UserID:      userID,
		TokenDigest: digest,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}
	f.tokens[digest] = rt
	return rt, nil
}

func (f *fakeRepo) GetRefreshTokenByDigest(ctx context.Context, digest string) (sqlc.RefreshToken, error) {
	rt, ok := f.tokens[digest]
	if !ok {
		return sqlc.RefreshToken{}, ErrNotFound
	}
	return rt, nil
}

func (f *fakeRepo) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	for k, rt := range f.tokens {
		if rt.ID == id {
			rt.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.tokens[k] = rt
		}
	}
	return nil
}

func (f *fakeRepo) RevokeAllRefreshTokensForUser(ctx context.Context, userID uuid.UUID) error {
	for k, rt := range f.tokens {
		if rt.UserID == userID {
			rt.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.tokens[k] = rt
		}
	}
	return nil
}

func TestService_RegisterThenLogin(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	access, refresh, err := svc.Register(ctx, "a@example.com", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)

	access2, refresh2, err := svc.Login(ctx, "a@example.com", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, access2)
	assert.NotEmpty(t, refresh2)
}

func TestService_Register_DuplicateEmail_Fails(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	_, _, err := svc.Register(ctx, "a@example.com", "password123")
	require.NoError(t, err)

	_, _, err = svc.Register(ctx, "a@example.com", "password123")
	assert.ErrorIs(t, err, ErrEmailTaken)
}

func TestService_Login_WrongPassword_Fails(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	_, _, err := svc.Register(ctx, "a@example.com", "password123")
	require.NoError(t, err)

	_, _, err = svc.Login(ctx, "a@example.com", "wrong-password")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestService_RefreshRotatesToken(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	_, refresh, err := svc.Register(ctx, "a@example.com", "password123")
	require.NoError(t, err)

	newAccess, newRefresh, err := svc.Refresh(ctx, refresh)
	require.NoError(t, err)
	assert.NotEmpty(t, newAccess)
	assert.NotEqual(t, refresh, newRefresh)

	// old refresh token must now be rejected
	_, _, err = svc.Refresh(ctx, refresh)
	assert.ErrorIs(t, err, ErrInvalidRefreshToken)
}

func TestService_LogoutRevokesRefresh(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	access, refresh, err := svc.Register(ctx, "a@example.com", "password123")
	require.NoError(t, err)

	userID, err := ParseAccessToken("secret", access)
	require.NoError(t, err)

	require.NoError(t, svc.Logout(ctx, userID))

	_, _, err = svc.Refresh(ctx, refresh)
	assert.ErrorIs(t, err, ErrInvalidRefreshToken)
}

// TestService_Register_EmailIsCaseInsensitive guards against "A@x.com" and
// "a@x.com" becoming two separate accounts, and against a user who
// registered with mixed case being unable to log back in.
func TestService_Register_EmailIsCaseInsensitive(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	ctx := context.Background()

	_, _, err := svc.Register(ctx, "Alice@Example.COM", "password123")
	require.NoError(t, err)

	_, _, err = svc.Register(ctx, "alice@example.com", "password123")
	assert.ErrorIs(t, err, ErrEmailTaken)

	_, _, err = svc.Login(ctx, "ALICE@EXAMPLE.COM", "password123")
	assert.NoError(t, err)
}
