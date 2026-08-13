package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/example/crush-club/apps/api/internal/scoring"
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
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a 0 à 100 lobby")
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
	if len(playerIDs) < zeroToHundredNomineeCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "0 à 100 needs at least three players")
		return
	}
	for _, id := range playerIDs {
		if !a.hub.isOnline(player.Code, id) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	var gameID string
	themeDeadline := time.Now().Add(themePhaseWindow)
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO zero_to_100_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, player.LobbyID, themeDeadline).Scan(&gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET status = 'in_game', revision = revision + 1, updated_at = now() WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleZeroToHundredThemeSubmissionTimeout(player.Code, gameID)
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

	gameID, phase, err := loadActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "zero_to_100_not_started", "The 0 à 100 game has not started yet")
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
	advanced, err := a.tryAdvanceZeroToHundredThemeCollection(r.Context(), tx, gameID, player.LobbyID, false)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if advanced {
		a.scheduleZeroToHundredThemeRankingTimeout(player.Code, gameID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) tryAdvanceZeroToHundredThemeCollection(ctx context.Context, tx pgx.Tx, gameID, lobbyID string, force bool) (advanced bool, err error) {
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return false, err
	}
	var submittedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM zero_to_100_theme_submissions WHERE game_id = $1
	`, gameID).Scan(&submittedCount); err != nil {
		return false, err
	}
	if !force && submittedCount < activeCount {
		return false, nil
	}
	deadline := time.Now().Add(themePhaseWindow)
	if _, err := tx.Exec(ctx, `
		UPDATE zero_to_100_games SET phase = 'ranking_themes', theme_phase_deadline = $2 WHERE id = $1
	`, gameID, deadline); err != nil {
		return false, err
	}
	return true, nil
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

	gameID, phase, err := loadActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "zero_to_100_not_started", "The 0 à 100 game has not started yet")
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
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return "", false, err
	}
	var votedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM zero_to_100_theme_rankings WHERE game_id = $1
	`, gameID).Scan(&votedCount); err != nil {
		return "", false, err
	}
	if !force && votedCount < activeCount {
		return "", false, nil
	}

	rows, err := tx.Query(ctx, `SELECT ranking FROM zero_to_100_theme_rankings WHERE game_id = $1`, gameID)
	if err != nil {
		return "", false, err
	}
	rankings := make([][]string, 0, activeCount)
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

