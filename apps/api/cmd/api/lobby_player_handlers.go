package main

import (
	"context"
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
	var maxPlayers *int
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
	if status == "completed" {
		writeError(w, http.StatusConflict, "lobby_already_started", "This lobby no longer accepts players")
		return
	}

	// A pseudo already in use by a player who was connected before and has
	// since gone offline is reclaimed instead of rejected, regardless of
	// lobby status. A currently online match, or a match that never
	// connected even once (disconnected_at IS NULL from the start), is still
	// a hard conflict — reclaim is for resuming an established presence, not
	// for hijacking a session that is mid-handshake.
	var existingPlayerID, existingDisplayName string
	err = tx.QueryRow(r.Context(), `
		SELECT id, display_name FROM players
		WHERE lobby_id = $1 AND lower(display_name) = lower($2)
		  AND excluded_at IS NULL AND disconnected_at IS NOT NULL
		FOR UPDATE
	`, lobbyID, input.DisplayName).Scan(&existingPlayerID, &existingDisplayName)
	switch {
	case err == nil:
		if a.hub != nil && a.hub.isOnline(code, existingPlayerID) {
			writeError(w, http.StatusConflict, "display_name_taken", "This display name is already used in the lobby")
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE players
			SET reconnect_token_hash = $1, disconnected_at = NULL, last_seen_at = now(), updated_at = now()
			WHERE id = $2
		`, hex.EncodeToString(reconnectHash[:]), existingPlayerID); err != nil {
			a.internalError(w, r, err)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
		`, lobbyID); err != nil {
			a.internalError(w, r, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			a.internalError(w, r, err)
			return
		}
		a.finishJoinLobby(w, r, authenticatedPlayer{
			ID:          existingPlayerID,
			LobbyID:     lobbyID,
			Code:        code,
			DisplayName: existingDisplayName,
		}, reconnectToken)
		return
	case errors.Is(err, pgx.ErrNoRows):
		// No existing player with this name: fall through to normal creation.
	default:
		a.internalError(w, r, err)
		return
	}

	var playerCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL
	`, lobbyID).Scan(&playerCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if maxPlayers != nil && playerCount >= *maxPlayers {
		writeError(w, http.StatusConflict, "lobby_full", "This lobby is full")
		return
	}

	// Joining a game already in progress needs the host's approval; the
	// lobby before the game stays open to anyone with the code.
	admissionStatus := "admitted"
	if status == "in_game" {
		admissionStatus = "pending"
	}
	var playerID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO players (
			lobby_id, display_name, is_host, adult_confirmed, reconnect_token_hash, admission_status
		)
		VALUES ($1, $2, false, true, $3, $4)
		RETURNING id
	`, lobbyID, input.DisplayName, hex.EncodeToString(reconnectHash[:]), admissionStatus).Scan(&playerID)
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
		      WHEN status = 'in_game' THEN status
		      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND is_active) < $2
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

	a.finishJoinLobby(w, r, authenticatedPlayer{
		ID:          playerID,
		LobbyID:     lobbyID,
		Code:        code,
		DisplayName: input.DisplayName,
	}, reconnectToken)
}

func (a *api) finishJoinLobby(w http.ResponseWriter, r *http.Request, player authenticatedPlayer, reconnectToken string) {
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusCreated, playerSessionResponse{
		ReconnectToken: reconnectToken,
		State:          state,
	})
}

