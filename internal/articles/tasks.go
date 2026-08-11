package articles

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"go-backend-template/internal/tasks"
)

// NewPublishedTaskHandler returns an asynq handler for TypeArticlePublished
// tasks. The handler re-derives all state from the payload's article ID, so
// it is safe to run more than once for the same task.
func NewPublishedTaskHandler(webhookURL string) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload tasks.ArticlePublishedPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal payload: %w", err)
		}
		return NotifyArticlePublishedWebhook(ctx, webhookURL, payload.ArticleID)
	}
}
