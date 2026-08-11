package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"go-backend-template/internal/httpserver/respond"
)

// Recoverer catches panics from downstream handlers, logs them at Error level
// with the request id and a stack trace, and replies with the standard JSON
// error envelope so a panic looks like every other 500 to API clients.
//
// It must be mounted after RequestID (so the request id is in context) and
// before SlogLogger (so SlogLogger observes the 500 status it writes).
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// http.ErrAbortHandler is the documented way for a handler to
			// abort a response without it being treated as an error; let it
			// propagate to net/http instead of logging/replacing it.
			if rec == http.ErrAbortHandler {
				panic(rec)
			}

			slog.ErrorContext(r.Context(), "panic recovered",
				"error", rec,
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", RequestIDFromContext(r.Context()),
				"stack", string(debug.Stack()),
			)
			respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "internal server error")
		}()

		next.ServeHTTP(w, r)
	})
}
