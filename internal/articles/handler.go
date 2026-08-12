package articles

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"go-backend-template/internal/db/sqlc"
	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/httpserver/request"
	"go-backend-template/internal/httpserver/respond"
)

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

	// middleware.RateLimit degrades to a no-op on a nil limiter, so the write
	// routes are declared once regardless of whether Redis is wired up.
	r.Group(func(wr chi.Router) {
		wr.Use(middleware.RateLimit(writeRateLimit, writeRateLimitKey))
		wr.Post("/", h.create)
		wr.Patch("/{id}", h.update)
		wr.Delete("/{id}", h.delete)
		wr.Post("/{id}/publish", h.publish)
	})
}

// notFoundErr maps ErrNotFound to the 404 every handler below returns for
// it. Not-found and not-owned are deliberately indistinguishable (see
// service.go), so this single mapping covers both cases.
var notFoundErr = respond.ErrMapping{
	Target:  ErrNotFound,
	Status:  http.StatusNotFound,
	Code:    respond.CodeNotFound,
	Message: "article not found",
}

type handler struct {
	svc *Service
}

func toResponse(a sqlc.Article) ArticleResponse {
	return ArticleResponse{ID: a.ID.String(), Title: a.Title, Body: a.Body, Summary: a.Summary, Status: a.Status}
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusNotFound, respond.CodeNotFound, "article not found")
		return uuid.Nil, false
	}
	return id, true
}

// create godoc
// @Summary      Create an article
// @Tags         articles
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body CreateArticleRequest true "Article payload"
// @Success      201 {object} ArticleResponse
// @Failure      422 {object} respond.ErrorResponse
// @Router       /articles [post]
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	var req CreateArticleRequest
	if !request.DecodeAndValidate(w, r, &req) {
		return
	}

	a, err := h.svc.Create(r.Context(), userID, req.Title, req.Body, req.Summary)
	if respond.MapError(w, err) {
		return
	}
	respond.JSON(w, http.StatusCreated, toResponse(a))
}

// get godoc
// @Summary      Get an article by ID
// @Tags         articles
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "Article ID"
// @Success      200 {object} ArticleResponse
// @Failure      404 {object} respond.ErrorResponse
// @Router       /articles/{id} [get]
func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	a, err := h.svc.Get(r.Context(), id, userID)
	if respond.MapError(w, err, notFoundErr) {
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}

// clampInt parses a query parameter as an integer, falling back to
// defaultVal on parse failure or when the value is below min. Page/pageSize
// must never be allowed to reach the service as <= 0, since the service
// computes a raw SQL OFFSET from them without validation.
func clampInt(raw string, defaultVal, min, max int32) int32 {
	// bitSize 32 makes an out-of-int32-range value an error rather than a
	// silent wrap: strconv.Atoi + int32(...) would turn 2147483648 into a
	// negative page that sails past the min check below.
	v, err := strconv.ParseInt(raw, 10, 32)
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

// list godoc
// @Summary      List the caller's articles
// @Tags         articles
// @Security     BearerAuth
// @Produce      json
// @Param        page query int false "Page number"
// @Param        page_size query int false "Page size (max 100)"
// @Param        status query string false "Filter by status"
// @Param        q query string false "Search title"
// @Success      200 {object} ListArticlesResponse
// @Router       /articles [get]
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	page := clampInt(r.URL.Query().Get("page"), 1, 1, 0)
	pageSize := clampInt(r.URL.Query().Get("page_size"), defaultPageSize, 1, maxPageSize)

	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	items, total, err := h.svc.List(r.Context(), userID, status, q, page, pageSize)
	if respond.MapError(w, err) {
		return
	}

	resp := ListArticlesResponse{Items: make([]ArticleResponse, 0, len(items)), Total: total, Page: page, PageSize: pageSize}
	for _, a := range items {
		resp.Items = append(resp.Items, toResponse(a))
	}
	respond.JSON(w, http.StatusOK, resp)
}

// update godoc
// @Summary      Update an article
// @Tags         articles
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id path string true "Article ID"
// @Param        body body UpdateArticleRequest true "Article payload"
// @Success      200 {object} ArticleResponse
// @Failure      422 {object} respond.ErrorResponse
// @Failure      404 {object} respond.ErrorResponse
// @Router       /articles/{id} [patch]
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req UpdateArticleRequest
	if !request.DecodeAndValidate(w, r, &req) {
		return
	}

	a, err := h.svc.Update(r.Context(), id, userID, req.Title, req.Body, req.Summary)
	if respond.MapError(w, err, notFoundErr) {
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}

// delete godoc
// @Summary      Delete an article
// @Tags         articles
// @Security     BearerAuth
// @Param        id path string true "Article ID"
// @Success      204 "No Content"
// @Failure      404 {object} respond.ErrorResponse
// @Router       /articles/{id} [delete]
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	err := h.svc.Delete(r.Context(), id, userID)
	if respond.MapError(w, err, notFoundErr) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publish godoc
// @Summary      Publish an article
// @Tags         articles
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "Article ID"
// @Success      200 {object} ArticleResponse
// @Failure      404 {object} respond.ErrorResponse
// @Router       /articles/{id}/publish [post]
func (h *handler) publish(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	a, err := h.svc.Publish(r.Context(), id, userID)
	if respond.MapError(w, err, notFoundErr) {
		return
	}
	respond.JSON(w, http.StatusOK, toResponse(a))
}
