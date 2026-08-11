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

// appName is reported by GET /. Rename it when you fork this template.
const appName = "go-backend-template"

type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	// Version is reported by GET /. Empty renders as "dev".
	Version        string
	Pool           *pgxpool.Pool
	AccountsSvc    *accounts.Service
	AuthRateLimit  *middleware.RateLimiter
	ArticlesSvc    *articles.Service
	WriteRateLimit *middleware.RateLimiter
	Redis          *redis.Client

	// ExtraRoutes is an optional hook to mount additional routes on the same
	// middleware chain without editing this file. It is also what
	// router_test.go uses to mount a deliberately-panicking route and assert
	// the Recoverer behavior end-to-end.
	ExtraRoutes func(chi.Router)
}

// NewRouter assembles the chi router and the global middleware chain.
//
// Order matters and follows the design spec:
//
//	RequestID  — mints/propagates X-Request-ID first so everything below can log it
//	RealIP     — resolves the client IP (trusted-proxy aware) for logging + rate limits
//	Recoverer  — turns panics into the standard 500 envelope
//	SlogLogger — logs the final status (including any 500 written by Recoverer)
//	CORS       — no-op unless CORS_ALLOWED_ORIGINS is set
//
// Route-specific middleware (RateLimit / JWTAuth) is mounted per-route by the
// individual RegisterRoutes functions.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	trustedProxies, err := middleware.ParseTrustedProxies(deps.Config.TrustedProxies)
	if err != nil {
		// Misconfigured TRUSTED_PROXIES silently degrades rate limiting, so
		// surface it loudly and fall back to "trust nothing" (RemoteAddr).
		logger := deps.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error("invalid TRUSTED_PROXIES, ignoring X-Forwarded-For entirely", "error", err)
		trustedProxies = nil
	}

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP(trustedProxies))
	r.Use(middleware.Recoverer)
	r.Use(middleware.SlogLogger)
	r.Use(middleware.CORS(deps.Config.CORSAllowedOrigins))

	// Health endpoints are unversioned infra probes mounted at the router root.
	// They are intentionally excluded from the generated OpenAPI/Swagger spec
	// (no swag annotations in internal/health/handler.go) because Swagger UI
	// would prefix them with /api/v1, advertising URLs the server never serves.
	// Do not re-add swag annotations for /health/* without also reconsidering
	// where the routes are mounted.
	version := deps.Version
	if version == "" {
		version = "dev"
	}
	health.RegisterRoutes(r, deps.Pool, deps.Redis, appName, version)

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

	if deps.ExtraRoutes != nil {
		deps.ExtraRoutes(r)
	}

	return r
}
