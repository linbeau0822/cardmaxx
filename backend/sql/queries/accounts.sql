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
-- Account metadata rarely changes, but a sync is the only time CardMaxx
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

-- name: UpdateAccountCreditCard :one
-- Links (or, when @credit_card_id is NULL, unlinks) an account to a
-- catalogue card. Scoped to the owning user through plaid_items so a user
-- can only relink their own accounts; no matching row means either the
-- account doesn't exist or isn't theirs, and the caller treats both as 404.
UPDATE accounts a
SET credit_card_id = @credit_card_id
FROM plaid_items i
WHERE a.id = @id
  AND a.plaid_item_id = i.id
  AND i.user_id = @user_id
RETURNING a.id, a.plaid_item_id, a.credit_card_id, a.plaid_account_id, a.name, a.official_name, a.type, a.subtype, a.mask, a.created_at, a.updated_at;
