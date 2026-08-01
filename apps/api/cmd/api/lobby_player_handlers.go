package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (a *api) handleJoinLobby(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if len([]rune(input.DisplayName)) < 2 || len([]rune(input.DisplayName)) > 24 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_player", "Display name must contain between 2 and 24 characters")
		return
	}

	reconnectToken, err := randomToken(32)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	reconnectHash := sha256.Sum256([]byte(reconnectToken))
	code := strings.ToUpper(r.PathValue("code"))
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var lobbyID, status string
	var maxPlayers int
	err = tx.QueryRow(r.Context(), `
		SELECT id, status, max_players
		FROM lobbies
		WHERE code = $1 AND status <> 'expired' AND expires_at > now()
		FOR UPDATE
	`, code).Scan(&lobbyID, &status, &maxPlayers)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "lobby_not_found", "Lobby not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if status == "in_game" || status == "completed" {
		writeError(w, http.StatusConflict, "lobby_already_started", "This lobby no longer accepts players")
		return
	}
	var playerCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL
	`, lobbyID).Scan(&playerCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if playerCount >= maxPlayers {
		writeError(w, http.StatusConflict, "lobby_full", "This lobby is full")
		return
	}

	var playerID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO players (
			lobby_id, display_name, is_host, adult_confirmed, reconnect_token_hash
		)
		VALUES ($1, $2, false, true, $3)
		RETURNING id
	`, lobbyID, input.DisplayName, hex.EncodeToString(reconnectHash[:])).Scan(&playerID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(w, http.StatusConflict, "display_name_taken", "This display name is already used in the lobby")
			return
		}
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies
		SET status = CASE
		      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL) < $2
		        THEN 'waiting_for_players'::lobby_status
		      ELSE 'preparing_photos'::lobby_status
		    END,
		    revision = revision + 1,
		    updated_at = now()
		WHERE id = $1
	`, lobbyID, minimumPlayerCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}

	player := authenticatedPlayer{
		ID:          playerID,
		LobbyID:     lobbyID,
		Code:        code,
		DisplayName: input.DisplayName,
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(code)
	writeJSON(w, http.StatusCreated, playerSessionResponse{
		ReconnectToken: reconnectToken,
		State:          state,
	})
}
