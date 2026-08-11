package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/httpserver/middleware"
)

// captureLogs swaps slog's default logger for one writing JSON into a buffer
// for the duration of fn, and returns the decoded records.
func captureLogs(t *testing.T, fn func()) []map[string]any {
	t.Helper()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	fn()

	var records []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		records = append(records, rec)
	}
	return records
}

func TestRecoverer_ReturnsStandardErrorEnvelope(t *testing.T) {
	h := middleware.RequestID(middleware.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})))

	rec := httptest.NewRecorder()
	records := captureLogs(t, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/kaboom", nil))
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "internal_error", body["error"]["code"])

	require.Len(t, records, 1)
	assert.Equal(t, "ERROR", records[0]["level"])
	assert.Equal(t, "panic recovered", records[0]["msg"])
	assert.NotEmpty(t, records[0]["request_id"])
	assert.NotEmpty(t, records[0]["stack"])
}

func TestRecoverer_PassesThroughNormalResponses(t *testing.T) {
	h := middleware.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)
}

func TestRecoverer_RepanicsOnErrAbortHandler(t *testing.T) {
	h := middleware.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	assert.PanicsWithError(t, http.ErrAbortHandler.Error(), func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestSlogLogger_LevelFollowsStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		level  string
	}{
		{http.StatusOK, "INFO"},
		{http.StatusNotFound, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
	} {
		h := middleware.RequestID(middleware.SlogLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
		})))

		records := captureLogs(t, func() {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/thing", nil))
		})

		require.Len(t, records, 1)
		assert.Equal(t, tc.level, records[0]["level"])
		assert.Equal(t, "http request", records[0]["msg"])
		assert.Equal(t, float64(tc.status), records[0]["status"])
		assert.Equal(t, "/thing", records[0]["path"])
		assert.NotEmpty(t, records[0]["request_id"])
	}
}

func TestSlogLogger_PreservesFlusher(t *testing.T) {
	// SSE handlers type-assert http.Flusher; the status-capturing wrapper
	// must not hide it.
	var sawFlusher bool
	h := middleware.SlogLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawFlusher = w.(http.Flusher)
	}))

	captureLogs(t, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background()))
	})
	assert.True(t, sawFlusher)
}
