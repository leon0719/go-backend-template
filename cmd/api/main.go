package main

import (
	"context"
	"log"
	"net/http"
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

func main() {
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
