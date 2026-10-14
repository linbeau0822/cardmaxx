-- name: GetOrCreateCategoryByName :one
-- Plaid reports categories as a free-text string per transaction; this maps
-- that string onto the shared categories vocabulary, creating the row the
-- first time a given name is seen. The no-op DO UPDATE makes RETURNING fire
-- on a conflict too, which plain DO NOTHING would not.
INSERT INTO categories (name) VALUES (@name)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
RETURNING *;
