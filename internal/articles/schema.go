package articles

type CreateArticleRequest struct {
	Title string `json:"title" validate:"required"`
	Body  string `json:"body"`
}

// UpdateArticleRequest is a partial update: a nil field means "leave
// unchanged". `omitempty` therefore skips validation when the field is
// absent, but `min=1` still applies when it IS supplied -- otherwise PATCH
// could blank a title that POST refuses to create empty.
type UpdateArticleRequest struct {
	Title *string `json:"title" validate:"omitempty,min=1"`
	Body  *string `json:"body"`
}

type ArticleResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

type ListArticlesResponse struct {
	Items    []ArticleResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int32             `json:"page"`
	PageSize int32             `json:"page_size"`
}
