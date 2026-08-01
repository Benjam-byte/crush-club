-- name: CreateLobby :one
INSERT INTO lobbies (
  code,
  max_players,
  settings,
  expires_at,
  owner_identity_id,
  game_config_id,
  game_config_version,
  game_config_snapshot
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetLobbyByCode :one
SELECT * FROM lobbies WHERE code = $1 LIMIT 1;

-- name: UpdateLobbyHost :exec
UPDATE lobbies SET host_player_id = $2, updated_at = now() WHERE id = $1;

-- name: ListLobbyPlayers :many
SELECT * FROM players WHERE lobby_id = $1 ORDER BY joined_at ASC;
