package accounts

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/httpserver/request"
	"go-backend-template/internal/httpserver/respond"
)

// authRateLimitKey derives a per-client-IP rate-limit key from the request.
//
// It uses middleware.ClientIP rather than r.RemoteAddr directly: behind the
// bundled Caddy reverse proxy RemoteAddr is always the proxy's container IP,
// which would collapse every internet client into one shared bucket. See
// middleware.RealIP for the X-Forwarded-For trust contract (TRUSTED_PROXIES).
func authRateLimitKey(r *http.Request) string {
	return "auth:" + middleware.ClientIP(r)
}

func RegisterRoutes(r chi.Router, svc *Service, jwtSecret string, rl *middleware.RateLimiter) {
	h := &handler{svc: svc}

	// middleware.RateLimit degrades to a no-op on a nil limiter, so these are
	// declared once regardless of whether Redis is wired up.
	r.Group(func(lr chi.Router) {
		lr.Use(middleware.RateLimit(rl, authRateLimitKey))
		lr.Post("/register", h.register)
		lr.Post("/login", h.login)
	})

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

// register godoc
// @Summary      Register a new user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body RegisterRequest true "Registration payload"
// @Success      201 {object} TokenResponse
// @Failure      422 {object} respond.ErrorResponse
// @Failure      409 {object} respond.ErrorResponse
// @Router       /auth/register [post]
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !request.DecodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if respond.MapError(w, err, respond.ErrMapping{
		Target: ErrEmailTaken, Status: http.StatusConflict, Code: respond.CodeConflict, Message: "email already registered",
	}) {
		return
	}
	respond.JSON(w, http.StatusCreated, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

// login godoc
// @Summary      Log in with email and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body LoginRequest true "Login payload"
// @Success      200 {object} TokenResponse
// @Failure      422 {object} respond.ErrorResponse
// @Failure      401 {object} respond.ErrorResponse
// @Router       /auth/login [post]
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !request.DecodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if respond.MapError(w, err, respond.ErrMapping{
		Target: ErrInvalidCredentials, Status: http.StatusUnauthorized, Code: respond.CodeUnauthorized, Message: "invalid email or password",
	}) {
		return
	}
	respond.JSON(w, http.StatusOK, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

// refresh godoc
// @Summary      Exchange a refresh token for a new token pair
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body RefreshRequest true "Refresh payload"
// @Success      200 {object} TokenResponse
// @Failure      422 {object} respond.ErrorResponse
// @Failure      401 {object} respond.ErrorResponse
// @Router       /auth/refresh [post]
func (h *handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if !request.DecodeAndValidate(w, r, &req) {
		return
	}

	access, refresh, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if respond.MapError(w, err, respond.ErrMapping{
		Target: ErrInvalidRefreshToken, Status: http.StatusUnauthorized, Code: respond.CodeUnauthorized, Message: "invalid or expired refresh token",
	}) {
		return
	}
	respond.JSON(w, http.StatusOK, TokenResponse{AccessToken: access, RefreshToken: refresh})
}

// logout godoc
// @Summary      Log out the current user
// @Tags         auth
// @Security     BearerAuth
// @Success      204 "No Content"
// @Failure      500 {object} respond.ErrorResponse
// @Router       /auth/logout [post]
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	if respond.MapError(w, h.svc.Logout(r.Context(), userID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// me godoc
// @Summary      Get the current authenticated user
// @Tags         auth
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} UserResponse
// @Failure      404 {object} respond.ErrorResponse
// @Router       /auth/me [get]
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	user, err := h.svc.Me(r.Context(), userID)
	if respond.MapError(w, err, respond.ErrMapping{
		Target: ErrNotFound, Status: http.StatusNotFound, Code: respond.CodeNotFound, Message: "user not found",
	}) {
		return
	}
	respond.JSON(w, http.StatusOK, UserResponse{ID: user.ID.String(), Email: user.Email})
}
