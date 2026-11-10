package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func defaultZeroToHundredThemes() []string {
	return []string{
		"Mettre le plus de nourriture dans sa bouche",
		"Résister à un fou rire",
		"Survivre seul·e en pleine nature",
		"Draguer une personne inconnue",
		"Garder un secret",
		"Improviser un discours de mariage",
	}
}

// --- lobby start & theme collection & ranking -------------------------------

func (a *api) handleStartZeroToHundredGame(w http.ResponseWriter, r *http.Request) {
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
	if mode != lobbyModeZeroToHundred {
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a 0 à 10 lobby")
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
	if len(playerIDs) < zeroToHundredMinPlayerCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "0 à 10 needs at least three players")
		return
	}
	for _, id := range playerIDs {
		if !a.hub.isOnline(player.Code, id) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	var gameID string
	themeDeadline := time.Now().Add(themeStepWindow(themeProposalStep, true))
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO zero_to_100_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, player.LobbyID, themeDeadline).Scan(&gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET status = 'in_game' WHERE id = $1
	`, player.LobbyID); err != nil {
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
	a.scheduleZeroToHundredThemeSubmissionTimeout(player.Code, gameID, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleSubmitZeroToHundredTheme(w http.ResponseWriter, r *http.Request) {
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
	input.Theme = strings.TrimSpace(input.Theme)
	if len([]rune(input.Theme)) > 80 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_theme", "Theme must be at most 80 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, phase, err := lockActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "zero_to_100_not_started", "The 0 à 10 game has not started yet")
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
		INSERT INTO zero_to_100_theme_submissions (game_id, player_id, theme_label)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, player_id) DO UPDATE SET theme_label = $3, submitted_at = now()
	`, gameID, player.ID, themeValue); err != nil {
		a.internalError(w, r, err)
		return
	}
	rankingDeadline, err := a.tryAdvanceZeroToHundredThemeCollection(r.Context(), tx, gameID, player.LobbyID, false)
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
		a.scheduleZeroToHundredThemeRankingTimeout(player.Code, gameID, *rankingDeadline)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryAdvanceZeroToHundredThemeCollection moves the game into ranking_themes once
// every active player has either proposed a theme or explicitly passed — or
// unconditionally when force is true (the submission deadline expired). It
// returns the ranking deadline it set, nil when the phase did not move.
func (a *api) tryAdvanceZeroToHundredThemeCollection(ctx context.Context, tx pgx.Tx, gameID, lobbyID string, force bool) (*time.Time, error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, zeroToHundredThemeSubmissions, gameID, 0)
		if err != nil {
			return nil, err
		}
		if len(awaiting) > 0 {
			return nil, nil
		}
	}
	deadline, err := themeRankingDeadline(ctx, tx, "zero_to_100_games", lobbyID, gameID, false)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE zero_to_100_games SET phase = 'ranking_themes', theme_phase_deadline = $2 WHERE id = $1
	`, gameID, deadline); err != nil {
		return nil, err
	}
	return &deadline, nil
}

func (a *api) handleRankZeroToHundredThemes(w http.ResponseWriter, r *http.Request) {
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

	gameID, phase, err := lockActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "zero_to_100_not_started", "The 0 à 10 game has not started yet")
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

	candidates, err := zeroToHundredThemeCandidates(r.Context(), tx, gameID)
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
		INSERT INTO zero_to_100_theme_rankings (game_id, voter_player_id, ranking)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, voter_player_id) DO UPDATE SET ranking = $3, submitted_at = now()
	`, gameID, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	nextRoundID, shouldScheduleNext, err := a.tryConcludeZeroToHundredRanking(r.Context(), tx, gameID, player.LobbyID, candidates, false)
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
	if shouldScheduleNext {
		a.scheduleZeroToHundredRoundTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) tryConcludeZeroToHundredRanking(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, candidates []string, force bool,
) (nextRoundID string, shouldScheduleNext bool, err error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, zeroToHundredThemeRankings, gameID, 0)
		if err != nil {
			return "", false, err
		}
		if len(awaiting) > 0 {
			return "", false, nil
		}
	}

	rows, err := tx.Query(ctx, `SELECT ranking FROM zero_to_100_theme_rankings WHERE game_id = $1`, gameID)
	if err != nil {
		return "", false, err
	}
	rankings := make([][]string, 0)
	for rows.Next() {
		var rankingJSON []byte
		if err := rows.Scan(&rankingJSON); err != nil {
			rows.Close()
			return "", false, err
		}
		var ranking []string
		if err := json.Unmarshal(rankingJSON, &ranking); err != nil {
			rows.Close()
			return "", false, err
		}
		rankings = append(rankings, ranking)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", false, err
	}
	rows.Close()

	selectedThemes := tallyThemeRanking(candidates, rankings)
	if len(selectedThemes) == 0 {
		return "", false, errors.New("zero to 100 ranking produced no selected themes")
	}
	for index, themeLabel := range selectedThemes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO zero_to_100_selected_themes (game_id, position, theme_label) VALUES ($1, $2, $3)
		`, gameID, index+1, themeLabel); err != nil {
			return "", false, err
		}
	}

	roundID, err := a.startZeroToHundredRound(ctx, tx, gameID, lobbyID, 1, selectedThemes[0])
	if err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE zero_to_100_games SET phase = 'playing' WHERE id = $1`, gameID); err != nil {
		return "", false, err
	}
	return roundID, true, nil
}

