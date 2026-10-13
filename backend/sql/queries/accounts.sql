-- name: ListAccountsByUser :many
SELECT a.*
FROM accounts a
JOIN plaid_items i ON i.id = a.plaid_item_id
WHERE i.user_id = @user_id
ORDER BY a.name;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = @id;

-- name: GetAccountByPlaidAccountID :one
SELECT * FROM accounts WHERE plaid_account_id = @plaid_account_id;

-- name: UpsertAccount :one
-- Account metadata rarely changes, but a sync is the only time CardWise
-- hears about it again, so each sync re-asserts the latest copy.
INSERT INTO accounts (
    plaid_item_id,
    plaid_account_id,
    name,
    official_name,
    type,
    subtype,
    mask
) VALUES (
    @plaid_item_id,
    @plaid_account_id,
    @name,
    @official_name,
    @type,
    @subtype,
    @mask
)
ON CONFLICT (plaid_account_id) DO UPDATE SET
    name          = EXCLUDED.name,
    official_name = EXCLUDED.official_name,
    type          = EXCLUDED.type,
    subtype       = EXCLUDED.subtype,
    mask          = EXCLUDED.mask
RETURNING *;
