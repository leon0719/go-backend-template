package realtime

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"go-backend-template/internal/httpserver/middleware"
	"go-backend-template/internal/httpserver/respond"
)

const demoResponse = "Hello from the SSE demo endpoint."

const (
	// tokenDelay fakes an LLM emitting one token at a time.
	tokenDelay = 10 * time.Millisecond

	// heartbeatInterval bounds how long the stream can stay silent. Idle
	// proxies (nginx, Cloudflare, load balancers) drop connections with no
	// traffic, so a periodic SSE comment frame (":\n\n" — ignored by
	// EventSource clients) keeps long-lived streams alive.
	heartbeatInterval = 15 * time.Second
)

func RegisterRoutes(r chi.Router, jwtSecret string) {
	r.Group(func(pr chi.Router) {
		pr.Use(middleware.JWTAuth(jwtSecret))
		pr.Get("/sse", streamDemo)
	})
}

// streamDemo godoc
// @Summary      Stream a demo Server-Sent Events response
// @Tags         realtime
// @Security     BearerAuth
// @Produce      text/event-stream
// @Success      200 {string} string "text/event-stream stream of `data: <word>` lines"
// @Failure      500 {object} respond.ErrorResponse
// @Router       /realtime/sse [get]
func streamDemo(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// Deliberately NO "Connection: keep-alive": it is a hop-by-hop header
	// that net/http manages itself on HTTP/1.1, and it is outright illegal
	// under HTTP/2 (Go strips it, other stacks may reject the response).
	//
	// Also disable proxy buffering, which would otherwise defeat streaming.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	tokens := strings.Fields(demoResponse)
	next := time.NewTimer(tokenDelay)
	defer next.Stop()

	for i := 0; i < len(tokens); {
		select {
		case <-r.Context().Done():
			// Client disconnected (or the server is shutting down): stop
			// immediately rather than writing into a dead connection.
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-next.C:
			fmt.Fprintf(w, "data: %s\n\n", tokens[i])
			flusher.Flush()
			i++
			next.Reset(tokenDelay)
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}