func (a *api) scheduleZeroToHundredThemeSubmissionTimeout(code, gameID string, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceZeroToHundredThemeSubmissionDeadline(code, gameID)
	})
}

func (a *api) forceZeroToHundredThemeSubmissionDeadline(code, gameID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin zero to 100 theme submission deadline transaction", "game_id", gameID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase, lobbyID string
	if err := tx.QueryRow(ctx, `SELECT phase, lobby_id FROM zero_to_100_games WHERE id = $1 FOR UPDATE`, gameID).Scan(&phase, &lobbyID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load zero to 100 game for theme submission deadline", "game_id", gameID, "error", err)
		}
		return
	}
	if phase != "collecting_themes" {
		return
	}
	rankingDeadline, err := a.tryAdvanceZeroToHundredThemeCollection(ctx, tx, gameID, lobbyID, true)
	if err != nil {
		a.logger.Error("unable to force zero to 100 theme collection deadline", "game_id", gameID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for zero to 100 theme submission deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit zero to 100 theme submission deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	if rankingDeadline != nil {
		a.scheduleZeroToHundredThemeRankingTimeout(code, gameID, *rankingDeadline)
	}
}

func (a *api) scheduleZeroToHundredThemeRankingTimeout(code, gameID string, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceZeroToHundredThemeRankingDeadline(code, gameID)
	})
}

