package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot_ReturnsNameAndVersion(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, nil, nil, "go-backend-template", "test")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "go-backend-template", body["name"])
	assert.Equal(t, "test", body["version"])

	// Anonymous callers must not learn the environment, build SHA, or
	// dependency status from this endpoint — that is reconnaissance.
	assert.NotContains(t, body, "env")
	assert.NotContains(t, body, "commit")
	assert.NotContains(t, body, "status")
}

func TestLive_AlwaysReturns200(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, nil, nil, "go-backend-template", "test")

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReady_NilDependencies_Returns200(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, nil, nil, "go-backend-template", "test")

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// closedPool returns a pgxpool that can never connect: the DSN points at a
// port nothing listens on, so Ping fails fast. No Docker required.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://nobody:nobody@127.0.0.1:1/nodb?sslmode=disable&connect_timeout=1")
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err) // NewWithConfig is lazy; it does not dial here
	t.Cleanup(pool.Close)
	return pool
}

func TestReady_DBUnreachable_Returns503(t *testing.T) {
	r := chi.NewRouter()
	// nil Redis is skipped, so this isolates the DB branch.
	RegisterRoutes(r, unreachablePool(t), nil, "go-backend-template", "test")

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "db_unreachable", body["status"])
}

// TestReady_RedisUnreachable_Returns503 exercises the second 503 branch: a
// closed client fails every command immediately, and a nil pool skips the DB
// check. No Docker needed.
func TestReady_RedisUnreachable_Returns503(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, rdb.Close())

	r := chi.NewRouter()
	RegisterRoutes(r, nil, rdb, "go-backend-template", "test")

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "redis_unreachable", body["status"])
}
