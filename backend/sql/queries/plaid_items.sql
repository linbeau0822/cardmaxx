-- name: CreatePlaidItem :one
INSERT INTO plaid_items (
    user_id,
    plaid_item_id,
    access_token
) VALUES (
    @user_id,
    @plaid_item_id,
    @access_token
)
RETURNING *;

-- name: ListPlaidItemsByUser :many
SELECT * FROM plaid_items WHERE user_id = @user_id ORDER BY created_at;

-- name: UpdatePlaidItemCursor :exec
-- Called after each page of /transactions/sync so a later page's failure
-- does not re-fetch transactions the previous page already persisted.
UPDATE plaid_items SET transaction_cursor = @transaction_cursor WHERE id = @id;
