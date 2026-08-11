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

// live handles the liveness probe.
//
// Intentionally NOT documented via swag/OpenAPI annotations: this endpoint is
// an unversioned infrastructure probe mounted at the router root (/health/live),
// outside the /api/v1 base path. Swagger UI prefixes every documented path with
// basePath, so annotating this route would advertise /api/v1/health/live in the
// generated spec — a URL the server does not actually serve (404). Do not re-add
// swag annotations here without also changing where the route is mounted.
func (h *handler) live(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready handles the readiness probe (checks DB and Redis connectivity).
//
// Intentionally NOT documented via swag/OpenAPI annotations: same reasoning as
// live() above — this is an unversioned infrastructure probe served outside
// /api/v1, and documenting it under that base path would misrepresent the
// actual served URL.
func (h *handler) ready(w http.ResponseWriter, r *http.Request) {
	// A nil dependency means "not configured" (e.g. in unit tests) and is
	// skipped rather than failing the probe. Each configured dependency is
	// checked independently, so both failure branches are reachable.
	ctx := r.Context()
	if h.pool != nil {
		if err := h.pool.Ping(ctx); err != nil {
			respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
			return
		}
	}
	if h.rdb != nil {
		if err := h.rdb.Ping(ctx).Err(); err != nil {
			respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "redis_unreachable"})
			return
		}
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
