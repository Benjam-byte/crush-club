package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// « Le classement secret »: every question completes "Pour combien d'argent
// accepterais-tu de … ?". Each round, everyone bets an amount in secret, the
// phones are lined up on the table from the smallest bet to the largest and
// numbered by tapping them, then the amounts are revealed one phone at a
// time. The group scores together: how many players were well placed.

func defaultSecretRankingThemes() []string {
	return []string{
		"ne plus parler à ton meilleur ami pendant un an",
		"te raser la tête en direct",
		"lire tes 50 derniers messages à voix haute",
		"manger un insecte vivant",
		"ne plus toucher à ton téléphone pendant un mois",
		"te faire tatouer le prénom de ton ex",
		"laisser ta mère lire ton historique de recherche",
		"passer une nuit seul·e dans un cimetière",
		"ne plus jamais manger de fromage",
		"appeler ton ex maintenant, en haut-parleur",
		"vivre un an sans internet",
		"garder ta pire photo en photo de profil pendant un mois",
		"chanter en karaoké devant tes collègues",
		"démissionner demain matin",
		"porter le même jean pendant un an",
	}
}

// normalizeSecretRankingTheme trims a proposal down to the end of the
// sentence: the fixed "Pour combien d'argent accepterais-tu de" start and the
// final question mark are shown by the client.
func normalizeSecretRankingTheme(theme string) string {
	theme = strings.TrimSpace(theme)
	lower := strings.ToLower(theme)
	for _, prefix := range []string{
		"pour combien d'argent accepterais-tu de ",
		"pour combien d’argent accepterais-tu de ",
		"pour combien d'argent accepterais-tu d'",
		"pour combien d’argent accepterais-tu d’",
	} {
		if strings.HasPrefix(lower, prefix) {
			theme = strings.TrimSpace(theme[len(prefix):])
			break
		}
	}
	return strings.TrimSpace(strings.TrimRight(theme, "? \u00a0"))
}

// --- lobby start & theme collection & ranking -------------------------------

