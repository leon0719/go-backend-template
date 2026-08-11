package tasks

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TypeArticlePublished = "article:published"

type ArticlePublishedPayload struct {
	ArticleID string `json:"article_id"`
}

func NewArticlePublishedTask(articleID string) (*asynq.Task, error) {
	payload, err := json.Marshal(ArticlePublishedPayload{ArticleID: articleID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeArticlePublished, payload), nil
}

// TypeHeartbeat is a periodic system task enqueued by cmd/scheduler (the
// asynq equivalent of Celery Beat). It exists to prove the scheduler ->
// queue -> worker path is alive end to end; see internal/tasks/heartbeat.go
// for the handler and docs/alerting.md for turning it into a real dead-man's
// switch (pinging an external heartbeat URL).
const TypeHeartbeat = "system:heartbeat"

func NewHeartbeatTask() *asynq.Task {
	return asynq.NewTask(TypeHeartbeat, nil)
}
