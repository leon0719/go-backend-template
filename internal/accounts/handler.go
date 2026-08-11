package accounts

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/httpserver/respond"
)

var validate = validator.New()

// authRateLimitKey derives a per-client-IP rate-limit key from the request,
// preferring the parsed host portion of RemoteAddr and falling back to the
// raw value if it isn't in host:port form.
func authRateLimitKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "auth:" + host
}

func RegisterRoutes(r chi.Router, svc *Service, jwtSecret string, rl *middleware.RateLimiter) {
	h := &handler{svc: svc}

	if rl != nil {
		r.With(middleware.RateLimit(rl, authRateLimitKey)).Post("/register", h.register)
		r.With(middleware.RateLimit(rl, authRateLimitKey)).Post("/login", h.login)
	} else {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
	}

	r.Post("/refresh", h.refresh)

	r.Group(func(pr chi.Router) {
		pr.Use(middleware.JWTAuth(jwtSecret))
		pr.Post("/logout", h.logout)
		pr.Get("/me", h.me)
	})
}

type handler struct {
	svc *Service
}

func decodeAndValidate[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, "invalid JSON body")
		return false
	}
	if err := validate.Struct(dst); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, err.Error())
		return false
	}
	return true
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !decodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrEmailTaken) {
		respond.Error(w, http.StatusConflict, "email_taken", "email already registered")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "registration failed")
		return
	}
	respond.JSON(w, http.StatusCreated, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !decodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "login failed")
		return
	}
	respond.JSON(w, http.StatusOK, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

func (h *handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if !decodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if errors.Is(err, ErrInvalidRefreshToken) {
		respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized, "invalid or expired refresh token")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "refresh failed")
		return
	}
	respond.JSON(w, http.StatusOK, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	if err := h.svc.Logout(r.Context(), userID); err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	user, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "user not found")
		return
	}
	respond.JSON(w, http.StatusOK, UserResponse{ID: user.ID.String(), Email: user.Email})
}
