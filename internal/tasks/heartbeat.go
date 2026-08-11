package tasks

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

// HandleHeartbeat is the handler for TypeHeartbeat. It deliberately lives in
// this package rather than under an app directory: it belongs to no domain,
// it is infrastructure. Everything else (article webhooks, etc.) follows the
// app convention and keeps its handler in internal/<app>/tasks.go.
//
// Right now it only logs. To turn it into a real dead-man's switch, GET an
// external heartbeat URL here (Healthchecks.io, Better Stack, ...) so a
// missing ping alerts you that the scheduler or worker has stopped.
func HandleHeartbeat(ctx context.Context, _ *asynq.Task) error {
	slog.InfoContext(ctx, "heartbeat", "task", TypeHeartbeat)
	return nil
}