func (a *api) forceZeroToHundredThemeRankingDeadline(code, gameID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin zero to 100 theme ranking deadline transaction", "game_id", gameID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase, lobbyID string
	if err := tx.QueryRow(ctx, `SELECT phase, lobby_id FROM zero_to_100_games WHERE id = $1 FOR UPDATE`, gameID).Scan(&phase, &lobbyID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load zero to 100 game for theme ranking deadline", "game_id", gameID, "error", err)
		}
		return
	}
	if phase != "ranking_themes" {
		return
	}
	candidates, err := zeroToHundredThemeCandidates(ctx, tx, gameID)
	if err != nil {
		a.logger.Error("unable to load zero to 100 theme candidates for ranking deadline", "game_id", gameID, "error", err)
		return
	}
	nextRoundID, shouldScheduleNext, err := a.tryConcludeZeroToHundredRanking(ctx, tx, gameID, lobbyID, candidates, true)
	if err != nil {
		a.logger.Error("unable to force zero to 100 theme ranking deadline", "game_id", gameID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for zero to 100 theme ranking deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit zero to 100 theme ranking deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	if shouldScheduleNext {
		a.scheduleZeroToHundredRoundTimeout(code, nextRoundID)
	}
}

func zeroToHundredThemeCandidates(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label
		FROM zero_to_100_theme_submissions
		WHERE game_id = $1 AND theme_label IS NOT NULL AND theme_label <> ''
		ORDER BY submitted_at, player_id
	`, gameID)
	if err != nil {
		return nil, err
	}
	candidates := make([]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			rows.Close()
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(label))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, label)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, fallback := range defaultZeroToHundredThemes() {
		if len(candidates) >= 3 {
			break
		}
		key := strings.ToLower(fallback)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, fallback)
	}
	return candidates, nil
}

// --- rounds: nominees, guesses, scoring, progression ------------------------

// selectZeroToHundredNominees draws up to five distinct active players at
// random (everyone when fewer are active). The draw is independent each
// round: someone can be nominated again.
func selectZeroToHundredNominees(playerIDs []string) ([]string, error) {
	if len(playerIDs) < zeroToHundredMinPlayerCount {
		return nil, errors.New("zero to 100 requires at least three players")
	}
	shuffled := make([]string, len(playerIDs))
	copy(shuffled, playerIDs)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled[:min(len(shuffled), zeroToHundredNomineeCount)], nil
}

func (a *api) startZeroToHundredRound(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, roundNumber int, themeLabel string,
) (string, error) {
	playerIDs, err := activeLobbyPlayerIDs(ctx, tx, lobbyID)
	if err != nil {
		return "", err
	}
	nomineeIDs, err := selectZeroToHundredNominees(playerIDs)
	if err != nil {
		return "", err
	}
	var roundID string
	deadline := time.Now().Add(zeroToHundredGuessWindow)
	if err := tx.QueryRow(ctx, `
		INSERT INTO zero_to_100_rounds (game_id, round_number, theme_label, submission_deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, gameID, roundNumber, themeLabel, deadline).Scan(&roundID); err != nil {
		return "", err
	}
	for seat, playerID := range nomineeIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO zero_to_100_nominees (round_id, player_id, seat) VALUES ($1, $2, $3)
		`, roundID, playerID, seat); err != nil {
			return "", err
		}
	}
	return roundID, nil
}

func (a *api) scheduleZeroToHundredRoundTimeout(code, roundID string) {
	time.AfterFunc(zeroToHundredGuessWindow, func() {
		a.forceZeroToHundredGuessDeadline(code, roundID)
	})
}

func (a *api) forceZeroToHundredGuessDeadline(code, roundID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin zero to 100 deadline transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase, lobbyID string
	if err := tx.QueryRow(ctx, `
		SELECT round.phase, game.lobby_id
		FROM zero_to_100_rounds AS round
		JOIN zero_to_100_games AS game ON game.id = round.game_id
		WHERE round.id = $1
		FOR UPDATE OF round
	`, roundID).Scan(&phase, &lobbyID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load zero to 100 round for deadline", "round_id", roundID, "error", err)
		}
		return
	}
	if phase != "guessing" {
		return
	}
	if err := a.closeZeroToHundredGuessing(ctx, tx, roundID); err != nil {
		a.logger.Error("unable to close zero to 100 guessing window", "round_id", roundID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for zero to 100 guess deadline", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit zero to 100 deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
}

// closeZeroToHundredGuessing scores every submitted guess against each
// nominee's target, then opens the shared results screen.
func (a *api) closeZeroToHundredGuessing(ctx context.Context, tx pgx.Tx, roundID string) error {
	if err := scoreZeroToHundredRound(ctx, tx, roundID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE zero_to_100_rounds SET phase = 'results' WHERE id = $1`, roundID)
	return err
}

