-- name: ListAccountsByUser :many
SELECT a.*
FROM accounts a
JOIN plaid_items i ON i.id = a.plaid_item_id
WHERE i.user_id = @user_id
ORDER BY a.name;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = @id;
