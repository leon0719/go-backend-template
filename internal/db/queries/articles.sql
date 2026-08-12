-- name: CreateArticle :one
INSERT INTO articles (user_id, title, body, summary)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetOwnedArticle :one
SELECT * FROM articles WHERE id = $1 AND user_id = $2;

-- name: ListOwnedArticles :many
SELECT * FROM articles
WHERE user_id = $1
  AND ($2::text = '' OR status = $2)
  AND ($3::text = '' OR title ILIKE '%' || $3 || '%')
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: CountOwnedArticles :one
SELECT count(*) FROM articles
WHERE user_id = $1
  AND ($2::text = '' OR status = $2)
  AND ($3::text = '' OR title ILIKE '%' || $3 || '%');

-- name: UpdateArticle :one
UPDATE articles
SET title = coalesce(sqlc.narg('title'), title),
    body = coalesce(sqlc.narg('body'), body),
    summary = coalesce(sqlc.narg('summary'), summary),
    updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteArticle :execrows
DELETE FROM articles WHERE id = $1 AND user_id = $2;

-- name: PublishArticleIfDraft :execrows
UPDATE articles SET status = 'published', updated_at = now()
WHERE id = $1 AND user_id = $2 AND status = 'draft';

-- name: ArchiveArticle :execrows
UPDATE articles SET status = 'archived', updated_at = now()
WHERE id = $1 AND user_id = $2 AND status != 'archived';

-- name: CreateArticleEvent :exec
INSERT INTO article_events (article_id, event_type) VALUES ($1, $2);