func (a *api) scheduleZeroToHundredThemeSubmissionTimeout(code, gameID string) {
	time.AfterFunc(themePhaseWindow, func() {
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
	advanced, err := a.tryAdvanceZeroToHundredThemeCollection(ctx, tx, gameID, lobbyID, true)
	if err != nil {
		a.logger.Error("unable to force zero to 100 theme collection deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit zero to 100 theme submission deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	if advanced {
		a.scheduleZeroToHundredThemeRankingTimeout(code, gameID)
	}
}

func (a *api) scheduleZeroToHundredThemeRankingTimeout(code, gameID string) {
	time.AfterFunc(themePhaseWindow, func() {
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

// selectZeroToHundredNominees draws three distinct active players at random.
// The draw is independent each round: someone can be nominated again.
func selectZeroToHundredNominees(playerIDs []string) ([]string, error) {
	if len(playerIDs) < zeroToHundredNomineeCount {
		return nil, errors.New("zero to 100 requires at least three players")
	}
	shuffled := make([]string, len(playerIDs))
	copy(shuffled, playerIDs)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled[:zeroToHundredNomineeCount], nil
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

	var phase string
	if err := tx.QueryRow(ctx, `SELECT phase FROM zero_to_100_rounds WHERE id = $1 FOR UPDATE`, roundID).Scan(&phase); err != nil {
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
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit zero to 100 deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
}

// closeZeroToHundredGuessing scores every submitted guess against each
// nominee's own self-placed truth, then opens the shared results screen.
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

	gameID, gamePhase, err := loadActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
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
		if !hasValue || value < 0 || value > 100 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_positions", "Every position must be between 0 and 100")
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

	activeCount, err := activeLobbyPlayerCount(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	var submittedCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(DISTINCT guesser_player_id) FROM zero_to_100_guesses WHERE round_id = $1
	`, round.ID).Scan(&submittedCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if submittedCount >= activeCount {
		if err := a.closeZeroToHundredGuessing(r.Context(), tx, round.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// scoreZeroToHundredRound computes and persists every guesser's points for
// the round: up to zeroToHundredProximityMaximumScore per nominee based on
// distance to that nominee's self-placed truth, plus zeroToHundredOrderBonus
// once (per guesser) if the relative order of their three guesses exactly
// matches the true relative order.
func scoreZeroToHundredRound(ctx context.Context, tx pgx.Tx, roundID string) error {
	rows, err := tx.Query(ctx, `
		SELECT guesser_player_id, nominee_player_id, position
		FROM zero_to_100_guesses WHERE round_id = $1
	`, roundID)
	if err != nil {
		return err
	}
	type guessRow struct {
		guesser, nominee string
		position         int
	}
	allGuesses := make([]guessRow, 0)
	for rows.Next() {
		var row guessRow
		if err := rows.Scan(&row.guesser, &row.nominee, &row.position); err != nil {
			rows.Close()
			return err
		}
		allGuesses = append(allGuesses, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	truthByNominee := make(map[string]int)
	guessesByGuesser := make(map[string]map[string]int)
	for _, row := range allGuesses {
		if row.guesser == row.nominee {
			truthByNominee[row.nominee] = row.position
			continue
		}
		if guessesByGuesser[row.guesser] == nil {
			guessesByGuesser[row.guesser] = make(map[string]int)
		}
		guessesByGuesser[row.guesser][row.nominee] = row.position
	}
	if len(truthByNominee) == 0 {
		return nil
	}
	trueOrder := sortNomineesByPosition(truthByNominee)

	for guesser, nomineeGuesses := range guessesByGuesser {
		orderBonus := 0
		if guessOrderMatchesTruth(nomineeGuesses, truthByNominee, trueOrder) {
			orderBonus = zeroToHundredOrderBonus
		}
		bonusApplied := false
		for nominee, guessedPosition := range nomineeGuesses {
			truth, hasTruth := truthByNominee[nominee]
			if !hasTruth {
				continue
			}
			points := scoring.IntegerRangeScore(truth, guessedPosition, 0, 100, zeroToHundredProximityMaximumScore)
			if !bonusApplied {
				points += orderBonus
				bonusApplied = true
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

func sortNomineesByPosition(positionByNominee map[string]int) []string {
	nominees := make([]string, 0, len(positionByNominee))
	for id := range positionByNominee {
		nominees = append(nominees, id)
	}
	sort.Slice(nominees, func(i, j int) bool {
		if positionByNominee[nominees[i]] != positionByNominee[nominees[j]] {
			return positionByNominee[nominees[i]] < positionByNominee[nominees[j]]
		}
		return nominees[i] < nominees[j]
	})
	return nominees
}

func guessOrderMatchesTruth(guesses, truth map[string]int, trueOrder []string) bool {
	if len(guesses) != len(truth) {
		return false
	}
	guessedPositions := make(map[string]int, len(truth))
	for nominee := range truth {
		value, hasGuess := guesses[nominee]
		if !hasGuess {
			return false
		}
		guessedPositions[nominee] = value
	}
	guessedOrder := sortNomineesByPosition(guessedPositions)
	for index := range trueOrder {
		if trueOrder[index] != guessedOrder[index] {
			return false
		}
	}
	return true
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

	gameID, gamePhase, err := loadActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
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

	_, gamePhase, err := loadActiveZeroToHundredGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && gamePhase != "completed" {
		writeError(w, http.StatusConflict, "zero_to_100_in_progress", "The current cycle has not finished yet")
		return
	}

	var gameID string
	themeDeadline := time.Now().Add(themePhaseWindow)
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO zero_to_100_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, player.LobbyID, themeDeadline).Scan(&gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET status = 'in_game', revision = revision + 1, updated_at = now() WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleZeroToHundredThemeSubmissionTimeout(player.Code, gameID)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- shared row loaders ------------------------------------------------

func loadActiveZeroToHundredGame(ctx context.Context, db dbQuerier, lobbyID string) (id, phase string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, phase FROM zero_to_100_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
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

func loadZeroToHundredReveal(ctx context.Context, db dbQuerier, roundID, currentPlayerID string) ([]zeroToHundredRevealEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT nominee.player_id, player.display_name,
		       COALESCE(truth.position, 0),
		       COALESCE((
		         SELECT avg(guess.position) FROM zero_to_100_guesses AS guess
		         WHERE guess.round_id = nominee.round_id AND guess.nominee_player_id = nominee.player_id
		           AND guess.guesser_player_id <> guess.nominee_player_id
		       ), 0),
		       (
		         SELECT position FROM zero_to_100_guesses
		         WHERE round_id = nominee.round_id AND guesser_player_id = $2 AND nominee_player_id = nominee.player_id
		           AND guesser_player_id <> nominee_player_id
		       )
		FROM zero_to_100_nominees AS nominee
		JOIN players AS player ON player.id = nominee.player_id
		LEFT JOIN zero_to_100_guesses AS truth
		  ON truth.round_id = nominee.round_id AND truth.guesser_player_id = nominee.player_id
		     AND truth.nominee_player_id = nominee.player_id
		WHERE nominee.round_id = $1
		ORDER BY nominee.seat
	`, roundID, currentPlayerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reveal := make([]zeroToHundredRevealEntryView, 0, zeroToHundredNomineeCount)
	for rows.Next() {
		var entry zeroToHundredRevealEntryView
		var myGuess *int
		if err := rows.Scan(&entry.PlayerID, &entry.DisplayName, &entry.TruePosition, &entry.AveragePosition, &myGuess); err != nil {
			return nil, err
		}
		entry.MyGuess = myGuess
		reveal = append(reveal, entry)
	}
	return reveal, rows.Err()
}

func loadZeroToHundredLeaderboard(ctx context.Context, db dbQuerier, gameID string) ([]fastBioLeaderboardEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT author.id, author.display_name,
		       COALESCE(sum(guess.points) FILTER (WHERE round.id IS NOT NULL), 0) AS total_score,
		       COALESCE(sum(guess.points) FILTER (WHERE round.round_number = $2), 0) AS round_score
		FROM players AS author
		LEFT JOIN zero_to_100_guesses AS guess
		  ON guess.guesser_player_id = author.id AND guess.guesser_player_id <> guess.nominee_player_id
		LEFT JOIN zero_to_100_rounds AS round ON round.id = guess.round_id AND round.game_id = $1
		WHERE author.lobby_id = (SELECT lobby_id FROM zero_to_100_games WHERE id = $1)
		  AND author.excluded_at IS NULL
		GROUP BY author.id, author.display_name
		ORDER BY total_score DESC, author.display_name ASC
	`, gameID, zeroToHundredRoundCount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leaderboard := make([]fastBioLeaderboardEntryView, 0)
	for rows.Next() {
		var entry fastBioLeaderboardEntryView
		if err := rows.Scan(&entry.PlayerID, &entry.DisplayName, &entry.Score, &entry.RoundScore); err != nil {
			return nil, err
		}
		leaderboard = append(leaderboard, entry)
	}
	return leaderboard, rows.Err()
}

// loadZeroToHundredState builds the 0 à 100 view of the lobby state for the
// current player, mirroring loadFastBioState's role for the Fast Bio mode.
// loadZeroToHundredThemeProgress populates the deadline and "X of Y" progress
// counters shared by both theme sub-phases. progressTable is always one of
// the two hardcoded table name constants below (never user input).
func (a *api) loadZeroToHundredThemeProgress(ctx context.Context, gameID, lobbyID, progressTable string, view *zeroToHundredStateView) error {
	var deadline *time.Time
	if err := a.pool.QueryRow(ctx, `
		SELECT theme_phase_deadline FROM zero_to_100_games WHERE id = $1
	`, gameID).Scan(&deadline); err != nil {
		return err
	}
	view.ThemeDeadline = deadline
	requiredCount, err := activeLobbyPlayerCount(ctx, a.pool, lobbyID)
	if err != nil {
		return err
	}
	view.ThemeProgressRequired = requiredCount
	var progressCount int
	query := "SELECT count(*) FROM " + progressTable + " WHERE game_id = $1"
	if err := a.pool.QueryRow(ctx, query, gameID).Scan(&progressCount); err != nil {
		return err
	}
	view.ThemeProgressCount = progressCount
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
		if err := a.loadZeroToHundredThemeProgress(ctx, gameID, currentPlayer.LobbyID, "zero_to_100_theme_submissions", &view); err != nil {
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
		if err := a.loadZeroToHundredThemeProgress(ctx, gameID, currentPlayer.LobbyID, "zero_to_100_theme_rankings", &view); err != nil {
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
			view.SubmissionProgressRequired = requiredCount
			if err := a.pool.QueryRow(ctx, `
				SELECT count(DISTINCT guesser_player_id) FROM zero_to_100_guesses WHERE round_id = $1
			`, round.ID).Scan(&view.SubmissionProgressCount); err != nil {
				return zeroToHundredStateView{}, err
			}
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
			reveal, err := loadZeroToHundredReveal(ctx, a.pool, round.ID, currentPlayer.ID)
			if err != nil {
				return zeroToHundredStateView{}, err
			}
			view.Reveal = reveal
			var roundScore int
			if err := a.pool.QueryRow(ctx, `
				SELECT COALESCE(sum(points), 0) FROM zero_to_100_guesses
				WHERE round_id = $1 AND guesser_player_id = $2 AND guesser_player_id <> nominee_player_id
			`, round.ID, currentPlayer.ID).Scan(&roundScore); err != nil {
				return zeroToHundredStateView{}, err
			}
			view.RoundScore = roundScore
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
