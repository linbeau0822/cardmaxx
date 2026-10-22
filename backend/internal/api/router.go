// Package api is the HTTP layer: routing, middleware and JSON responses. It is
// the only package that knows about net/http.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
	"github.com/linbeau0822/cardmaxx/backend/internal/service"
)

// requestTimeout bounds how long a handler may run. It sits inside the
// server's WriteTimeout so a slow handler is cancelled (and its database
// queries with it) before the connection is torn down.
const requestTimeout = 25 * time.Second

// NewRouter builds the application's HTTP handler.
//
// currentUserID stands in for authentication (Phase 8): every Phase 2 route
// acts on this one implicit user.
// aiSvc is nil when OPENAI_API_KEY isn't configured; handleAIAsk reports
// that as a 503 instead of the router omitting the route.
func NewRouter(cfg config.ServerConfig, logger *slog.Logger, plaidSvc *service.PlaidService, aiSvc *service.AIService, currentUserID uuid.UUID) http.Handler {
	srv := &Server{plaid: plaidSvc, ai: aiSvc, currentUserID: currentUserID, logger: logger}
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
			r.Post("/link-token", srv.handleCreateLinkToken)
			r.Post("/exchange-token", srv.handleExchangeToken)
			r.Post("/sync", srv.handleSync)
		})

		r.Get("/accounts", srv.handleListAccounts)
		r.Patch("/accounts/{id}", srv.handleLinkAccountCard)
		r.Get("/transactions", srv.handleListTransactions)
		r.Get("/cards", srv.handleListCards)
		r.Get("/cards/{id}/value", srv.handleCardValue)
		r.Get("/analysis/spending", srv.handleSpendingAnalysis)
		r.Get("/recommendations", srv.handleRecommendations)
		r.Get("/recommendations/portfolio", srv.handleOptimizePortfolio)

		r.Post("/ai/ask", srv.handleAIAsk)
	})

	return r
}
