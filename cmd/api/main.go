package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"

	"go-backend-template/internal/accounts"
	"go-backend-template/internal/articles"
	"go-backend-template/internal/config"
	"go-backend-template/internal/db"
	"go-backend-template/internal/db/sqlc"
	"go-backend-template/internal/httpserver"
	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/logging"
)

// authRateLimit and authRateLimitWindow bound the number of auth requests
// (register/login) a single client IP may make. 10 requests per minute is a
// reasonable default that allows normal retry/typo behavior while blocking
// brute-force and credential-stuffing attempts.
//
// articlesWriteRateLimit and articlesWriteRateLimitWindow bound the number of
// article write requests (create/update/delete/publish) a single
// authenticated user may make.
const (
	authRateLimit       = 10
	authRateLimitWindow = 60 * time.Second

	articlesWriteRateLimit       = 30
	articlesWriteRateLimitWindow = 60 * time.Second
)

// HTTP server timeouts. See the http.Server literal in main for why
// WriteTimeout is intentionally left unset (SSE).
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownGrace     = 20 * time.Second

	// healthcheckTimeout bounds the one-shot -healthcheck probe.
	healthcheckTimeout = 3 * time.Second
)

// @title           go-backend-template API
// @version         1.0
// @description     Example Go backend template API (accounts, articles, realtime).
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// healthcheckFlag, when passed as the process's first argument, makes the
// binary act as a one-shot health probe instead of starting the server: it
// GETs its own /health/live endpoint and exits 0/1 accordingly. This exists
// so the distroless prod image (which has no shell, curl, or wget) can still
// back a Docker HEALTHCHECK by invoking the /api binary itself.
const healthcheckFlag = "-healthcheck"

// GitCommitSHA is injected at build time via -ldflags "-X main.GitCommitSHA=...".
// See docker/Dockerfile.prod's GIT_COMMIT_SHA build arg. Left empty in
// non-container builds.
var GitCommitSHA string

func main() {
	if len(os.Args) > 1 && os.Args[1] == healthcheckFlag {
		runHealthcheck()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := logging.New(cfg)
	// Install as the process-wide default so packages that log via the slog
	// package-level functions (e.g. the middleware chain, ratelimit's
	// fail-open path) use the configured handler/level rather than the stdlib
	// default text handler.
	slog.SetDefault(logger)
	logger.Info("starting", "commit", GitCommitSHA)

	// Shut down gracefully on SIGINT/SIGTERM so in-flight requests finish and
	// the deferred pool/asynq client Close calls actually run.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect db", "error", err)
		return
	}
	defer pool.Close()

	redisOpt, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("parse redis url", "error", err)
		return
	}
	rdb := goredis.NewClient(redisOpt)

	asynqRedisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("parse redis url for asynq", "error", err)
		return
	}
	asynqClient := asynq.NewClient(asynqRedisOpt)
	defer asynqClient.Close()

	accountsRepo := accounts.NewRepository(sqlc.New(pool))
	accountsSvc := accounts.NewService(accountsRepo, cfg.JWTSecret)
	authRateLimiter := middleware.NewRateLimiter(rdb, authRateLimit, authRateLimitWindow)

	articlesRepo := articles.NewRepository(pool)
	articlesSvc := articles.NewService(articlesRepo, func(task *asynq.Task) error {
		_, err := asynqClient.Enqueue(task)
		return err
	})
	articlesWriteRateLimiter := middleware.NewRateLimiter(rdb, articlesWriteRateLimit, articlesWriteRateLimitWindow)

	router := httpserver.NewRouter(httpserver.Deps{
		Config:         cfg,
		Logger:         logger,
		Version:        GitCommitSHA,
		Pool:           pool,
		AccountsSvc:    accountsSvc,
		AuthRateLimit:  authRateLimiter,
		ArticlesSvc:    articlesSvc,
		WriteRateLimit: articlesWriteRateLimiter,
		Redis:          rdb,
	})

	addr := ":" + strconv.Itoa(cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
		// Bound how long a slow client may take to send request headers /
		// the full request. Without ReadHeaderTimeout the server is exposed
		// to Slowloris-style resource exhaustion (also gosec G114).
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		// WriteTimeout is deliberately 0 (unbounded): it caps the time from
		// the end of the request headers to the end of the response write,
		// which would kill long-lived SSE streams (/api/v1/realtime/sse)
		// mid-flight. Bound streaming duration inside the handler instead.
		WriteTimeout: 0,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("server exited", "error", err)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections", "grace", shutdownGrace)
		stop() // restore default signal handling: a second Ctrl-C exits now
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
		logger.Info("server stopped")
	}
}

// runHealthcheck implements the -healthcheck flag: it loads only the
// server-related config (Env/Port, via config.LoadServerOnly) so it has no
// dependency on DATABASE_URL/REDIS_URL/JWT_SECRET being set — a health probe
// must not fail just because config validation would — then GETs
// http://127.0.0.1:<port>/health/live and exits 0 on a 2xx response or 1
// otherwise.
func runHealthcheck() {
	// Split so every cleanup (defer) runs before os.Exit.
	if err := probeLiveness(); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
}

func probeLiveness() error {
	serverCfg, err := config.LoadServerOnly()
	if err != nil {
		return fmt.Errorf("load config failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/health/live", serverCfg.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unhealthy status %d", resp.StatusCode)
	}
	return nil
}
