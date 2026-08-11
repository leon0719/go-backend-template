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
