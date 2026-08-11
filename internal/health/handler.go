package health

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"go-backend-template/internal/httpserver/respond"
)

func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, rdb *redis.Client) {
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		// When dependencies are not configured (e.g., in unit tests),
		// return 200 OK to indicate the service itself is running.
		if pool == nil || rdb == nil {
			respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		ctx := r.Context()
		if err := pool.Ping(ctx); err != nil {
			respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
			return
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "redis_unreachable"})
			return
		}
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