func (a *api) handleStartSecretRankingGame(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can start the game")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var lobbyStatus, mode string
	if err := tx.QueryRow(r.Context(), `
		SELECT status, mode FROM lobbies WHERE id = $1 FOR UPDATE
	`, player.LobbyID).Scan(&lobbyStatus, &mode); err != nil {
		a.internalError(w, r, err)
		return
	}
	if mode != lobbyModeSecretRanking {
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a Classement secret lobby")
		return
	}
	if lobbyStatus == "in_game" || lobbyStatus == "completed" {
		_ = tx.Rollback(r.Context())
		a.writeLobbyStateResponse(w, r, player, http.StatusOK)
		return
	}

	playerIDs, err := activeLobbyPlayerIDs(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if len(playerIDs) < secretRankingMinPlayerCount || len(playerIDs) > secretRankingMaxPlayerCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "Le classement secret needs three to ten players")
		return
	}
	for _, id := range playerIDs {
		if !a.hub.isOnline(player.Code, id) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	gameID, themeDeadline, err := createSecretRankingGame(r.Context(), tx, player.LobbyID, true)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleSecretRankingThemeSubmissionTimeout(player.Code, gameID, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// createSecretRankingGame opens a new game on its theme collection and puts
// the lobby in game. firstThemeSelection is true for the lobby's first game,
// which gets the full theme window.
func createSecretRankingGame(ctx context.Context, tx pgx.Tx, lobbyID string, firstThemeSelection bool) (string, time.Time, error) {
	var gameID string
	deadline := time.Now().Add(themeStepWindow(themeProposalStep, firstThemeSelection))
	if err := tx.QueryRow(ctx, `
		INSERT INTO secret_ranking_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, lobbyID, deadline).Scan(&gameID); err != nil {
		return "", time.Time{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE lobbies SET status = 'in_game' WHERE id = $1`, lobbyID); err != nil {
		return "", time.Time{}, err
	}
	return gameID, deadline, bumpLobbyRevision(ctx, tx, lobbyID)
}

func (a *api) handleSubmitSecretRankingTheme(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Theme string `json:"theme"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	input.Theme = normalizeSecretRankingTheme(input.Theme)
	if len([]rune(input.Theme)) > secretRankingThemeMaxLength {
		writeError(w, http.StatusUnprocessableEntity, "invalid_theme", "Theme must be at most 80 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, phase, err := lockActiveSecretRankingGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "secret_ranking_not_started", "The game has not started yet")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if phase != "collecting_themes" {
		writeError(w, http.StatusConflict, "themes_locked", "Theme submissions are closed")
		return
	}

	var themeValue *string
	if input.Theme != "" {
		themeValue = &input.Theme
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO secret_ranking_theme_submissions (game_id, player_id, theme_label)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, player_id) DO UPDATE SET theme_label = $3, submitted_at = now()
	`, gameID, player.ID, themeValue); err != nil {
		a.internalError(w, r, err)
		return
	}
	rankingDeadline, err := tryAdvanceSecretRankingThemeCollection(r.Context(), tx, gameID, player.LobbyID, false)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if rankingDeadline != nil {
		a.scheduleSecretRankingThemeRankingTimeout(player.Code, gameID, *rankingDeadline)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryAdvanceSecretRankingThemeCollection moves the game into ranking_themes
// once every active player has proposed a question or passed, or
// unconditionally when force is true (the deadline expired). It returns the
// ranking deadline it set, nil when the phase did not move.
func tryAdvanceSecretRankingThemeCollection(ctx context.Context, tx pgx.Tx, gameID, lobbyID string, force bool) (*time.Time, error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, secretRankingThemeSubmissions, gameID, 0)
		if err != nil {
			return nil, err
		}
		if len(awaiting) > 0 {
			return nil, nil
		}
	}
	deadline, err := themeRankingDeadline(ctx, tx, "secret_ranking_games", lobbyID, gameID, false)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE secret_ranking_games SET phase = 'ranking_themes', theme_phase_deadline = $2 WHERE id = $1
	`, gameID, deadline); err != nil {
		return nil, err
	}
	return &deadline, nil
}

func (a *api) handleRankSecretRankingThemes(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Ranking []string `json:"ranking"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, phase, err := lockActiveSecretRankingGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "secret_ranking_not_started", "The game has not started yet")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if phase != "ranking_themes" {
		writeError(w, http.StatusConflict, "ranking_locked", "Theme ranking is not open")
		return
	}

	candidates, err := secretRankingThemeCandidates(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if !rankingMatchesCandidates(input.Ranking, candidates) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_ranking", "The ranking must contain every candidate theme exactly once")
		return
	}
	rankingJSON, err := json.Marshal(input.Ranking)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO secret_ranking_theme_rankings (game_id, voter_player_id, ranking)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, voter_player_id) DO UPDATE SET ranking = $3, submitted_at = now()
	`, gameID, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	round, started, err := tryConcludeSecretRankingRanking(r.Context(), tx, gameID, player.LobbyID, candidates, false)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if started {
		a.scheduleSecretRankingSecretTimeout(player.Code, round.id, round.deadline)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// startedSecretRankingRound is the round a transaction opened, whose secret
// deadline timer must start once it commits.
type startedSecretRankingRound struct {
	id       string
	deadline time.Time
}

func tryConcludeSecretRankingRanking(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, candidates []string, force bool,
) (startedSecretRankingRound, bool, error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, secretRankingThemeRankings, gameID, 0)
		if err != nil {
			return startedSecretRankingRound{}, false, err
		}
		if len(awaiting) > 0 {
			return startedSecretRankingRound{}, false, nil
		}
	}

	rows, err := tx.Query(ctx, `SELECT ranking FROM secret_ranking_theme_rankings WHERE game_id = $1`, gameID)
	if err != nil {
		return startedSecretRankingRound{}, false, err
	}
	rankings := make([][]string, 0)
	for rows.Next() {
		var rankingJSON []byte
		if err := rows.Scan(&rankingJSON); err != nil {
			rows.Close()
			return startedSecretRankingRound{}, false, err
		}
		var ranking []string
		if err := json.Unmarshal(rankingJSON, &ranking); err != nil {
			rows.Close()
			return startedSecretRankingRound{}, false, err
		}
		rankings = append(rankings, ranking)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return startedSecretRankingRound{}, false, err
	}
	rows.Close()

	selectedThemes := tallyThemeRanking(candidates, rankings)
	if len(selectedThemes) == 0 {
		return startedSecretRankingRound{}, false, errors.New("secret ranking theme ranking produced no selected themes")
	}
	for index, themeLabel := range selectedThemes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO secret_ranking_selected_themes (game_id, position, theme_label) VALUES ($1, $2, $3)
		`, gameID, index+1, themeLabel); err != nil {
			return startedSecretRankingRound{}, false, err
		}
	}

	round, err := startSecretRankingRound(ctx, tx, gameID, 1, selectedThemes[0])
	if err != nil {
		return startedSecretRankingRound{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE secret_ranking_games SET phase = 'playing' WHERE id = $1`, gameID); err != nil {
		return startedSecretRankingRound{}, false, err
	}
	return round, true, nil
}

func (a *api) scheduleSecretRankingThemeSubmissionTimeout(code, gameID string, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceSecretRankingThemeDeadline(code, gameID, "collecting_themes")
	})
}

func (a *api) scheduleSecretRankingThemeRankingTimeout(code, gameID string, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceSecretRankingThemeDeadline(code, gameID, "ranking_themes")
	})
}

// forceSecretRankingThemeDeadline closes the theme step expectedPhase when
// its deadline passes. It does nothing once the game has moved on.
func (a *api) forceSecretRankingThemeDeadline(code, gameID, expectedPhase string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin secret ranking theme deadline transaction", "game_id", gameID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase, lobbyID string
	if err := tx.QueryRow(ctx, `
		SELECT phase, lobby_id FROM secret_ranking_games WHERE id = $1 FOR UPDATE
	`, gameID).Scan(&phase, &lobbyID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load secret ranking game for theme deadline", "game_id", gameID, "error", err)
		}
		return
	}
	if phase != expectedPhase {
		return
	}

	var timers phaseTimers
	switch phase {
	case "collecting_themes":
		rankingDeadline, err := tryAdvanceSecretRankingThemeCollection(ctx, tx, gameID, lobbyID, true)
		if err != nil {
			a.logger.Error("unable to force secret ranking theme collection deadline", "game_id", gameID, "error", err)
			return
		}
		if rankingDeadline != nil {
			timers.add(func() { a.scheduleSecretRankingThemeRankingTimeout(code, gameID, *rankingDeadline) })
		}
	case "ranking_themes":
		candidates, err := secretRankingThemeCandidates(ctx, tx, gameID)
		if err != nil {
			a.logger.Error("unable to load secret ranking theme candidates", "game_id", gameID, "error", err)
			return
		}
		round, started, err := tryConcludeSecretRankingRanking(ctx, tx, gameID, lobbyID, candidates, true)
		if err != nil {
			a.logger.Error("unable to force secret ranking theme ranking deadline", "game_id", gameID, "error", err)
			return
		}
		if started {
			timers.add(func() { a.scheduleSecretRankingSecretTimeout(code, round.id, round.deadline) })
		}
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for secret ranking theme deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit secret ranking theme deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	timers.start()
}

func secretRankingThemeCandidates(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label
		FROM secret_ranking_theme_submissions
		WHERE game_id = $1 AND theme_label IS NOT NULL AND theme_label <> ''
		ORDER BY submitted_at, player_id
	`, gameID)
	if err != nil {
		return nil, err
	}
	proposals := make([]string, 0)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			rows.Close()
			return nil, err
		}
		proposals = append(proposals, label)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return mergeSecretRankingCandidates(proposals, defaultSecretRankingThemes()), nil
}