func (a *api) handleSubmitZeroToHundredGuess(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Positions map[string]int `json:"positions"`
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

	gameID, gamePhase, err := lockActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "zero_to_100_not_playing", "No round is currently open for guesses")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentZeroToHundredRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "guessing" {
		writeError(w, http.StatusConflict, "zero_to_100_round_locked", "This round no longer accepts guesses")
		return
	}

	nomineeIDs, err := loadZeroToHundredNomineeIDs(r.Context(), tx, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if len(input.Positions) != len(nomineeIDs) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_positions", "Submit exactly one position per nominee")
		return
	}
	for _, nomineeID := range nomineeIDs {
		value, hasValue := input.Positions[nomineeID]
		if !hasValue || value < 0 || value > zeroToHundredScaleMax {
			writeError(w, http.StatusUnprocessableEntity, "invalid_positions", "Every position must be between 0 and 10")
			return
		}
	}
	for nomineeID, value := range input.Positions {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO zero_to_100_guesses (round_id, guesser_player_id, nominee_player_id, position)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (round_id, guesser_player_id, nominee_player_id) DO UPDATE SET position = $4, created_at = now()
		`, round.ID, player.ID, nomineeID, value); err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	awaiting, err := zeroToHundredGuessAwaitingIDs(r.Context(), tx, player.LobbyID, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if len(awaiting) == 0 {
		if err := a.closeZeroToHundredGuessing(r.Context(), tx, round.ID); err != nil {
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

// scoreZeroToHundredRound persists every guesser's round score: one point on
// each guess whose nominee they ranked at the same place as on the podium,
// their own self-placement included, and zero on the others.
func scoreZeroToHundredRound(ctx context.Context, tx pgx.Tx, roundID string) error {
	guesses, err := loadZeroToHundredRoundGuesses(ctx, tx, roundID)
	if err != nil {
		return err
	}
	truthByNominee, positionsByGuesser := groupZeroToHundredGuesses(guesses)
	targets := zeroToHundredTargets(truthByNominee, zeroToHundredAverages(guesses))

	for guesser, positions := range positionsByGuesser {
		score := scoreZeroToHundredGuesser(positions, targets)
		for nominee := range positions {
			points := 0
			if score.Correct[nominee] {
				points = 1
			}
			if _, err := tx.Exec(ctx, `
				UPDATE zero_to_100_guesses SET points = $1
				WHERE round_id = $2 AND guesser_player_id = $3 AND nominee_player_id = $4
			`, points, roundID, guesser, nominee); err != nil {
				return err
			}
		}
	}
	return nil
}

// zeroToHundredGuesserScore is how one guesser ranked the nominees that have
// a target, and which of those ranks match the podium.
type zeroToHundredGuesserScore struct {
	Ranks        map[string]int
	Correct      map[string]bool
	CorrectCount int
	PerfectOrder bool
}

// scoreZeroToHundredGuesser ranks the nominees that have a target by the
// guesser's positions, highest first, and checks each rank against the
// podium. A nominee is well placed when the guesser's rank falls within the
// ranks their target spans: nominees tied on the podium accept either order,
// while a tie the guesser made only gets the better rank. A perfect order
// needs every one of at least two targets well placed, which is the same as
// ranking every pair of targets the right way round.
func scoreZeroToHundredGuesser(positions map[string]int, targets map[string]float64) zeroToHundredGuesserScore {
	placed := make(map[string]int, len(targets))
	for nominee := range targets {
		if position, hasPosition := positions[nominee]; hasPosition {
			placed[nominee] = position
		}
	}
	targetRanks := zeroToHundredRanks(targets)
	score := zeroToHundredGuesserScore{
		Ranks:   zeroToHundredRanks(placed),
		Correct: make(map[string]bool, len(placed)),
	}
	for nominee, rank := range score.Ranks {
		tiedCount := 0
		for _, other := range targets {
			if other == targets[nominee] {
				tiedCount++
			}
		}
		targetRank := targetRanks[nominee]
		if rank >= targetRank && rank < targetRank+tiedCount {
			score.Correct[nominee] = true
			score.CorrectCount++
		}
	}
	score.PerfectOrder = len(targets) >= 2 && score.CorrectCount == len(targets)
	return score
}

// zeroToHundredRanks ranks values from the highest down: 1 for the highest,
// equal values sharing the best rank of their group (1, 2, 2, 4).
func zeroToHundredRanks[V int | float64](values map[string]V) map[string]int {
	ranks := make(map[string]int, len(values))
	for id, value := range values {
		rank := 1
		for _, other := range values {
			if other > value {
				rank++
			}
		}
		ranks[id] = rank
	}
	return ranks
}

// groupZeroToHundredGuesses indexes a round's guesses by nominee self-placement
// (the truth) and by guesser (every position they submitted, their own
// self-placement included).
func groupZeroToHundredGuesses(guesses []zeroToHundredGuessRow) (truthByNominee map[string]int, positionsByGuesser map[string]map[string]int) {
	truthByNominee = make(map[string]int)
	positionsByGuesser = make(map[string]map[string]int)
	for _, guess := range guesses {
		if guess.Guesser == guess.Nominee {
			truthByNominee[guess.Nominee] = guess.Position
		}
		if positionsByGuesser[guess.Guesser] == nil {
			positionsByGuesser[guess.Guesser] = make(map[string]int)
		}
		positionsByGuesser[guess.Guesser][guess.Nominee] = guess.Position
	}
	return truthByNominee, positionsByGuesser
}

// zeroToHundredAverages averages the positions every player gave each nominee,
// self-placements excluded.
func zeroToHundredAverages(guesses []zeroToHundredGuessRow) map[string]float64 {
	sumByNominee := make(map[string]int)
	countByNominee := make(map[string]int)
	for _, guess := range guesses {
		if guess.Guesser == guess.Nominee {
			continue
		}
		sumByNominee[guess.Nominee] += guess.Position
		countByNominee[guess.Nominee]++
	}
	averages := make(map[string]float64, len(countByNominee))
	for nominee, count := range countByNominee {
		averages[nominee] = float64(sumByNominee[nominee]) / float64(count)
	}
	return averages
}

// zeroToHundredTargets blends each nominee's self-placement
// (zeroToHundredSelfPlacementWeight) with the group's average position for
// them, rounded to the displayed decimal. A nominee nobody else placed keeps
// their self-placement; one who never placed themselves gets no target.
func zeroToHundredTargets(truthByNominee map[string]int, averages map[string]float64) map[string]float64 {
	targets := make(map[string]float64, len(truthByNominee))
	for nominee, selfPlacement := range truthByNominee {
		target := float64(selfPlacement)
		if average, hasAverage := averages[nominee]; hasAverage {
			target = zeroToHundredSelfPlacementWeight*target + (1-zeroToHundredSelfPlacementWeight)*average
		}
		targets[nominee] = roundToTenth(target)
	}
	return targets
}

func roundToTenth(value float64) float64 {
	return math.Round(value*10) / 10
}

func (a *api) handleStartNextZeroToHundredRound(w http.ResponseWriter, r *http.Request) {
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

	gameID, gamePhase, err := lockActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "zero_to_100_not_playing", "No round is currently in progress")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentZeroToHundredRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "results" {
		writeError(w, http.StatusConflict, "zero_to_100_round_locked", "This round has not finished yet")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE zero_to_100_rounds SET phase = 'completed', completed_at = now() WHERE id = $1
	`, round.ID); err != nil {
		a.internalError(w, r, err)
		return
	}

	var nextRoundID string
	shouldScheduleNext := false
	if round.RoundNumber >= zeroToHundredRoundCount {
		if _, err := tx.Exec(r.Context(), `
			UPDATE zero_to_100_games SET phase = 'completed', completed_at = now() WHERE id = $1
		`, gameID); err != nil {
			a.internalError(w, r, err)
			return
		}
	} else {
		var nextThemeLabel string
		if err := tx.QueryRow(r.Context(), `
			SELECT theme_label FROM zero_to_100_selected_themes WHERE game_id = $1 AND position = $2
		`, gameID, round.RoundNumber+1).Scan(&nextThemeLabel); err != nil {
			a.internalError(w, r, err)
			return
		}
		nextRoundID, err = a.startZeroToHundredRound(r.Context(), tx, gameID, player.LobbyID, round.RoundNumber+1, nextThemeLabel)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		shouldScheduleNext = true
	}

	if err := bumpLobbyRevision(r.Context(), tx, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if shouldScheduleNext {
		a.scheduleZeroToHundredRoundTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleReplayZeroToHundred(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can start a new cycle")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	_, gamePhase, err := lockActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && gamePhase != "completed" {
		writeError(w, http.StatusConflict, "zero_to_100_in_progress", "The current cycle has not finished yet")
		return
	}

	var gameID string
	themeDeadline := time.Now().Add(themeStepWindow(themeProposalStep, false))
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO zero_to_100_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, player.LobbyID, themeDeadline).Scan(&gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET status = 'in_game' WHERE id = $1
	`, player.LobbyID); err != nil {
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
	a.scheduleZeroToHundredThemeSubmissionTimeout(player.Code, gameID, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- shared row loaders ------------------------------------------------

func loadActiveZeroToHundredGame(ctx context.Context, db dbQuerier, lobbyID string) (id, phase string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, phase FROM zero_to_100_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

// lockActiveZeroToHundredGame is loadActiveZeroToHundredGame for write paths: the row lock
// serializes every command, deadline and departure of a lobby, so "everyone
// answered" checks never miss a concurrent answer or departure.
func lockActiveZeroToHundredGame(ctx context.Context, tx pgx.Tx, lobbyID string) (id, phase string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id, phase FROM zero_to_100_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

type zeroToHundredRoundRow struct {
	ID                 string
	RoundNumber        int
	ThemeLabel         string
	Phase              string
	SubmissionDeadline time.Time
}

func loadCurrentZeroToHundredRound(ctx context.Context, db dbQuerier, gameID string) (zeroToHundredRoundRow, error) {
	var round zeroToHundredRoundRow
	err := db.QueryRow(ctx, `
		SELECT id, round_number, theme_label, phase, submission_deadline
		FROM zero_to_100_rounds
		WHERE game_id = $1
		ORDER BY round_number DESC
		LIMIT 1
	`, gameID).Scan(&round.ID, &round.RoundNumber, &round.ThemeLabel, &round.Phase, &round.SubmissionDeadline)
	return round, err
}

func loadZeroToHundredNomineeIDs(ctx context.Context, db dbQuerier, roundID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT player_id FROM zero_to_100_nominees WHERE round_id = $1 ORDER BY seat
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, zeroToHundredNomineeCount)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type zeroToHundredNomineeRow struct {
	PlayerID    string
	DisplayName string
}

func loadZeroToHundredNominees(ctx context.Context, db dbQuerier, roundID string) ([]zeroToHundredNomineeRow, error) {
	rows, err := db.Query(ctx, `
		SELECT nominee.player_id, player.display_name
		FROM zero_to_100_nominees AS nominee
		JOIN players AS player ON player.id = nominee.player_id
		WHERE nominee.round_id = $1
		ORDER BY nominee.seat
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nominees := make([]zeroToHundredNomineeRow, 0, zeroToHundredNomineeCount)
	for rows.Next() {
		var nominee zeroToHundredNomineeRow
		if err := rows.Scan(&nominee.PlayerID, &nominee.DisplayName); err != nil {
			return nil, err
		}
		nominees = append(nominees, nominee)
	}
	return nominees, rows.Err()
}

func loadZeroToHundredSelectedThemes(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label FROM zero_to_100_selected_themes WHERE game_id = $1 ORDER BY position
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	themes := make([]string, 0, zeroToHundredRoundCount)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		themes = append(themes, label)
	}
	return themes, rows.Err()
}

