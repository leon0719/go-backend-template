package middleware

import (
	"net/http"
	"strings"
)

// corsAllowedMethods / corsAllowedHeaders describe what this API accepts.
// Widen them if you add endpoints using other methods or custom headers.
const (
	corsAllowedMethods = "GET, POST, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type, X-Request-ID"
	corsExposedHeaders = "X-Request-ID"
	corsMaxAge         = "600"
)

// CORS emits cross-origin response headers for an explicit allow-list of
// origins (config: CORS_ALLOWED_ORIGINS).
//
// With an empty allow-list — the template's default — CORS is DISABLED: no
// headers are emitted at all, so browsers apply the same-origin policy. This
// is the safe default; a template must not ship an open API. Credentials
// (cookies/Authorization) are allowed only for explicitly listed origins, and
// "*" is deliberately not special-cased, since `Access-Control-Allow-Origin: *`
// combined with credentials is rejected by browsers anyway.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				// Unknown origin: emit nothing and let the browser block it.
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Expose-Headers", corsExposedHeaders)
			// The response varies by Origin, so caches must not share it.
			h.Add("Vary", "Origin")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", corsAllowedMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				h.Set("Access-Control-Max-Age", corsMaxAge)
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
