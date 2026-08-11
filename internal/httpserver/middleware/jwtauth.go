package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"go-backend-template/internal/httpserver/respond"
	"go-backend-template/internal/jwtutil"
)

type userCtxKey string

const userIDKey userCtxKey = "user_id"

func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized, "missing or malformed Authorization header")
				return
			}

			userID, err := jwtutil.ParseAccessToken(secret, parts[1])
			if err != nil {
				respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}
