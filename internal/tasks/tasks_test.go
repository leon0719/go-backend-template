package tasks

import (
	"context"
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

func TestNewHeartbeatTask(t *testing.T) {
	task := NewHeartbeatTask()
	assert.Equal(t, TypeHeartbeat, task.Type())
	assert.Empty(t, task.Payload())
}

func TestHandleHeartbeat(t *testing.T) {
	assert.NoError(t, HandleHeartbeat(context.Background(), NewHeartbeatTask()))
}
