package articles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/tasks"
)

func TestNotifyArticlePublishedWebhook_NoopWhenURLEmpty(t *testing.T) {
	err := NotifyArticlePublishedWebhook(context.Background(), "", "article-1")
	assert.NoError(t, err)
}

func TestNotifyArticlePublishedWebhook_PostsPayload(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := NotifyArticlePublishedWebhook(context.Background(), server.URL, "article-1")
	require.NoError(t, err)
	assert.Equal(t, "article-1", gotBody["article_id"])
}

func TestPublishedTaskHandler_CallsWebhook(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var handler func(context.Context, *asynq.Task) error = NewPublishedTaskHandler(server.URL)

	task, err := tasks.NewArticlePublishedTask("article-1")
	require.NoError(t, err)

	require.NoError(t, handler(context.Background(), task))
	assert.True(t, called)
}