func (a *api) handleLeaveLobby(w http.ResponseWriter, r *http.Request) {
	rawToken := bearerToken(r)
	if rawToken == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tokenHash := sha256.Sum256([]byte(rawToken))
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var playerID, lobbyID string
	err = tx.QueryRow(r.Context(), `
		SELECT player.id, player.lobby_id
		FROM players AS player
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE lobby.code = $1 AND player.reconnect_token_hash = $2 AND player.excluded_at IS NULL
	`, strings.ToUpper(r.PathValue("code")), hex.EncodeToString(tokenHash[:])).Scan(&playerID, &lobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	lobby, err := lockLobby(r.Context(), tx, lobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	member, err := lockLobbyMember(r.Context(), tx, lobby.ID, playerID)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	afterCommit, err := a.detachPlayer(r.Context(), tx, lobby, member, exclusionReasonLeft)
	if err != nil {
		a.internalError(w, r, err)
		return
	}

	var remainingPlayerCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(*) FROM players WHERE lobby_id = $1 AND is_active
	`, lobby.ID).Scan(&remainingPlayerCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	// Nobody left to play, and nobody left to let the waiting players in:
	// the lobby closes.
	shouldPurge := remainingPlayerCount == 0
	if shouldPurge {
		if _, err := tx.Exec(r.Context(), `
			UPDATE lobbies
			SET status = 'expired', expires_at = now(), revision = revision + 1, updated_at = now()
			WHERE id = $1
		`, lobby.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.closePlayer(lobby.Code, member.ID, closeInvalidSession, "left the lobby")
	if afterCommit != nil && !shouldPurge {
		afterCommit()
	}
	a.hub.publish(lobby.Code)
	if shouldPurge {
		a.purgeLobby(r.Context(), lobby.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCloseLobby lets the host end the lobby for everyone at once, instead of
// every player leaving one by one: their connections close with
// closeLobbyClosed, which sends them home, and the lobby's data and photos are
// deleted right away.
func (a *api) handleCloseLobby(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can close the lobby")
		return
	}
	if _, err := a.pool.Exec(r.Context(), `
		UPDATE lobbies
		SET status = 'expired', expires_at = now(), revision = revision + 1, updated_at = now()
		WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.closeLobby(player.Code, closeLobbyClosed, "lobby closed by the host")
	a.purgeLobby(r.Context(), player.LobbyID)
	w.WriteHeader(http.StatusNoContent)
}

// --- host moderation: kick, admit, refuse ------------------------------------

func (a *api) handleExcludePlayer(w http.ResponseWriter, r *http.Request) {
	a.moderatePlayer(w, r, func(member lobbyMember) (string, bool) {
		// Removing someone who was still waiting for approval is refusing
		// them, which their client reports differently.
		if !member.Admitted {
			return exclusionReasonRefused, true
		}
		return exclusionReasonKicked, true
	})
}

func (a *api) handleRefusePlayer(w http.ResponseWriter, r *http.Request) {
	a.moderatePlayer(w, r, func(member lobbyMember) (string, bool) {
		if member.Admitted {
			return "", false
		}
		return exclusionReasonRefused, true
	})
}

// moderatePlayer removes another player on the host's behalf. reasonFor
// picks the exclusion reason for the target, or refuses the action (ok
// false) when the target is not in the expected admission state.
func (a *api) moderatePlayer(w http.ResponseWriter, r *http.Request, reasonFor func(lobbyMember) (reason string, ok bool)) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can do this")
		return
	}
	targetPlayerID := r.PathValue("playerID")
	if targetPlayerID == player.ID {
		writeError(w, http.StatusUnprocessableEntity, "cannot_exclude_self", "The host cannot exclude themselves")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	lobby, err := lockLobby(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	member, err := lockLobbyMember(r.Context(), tx, lobby.ID, targetPlayerID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "player_not_found", "Player not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if member.IsHost {
		writeError(w, http.StatusConflict, "cannot_exclude_host", "The lobby host cannot be excluded")
		return
	}
	reason, ok := reasonFor(member)
	if !ok {
		writeError(w, http.StatusConflict, "player_not_pending", "This player is not waiting for approval")
		return
	}
	afterCommit, err := a.detachPlayer(r.Context(), tx, lobby, member, reason)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.closePlayer(lobby.Code, member.ID, closeCodeForExclusion(reason), "removed by the host")
	if afterCommit != nil {
		afterCommit()
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleAdmitPlayer(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can do this")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	lobby, err := lockLobby(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	member, err := lockLobbyMember(r.Context(), tx, lobby.ID, r.PathValue("playerID"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "player_not_found", "Player not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if member.Admitted {
		writeError(w, http.StatusConflict, "player_not_pending", "This player is not waiting for approval")
		return
	}
	// Letting someone in only adds work to the current phase, so nothing
	// needs re-evaluating: classic newcomers still prepare their photos and
	// join at the next round, the other modes count them from now on.
	if _, err := tx.Exec(r.Context(), `
		UPDATE players SET admission_status = 'admitted', updated_at = now() WHERE id = $1
	`, member.ID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := bumpLobbyRevision(r.Context(), tx, lobby.ID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- shared departure core ---------------------------------------------------

type lobbyRef struct {
	ID     string
	Code   string
	Mode   string
	Status string
}

// lockLobby takes the lobby row lock every membership change starts with.
func lockLobby(ctx context.Context, tx pgx.Tx, lobbyID string) (lobbyRef, error) {
	lobby := lobbyRef{ID: lobbyID}
	err := tx.QueryRow(ctx, `
		SELECT code, mode, status FROM lobbies WHERE id = $1 FOR UPDATE
	`, lobbyID).Scan(&lobby.Code, &lobby.Mode, &lobby.Status)
	return lobby, err
}

type lobbyMember struct {
	ID       string
	IsHost   bool
	Admitted bool
}

// lockLobbyMember locks a player still in the lobby, admitted or pending.
func lockLobbyMember(ctx context.Context, tx pgx.Tx, lobbyID, playerID string) (lobbyMember, error) {
	member := lobbyMember{ID: playerID}
	err := tx.QueryRow(ctx, `
		SELECT is_host, admission_status = 'admitted'
		FROM players
		WHERE id = $1 AND lobby_id = $2 AND excluded_at IS NULL
		FOR UPDATE
	`, playerID, lobbyID).Scan(&member.IsHost, &member.Admitted)
	return member, err
}

// detachPlayer takes a player out of the lobby: they lose their seat, the
// host role moves on if they held it, and their game stops waiting for them.
// afterCommit, when not nil, must run once tx has committed.
func (a *api) detachPlayer(ctx context.Context, tx pgx.Tx, lobby lobbyRef, member lobbyMember, reason string) (afterCommit func(), err error) {
	if _, err := tx.Exec(ctx, `
		UPDATE players
		SET excluded_at = now(), exclusion_reason = $2, disconnected_at = COALESCE(disconnected_at, now()),
		    is_host = false, updated_at = now()
		WHERE id = $1
	`, member.ID, reason); err != nil {
		return nil, err
	}
	if member.IsHost {
		if err := transferHostAfterDeparture(ctx, tx, lobby.ID); err != nil {
			return nil, err
		}
	}
	if member.Admitted {
		afterCommit, err = a.removePlayerFromGame(ctx, tx, lobby, member.ID)
		if err != nil {
			return nil, err
		}
	}
	return afterCommit, bumpLobbyRevision(ctx, tx, lobby.ID)
}

// transferHostAfterDeparture hands the host role to the earliest active
// player, connected ones first.
func transferHostAfterDeparture(ctx context.Context, tx pgx.Tx, lobbyID string) error {
	var successorID string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM players
		WHERE lobby_id = $1 AND is_active
		ORDER BY (disconnected_at IS NULL) DESC, joined_at, id
		LIMIT 1
	`, lobbyID).Scan(&successorID)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err := tx.Exec(ctx, `UPDATE lobbies SET host_player_id = NULL WHERE id = $1`, lobbyID)
		return err
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE players SET is_host = true, updated_at = now() WHERE id = $1`, successorID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE lobbies SET host_player_id = $2 WHERE id = $1`, lobbyID, successorID)
	return err
}

// removePlayerFromGame makes the lobby's game stop waiting for a departed
// admitted player. Classic games are updated inside tx; the other modes are
// re-evaluated in their own transaction through the returned afterCommit.
func (a *api) removePlayerFromGame(ctx context.Context, tx pgx.Tx, lobby lobbyRef, playerID string) (afterCommit func(), err error) {
	switch lobby.Status {
	case "in_game":
		if lobby.Mode == lobbyModeClassic {
			return nil, excludeGameParticipant(ctx, tx, lobby.ID, playerID)
		}
		return func() { a.settleGameAfterDeparture(lobby.Code, lobby.ID, lobby.Mode) }, nil
	case "waiting_for_players", "preparing_photos", "ready_to_start":
		return nil, refreshPreGameLobbyStatus(ctx, tx, lobby.ID)
	}
	return nil, nil
}

// refreshPreGameLobbyStatus recomputes the status of a lobby that has not
// started its game from its active players.
func refreshPreGameLobbyStatus(ctx context.Context, tx pgx.Tx, lobbyID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE lobbies
		SET status = CASE
		      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND is_active) < $2
		        THEN 'waiting_for_players'::lobby_status
		      WHEN NOT (SELECT bool_and(ready_status = 'ready') FROM players WHERE lobby_id = $1 AND is_active)
		        THEN 'preparing_photos'::lobby_status
		      ELSE 'ready_to_start'::lobby_status
		    END,
		    updated_at = now()
		WHERE id = $1
	`, lobbyID, minimumPlayerCount)
	return err
}
