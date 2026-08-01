-- name: CreatePlayer :one
INSERT INTO players (
  lobby_id,
  display_name,
  color,
  is_host,
  adult_confirmed,
  reconnect_token_hash
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPlayer :one
SELECT * FROM players WHERE id = $1;

-- name: SetPlayerReadyStatus :exec
UPDATE players SET ready_status = $2, last_seen_at = now() WHERE id = $1;
