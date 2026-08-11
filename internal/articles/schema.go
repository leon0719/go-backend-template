package articles

type CreateArticleRequest struct {
	Title string `json:"title" validate:"required"`
	Body  string `json:"body"`
}

type UpdateArticleRequest struct {
	Title *string `json:"title"`
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
