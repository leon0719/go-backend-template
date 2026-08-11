package articles

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/accounts"
	"go-backend-template/internal/httpserver/request"
)

func setupArticlesRouter(t *testing.T) (chi.Router, string) {
	svc := NewService(newFakeArticlesRepo(), func(t *asynq.Task) error { return nil })
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	userID := uuid.New()
	token, err := accounts.NewAccessToken("secret", userID, 15*time.Minute)
	require.NoError(t, err)
	return r, token
}

func createArticle(t *testing.T, r chi.Router, token string) ArticleResponse {
	t.Helper()
	body, _ := json.Marshal(CreateArticleRequest{Title: "Hello", Body: "World"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created ArticleResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	return created
}

func TestHandler_CreateAndGetArticle(t *testing.T) {
	r, token := setupArticlesRouter(t)

	created := createArticle(t, r, token)
	assert.Equal(t, "Hello", created.Title)

	getReq := httptest.NewRequest(http.MethodGet, "/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	assert.Equal(t, http.StatusOK, getRec.Code)
}

// TestHandler_GetArticle_NotOwned_Returns404 is the IDOR-protection test: an
// article created by user A must not be readable by user B, even when B
// authenticates correctly with a token signed by the same secret. Both users
// go through the SAME router instance so this genuinely exercises ownership
// scoping end-to-end rather than just asserting nothing.
func TestHandler_GetArticle_NotOwned_Returns404(t *testing.T) {
	svc := NewService(newFakeArticlesRepo(), func(t *asynq.Task) error { return nil })
	r := chi.NewRouter()
	RegisterRoutes(r, svc, "secret", nil)

	userA := uuid.New()
	tokenA, err := accounts.NewAccessToken("secret", userA, 15*time.Minute)
	require.NoError(t, err)

	userB := uuid.New()
	tokenB, err := accounts.NewAccessToken("secret", userB, 15*time.Minute)
	require.NoError(t, err)

	created := createArticle(t, r, tokenA)

	getReq := httptest.NewRequest(http.MethodGet, "/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tokenB)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestHandler_ListArticles_Pagination(t *testing.T) {
	r, token := setupArticlesRouter(t)

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(CreateArticleRequest{Title: "T", Body: "B"})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var list ListArticlesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, int64(3), list.Total)
}

// TestHandler_ListArticles_ClampsInvalidPagination guards against Service.List
// receiving a page <= 0 (which would produce a negative SQL OFFSET and a raw
// 500 from Postgres in production). page=0 and page_size=-5 must be clamped
// to sane defaults rather than passed through.
func TestHandler_ListArticles_ClampsInvalidPagination(t *testing.T) {
	r, token := setupArticlesRouter(t)

	createArticle(t, r, token)

	req := httptest.NewRequest(http.MethodGet, "/?page=0&page_size=-5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var list ListArticlesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Equal(t, int32(1), list.Page)
	assert.Equal(t, int32(defaultPageSize), list.PageSize)
}

func TestHandler_PublishArticle(t *testing.T) {
	r, token := setupArticlesRouter(t)

	created := createArticle(t, r, token)

	pubReq := httptest.NewRequest(http.MethodPost, "/"+created.ID+"/publish", nil)
	pubReq.Header.Set("Authorization", "Bearer "+token)
	pubRec := httptest.NewRecorder()
	r.ServeHTTP(pubRec, pubReq)

	require.Equal(t, http.StatusOK, pubRec.Code)
	var published ArticleResponse
	require.NoError(t, json.Unmarshal(pubRec.Body.Bytes(), &published))
	assert.Equal(t, "published", published.Status)
}

// TestHandler_UpdateArticle_RejectsBlankTitle guards the asymmetry that used
// to exist between POST and PATCH: create required a non-empty title, but
// update skipped validation entirely and happily blanked it.
func TestHandler_UpdateArticle_RejectsBlankTitle(t *testing.T) {
	r, token := setupArticlesRouter(t)
	created := createArticle(t, r, token)

	blank := ""
	body, _ := json.Marshal(UpdateArticleRequest{Title: &blank})
	req := httptest.NewRequest(http.MethodPatch, "/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandler_UpdateArticle_OmittedTitleIsAllowed(t *testing.T) {
	r, token := setupArticlesRouter(t)
	created := createArticle(t, r, token)

	newBody := "updated"
	body, _ := json.Marshal(UpdateArticleRequest{Body: &newBody})
	req := httptest.NewRequest(http.MethodPatch, "/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var updated ArticleResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Hello", updated.Title)
	assert.Equal(t, "updated", updated.Body)
}

// TestHandler_CreateArticle_RejectsOversizedBody covers the shared
// MaxBytesReader limit in internal/httpserver/request.
func TestHandler_CreateArticle_RejectsOversizedBody(t *testing.T) {
	r, token := setupArticlesRouter(t)

	huge := bytes.Repeat([]byte("a"), int(request.MaxBodyBytes)+1024)
	payload := append(append([]byte(`{"title":"x","body":"`), huge...), []byte(`"}`)...)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// TestHandler_ListArticles_HugePageDoesNotOverflow guards the int32 OFFSET
// overflow: (page-1)*page_size used to wrap negative and 500.
func TestHandler_ListArticles_HugePageDoesNotOverflow(t *testing.T) {
	r, token := setupArticlesRouter(t)
	createArticle(t, r, token)

	req := httptest.NewRequest(http.MethodGet, "/?page=100000000&page_size=100", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp ListArticlesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Items)
}
