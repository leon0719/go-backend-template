package main

import (
	"log"
	"os"

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

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("parse redis url", "error", err)
		os.Exit(1)
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 10})

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeArticlePublished, articles.NewPublishedTaskHandler(cfg.ArticlePublishedWebhookURL))

	logger.Info("starting worker")
	if err := srv.Run(mux); err != nil {
		logger.Error("worker exited", "error", err)
		os.Exit(1)
	}
}
