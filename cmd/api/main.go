package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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

	ctx := context.Background()
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

	articlesRepo := articles.NewRepository(sqlc.New(pool))
	articlesSvc := articles.NewService(articlesRepo, func(task *asynq.Task) error {
		_, err := asynqClient.Enqueue(task)
		return err
	})
	articlesWriteRateLimiter := middleware.NewRateLimiter(rdb, articlesWriteRateLimit, articlesWriteRateLimitWindow)

	router := httpserver.NewRouter(httpserver.Deps{
		Config:         cfg,
		Logger:         logger,
		Pool:           pool,
		AccountsSvc:    accountsSvc,
		AuthRateLimit:  authRateLimiter,
		ArticlesSvc:    articlesSvc,
		WriteRateLimit: articlesWriteRateLimiter,
		Redis:          rdb,
	})

	addr := ":" + strconv.Itoa(cfg.Port)
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		logger.Error("server exited", "error", err)
	}
}

// runHealthcheck implements the -healthcheck flag: it reads PORT from the
// environment (falling back to 8000, the same default config.Load uses),
// GETs http://127.0.0.1:<port>/health/live, and exits 0 on a 2xx response or
// 1 otherwise. It intentionally avoids config.Load/godotenv so it has no
// dependency on DATABASE_URL/REDIS_URL/JWT_SECRET being set — a health probe
// must not fail just because config validation would.
func runHealthcheck() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health/live", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintln(os.Stderr, "healthcheck: unhealthy status", resp.StatusCode)
		os.Exit(1)
	}
}
