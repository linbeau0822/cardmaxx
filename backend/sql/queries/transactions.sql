-- name: CreateTransaction :one
-- Inserts a transaction. Plaid's transaction ids are unique, so re-running a
-- sync that already persisted a row returns a unique-violation error rather
-- than silently duplicating spend. Phase 2 replaces this with an upsert that
-- also handles Plaid's "modified" and pending-to-posted transitions.
INSERT INTO transactions (
    plaid_transaction_id,
    account_id,
    category_id,
    amount_cents,
    date,
    name,
    merchant_name,
    pending,
    pending_transaction_id
) VALUES (
    @plaid_transaction_id,
    @account_id,
    @category_id,
    @amount_cents,
    @date,
    @name,
    @merchant_name,
    @pending,
    @pending_transaction_id
)
RETURNING *;

-- name: ListTransactionsByUser :many
-- Transactions are owned transitively: transaction -> account -> plaid_item -> user.
-- Every read is scoped by user so no query can leak another user's spend.
SELECT t.*
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
WHERE i.user_id = @user_id
ORDER BY t.date DESC, t.id
LIMIT @result_limit OFFSET @result_offset;

-- name: ListTransactionsByAccount :many
SELECT *
FROM transactions
WHERE account_id = @account_id
  AND date >= @start_date
  AND date <= @end_date
ORDER BY date DESC, id;
