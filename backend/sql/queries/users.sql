-- name: GetFirstUser :one
-- There is no authentication yet (Phase 8), so the application runs as a
-- single implicit user. This finds it.
SELECT * FROM users ORDER BY created_at LIMIT 1;

-- name: CreateUser :one
INSERT INTO users DEFAULT VALUES RETURNING *;
