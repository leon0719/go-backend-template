package middleware

import (
	"errors"
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
// INSIDE SlogLogger — that is, registered after it, since chi runs the
// first-registered middleware outermost. Only then does the 500 written here
// travel back out through SlogLogger's status-capturing writer and get an
// access-log line; with the two the other way round, panicking requests were
// logged as a panic but never appeared in the access log at all.
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
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
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
