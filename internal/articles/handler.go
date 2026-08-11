package articles

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"go-backend-template/internal/db/sqlc"
	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/httpserver/respond"
)

var articleValidate = validator.New()

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// writeRateLimitKey derives a per-user rate-limit key for write operations on
// articles. These routes sit behind JWTAuth, so the user ID is always present
// in the request context by the time this runs.
func writeRateLimitKey(r *http.Request) string {
	userID, _ := middleware.UserIDFromContext(r.Context())
	return "articles:write:" + userID.String()
}

func RegisterRoutes(r chi.Router, svc *Service, jwtSecret string, writeRateLimit *middleware.RateLimiter) {
	h := &handler{svc: svc}

	r.Use(middleware.JWTAuth(jwtSecret))

	r.Get("/", h.list)
	r.Get("/{id}", h.get)

	if writeRateLimit != nil {
		r.With(middleware.RateLimit(writeRateLimit, writeRateLimitKey)).Post("/", h.create)
		r.With(middleware.RateLimit(writeRateLimit, writeRateLimitKey)).Patch("/{id}", h.update)
		r.With(middleware.RateLimit(writeRateLimit, writeRateLimitKey)).Delete("/{id}", h.delete)
		r.With(middleware.RateLimit(writeRateLimit, writeRateLimitKey)).Post("/{id}/publish", h.publish)
	} else {
		r.Post("/", h.create)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/publish", h.publish)
	}
}

type handler struct {
	svc *Service
}

func toResponse(a sqlc.Article) ArticleResponse {
	return ArticleResponse{ID: a.ID.String(), Title: a.Title, Body: a.Body, Status: a.Status}
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	var req CreateArticleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, "invalid JSON body")
		return
	}
	if err := articleValidate.Struct(req); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, err.Error())
		return
	}

	a, err := h.svc.Create(r.Context(), userID, req.Title, req.Body)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "create failed")
		return
	}
	respond.JSON(w, http.StatusCreated, toResponse(a))
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	a, err := h.svc.Get(r.Context(), id, userID)
	if errors.Is(err, ErrNotFound) {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "get failed")
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}

// clampInt parses a query parameter as an integer, falling back to
// defaultVal on parse failure or when the value is below min. Page/pageSize
// must never be allowed to reach the service as <= 0, since the service
// computes a raw SQL OFFSET from them without validation.
func clampInt(raw string, defaultVal, min, max int32) int32 {
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	n := int32(v)
	if n < min {
		return defaultVal
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	page := clampInt(r.URL.Query().Get("page"), 1, 1, 0)
	pageSize := clampInt(r.URL.Query().Get("page_size"), defaultPageSize, 1, maxPageSize)

	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	items, total, err := h.svc.List(r.Context(), userID, status, q, page, pageSize)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "list failed")
		return
	}

	resp := ListArticlesResponse{Items: make([]ArticleResponse, 0, len(items)), Total: total, Page: page, PageSize: pageSize}
	for _, a := range items {
		resp.Items = append(resp.Items, toResponse(a))
	}
	respond.JSON(w, http.StatusOK, resp)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req UpdateArticleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, respond.CodeValidation, "invalid JSON body")
		return
	}

	a, err := h.svc.Update(r.Context(), id, userID, req.Title, req.Body)
	if errors.Is(err, ErrNotFound) {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "update failed")
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	err := h.svc.Delete(r.Context(), id, userID)
	if errors.Is(err, ErrNotFound) {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) publish(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	a, err := h.svc.Publish(r.Context(), id, userID)
	if errors.Is(err, ErrNotFound) {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "publish failed")
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}
