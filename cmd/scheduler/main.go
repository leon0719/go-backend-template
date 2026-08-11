// Command scheduler runs the asynq.Scheduler — the Celery Beat equivalent —
// which enqueues periodic tasks onto the same Redis queues cmd/worker
// consumes.
//
// It is a SEPARATE process from the worker on purpose: workers are meant to
// be scaled horizontally, and running a scheduler inside each replica would
// enqueue every periodic task once per replica. Run exactly one scheduler.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"go-backend-template/internal/config"
	"go-backend-template/internal/logging"
	"go-backend-template/internal/tasks"
)

// heartbeatSchedule is standard cron syntax (asynq also accepts "@every 5m").
const heartbeatSchedule = "*/5 * * * *"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := logging.New(cfg)
	slog.SetDefault(logger)

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		logger.Error("parse redis url", "error", err)
		os.Exit(1)
	}

	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{Location: time.UTC})

	if _, err := scheduler.Register(heartbeatSchedule, tasks.NewHeartbeatTask()); err != nil {
		logger.Error("register heartbeat task", "error", err)
		os.Exit(1)
	}

	// Start is non-blocking so we can drain on SIGINT/SIGTERM ourselves.
	if err := scheduler.Start(); err != nil {
		logger.Error("scheduler failed to start", "error", err)
		os.Exit(1)
	}
	logger.Info("scheduler started", "heartbeat_schedule", heartbeatSchedule)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutdown signal received, stopping scheduler")
	scheduler.Shutdown()
	logger.Info("scheduler stopped")
}
