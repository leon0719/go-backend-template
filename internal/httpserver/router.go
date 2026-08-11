package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"go-backend-template/internal/accounts"
	"go-backend-template/internal/articles"
	"go-backend-template/internal/config"
	"go-backend-template/internal/health"
	_ "go-backend-template/internal/httpserver/docs"
	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/realtime"
)

type Deps struct {
	Config         *config.Config
	Logger         *slog.Logger
	Pool           *pgxpool.Pool
	AccountsSvc    *accounts.Service
	AuthRateLimit  *middleware.RateLimiter
	ArticlesSvc    *articles.Service
	WriteRateLimit *middleware.RateLimiter
	Redis          *redis.Client
}

func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)

	// Health endpoints are unversioned infra probes mounted at the router root.
	// They are intentionally excluded from the generated OpenAPI/Swagger spec
	// (no swag annotations in internal/health/handler.go) because Swagger UI
	// would prefix them with /api/v1, advertising URLs the server never serves.
	// Do not re-add swag annotations for /health/* without also reconsidering
	// where the routes are mounted.
	health.RegisterRoutes(r, deps.Pool, deps.Redis)

	r.Get("/api/docs/*", httpSwagger.WrapHandler)

	r.Route("/api/v1/auth", func(ar chi.Router) {
		accounts.RegisterRoutes(ar, deps.AccountsSvc, deps.Config.JWTSecret, deps.AuthRateLimit)
	})

	r.Route("/api/v1/articles", func(ar chi.Router) {
		articles.RegisterRoutes(ar, deps.ArticlesSvc, deps.Config.JWTSecret, deps.WriteRateLimit)
	})

	r.Route("/api/v1/realtime", func(rr chi.Router) {
		realtime.RegisterRoutes(rr, deps.Config.JWTSecret)
	})

	return r
}
