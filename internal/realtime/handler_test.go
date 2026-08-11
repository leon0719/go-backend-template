package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/jwtutil"
)

func TestSSE_StreamsTokensThenDone(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, "secret")

	userID := uuid.New()
	token, err := jwtutil.NewAccessToken("secret", userID, 15*time.Minute)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not finish in time")
	}

	body := rec.Body.String()
	assert.True(t, strings.Contains(body, "data: "))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]"))
}

func TestSSE_NoToken_Returns401(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, "secret")

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestSSE_ClientCancellationStopsStream covers mid-stream disconnect: the
// handler must return promptly on r.Context() cancellation instead of writing
// into a dead connection, and must not emit the terminal [DONE] frame.
func TestSSE_ClientCancellationStopsStream(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, "secret")

	token, err := jwtutil.NewAccessToken("secret", uuid.New(), 15*time.Minute)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	// Cancel before the first token is due, so the stream ends early.
	cancel()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after client cancellation")
	}

	assert.NotContains(t, rec.Body.String(), "[DONE]")
}

// The Connection header is hop-by-hop and illegal under HTTP/2; net/http
// manages connection reuse itself, so the handler must not set it.
func TestSSE_DoesNotSetHopByHopConnectionHeader(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, "secret")

	token, err := jwtutil.NewAccessToken("secret", uuid.New(), 15*time.Minute)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Connection"))
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
}
