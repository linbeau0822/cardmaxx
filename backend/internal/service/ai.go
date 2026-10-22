package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/linbeau0822/cardmaxx/backend/internal/ai"
)

// aiSystemPrompt enforces design principle 2 from the README: the model
// explains numbers CardMaxx already computed deterministically, it never
// invents or recomputes a reward, fee, or dollar figure of its own. It also
// translates the context's internal units (cents, basis points, upper-case
// category codes) into the plain-English form a user reads, since that's
// formatting the given number, not computing a new one.
const aiSystemPrompt = `You are the CardMaxx assistant. Answer the user's question about their ` +
	`spending and credit-card recommendations using ONLY the JSON context provided. ` +
	`Every dollar amount in that context was already computed deterministically by CardMaxx; ` +
	`never recompute, estimate, or invent a reward rate, fee, or dollar figure - only reference and explain ` +
	`the numbers you were given. If the context doesn't contain what's needed to answer, say so. Be concise. ` +
	`Format your answer for a normal reader, not a developer: ` +
	`amounts in the context are in cents, so convert to dollars (12345 -> "$123.45"); ` +
	`reward rates are in basis points, so convert to a percent (250 -> "2.5%") and state only the percent - ` +
	`never say "bps", "basis points", or "cents" anywhere in your answer; ` +
	`category names are upper-case codes (e.g. "TRANSPORTATION"), so present them in normal title case (e.g. "Transportation"). ` +
	`When asked why a card was recommended, ground your answer in "trailing_12_month_spend_by_category" and that ` +
	`card's "categories_won" in "card_recommendations": name the specific categories driving the recommendation ` +
	`and how much the user spent in them over the last 12 months, not just the card's overall rate or fee.`

// aiTopMerchantsLimit mirrors the dashboard's merchant leaderboard size
// (api.topMerchantsLimit) so the AI sees the same top merchants a user does.
const aiTopMerchantsLimit = 10

// AIService answers natural-language questions about a user's finances by
// handing an LLM the numbers PlaidService already computed and asking it to
// explain them - never to calculate anything itself.
type AIService struct {
	plaid  *PlaidService
	client *ai.Client
}

// NewAIService builds an AIService. client is nil-safe to construct but
// NewAIService should only be called once an OpenAI API key is configured;
// callers without one should simply not construct an AIService.
func NewAIService(plaidSvc *PlaidService, client *ai.Client) *AIService {
	return &AIService{plaid: plaidSvc, client: client}
}

// Ask answers a free-text question about the user's spending, card values,
// recommendations, or portfolio options, grounded in that already-computed
// data. It covers Phase 7's three use cases (recommendation explanations,
// spending questions, and explaining calculation results) with one endpoint:
// the question's own wording decides which of those it's asking about.
func (s *AIService) Ask(ctx context.Context, userID uuid.UUID, question string) (string, error) {
	contextJSON, err := s.buildPromptContext(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("build ai context: %w", err)
	}
	return s.client.Complete(ctx, aiSystemPrompt, "Context:\n"+contextJSON+"\n\nQuestion: "+question)
}

// buildPromptContext gathers the user's spending and card data as JSON, in the
// same shape the dashboard's own endpoints already return it.
func (s *AIService) buildPromptContext(ctx context.Context, userID uuid.UUID) (string, error) {
	months, err := s.plaid.SpendingByMonth(ctx, userID)
	if err != nil {
		return "", err
	}

	var selectedMonth time.Time
	if len(months) > 0 {
		selectedMonth = months[len(months)-1].Month
	}
	var categories any = []struct{}{}
	if !selectedMonth.IsZero() {
		categories, err = s.plaid.SpendingByCategoryForMonth(ctx, userID, selectedMonth)
		if err != nil {
			return "", err
		}
	}

	merchants, err := s.plaid.TopMerchants(ctx, userID, aiTopMerchantsLimit)
	if err != nil {
		return "", err
	}
	// The same trailing-12-month spend RecommendCards and OptimizePortfolio
	// compute their numbers from, so an explanation can cite the transactions
	// behind a recommendation rather than just its rate and fee.
	trailingYearSpend, err := s.plaid.trailingYearSpend(ctx, userID)
	if err != nil {
		return "", err
	}
	recommendations, err := s.plaid.RecommendCards(ctx, userID)
	if err != nil {
		return "", err
	}
	portfolioOptions, err := s.plaid.OptimizePortfolio(ctx, userID)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"spending_by_month":                   months,
		"spending_by_category_latest":         categories,
		"latest_month":                        selectedMonth,
		"top_merchants":                       merchants,
		"trailing_12_month_spend_by_category": trailingYearSpend,
		"card_recommendations":                recommendations,
		"portfolio_options_by_size":           portfolioOptions,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode context: %w", err)
	}
	return string(body), nil
}
