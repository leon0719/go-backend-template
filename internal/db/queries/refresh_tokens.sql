-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_digest, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRefreshTokenByDigest :one
SELECT * FROM refresh_tokens WHERE token_digest = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1;

-- name: RevokeAllRefreshTokensForUser :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
