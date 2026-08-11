package tasks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewArticlePublishedTask(t *testing.T) {
	task, err := NewArticlePublishedTask("article-123")
	require.NoError(t, err)
	assert.Equal(t, TypeArticlePublished, task.Type())

	var payload ArticlePublishedPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &payload))
	assert.Equal(t, "article-123", payload.ArticleID)
}
