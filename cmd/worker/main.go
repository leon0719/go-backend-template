package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"go-backend-template/internal/articles"
	"go-backend-template/internal/config"
	"go-backend-template/internal/logging"
	"go-backend-template/internal/tasks"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := logging.New(cfg)
	// Install as the process-wide default so packages logging via the slog
	// package-level functions use the configured handler (see cmd/api).
	slog.SetDefault(logger)

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("parse redis url", "error", err)
		os.Exit(1)
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 10})

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeArticlePublished, articles.NewPublishedTaskHandler(cfg.ArticlePublishedWebhookURL))
	// Periodic task enqueued by cmd/scheduler (the Celery Beat equivalent).
	mux.HandleFunc(tasks.TypeHeartbeat, tasks.HandleHeartbeat)

	// srv.Start is non-blocking (unlike srv.Run, which installs asynq's own
	// signal handling); using it lets us drain in-flight tasks explicitly on
	// SIGINT/SIGTERM instead of dropping them at deploy time.
	if err := srv.Start(mux); err != nil {
		logger.Error("worker failed to start", "error", err)
		os.Exit(1)
	}
	logger.Info("worker started")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutdown signal received, draining in-flight tasks")
	// Shutdown stops fetching new tasks and waits for in-flight handlers to
	// finish (bounded by asynq.Config.ShutdownTimeout, 8s by default).
	srv.Shutdown()
	logger.Info("worker stopped")
}
