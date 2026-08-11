package health

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"go-backend-template/internal/httpserver/respond"
)

func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, rdb *redis.Client) {
	h := &handler{pool: pool, rdb: rdb}

	r.Get("/health/live", h.live)
	r.Get("/health/ready", h.ready)
}

type handler struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

// live godoc
// @Summary      Liveness probe
// @Tags         health
// @Produce      json
// @Success      200 {object} map[string]string
// @Router       /health/live [get]
func (h *handler) live(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready godoc
// @Summary      Readiness probe (checks DB and Redis connectivity)
// @Tags         health
// @Produce      json
// @Success      200 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /health/ready [get]
func (h *handler) ready(w http.ResponseWriter, r *http.Request) {
	// When dependencies are not configured (e.g., in unit tests),
	// return 200 OK to indicate the service itself is running.
	if h.pool == nil || h.rdb == nil {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	ctx := r.Context()
	if err := h.pool.Ping(ctx); err != nil {
		respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
		return
	}
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "redis_unreachable"})
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