// mergeSecretRankingCandidates deduplicates the proposals (case and spacing
// insensitive) and tops them up with defaults until there are three.
func mergeSecretRankingCandidates(proposals, defaults []string) []string {
	candidates := make([]string, 0, max(len(proposals), 3))
	seen := make(map[string]struct{})
	add := func(label string) {
		key := strings.ToLower(strings.TrimSpace(label))
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, label)
	}
	for _, proposal := range proposals {
		add(proposal)
	}
	for _, fallback := range defaults {
		if len(candidates) >= 3 {
			break
		}
		add(fallback)
	}
	return candidates
}

// --- rounds: secret bets ----------------------------------------------------

func startSecretRankingRound(ctx context.Context, tx pgx.Tx, gameID string, roundNumber int, themeLabel string) (startedSecretRankingRound, error) {
	round := startedSecretRankingRound{deadline: time.Now().Add(secretRankingAmountWindow)}
	err := tx.QueryRow(ctx, `
		INSERT INTO secret_ranking_rounds (game_id, round_number, theme_label, secret_deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, gameID, roundNumber, themeLabel, round.deadline).Scan(&round.id)
	return round, err
}

func (a *api) scheduleSecretRankingSecretTimeout(code, roundID string, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceSecretRankingSecretDeadline(code, roundID)
	})
}

// forceSecretRankingSecretDeadline closes the bets when the secret window
// expires: the players who bet take part, the others watch this round. With
// fewer than two bets the window starts over instead.
func (a *api) forceSecretRankingSecretDeadline(code, roundID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin secret ranking bet deadline transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	round, lobbyID, err := lockSecretRankingRound(ctx, tx, roundID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load secret ranking round for bet deadline", "round_id", roundID, "error", err)
		}
		return
	}
	// A deadline pushed back since this timer was set has its own timer.
	if round.Phase != "secret" || time.Now().Before(round.SecretDeadline) {
		return
	}
	closed, err := closeSecretRankingSecret(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to close secret ranking bets", "round_id", roundID, "error", err)
		return
	}
	var timers phaseTimers
	if !closed {
		deadline := time.Now().Add(secretRankingAmountWindow)
		if _, err := tx.Exec(ctx, `
			UPDATE secret_ranking_rounds SET secret_deadline = $2 WHERE id = $1
		`, roundID, deadline); err != nil {
			a.logger.Error("unable to extend secret ranking bet deadline", "round_id", roundID, "error", err)
			return
		}
		timers.add(func() { a.scheduleSecretRankingSecretTimeout(code, roundID, deadline) })
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for secret ranking bet deadline", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit secret ranking bet deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
	timers.start()
}

// closeSecretRankingSecret sends the round to the table with the active
// players who bet. It reports false, leaving the round open, when fewer than
// secretRankingMinParticipants of them did.
func closeSecretRankingSecret(ctx context.Context, tx pgx.Tx, roundID string) (bool, error) {
	if _, err := tx.Exec(ctx, `
		DELETE FROM secret_ranking_entries AS entry
		USING players AS player
		WHERE entry.round_id = $1 AND player.id = entry.player_id AND NOT player.is_active
	`, roundID); err != nil {
		return false, err
	}
	participants, err := secretRankingParticipantCount(ctx, tx, roundID)
	if err != nil {
		return false, err
	}
	if participants < secretRankingMinParticipants {
		return false, nil
	}
	_, err = tx.Exec(ctx, `
		UPDATE secret_ranking_rounds SET phase = 'placing', lock_at = NULL WHERE id = $1
	`, roundID)
	return err == nil, err
}

func (a *api) handleSubmitSecretRankingAmount(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Amount *int64 `json:"amount"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	if input.Amount == nil || *input.Amount < 0 || *input.Amount > secretRankingMaxAmount {
		writeError(w, http.StatusUnprocessableEntity, "invalid_amount", "The amount must be a whole number of euros between 0 and 1 000 000 000")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	round, ok := a.lockPlayingSecretRankingRound(w, r, tx, player.LobbyID)
	if !ok {
		return
	}
	if round.Phase != "secret" {
		writeError(w, http.StatusConflict, "secret_ranking_round_locked", "Bets are closed for this round")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO secret_ranking_entries (round_id, player_id, amount)
		VALUES ($1, $2, $3)
		ON CONFLICT (round_id, player_id) DO UPDATE SET amount = $3, created_at = now()
	`, round.ID, player.ID, *input.Amount); err != nil {
		a.internalError(w, r, err)
		return
	}
	awaiting, err := secretRankingAmountAwaitingIDs(r.Context(), tx, player.LobbyID, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if len(awaiting) == 0 {
		if _, err := closeSecretRankingSecret(r.Context(), tx, round.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- rounds: numbering the phones on the table ------------------------------

func (a *api) handleSetSecretRankingPosition(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Position *int `json:"position"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	round, ok := a.lockPlayingSecretRankingRound(w, r, tx, player.LobbyID)
	if !ok {
		return
	}
	if round.Phase != "placing" {
		writeError(w, http.StatusConflict, "secret_ranking_round_locked", "The phones can no longer be numbered")
		return
	}
	participants, err := secretRankingParticipantCount(r.Context(), tx, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if input.Position == nil || *input.Position < 1 || *input.Position > participants {
		writeError(w, http.StatusUnprocessableEntity, "invalid_table_position", "The position must be between 1 and the number of phones")
		return
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE secret_ranking_entries SET position = $3 WHERE round_id = $1 AND player_id = $2
	`, round.ID, player.ID, *input.Position)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusForbidden, "not_participant", "You did not bet in this round")
		return
	}
	lockAt, err := refreshSecretRankingLock(r.Context(), tx, round)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if lockAt != nil {
		a.scheduleSecretRankingLockTimeout(player.Code, round.ID, *lockAt)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// refreshSecretRankingLock arms the lock countdown when the positions have
// just become a full permutation of 1..N, and disarms it as soon as they are
// not one any more. It returns the lock time to schedule when it armed one.
func refreshSecretRankingLock(ctx context.Context, tx pgx.Tx, round secretRankingRoundRow) (*time.Time, error) {
	positions, err := loadSecretRankingPositions(ctx, tx, round.ID)
	if err != nil {
		return nil, err
	}
	if !isPositionPermutation(positions) {
		if round.LockAt == nil {
			return nil, nil
		}
		_, err := tx.Exec(ctx, `UPDATE secret_ranking_rounds SET lock_at = NULL WHERE id = $1`, round.ID)
		return nil, err
	}
	if round.LockAt != nil {
		return nil, nil
	}
	lockAt := time.Now().Add(secretRankingLockDelay)
	if _, err := tx.Exec(ctx, `
		UPDATE secret_ranking_rounds SET lock_at = $2, positions_reset_at = NULL WHERE id = $1
	`, round.ID, lockAt); err != nil {
		return nil, err
	}
	return &lockAt, nil
}

func (a *api) scheduleSecretRankingLockTimeout(code, roundID string, lockAt time.Time) {
	time.AfterFunc(time.Until(lockAt), func() {
		a.forceSecretRankingLock(code, roundID)
	})
}

// forceSecretRankingLock scores the round and starts the reveal when the
// lock countdown is still armed and has run out. A timer whose countdown was
// cancelled, or re-armed later, finds nothing to do.
func (a *api) forceSecretRankingLock(code, roundID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin secret ranking lock transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	round, lobbyID, err := lockSecretRankingRound(ctx, tx, roundID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load secret ranking round for lock", "round_id", roundID, "error", err)
		}
		return
	}
	if round.Phase != "placing" || round.LockAt == nil || time.Now().Before(*round.LockAt) {
		return
	}
	positions, err := loadSecretRankingPositions(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to load secret ranking positions for lock", "round_id", roundID, "error", err)
		return
	}
	if !isPositionPermutation(positions) {
		return
	}
	if err := scoreSecretRankingRound(ctx, tx, roundID); err != nil {
		a.logger.Error("unable to score secret ranking round", "round_id", roundID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for secret ranking lock", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit secret ranking reveal", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
}

// scoreSecretRankingRound marks every participant well placed or not, then
// opens the domino reveal.
func scoreSecretRankingRound(ctx context.Context, tx pgx.Tx, roundID string) error {
	rows, err := tx.Query(ctx, `
		SELECT player_id, amount, position FROM secret_ranking_entries WHERE round_id = $1 ORDER BY player_id
	`, roundID)
	if err != nil {
		return err
	}
	type scoredEntry struct {
		playerID string
		amount   int64
		position int
	}
	entries := make([]scoredEntry, 0)
	for rows.Next() {
		var entry scoredEntry
		var position *int
		if err := rows.Scan(&entry.playerID, &entry.amount, &position); err != nil {
			rows.Close()
			return err
		}
		if position != nil {
			entry.position = *position
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	amounts := make([]int64, len(entries))
	for index, entry := range entries {
		amounts[index] = entry.amount
	}
	ranges := secretRankingRankRanges(amounts)
	wellPlacedCount := 0
	for index, entry := range entries {
		wellPlaced := ranges[index].contains(entry.position)
		if wellPlaced {
			wellPlacedCount++
		}
		if _, err := tx.Exec(ctx, `
			UPDATE secret_ranking_entries SET well_placed = $3, true_rank_min = $4, true_rank_max = $5
			WHERE round_id = $1 AND player_id = $2
		`, roundID, entry.playerID, wellPlaced, ranges[index].min, ranges[index].max); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE secret_ranking_rounds
		SET phase = 'revealing', lock_at = NULL, reveal_at = $2,
		    well_placed_count = $3, participant_count = $4
		WHERE id = $1
	`, roundID, time.Now().Add(secretRankingRevealLead), wellPlacedCount, len(entries))
	return err
}

// --- rounds: debrief, next round, replay ------------------------------------

// handleOpenSecretRankingDebrief moves everyone from the table to the debrief
// once the reveal has played out. Anyone may call it, and calling it again is
// harmless.
func (a *api) handleOpenSecretRankingDebrief(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	round, ok := a.lockPlayingSecretRankingRound(w, r, tx, player.LobbyID)
	if !ok {
		return
	}
	if round.Phase == "results" {
		_ = tx.Rollback(r.Context())
		a.writeLobbyStateResponse(w, r, player, http.StatusOK)
		return
	}
	if round.Phase != "revealing" || round.RevealAt == nil || round.ParticipantCount == nil {
		writeError(w, http.StatusConflict, "secret_ranking_round_locked", "The reveal has not started yet")
		return
	}
	if time.Now().Before(secretRankingRevealEndsAt(*round.RevealAt, *round.ParticipantCount)) {
		writeError(w, http.StatusConflict, "reveal_in_progress", "The reveal is still playing")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE secret_ranking_rounds SET phase = 'results' WHERE id = $1`, round.ID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleStartNextSecretRankingRound(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can start the next round")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	round, ok := a.lockPlayingSecretRankingRound(w, r, tx, player.LobbyID)
	if !ok {
		return
	}
	if round.Phase != "results" {
		writeError(w, http.StatusConflict, "secret_ranking_round_locked", "This round has not finished yet")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE secret_ranking_rounds SET phase = 'completed', completed_at = now() WHERE id = $1
	`, round.ID); err != nil {
		a.internalError(w, r, err)
		return
	}

	var nextThemeLabel string
	err = tx.QueryRow(r.Context(), `
		SELECT theme_label FROM secret_ranking_selected_themes WHERE game_id = $1 AND position = $2
	`, round.GameID, round.RoundNumber+1).Scan(&nextThemeLabel)
	var nextRound startedSecretRankingRound
	started := false
	switch {
	case errors.Is(err, pgx.ErrNoRows) || round.RoundNumber >= secretRankingRoundCount:
		if _, err := tx.Exec(r.Context(), `
			UPDATE secret_ranking_games SET phase = 'completed', completed_at = now() WHERE id = $1
		`, round.GameID); err != nil {
			a.internalError(w, r, err)
			return
		}
	case err != nil:
		a.internalError(w, r, err)
		return
	default:
		nextRound, err = startSecretRankingRound(r.Context(), tx, round.GameID, round.RoundNumber+1, nextThemeLabel)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		started = true
	}

	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if started {
		a.scheduleSecretRankingSecretTimeout(player.Code, nextRound.id, nextRound.deadline)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleReplaySecretRanking(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can start a new game")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	_, gamePhase, err := lockActiveSecretRankingGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && gamePhase != "completed" {
		writeError(w, http.StatusConflict, "secret_ranking_in_progress", "The current game has not finished yet")
		return
	}
	activeCount, err := activeLobbyPlayerCount(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if activeCount < secretRankingMinPlayerCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "Le classement secret needs three to ten players")
		return
	}
	gameID, themeDeadline, err := createSecretRankingGame(r.Context(), tx, player.LobbyID, false)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleSecretRankingThemeSubmissionTimeout(player.Code, gameID, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- departures --------------------------------------------------------------

func (a *api) settleSecretRankingGame(ctx context.Context, tx pgx.Tx, code, lobbyID string) (phaseTimers, error) {
	gameID, phase, err := lockActiveSecretRankingGame(ctx, tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && phase == "completed") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enough, err := hasEnoughActivePlayers(ctx, tx, lobbyID, lobbyModeSecretRanking)
	if err != nil {
		return nil, err
	}
	if !enough {
		return nil, abortUncappedGame(ctx, tx, "secret_ranking_rounds", "secret_ranking_games", gameID)
	}

	var timers phaseTimers
	switch phase {
	case "collecting_themes":
		rankingDeadline, err := tryAdvanceSecretRankingThemeCollection(ctx, tx, gameID, lobbyID, false)
		if err != nil {
			return nil, err
		}
		if rankingDeadline != nil {
			timers.add(func() { a.scheduleSecretRankingThemeRankingTimeout(code, gameID, *rankingDeadline) })
		}
	case "ranking_themes":
		candidates, err := secretRankingThemeCandidates(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		round, started, err := tryConcludeSecretRankingRanking(ctx, tx, gameID, lobbyID, candidates, false)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleSecretRankingSecretTimeout(code, round.id, round.deadline) })
		}
	case "playing":
		round, err := loadCurrentSecretRankingRound(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		switch round.Phase {
		case "secret":
			awaiting, err := secretRankingAmountAwaitingIDs(ctx, tx, lobbyID, round.ID)
			if err != nil || len(awaiting) > 0 {
				return nil, err
			}
			if _, err := closeSecretRankingSecret(ctx, tx, round.ID); err != nil {
				return nil, err
			}
		case "placing":
			return nil, removeDepartedSecretRankingParticipants(ctx, tx, round.ID)
		}
	}
	return timers, nil
}

// removeDepartedSecretRankingParticipants takes the phones of departed
// players off the table. Their numbers no longer add up, so every number is
// wiped and the table numbers again; a round left with fewer than two phones
// is cancelled.
func removeDepartedSecretRankingParticipants(ctx context.Context, tx pgx.Tx, roundID string) error {
	tag, err := tx.Exec(ctx, `
		DELETE FROM secret_ranking_entries AS entry
		USING players AS player
		WHERE entry.round_id = $1 AND player.id = entry.player_id AND NOT player.is_active
	`, roundID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	participants, err := secretRankingParticipantCount(ctx, tx, roundID)
	if err != nil {
		return err
	}
	if participants < secretRankingMinParticipants {
		_, err := tx.Exec(ctx, `
			UPDATE secret_ranking_rounds
			SET phase = 'results', cancelled = true, lock_at = NULL,
			    participant_count = $2, well_placed_count = 0
			WHERE id = $1
		`, roundID, participants)
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE secret_ranking_entries SET position = NULL WHERE round_id = $1
	`, roundID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE secret_ranking_rounds SET lock_at = NULL, positions_reset_at = now() WHERE id = $1
	`, roundID)
	return err
}

// --- pure rules --------------------------------------------------------------

// secretRankingRankRange is the span of places a bet may take in the true
// order: equal amounts share one, so they are interchangeable.
type secretRankingRankRange struct {
	min, max int
}

func (rankRange secretRankingRankRange) contains(position int) bool {
	return position >= rankRange.min && position <= rankRange.max
}

// secretRankingRankRanges returns, for each amount, the places it may take
// in the line sorted from the smallest amount (place 1) to the largest.
func secretRankingRankRanges(amounts []int64) []secretRankingRankRange {
	ranges := make([]secretRankingRankRange, len(amounts))
	for index, amount := range amounts {
		smaller, equal := 0, 0
		for _, other := range amounts {
			switch {
			case other < amount:
				smaller++
			case other == amount:
				equal++
			}
		}
		ranges[index] = secretRankingRankRange{min: smaller + 1, max: smaller + equal}
	}
	return ranges
}

// isPositionPermutation reports whether the positions number the phones
// 1..N exactly once each. 0 stands for a phone not numbered yet.
func isPositionPermutation(positions []int) bool {
	if len(positions) == 0 {
		return false
	}
	seen := make([]bool, len(positions)+1)
	for _, position := range positions {
		if position < 1 || position > len(positions) || seen[position] {
			return false
		}
		seen[position] = true
	}
	return true
}

// secretRankingRevealEndsAt is when the domino reveal of participants phones
// has played out, verdict included: the debrief can open.
func secretRankingRevealEndsAt(revealAt time.Time, participants int) time.Time {
	return revealAt.Add(time.Duration(max(participants-1, 0))*secretRankingRevealStep + secretRankingRevealFinale)
}

// secretRankingExtremes lists the players with the smallest and the largest
// amount, ties included.
func secretRankingExtremes(entries []secretRankingEntryRow) (smallest, largest []secretRankingEntryRow) {
	for _, entry := range entries {
		switch {
		case len(smallest) == 0 || entry.Amount < smallest[0].Amount:
			smallest = []secretRankingEntryRow{entry}
		case entry.Amount == smallest[0].Amount:
			smallest = append(smallest, entry)
		}
		switch {
		case len(largest) == 0 || entry.Amount > largest[0].Amount:
			largest = []secretRankingEntryRow{entry}
		case entry.Amount == largest[0].Amount:
			largest = append(largest, entry)
		}
	}
	return smallest, largest
}

// --- shared row loaders ------------------------------------------------------

func loadActiveSecretRankingGame(ctx context.Context, db dbQuerier, lobbyID string) (id, phase string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, phase FROM secret_ranking_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

// lockActiveSecretRankingGame is loadActiveSecretRankingGame for write paths:
// the row lock serializes every command, deadline and departure of a lobby.
func lockActiveSecretRankingGame(ctx context.Context, tx pgx.Tx, lobbyID string) (id, phase string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id, phase FROM secret_ranking_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

// lockPlayingSecretRankingGame locks the lobby's game and loads its current
// round for a round command, answering the request itself when there is no
// round in play.
func (a *api) lockPlayingSecretRankingRound(w http.ResponseWriter, r *http.Request, tx pgx.Tx, lobbyID string) (secretRankingRoundRow, bool) {
	gameID, gamePhase, err := lockActiveSecretRankingGame(r.Context(), tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "secret_ranking_not_playing", "No round is currently in progress")
		return secretRankingRoundRow{}, false
	}
	if err != nil {
		a.internalError(w, r, err)
		return secretRankingRoundRow{}, false
	}
	round, err := loadCurrentSecretRankingRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return secretRankingRoundRow{}, false
	}
	return round, true
}

type secretRankingRoundRow struct {
	ID               string
	GameID           string
	RoundNumber      int
	ThemeLabel       string
	Phase            string
	SecretDeadline   time.Time
	LockAt           *time.Time
	PositionsResetAt *time.Time
	RevealAt         *time.Time
	WellPlacedCount  *int
	ParticipantCount *int
	Cancelled        bool
}

const secretRankingRoundColumns = `
	round.id, round.game_id, round.round_number, round.theme_label, round.phase, round.secret_deadline,
	round.lock_at, round.positions_reset_at, round.reveal_at, round.well_placed_count,
	round.participant_count, round.cancelled`

func scanSecretRankingRound(row rowScanner, round *secretRankingRoundRow) error {
	return row.Scan(
		&round.ID, &round.GameID, &round.RoundNumber, &round.ThemeLabel, &round.Phase, &round.SecretDeadline,
		&round.LockAt, &round.PositionsResetAt, &round.RevealAt, &round.WellPlacedCount,
		&round.ParticipantCount, &round.Cancelled,
	)
}

func loadCurrentSecretRankingRound(ctx context.Context, db dbQuerier, gameID string) (secretRankingRoundRow, error) {
	var round secretRankingRoundRow
	err := scanSecretRankingRound(db.QueryRow(ctx, `
		SELECT `+secretRankingRoundColumns+`
		FROM secret_ranking_rounds AS round
		WHERE round.game_id = $1
		ORDER BY round.round_number DESC
		LIMIT 1
	`, gameID), &round)
	return round, err
}

// lockSecretRankingRound loads a round for a deadline timer, locking its game
// row first like every other write path of the lobby.
func lockSecretRankingRound(ctx context.Context, tx pgx.Tx, roundID string) (secretRankingRoundRow, string, error) {
	var lobbyID string
	if err := tx.QueryRow(ctx, `
		SELECT game.lobby_id
		FROM secret_ranking_rounds AS round
		JOIN secret_ranking_games AS game ON game.id = round.game_id
		WHERE round.id = $1
		FOR UPDATE OF game
	`, roundID).Scan(&lobbyID); err != nil {
		return secretRankingRoundRow{}, "", err
	}
	var round secretRankingRoundRow
	err := scanSecretRankingRound(tx.QueryRow(ctx, `
		SELECT `+secretRankingRoundColumns+` FROM secret_ranking_rounds AS round WHERE round.id = $1
	`, roundID), &round)
	return round, lobbyID, err
}

func secretRankingParticipantCount(ctx context.Context, db dbQuerier, roundID string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT count(*) FROM secret_ranking_entries WHERE round_id = $1`, roundID).Scan(&count)
	return count, err
}

// loadSecretRankingPositions lists the participants' positions, 0 for a phone
// not numbered yet.
func loadSecretRankingPositions(ctx context.Context, db dbQuerier, roundID string) ([]int, error) {
	rows, err := db.Query(ctx, `
		SELECT COALESCE(position, 0) FROM secret_ranking_entries WHERE round_id = $1
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	positions := make([]int, 0)
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return positions, rows.Err()
}

type secretRankingEntryRow struct {
	PlayerID    string
	DisplayName string
	ColorIndex  int
	Amount      int64
	Position    *int
	WellPlaced  *bool
	TrueRankMin *int
	TrueRankMax *int
}

// loadSecretRankingEntries lists a round's participants by arrival in the
// lobby. ColorIndex counts everyone who joined the lobby before them, so a
// phone keeps its color for the whole lobby.
func loadSecretRankingEntries(ctx context.Context, db dbQuerier, roundID string) ([]secretRankingEntryRow, error) {
	rows, err := db.Query(ctx, `
		SELECT entry.player_id, player.display_name,
		       (SELECT count(*) FROM players AS earlier
		        WHERE earlier.lobby_id = player.lobby_id
		          AND (earlier.joined_at, earlier.id) < (player.joined_at, player.id)),
		       entry.amount, entry.position, entry.well_placed, entry.true_rank_min, entry.true_rank_max
		FROM secret_ranking_entries AS entry
		JOIN players AS player ON player.id = entry.player_id
		WHERE entry.round_id = $1
		ORDER BY player.joined_at, player.id
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]secretRankingEntryRow, 0)
	for rows.Next() {
		var entry secretRankingEntryRow
		if err := rows.Scan(
			&entry.PlayerID, &entry.DisplayName, &entry.ColorIndex, &entry.Amount,
			&entry.Position, &entry.WellPlaced, &entry.TrueRankMin, &entry.TrueRankMax,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func loadSecretRankingSelectedThemes(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label FROM secret_ranking_selected_themes WHERE game_id = $1 ORDER BY position
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	themes := make([]string, 0, secretRankingRoundCount)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		themes = append(themes, label)
	}
	return themes, rows.Err()
}

// --- state view ----------------------------------------------------------------

// buildSecretRankingReveal lays out the line in position order, once the
// round is scored.
func buildSecretRankingReveal(entries []secretRankingEntryRow) []secretRankingRevealEntryView {
	reveal := make([]secretRankingRevealEntryView, 0, len(entries))
	for _, entry := range entries {
		if entry.Position == nil || entry.WellPlaced == nil || entry.TrueRankMin == nil || entry.TrueRankMax == nil {
			continue
		}
		reveal = append(reveal, secretRankingRevealEntryView{
			PlayerID:    entry.PlayerID,
			DisplayName: entry.DisplayName,
			ColorIndex:  entry.ColorIndex,
			Position:    *entry.Position,
			Amount:      entry.Amount,
			WellPlaced:  *entry.WellPlaced,
			TrueRankMin: *entry.TrueRankMin,
			TrueRankMax: *entry.TrueRankMax,
		})
	}
	sort.Slice(reveal, func(i, j int) bool { return reveal[i].Position < reveal[j].Position })
	return reveal
}

func (a *api) loadSecretRankingThemeProgress(ctx context.Context, gameID, lobbyID string, table themeStepTable, view *secretRankingStateView) error {
	var deadline *time.Time
	if err := a.pool.QueryRow(ctx, `
		SELECT theme_phase_deadline FROM secret_ranking_games WHERE id = $1
	`, gameID).Scan(&deadline); err != nil {
		return err
	}
	view.ThemeDeadline = deadline
	progress, err := loadThemeStepProgress(ctx, a.pool, lobbyID, table, gameID, 0)
	if err != nil {
		return err
	}
	view.ThemeProgressCount = progress.count
	view.ThemeProgressRequired = progress.required
	view.AwaitingPlayerIDs = progress.awaiting
	return nil
}

func (a *api) loadSecretRankingState(ctx context.Context, currentPlayer authenticatedPlayer) (secretRankingStateView, error) {
	gameID, gamePhase, err := loadActiveSecretRankingGame(ctx, a.pool, currentPlayer.LobbyID)
	if err != nil {
		return secretRankingStateView{}, err
	}
	view := secretRankingStateView{ID: gameID, Phase: gamePhase}

	switch gamePhase {
	case "collecting_themes":
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM secret_ranking_theme_submissions WHERE game_id = $1 AND player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return secretRankingStateView{}, err
		}
		view.ThemeSubmitted = exists
		if err := a.loadSecretRankingThemeProgress(ctx, gameID, currentPlayer.LobbyID, secretRankingThemeSubmissions, &view); err != nil {
			return secretRankingStateView{}, err
		}

	case "ranking_themes":
		view.ThemeSubmitted = true
		candidates, err := secretRankingThemeCandidates(ctx, a.pool, gameID)
		if err != nil {
			return secretRankingStateView{}, err
		}
		view.ThemeCandidates = candidates
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM secret_ranking_theme_rankings WHERE game_id = $1 AND voter_player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return secretRankingStateView{}, err
		}
		view.ThemeRanked = exists
		if err := a.loadSecretRankingThemeProgress(ctx, gameID, currentPlayer.LobbyID, secretRankingThemeRankings, &view); err != nil {
			return secretRankingStateView{}, err
		}

	case "playing":
		view.ThemeSubmitted = true
		view.ThemeRanked = true
		if err := a.fillSecretRankingRound(ctx, currentPlayer, gameID, &view); err != nil {
			return secretRankingStateView{}, err
		}

	case "completed":
		selectedThemes, err := loadSecretRankingSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return secretRankingStateView{}, err
		}
		view.SelectedThemes = selectedThemes
		view.TotalRounds = len(selectedThemes)
		rounds, err := a.loadSecretRankingRoundSummaries(ctx, gameID)
		if err != nil {
			return secretRankingStateView{}, err
		}
		view.Rounds = rounds
	}
	return view, nil
}

// fillSecretRankingRound adds the current round to the view, from the current
// player's point of view.
func (a *api) fillSecretRankingRound(ctx context.Context, currentPlayer authenticatedPlayer, gameID string, view *secretRankingStateView) error {
	selectedThemes, err := loadSecretRankingSelectedThemes(ctx, a.pool, gameID)
	if err != nil {
		return err
	}
	view.SelectedThemes = selectedThemes
	round, err := loadCurrentSecretRankingRound(ctx, a.pool, gameID)
	if err != nil {
		return err
	}
	view.RoundNumber = round.RoundNumber
	view.TotalRounds = len(selectedThemes)
	view.IsLastRound = round.RoundNumber >= len(selectedThemes) || round.RoundNumber >= secretRankingRoundCount
	view.RoundPhase = round.Phase
	view.ThemeLabel = round.ThemeLabel
	view.Cancelled = round.Cancelled

	entries, err := loadSecretRankingEntries(ctx, a.pool, round.ID)
	if err != nil {
		return err
	}
	view.ParticipantCount = len(entries)
	view.Seats = make([]secretRankingSeatView, 0, len(entries))
	for _, entry := range entries {
		if entry.Position != nil {
			view.NumberedCount++
		}
		view.Seats = append(view.Seats, secretRankingSeatView{
			PlayerID:    entry.PlayerID,
			DisplayName: entry.DisplayName,
			ColorIndex:  entry.ColorIndex,
			Position:    entry.Position,
		})
		if entry.PlayerID != currentPlayer.ID {
			continue
		}
		view.IsParticipant = true
		view.Submitted = true
		view.MyPosition = entry.Position
		if round.Phase == "secret" || round.Phase == "placing" {
			amount := entry.Amount
			view.MyAmount = &amount
		}
	}

	switch round.Phase {
	case "secret":
		deadline := round.SecretDeadline
		view.SecretDeadline = &deadline
		required, err := activeLobbyPlayerCount(ctx, a.pool, currentPlayer.LobbyID)
		if err != nil {
			return err
		}
		awaiting, err := secretRankingAmountAwaitingIDs(ctx, a.pool, currentPlayer.LobbyID, round.ID)
		if err != nil {
			return err
		}
		progress := newStepProgress(required, awaiting)
		view.SubmissionProgressCount = progress.count
		view.SubmissionProgressRequired = progress.required
		view.AwaitingPlayerIDs = progress.awaiting

	case "placing":
		view.LockAt = round.LockAt
		view.PositionsResetAt = round.PositionsResetAt
		awaiting, err := secretRankingPositionAwaitingIDs(ctx, a.pool, round.ID)
		if err != nil {
			return err
		}
		view.AwaitingPlayerIDs = awaiting

	case "revealing", "results", "completed":
		if round.WellPlacedCount != nil {
			view.WellPlacedCount = *round.WellPlacedCount
		}
		if round.ParticipantCount != nil {
			view.ParticipantCount = *round.ParticipantCount
		}
		if round.Cancelled || round.RevealAt == nil {
			break
		}
		view.Reveal = buildSecretRankingReveal(entries)
		view.RevealAt = round.RevealAt
		view.RevealStepMs = int(secretRankingRevealStep / time.Millisecond)
		endsAt := secretRankingRevealEndsAt(*round.RevealAt, len(view.Reveal))
		view.RevealEndsAt = &endsAt
	}
	return nil
}

func (a *api) loadSecretRankingRoundSummaries(ctx context.Context, gameID string) ([]secretRankingRoundSummaryView, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT `+secretRankingRoundColumns+`
		FROM secret_ranking_rounds AS round
		WHERE round.game_id = $1 AND (round.reveal_at IS NOT NULL OR round.cancelled)
		ORDER BY round.round_number
	`, gameID)
	if err != nil {
		return nil, err
	}
	rounds := make([]secretRankingRoundRow, 0, secretRankingRoundCount)
	for rows.Next() {
		var round secretRankingRoundRow
		if err := scanSecretRankingRound(rows, &round); err != nil {
			rows.Close()
			return nil, err
		}
		rounds = append(rounds, round)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	summaries := make([]secretRankingRoundSummaryView, 0, len(rounds))
	for _, round := range rounds {
		summary := secretRankingRoundSummaryView{
			RoundNumber:       round.RoundNumber,
			ThemeLabel:        round.ThemeLabel,
			Cancelled:         round.Cancelled,
			SmallestPlayerIDs: make([]string, 0),
			LargestPlayerIDs:  make([]string, 0),
			SmallestNames:     make([]string, 0),
			LargestNames:      make([]string, 0),
		}
		if round.WellPlacedCount != nil {
			summary.WellPlacedCount = *round.WellPlacedCount
		}
		if round.ParticipantCount != nil {
			summary.ParticipantCount = *round.ParticipantCount
		}
		if !round.Cancelled {
			entries, err := loadSecretRankingEntries(ctx, a.pool, round.ID)
			if err != nil {
				return nil, err
			}
			smallest, largest := secretRankingExtremes(entries)
			for _, entry := range smallest {
				summary.SmallestPlayerIDs = append(summary.SmallestPlayerIDs, entry.PlayerID)
				summary.SmallestNames = append(summary.SmallestNames, entry.DisplayName)
			}
			for _, entry := range largest {
				summary.LargestPlayerIDs = append(summary.LargestPlayerIDs, entry.PlayerID)
				summary.LargestNames = append(summary.LargestNames, entry.DisplayName)
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}
