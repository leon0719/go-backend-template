package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"go-backend-template/internal/accounts"
	"go-backend-template/internal/config"
	"go-backend-template/internal/health"
	"go-backend-template/internal/httpserver/middleware"
)

type Deps struct {
	Config        *config.Config
	Logger        *slog.Logger
	Pool          *pgxpool.Pool
	AccountsSvc   *accounts.Service
	AuthRateLimit *middleware.RateLimiter
	Redis         *redis.Client
}

func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)

	health.RegisterRoutes(r, deps.Pool, deps.Redis)

	r.Route("/api/v1/auth", func(ar chi.Router) {
		accounts.RegisterRoutes(ar, deps.AccountsSvc, deps.Config.JWTSecret, deps.AuthRateLimit)
	})

	return r
}