type zeroToHundredGuessRow struct {
	Guesser     string
	GuesserName string
	Nominee     string
	Position    int
}

func loadZeroToHundredRoundGuesses(ctx context.Context, db dbQuerier, roundID string) ([]zeroToHundredGuessRow, error) {
	rows, err := db.Query(ctx, `
		SELECT guess.guesser_player_id, guesser.display_name, guess.nominee_player_id, guess.position
		FROM zero_to_100_guesses AS guess
		JOIN players AS guesser ON guesser.id = guess.guesser_player_id
		WHERE guess.round_id = $1
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	guesses := make([]zeroToHundredGuessRow, 0)
	for rows.Next() {
		var guess zeroToHundredGuessRow
		if err := rows.Scan(&guess.Guesser, &guess.GuesserName, &guess.Nominee, &guess.Position); err != nil {
			return nil, err
		}
		guesses = append(guesses, guess)
	}
	return guesses, rows.Err()
}

// loadZeroToHundredScoredGuesses returns the guesses of every round of the
// game that has been scored (in results or completed), by round ID.
func loadZeroToHundredScoredGuesses(ctx context.Context, db dbQuerier, gameID string) (map[string][]zeroToHundredGuessRow, error) {
	rows, err := db.Query(ctx, `
		SELECT guess.round_id, guess.guesser_player_id, guess.nominee_player_id, guess.position
		FROM zero_to_100_guesses AS guess
		JOIN zero_to_100_rounds AS round ON round.id = guess.round_id
		WHERE round.game_id = $1 AND round.phase IN ('results', 'completed')
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	guessesByRound := make(map[string][]zeroToHundredGuessRow)
	for rows.Next() {
		var roundID string
		var guess zeroToHundredGuessRow
		if err := rows.Scan(&roundID, &guess.Guesser, &guess.Nominee, &guess.Position); err != nil {
			return nil, err
		}
		guessesByRound[roundID] = append(guessesByRound[roundID], guess)
	}
	return guessesByRound, rows.Err()
}

// buildZeroToHundredReveal lays out the round results, the same for every
// player apart from IsCurrentPlayer. The reveal follows seat order (the seat
// picks each nominee's color): self-placement, group average, target and
// podium rank. The guessers list everyone who answered with their ranking of
// the nominees that have a target, best score first, ties by name.
func buildZeroToHundredReveal(
	nominees []zeroToHundredNomineeRow, guesses []zeroToHundredGuessRow, currentPlayerID string,
) (reveal []zeroToHundredRevealEntryView, guessers []zeroToHundredGuesserView) {
	truthByNominee, positionsByGuesser := groupZeroToHundredGuesses(guesses)
	averages := zeroToHundredAverages(guesses)
	targets := zeroToHundredTargets(truthByNominee, averages)
	targetRanks := zeroToHundredRanks(targets)

	seatByNominee := make(map[string]int, len(nominees))
	reveal = make([]zeroToHundredRevealEntryView, 0, len(nominees))
	for seat, nominee := range nominees {
		seatByNominee[nominee.PlayerID] = seat
		entry := zeroToHundredRevealEntryView{
			PlayerID:        nominee.PlayerID,
			DisplayName:     nominee.DisplayName,
			IsCurrentPlayer: nominee.PlayerID == currentPlayerID,
		}
		if truth, hasTruth := truthByNominee[nominee.PlayerID]; hasTruth {
			entry.TruePosition = &truth
		}
		if average, hasAverage := averages[nominee.PlayerID]; hasAverage {
			rounded := roundToTenth(average)
			entry.AveragePosition = &rounded
		}
		if target, hasTarget := targets[nominee.PlayerID]; hasTarget {
			rank := targetRanks[nominee.PlayerID]
			entry.TargetPosition = &target
			entry.TargetRank = &rank
		}
		reveal = append(reveal, entry)
	}

	nameByGuesser := make(map[string]string, len(positionsByGuesser))
	for _, guess := range guesses {
		nameByGuesser[guess.Guesser] = guess.GuesserName
	}
	guessers = make([]zeroToHundredGuesserView, 0, len(positionsByGuesser))
	for guesser, positions := range positionsByGuesser {
		score := scoreZeroToHundredGuesser(positions, targets)
		placements := make([]zeroToHundredPlacementView, 0, len(score.Ranks))
		for nominee, rank := range score.Ranks {
			placements = append(placements, zeroToHundredPlacementView{
				NomineeID: nominee,
				Position:  positions[nominee],
				Rank:      rank,
				Correct:   score.Correct[nominee],
			})
		}
		slices.SortFunc(placements, func(left, right zeroToHundredPlacementView) int {
			return cmp.Or(cmp.Compare(left.Rank, right.Rank), cmp.Compare(seatByNominee[left.NomineeID], seatByNominee[right.NomineeID]))
		})
		guessers = append(guessers, zeroToHundredGuesserView{
			PlayerID:        guesser,
			DisplayName:     nameByGuesser[guesser],
			IsCurrentPlayer: guesser == currentPlayerID,
			Placements:      placements,
			CorrectCount:    score.CorrectCount,
			PerfectOrder:    score.PerfectOrder,
		})
	}
	slices.SortFunc(guessers, func(left, right zeroToHundredGuesserView) int {
		return cmp.Or(
			cmp.Compare(right.CorrectCount, left.CorrectCount),
			cmp.Compare(left.DisplayName, right.DisplayName),
			cmp.Compare(left.PlayerID, right.PlayerID),
		)
	})
	return reveal, guessers
}

// buildZeroToHundredLeaderboard totals each player's well-placed nominees and
// perfect orders over the scored rounds, best score first, ties by name.
// Players who left are not listed, even if they scored.
func buildZeroToHundredLeaderboard(
	players []zeroToHundredNomineeRow, guessesByRound map[string][]zeroToHundredGuessRow,
) []zeroToHundredLeaderboardEntryView {
	indexByPlayer := make(map[string]int, len(players))
	leaderboard := make([]zeroToHundredLeaderboardEntryView, 0, len(players))
	for index, player := range players {
		indexByPlayer[player.PlayerID] = index
		leaderboard = append(leaderboard, zeroToHundredLeaderboardEntryView{PlayerID: player.PlayerID, DisplayName: player.DisplayName})
	}
	for _, guesses := range guessesByRound {
		truthByNominee, positionsByGuesser := groupZeroToHundredGuesses(guesses)
		targets := zeroToHundredTargets(truthByNominee, zeroToHundredAverages(guesses))
		for guesser, positions := range positionsByGuesser {
			index, isListed := indexByPlayer[guesser]
			if !isListed {
				continue
			}
			score := scoreZeroToHundredGuesser(positions, targets)
			leaderboard[index].Score += score.CorrectCount
			if score.PerfectOrder {
				leaderboard[index].PerfectOrders++
			}
		}
	}
	slices.SortFunc(leaderboard, func(left, right zeroToHundredLeaderboardEntryView) int {
		return cmp.Or(cmp.Compare(right.Score, left.Score), cmp.Compare(left.DisplayName, right.DisplayName))
	})
	return leaderboard
}

func loadZeroToHundredLeaderboard(ctx context.Context, db dbQuerier, gameID string) ([]zeroToHundredLeaderboardEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT player.id, player.display_name
		FROM players AS player
		WHERE player.lobby_id = (SELECT lobby_id FROM zero_to_100_games WHERE id = $1)
		  AND player.is_active
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := make([]zeroToHundredNomineeRow, 0)
	for rows.Next() {
		var player zeroToHundredNomineeRow
		if err := rows.Scan(&player.PlayerID, &player.DisplayName); err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	guessesByRound, err := loadZeroToHundredScoredGuesses(ctx, db, gameID)
	if err != nil {
		return nil, err
	}
	return buildZeroToHundredLeaderboard(players, guessesByRound), nil
}

// loadZeroToHundredState builds the 0 à 10 view of the lobby state for the
// current player, mirroring loadFastBioState's role for the Fast Bio mode.
// loadZeroToHundredThemeProgress populates the deadline, the "X of Y" progress
// counters and the awaited players shared by both theme sub-phases.
func (a *api) loadZeroToHundredThemeProgress(ctx context.Context, gameID, lobbyID string, table themeStepTable, view *zeroToHundredStateView) error {
	var deadline *time.Time
	if err := a.pool.QueryRow(ctx, `
		SELECT theme_phase_deadline FROM zero_to_100_games WHERE id = $1
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

func (a *api) loadZeroToHundredState(ctx context.Context, currentPlayer authenticatedPlayer) (zeroToHundredStateView, error) {
	gameID, gamePhase, err := loadActiveZeroToHundredGame(ctx, a.pool, currentPlayer.LobbyID)
	if err != nil {
		return zeroToHundredStateView{}, err
	}
	view := zeroToHundredStateView{ID: gameID, Phase: gamePhase}

	switch gamePhase {
	case "collecting_themes":
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM zero_to_100_theme_submissions WHERE game_id = $1 AND player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return zeroToHundredStateView{}, err
		}
		view.ThemeSubmitted = exists
		if err := a.loadZeroToHundredThemeProgress(ctx, gameID, currentPlayer.LobbyID, zeroToHundredThemeSubmissions, &view); err != nil {
			return zeroToHundredStateView{}, err
		}

	case "ranking_themes":
		view.ThemeSubmitted = true
		candidates, err := zeroToHundredThemeCandidates(ctx, a.pool, gameID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.ThemeCandidates = candidates
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM zero_to_100_theme_rankings WHERE game_id = $1 AND voter_player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return zeroToHundredStateView{}, err
		}
		view.ThemeRanked = exists
		if err := a.loadZeroToHundredThemeProgress(ctx, gameID, currentPlayer.LobbyID, zeroToHundredThemeRankings, &view); err != nil {
			return zeroToHundredStateView{}, err
		}

	case "playing":
		view.ThemeSubmitted = true
		view.ThemeRanked = true
		selectedThemes, err := loadZeroToHundredSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.SelectedThemes = selectedThemes

		round, err := loadCurrentZeroToHundredRound(ctx, a.pool, gameID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.RoundNumber = round.RoundNumber
		view.TotalRounds = zeroToHundredRoundCount
		view.RoundPhase = round.Phase
		view.ThemeLabel = round.ThemeLabel
		if round.Phase == "guessing" {
			deadline := round.SubmissionDeadline
			view.SubmissionDeadline = &deadline
			requiredCount, err := activeLobbyPlayerCount(ctx, a.pool, currentPlayer.LobbyID)
			if err != nil {
				return zeroToHundredStateView{}, err
			}
			awaiting, err := zeroToHundredGuessAwaitingIDs(ctx, a.pool, currentPlayer.LobbyID, round.ID)
			if err != nil {
				return zeroToHundredStateView{}, err
			}
			progress := newStepProgress(requiredCount, awaiting)
			view.SubmissionProgressRequired = progress.required
			view.SubmissionProgressCount = progress.count
			view.AwaitingPlayerIDs = progress.awaiting
		}

		nominees, err := loadZeroToHundredNominees(ctx, a.pool, round.ID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.Nominees = make([]zeroToHundredNomineeView, 0, len(nominees))
		for _, nominee := range nominees {
			isCurrentPlayer := nominee.PlayerID == currentPlayer.ID
			if isCurrentPlayer {
				view.IsNominee = true
			}
			view.Nominees = append(view.Nominees, zeroToHundredNomineeView{
				PlayerID:        nominee.PlayerID,
				DisplayName:     nominee.DisplayName,
				IsCurrentPlayer: isCurrentPlayer,
			})
		}

		var submittedCount int
		if err := a.pool.QueryRow(ctx, `
			SELECT count(*) FROM zero_to_100_guesses WHERE round_id = $1 AND guesser_player_id = $2
		`, round.ID, currentPlayer.ID).Scan(&submittedCount); err != nil {
			return zeroToHundredStateView{}, err
		}
		view.Submitted = len(nominees) > 0 && submittedCount >= len(nominees)

		if round.Phase == "results" || round.Phase == "completed" {
			guesses, err := loadZeroToHundredRoundGuesses(ctx, a.pool, round.ID)
			if err != nil {
				return zeroToHundredStateView{}, err
			}
			view.Reveal, view.Guessers = buildZeroToHundredReveal(nominees, guesses, currentPlayer.ID)
		}

	case "completed":
		leaderboard, err := loadZeroToHundredLeaderboard(ctx, a.pool, gameID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.Leaderboard = leaderboard
		selectedThemes, err := loadZeroToHundredSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return zeroToHundredStateView{}, err
		}
		view.SelectedThemes = selectedThemes
	}

	return view, nil
}
