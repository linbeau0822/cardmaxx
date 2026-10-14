package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
	"github.com/linbeau0822/cardmaxx/backend/internal/service"
)

// Server holds the dependencies Phase 2's handlers need.
//
// currentUserID stands in for authentication, which does not exist until
// Phase 8: CardMaxx runs as a single implicit user until then, and every
// handler scopes its queries to this id.
type Server struct {
	plaid         *service.PlaidService
	currentUserID uuid.UUID
	logger        *slog.Logger
}

// handleHealth reports that the process is up and serving.
//
// It intentionally does not check PostgreSQL: this is a liveness probe, and a
// dependency check belongs in a separate readiness endpoint so a transient
// database blip cannot get a healthy process restarted.
//
// TODO(phase-8): add GET /ready that pings the database pool.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// notImplemented builds a handler for a route whose contract exists but whose
// implementation lands in a later phase. Returning 501 keeps the API honest: a
// caller can tell "not built yet" apart from "built and returned no data".
func notImplemented(phase string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, CodeNotImplemented,
			"this endpoint is not implemented yet; planned for "+phase)
	}
}

// handleCreateLinkToken requests a Plaid Link token for the current user.
func (s *Server) handleCreateLinkToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.plaid.CreateLinkToken(r.Context(), s.currentUserID)
	if err != nil {
		s.writePlaidError(w, r, "create link token", err)
		return
	}
	writeJSON(w, http.StatusOK, token)
}

type exchangeTokenRequest struct {
	PublicToken string `json:"public_token"`
}

// handleExchangeToken trades a public token from Plaid Link for an access
// token and stores the resulting Item.
func (s *Server) handleExchangeToken(w http.ResponseWriter, r *http.Request) {
	var req exchangeTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "request body must be JSON with a public_token field")
		return
	}
	if req.PublicToken == "" {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "public_token is required")
		return
	}

	item, err := s.plaid.ExchangePublicToken(r.Context(), s.currentUserID, req.PublicToken)
	if err != nil {
		s.writePlaidError(w, r, "exchange public token", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"item_id": item.PlaidItemID})
}

// handleSync pulls new transaction activity for every Item the current user
// has linked.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	results, err := s.plaid.SyncAllForUser(r.Context(), s.currentUserID)
	if err != nil {
		s.writePlaidError(w, r, "sync transactions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

// handleListAccounts lists every account linked to the current user.
func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.plaid.ListAccounts(r.Context(), s.currentUserID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list accounts failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "failed to list accounts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

const (
	defaultTransactionsLimit = 50
	maxTransactionsLimit     = 200
)

// handleListTransactions lists a page of the current user's transactions,
// most recent first.
func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	limit, err := parseBoundedInt(r.URL.Query().Get("limit"), defaultTransactionsLimit, 1, maxTransactionsLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "limit must be an integer between 1 and "+strconv.Itoa(maxTransactionsLimit))
		return
	}
	offset, err := parseBoundedInt(r.URL.Query().Get("offset"), 0, 0, 1<<31-1)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "offset must be a non-negative integer")
		return
	}

	transactions, err := s.plaid.ListTransactions(r.Context(), s.currentUserID, int32(limit), int32(offset))
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list transactions failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "failed to list transactions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": transactions})
}

// monthSpending is a single month's totals, with the month formatted as
// "2006-01-02" so it round-trips as the `month` query param on this same
// endpoint. Serializing db.GetSpendingByMonthRow directly would instead emit
// time.Time's default RFC3339 ("2026-03-01T00:00:00Z"), which the frontend's
// month selector couldn't match back up against `selected_month` below.
type monthSpending struct {
	Month            string `json:"month"`
	TotalCents       int64  `json:"total_cents"`
	TransactionCount int64  `json:"transaction_count"`
}

// handleSpendingAnalysis totals the current user's reward-eligible spend by
// calendar month, plus the per-category breakdown for one selected month (the
// `month` query param, e.g. "2026-03-01"; defaults to the most recent month
// with activity).
func (s *Server) handleSpendingAnalysis(w http.ResponseWriter, r *http.Request) {
	months, err := s.plaid.SpendingByMonth(r.Context(), s.currentUserID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "spending by month failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "failed to compute spending by month")
		return
	}

	selectedMonth := r.URL.Query().Get("month")
	var month time.Time
	if selectedMonth != "" {
		month, err = time.Parse("2006-01-02", selectedMonth)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeBadRequest, "month must be formatted as YYYY-MM-DD")
			return
		}
	} else if len(months) > 0 {
		month = months[len(months)-1].Month
	}

	categories := []db.GetSpendingByCategoryForMonthRow{}
	if !month.IsZero() {
		categories, err = s.plaid.SpendingByCategoryForMonth(r.Context(), s.currentUserID, month)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "spending by category for month failed", slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, CodeInternal, "failed to compute spending by category")
			return
		}
	}

	allCategories, err := s.plaid.AllCategories(r.Context(), s.currentUserID)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "list all categories failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "failed to list categories")
		return
	}

	merchants, err := s.plaid.TopMerchants(r.Context(), s.currentUserID, topMerchantsLimit)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "top merchants failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, CodeInternal, "failed to compute top merchants")
		return
	}

	monthsResp := make([]monthSpending, len(months))
	for i, m := range months {
		monthsResp[i] = monthSpending{Month: m.Month.Format("2006-01-02"), TotalCents: m.TotalCents, TransactionCount: m.TransactionCount}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"categories":     categories,
		"all_categories": allCategories,
		"months":         monthsResp,
		"selected_month": month.Format("2006-01-02"),
		"top_merchants":  merchants,
	})
}

// topMerchantsLimit caps the merchant ranking to a top-N list; the dashboard
// shows a leaderboard, not a full export, so there's no pagination to size for.
const topMerchantsLimit = 10

// parseBoundedInt parses raw as an int, applying fallback when raw is empty
// and rejecting anything outside [min, max].
func parseBoundedInt(raw string, fallback, min, max int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, errors.New("out of range")
	}
	return n, nil
}

// writePlaidError reports a failed Plaid call. A Plaid APIError carries a
// message safe to surface to the caller (Plaid's own documented error
// messages); anything else is an internal failure whose detail stays in the
// logs.
func (s *Server) writePlaidError(w http.ResponseWriter, r *http.Request, action string, err error) {
	var apiErr *plaid.APIError
	if errors.As(err, &apiErr) {
		s.logger.WarnContext(r.Context(), "plaid rejected request",
			slog.String("action", action), slog.Any("error", apiErr))
		writeError(w, http.StatusBadGateway, CodeInternal, "plaid: "+apiErr.Message)
		return
	}
	s.logger.ErrorContext(r.Context(), action+" failed", slog.Any("error", err))
	writeError(w, http.StatusInternalServerError, CodeInternal, "failed to "+action)
}
