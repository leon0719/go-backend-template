package realtime

import (
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
