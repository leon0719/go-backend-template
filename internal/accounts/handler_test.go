package accounts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_RegisterAndMe(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	body, _ := json.Marshal(RegisterRequest{Email: "a@example.com", Password: "password123"})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var tokens TokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokens))
	assert.NotEmpty(t, tokens.AccessToken)

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	meRec := httptest.NewRecorder()
	r.ServeHTTP(meRec, meReq)

	require.Equal(t, http.StatusOK, meRec.Code)
	var user UserResponse
	require.NoError(t, json.Unmarshal(meRec.Body.Bytes(), &user))
	assert.Equal(t, "a@example.com", user.Email)
}

func TestHandler_Register_InvalidBody_Returns422(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	body, _ := json.Marshal(RegisterRequest{Email: "not-an-email", Password: "short"})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandler_Login_WrongPassword_Returns401(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	regBody, _ := json.Marshal(RegisterRequest{Email: "a@example.com", Password: "password123"})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(regBody)))

	loginBody, _ := json.Marshal(LoginRequest{Email: "a@example.com", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_Me_NoToken_Returns401(t *testing.T) {
	svc := NewService(newFakeRepo(), "secret")
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
