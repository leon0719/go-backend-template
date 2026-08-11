package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/config"
	"go-backend-template/internal/httpserver"
)

// newTestRouter builds the REAL assembled router (global middleware chain
// included) with nil service dependencies. Only routes that don't touch those
// dependencies are exercised here; the point of these tests is the chain
// itself, which every handler-level test bypasses.
func newTestRouter(t *testing.T, cfg *config.Config, extra func(chi.Router)) http.Handler {
	t.Helper()

	// Keep test output clean: the chain logs one record per request.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	return httpserver.NewRouter(httpserver.Deps{
		Config:      cfg,
		Logger:      logger,
		ExtraRoutes: extra,
	})
}

func baseConfig() *config.Config {
	cfg := &config.Config{JWTSecret: "test-secret"}
	cfg.Env = "test"
	return cfg
}

func TestRouter_PanicYieldsStandardErrorEnvelope(t *testing.T) {
	r := newTestRouter(t, baseConfig(), func(er chi.Router) {
		er.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "internal_error", body["error"]["code"])
}

func TestRouter_UnknownPathReturns404(t *testing.T) {
	r := newTestRouter(t, baseConfig(), nil)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRouter_EchoesSuppliedRequestID(t *testing.T) {
	r := newTestRouter(t, baseConfig(), nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("X-Request-ID", "req-abc-123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "req-abc-123", rec.Header().Get("X-Request-ID"))
}

func TestRouter_GeneratesRequestIDWhenAbsent(t *testing.T) {
	r := newTestRouter(t, baseConfig(), nil)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestRouter_CORSDisabledByDefault(t *testing.T) {
	r := newTestRouter(t, baseConfig(), nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestRouter_CORSEnabledForConfiguredOrigin(t *testing.T) {
	cfg := baseConfig()
	cfg.CORSAllowedOrigins = []string{"https://app.example.com"}
	r := newTestRouter(t, cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	// A different origin must not get the header.
	req2 := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req2.Header.Set("Origin", "https://evil.example")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	assert.Empty(t, rec2.Header().Get("Access-Control-Allow-Origin"))
}

func TestRouter_InvalidTrustedProxiesDegradesSafely(t *testing.T) {
	cfg := baseConfig()
	cfg.TrustedProxies = []string{"garbage"}

	// Must not panic, and X-Forwarded-For must be ignored (trust nothing).
	r := newTestRouter(t, cfg, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouter_AuthRoutesAreMounted(t *testing.T) {
	r := newTestRouter(t, baseConfig(), nil)

	// No Authorization header -> the JWT middleware rejects before any
	// (nil) service is touched, proving the route is wired.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/articles", nil))
	assert.Equal(t, http.StatusUnauthorized, rec2.Code)

	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/v1/realtime/sse", nil))
	assert.Equal(t, http.StatusUnauthorized, rec3.Code)
}
