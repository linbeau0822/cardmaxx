-- name: UpsertTransaction :one
-- Plaid's sync feed reports the same transaction again when it is modified
-- (e.g. a pending charge posting), so re-running a sync updates the existing
-- row instead of erroring on the unique plaid_transaction_id.
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
ON CONFLICT (plaid_transaction_id) DO UPDATE SET
    category_id             = EXCLUDED.category_id,
    amount_cents            = EXCLUDED.amount_cents,
    date                    = EXCLUDED.date,
    name                    = EXCLUDED.name,
    merchant_name           = EXCLUDED.merchant_name,
    pending                 = EXCLUDED.pending,
    pending_transaction_id  = EXCLUDED.pending_transaction_id
RETURNING *;

-- name: DeleteTransactionByPlaidID :exec
DELETE FROM transactions WHERE plaid_transaction_id = @plaid_transaction_id;

-- name: ListTransactionsByUser :many
-- Transactions are owned transitively: transaction -> account -> plaid_item -> user.
-- Every read is scoped by user so no query can leak another user's spend.
-- category_name is joined in so callers can display it without a second
-- lookup against the categories table.
SELECT t.*, c.name AS category_name
FROM transactions t
JOIN accounts a ON a.id = t.account_id
JOIN plaid_items i ON i.id = a.plaid_item_id
LEFT JOIN categories c ON c.id = t.category_id
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
