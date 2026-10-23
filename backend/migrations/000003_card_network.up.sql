-- The card network (Visa, Mastercard, American Express, Discover, ...) drives
-- which logo the frontend shows next to a card; it's independent of issuer
-- (e.g. Citi issues a Visa-network card).
ALTER TABLE credit_cards ADD COLUMN network TEXT NOT NULL DEFAULT 'Visa';
ALTER TABLE credit_cards ALTER COLUMN network DROP DEFAULT;

UPDATE credit_cards SET network = 'American Express' WHERE issuer = 'American Express';
