package articles

import (
	"context"
	"encoding/json"
	"errors"
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

// Tests for permanent vs transient webhook errors

func TestNotifyArticlePublishedWebhook_Returns4xxAsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400
	}))
	defer server.Close()

	err := NotifyArticlePublishedWebhook(context.Background(), server.URL, "article-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWebhookPermanent), "expected error to wrap ErrWebhookPermanent")
}

func TestNotifyArticlePublishedWebhook_Returns5xxAsTransient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // 500
	}))
	defer server.Close()

	err := NotifyArticlePublishedWebhook(context.Background(), server.URL, "article-1")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrWebhookPermanent), "expected error to NOT wrap ErrWebhookPermanent for 5xx")
}

func TestPublishedTaskHandler_MalformedPayloadGetsSkipRetry(t *testing.T) {
	handler := NewPublishedTaskHandler("")

	// Create a task with malformed payload
	task := asynq.NewTask(tasks.TypeArticlePublished, []byte("not json"))

	err := handler(context.Background(), task)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry), "expected error to wrap asynq.SkipRetry")
}

func TestPublishedTaskHandler_Webhook4xxGetsSkipRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400
	}))
	defer server.Close()

	handler := NewPublishedTaskHandler(server.URL)

	task, err := tasks.NewArticlePublishedTask("article-1")
	require.NoError(t, err)

	err = handler(context.Background(), task)
	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry), "expected error to wrap asynq.SkipRetry for 4xx")
}

func TestPublishedTaskHandler_Webhook5xxDoesNotGetSkipRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // 500
	}))
	defer server.Close()

	handler := NewPublishedTaskHandler(server.URL)

	task, err := tasks.NewArticlePublishedTask("article-1")
	require.NoError(t, err)

	err = handler(context.Background(), task)
	require.Error(t, err)
	assert.False(t, errors.Is(err, asynq.SkipRetry), "expected error to NOT wrap asynq.SkipRetry for 5xx")
}
