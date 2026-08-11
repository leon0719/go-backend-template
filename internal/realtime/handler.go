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
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, word := range strings.Fields(demoResponse) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
		fmt.Fprintf(w, "data: %s\n\n", word)
		flusher.Flush()
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}
