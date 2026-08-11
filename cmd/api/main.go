package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"go-backend-template/internal/accounts"
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
const (
	authRateLimit       = 10
	authRateLimitWindow = 60 * time.Second
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

	accountsRepo := accounts.NewRepository(sqlc.New(pool))
	accountsSvc := accounts.NewService(accountsRepo, cfg.JWTSecret)
	authRateLimiter := middleware.NewRateLimiter(rdb, authRateLimit, authRateLimitWindow)

	router := httpserver.NewRouter(httpserver.Deps{
		Config:        cfg,
		Logger:        logger,
		Pool:          pool,
		AccountsSvc:   accountsSvc,
		AuthRateLimit: authRateLimiter,
		Redis:         rdb,
	})

	addr := ":" + strconv.Itoa(cfg.Port)
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		logger.Error("server exited", "error", err)
	}
}
