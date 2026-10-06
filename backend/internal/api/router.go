// Package api is the HTTP layer: routing, middleware and JSON responses. It is
// the only package that knows about net/http.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
)

// requestTimeout bounds how long a handler may run. It sits inside the
// server's WriteTimeout so a slow handler is cancelled (and its database
// queries with it) before the connection is torn down.
const requestTimeout = 25 * time.Second

// NewRouter builds the application's HTTP handler.
//
// Handlers take no dependencies: /health needs none and every other route is a
// 501 placeholder. Dependencies get a parameter here when a handler first
// needs one.
func NewRouter(cfg config.ServerConfig, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// Order matters: RequestID must run before the logger so every line is
	// correlatable, and Recoverer must wrap the handlers it protects.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(cors(cfg.AllowedOrigins))
	r.Use(middleware.Timeout(requestTimeout))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, CodeNotFound, "no route matches "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed,
			r.Method+" is not allowed on "+r.URL.Path)
	})

	// Liveness check. Deliberately outside /api/v1 and dependency-free so it
	// answers even when PostgreSQL or Plaid is unavailable.
	r.Get("/health", handleHealth)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/plaid", func(r chi.Router) {
			r.Post("/link-token", notImplemented("phase 2 (Plaid integration)"))
			r.Post("/exchange-token", notImplemented("phase 2 (Plaid integration)"))
			r.Post("/sync", notImplemented("phase 2 (Plaid integration)"))
		})

		r.Get("/accounts", notImplemented("phase 2 (Plaid integration)"))
		r.Get("/transactions", notImplemented("phase 2 (Plaid integration)"))
		r.Get("/cards", notImplemented("phase 4 (reward engine)"))
		r.Get("/analysis/spending", notImplemented("phase 3 (spending analysis)"))
		r.Get("/recommendations", notImplemented("phase 5 (recommendations)"))
	})

	return r
}
