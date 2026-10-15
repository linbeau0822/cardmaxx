-- Seed the MVP card catalogue: five popular cards chosen so each covers a
-- distinct spending intent (travel, dining/groceries, flat everyday, gas,
-- online shopping) rather than two cards competing for the same niche.
--
-- Category names match Plaid's personal_finance_category.primary values
-- exactly (e.g. 'FOOD_AND_DRINK', 'TRAVEL') so these rows line up with the
-- categories GetOrCreateCategoryByName creates from real transactions.
-- Plaid's primary-level taxonomy doesn't separate groceries from general
-- retail, so Amex Gold's grocery bonus and Prime Visa's shopping bonus both
-- key off GENERAL_MERCHANDISE; that's a taxonomy limitation, not a bug here.

INSERT INTO categories (name) VALUES
    ('TRAVEL'),
    ('FOOD_AND_DRINK'),
    ('GENERAL_MERCHANDISE'),
    ('TRANSPORTATION')
ON CONFLICT (name) DO NOTHING;

INSERT INTO credit_cards (name, issuer, annual_fee_cents, base_reward_rate_bps) VALUES
    ('Sapphire Preferred',    'Chase',             9500, 100),
    ('Gold Card',             'American Express', 32500, 100),
    ('Freedom Unlimited',     'Chase',                0, 150),
    ('Costco Anywhere Visa',  'Citi',                 0, 100),
    ('Amazon Prime Visa',     'Chase',                0, 100)
ON CONFLICT (issuer, name) DO NOTHING;

-- reward_cap_cents of 2,500,000 on Amex Gold's grocery row is the card's
-- real $25,000/year bonus-spend cap; beyond it, spend earns the base rate.
INSERT INTO card_reward_categories (credit_card_id, category_id, reward_rate_bps, reward_cap_cents)
SELECT cc.id, cat.id, v.reward_rate_bps, v.reward_cap_cents
FROM (VALUES
    ('Chase',             'Sapphire Preferred',   'TRAVEL',               200, NULL::BIGINT),
    ('Chase',             'Sapphire Preferred',   'FOOD_AND_DRINK',       300, NULL::BIGINT),
    ('American Express',  'Gold Card',             'FOOD_AND_DRINK',       400, NULL::BIGINT),
    ('American Express',  'Gold Card',             'GENERAL_MERCHANDISE',  400, 2500000::BIGINT),
    ('Citi',               'Costco Anywhere Visa', 'TRANSPORTATION',       400, NULL::BIGINT),
    ('Chase',             'Amazon Prime Visa',     'GENERAL_MERCHANDISE',  500, NULL::BIGINT)
) AS v(issuer, card_name, category_name, reward_rate_bps, reward_cap_cents)
JOIN credit_cards cc ON cc.issuer = v.issuer AND cc.name = v.card_name
JOIN categories cat ON cat.name = v.category_name
ON CONFLICT (credit_card_id, category_id) DO NOTHING;
