package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// discardLogger implements go-redis's internal Logging interface as a no-op,
// used to silence go-redis's own connection-pool retry logging so the test
// output stays pristine; we assert on our own slog line separately.
type discardLogger struct{}

func (discardLogger) Printf(context.Context, string, ...any) {}

// TestRateLimit_FailsOpenOnRedisError verifies that when the backing Redis
// store is unreachable, the middleware logs the error and allows the
// request through rather than returning a 500. This does not require
// Docker/testcontainers: it points at a closed local port.
func TestRateLimit_FailsOpenOnRedisError(t *testing.T) {
	goredis.SetLogger(discardLogger{})

	rdb := goredis.NewClient(&goredis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
		MaxRetries:  -1,
	})
	rl := NewRateLimiter(rdb, 5, time.Minute)

	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	mw := RateLimit(rl, func(r *http.Request) string { return "test-key" })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called (fail open) when Redis is unreachable")
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, http.StatusOK, rec.Code)
}
