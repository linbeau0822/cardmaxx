-- card_reward_categories rows cascade-delete with their credit_cards row.
-- Categories are shared vocabulary that real transactions may also
-- reference by now, so they are intentionally left in place.
DELETE FROM credit_cards WHERE (issuer, name) IN (
    ('Chase',             'Sapphire Preferred'),
    ('American Express',  'Gold Card'),
    ('Chase',             'Freedom Unlimited'),
    ('Citi',               'Costco Anywhere Visa'),
    ('Chase',             'Amazon Prime Visa')
);
