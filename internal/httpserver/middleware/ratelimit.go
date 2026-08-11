package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"go-backend-template/internal/httpserver/respond"
)

type RateLimiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

func NewRateLimiter(rdb *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{rdb: rdb, limit: limit, window: window}
}

// allowScript atomically increments the counter for a key and, only on the
// first request of the window, sets its expiry. Running this as a single
// Redis Lua script prevents a crash/disconnect between INCR and EXPIRE from
// leaving a key with no TTL (which would permanently rate-limit the key).
var allowScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return current
`)

func (rl *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	fullKey := "ratelimit:" + key
	count, err := allowScript.Run(ctx, rl.rdb, []string{fullKey}, int(rl.window.Seconds())).Int64()
	if err != nil {
		return false, err
	}
	return count <= int64(rl.limit), nil
}

func RateLimit(rl *RateLimiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, err := rl.Allow(r.Context(), keyFunc(r))
			if err != nil {
				// Fail open: rate limiting is an abuse-prevention mechanism,
				// not a critical dependency. A Redis outage should not take
				// down the whole auth surface.
				slog.Error("rate limit check failed, allowing request", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				respond.Error(w, http.StatusTooManyRequests, respond.CodeRateLimited, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
