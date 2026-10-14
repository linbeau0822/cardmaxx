-- name: GetSpendingByMonth :many
-- Totals spend per calendar month for the reward engine's purposes:
-- TRANSFER_IN, TRANSFER_OUT and LOAN_PAYMENTS are money movement, not card
-- purchases, and never earn rewards, so they're excluded here rather than in
-- every caller.
SELECT date_trunc('month', t.date)::DATE AS month, SUM(t.amount_cents)::BIGINT AS total_cents, COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
JOIN categories c ON c.id = t.category_id
WHERE i.user_id = @user_id
  AND c.name NOT IN ('TRANSFER_IN', 'TRANSFER_OUT', 'LOAN_PAYMENTS')
GROUP BY month
ORDER BY month;

-- name: GetAllCategoriesForUser :many
-- Every reward-eligible category the user has ever spent in, ordered by
-- all-time total so the chart's category order stays fixed across months
-- (months with no spend in a category still render it, at zero).
SELECT c.name AS category_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
JOIN categories c ON c.id = t.category_id
WHERE i.user_id = @user_id
  AND c.name NOT IN ('TRANSFER_IN', 'TRANSFER_OUT', 'LOAN_PAYMENTS')
GROUP BY c.name
ORDER BY SUM(t.amount_cents) DESC;

-- name: GetTopMerchants :many
-- All-time reward-eligible spend per merchant, ranked for the dashboard's
-- top-N list. merchant_name is null when Plaid couldn't resolve one, so
-- those fall back to the transaction's own name rather than being dropped.
SELECT COALESCE(t.merchant_name, t.name) AS merchant_name, SUM(t.amount_cents)::BIGINT AS total_cents, COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
JOIN categories c ON c.id = t.category_id
WHERE i.user_id = @user_id
  AND c.name NOT IN ('TRANSFER_IN', 'TRANSFER_OUT', 'LOAN_PAYMENTS')
GROUP BY COALESCE(t.merchant_name, t.name)
ORDER BY total_cents DESC
LIMIT @result_limit;

-- name: GetSpendingByCategoryForMonth :many
-- Same reward-eligible filter as GetSpendingByMonth, scoped to a single
-- calendar month for the bar chart's month drill-down.
SELECT c.name AS category_name, SUM(t.amount_cents)::BIGINT AS total_cents, COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
JOIN categories c ON c.id = t.category_id
WHERE i.user_id = @user_id
  AND c.name NOT IN ('TRANSFER_IN', 'TRANSFER_OUT', 'LOAN_PAYMENTS')
  AND date_trunc('month', t.date)::DATE = @month::DATE
GROUP BY c.name
ORDER BY total_cents DESC;
