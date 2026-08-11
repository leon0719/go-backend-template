package middleware

import (
	"context"
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

func (rl *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	fullKey := "ratelimit:" + key
	count, err := rl.rdb.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := rl.rdb.Expire(ctx, fullKey, rl.window).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(rl.limit), nil
}

func RateLimit(rl *RateLimiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, err := rl.Allow(r.Context(), keyFunc(r))
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "rate limit check failed")
				return
			}
			if !allowed {
				respond.Error(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
