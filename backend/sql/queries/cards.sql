-- name: ListCreditCards :many
-- The credit-card catalogue is global reference data, not per-user.
SELECT * FROM credit_cards ORDER BY issuer, name;

-- name: GetCreditCard :one
SELECT * FROM credit_cards WHERE id = @id;

-- name: ListCardRewardCategories :many
-- Bonus-earning rules for a single card, highest rate first so the reward
-- engine can evaluate the most valuable rule first.
SELECT *
FROM card_reward_categories
WHERE credit_card_id = @credit_card_id
ORDER BY reward_rate_bps DESC, category;
