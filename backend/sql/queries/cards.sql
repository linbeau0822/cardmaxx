-- name: ListCreditCards :many
-- The credit-card catalogue is global reference data, not per-user.
SELECT * FROM credit_cards ORDER BY issuer, name;

-- name: GetCreditCard :one
SELECT * FROM credit_cards WHERE id = @id;

-- name: ListCardRewardCategories :many
-- Bonus-earning rules for a single card, highest rate first so the reward
-- engine can evaluate the most valuable rule first. Joined with categories
-- so callers get the category name, not just its id.
SELECT crc.*, cat.name AS category_name
FROM card_reward_categories crc
JOIN categories cat ON cat.id = crc.category_id
WHERE crc.credit_card_id = @credit_card_id
ORDER BY crc.reward_rate_bps DESC, cat.name;
