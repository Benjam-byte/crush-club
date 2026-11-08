package main

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// wavelengthSpectrum is a round's subject: the two opposite poles of the dial.
type wavelengthSpectrum struct {
	Left  string
	Right string
}

// label is the spectrum's identity in the shared theme-ranking flow, which
// only deals with plain strings.
func (spectrum wavelengthSpectrum) label() string {
	return spectrum.Left + " ↔ " + spectrum.Right
}

func defaultWavelengthSpectra() []wavelengthSpectrum {
	return []wavelengthSpectrum{
		{Left: "Froid", Right: "Chaud"},
		{Left: "Red flag", Right: "Green flag"},
		{Left: "Ringard", Right: "Stylé"},
		{Left: "Inutile", Right: "Indispensable"},
		{Left: "Premier date raté", Right: "Premier date parfait"},
		{Left: "Sous-coté", Right: "Surcoté"},
		{Left: "Gênant", Right: "Classe"},
		{Left: "Facile", Right: "Impossible"},
		{Left: "Mauvaise excuse", Right: "Excuse imparable"},
		{Left: "Ennuyeux", Right: "Passionnant"},
		{Left: "Sain", Right: "Malsain"},
		{Left: "Pas romantique", Right: "Ultra romantique"},
		{Left: "Talent inutile", Right: "Talent utile"},
		{Left: "Fragile", Right: "Indestructible"},
		{Left: "Pas cher", Right: "Hors de prix"},
		{Left: "Banal", Right: "Rarissime"},
		{Left: "Pas sexy", Right: "Très sexy"},
		{Left: "Pardonnable", Right: "Impardonnable"},
		{Left: "Enfantin", Right: "Très adulte"},
		{Left: "Cadeau nul", Right: "Cadeau de rêve"},
		{Left: "Rassurant", Right: "Terrifiant"},
		{Left: "Calme", Right: "Chaotique"},
		{Left: "Mauvaise idée", Right: "Idée géniale"},
		{Left: "Doux", Right: "Piquant"},
	}
}

// --- lobby start & per-round theme collection & ranking ----------------------

func (a *api) handleStartWavelengthGame(w http.ResponseWriter, r *http.Request) {
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
	if mode != lobbyModeWavelength {
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a Wavelength lobby")
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
	if len(playerIDs) < minimumPlayerCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "Wavelength needs at least two players")
		return
	}
	for _, id := range playerIDs {
		if !a.hub.isOnline(player.Code, id) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	gameID, themeDeadline, err := createWavelengthGame(r.Context(), tx, player.LobbyID, true)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleWavelengthThemeSubmissionTimeout(player.Code, gameID, 1, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// createWavelengthGame opens a new game on round 1's theme collection and
// puts the lobby in game. firstThemeSelection is true for the lobby's first
// game, which gets the full theme window.
func createWavelengthGame(ctx context.Context, tx pgx.Tx, lobbyID string, firstThemeSelection bool) (string, time.Time, error) {
	var gameID string
	deadline := time.Now().Add(themeStepWindow(themeProposalStep, firstThemeSelection))
	if err := tx.QueryRow(ctx, `
		INSERT INTO wavelength_games (lobby_id, theme_phase_deadline) VALUES ($1, $2) RETURNING id
	`, lobbyID, deadline).Scan(&gameID); err != nil {
		return "", time.Time{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE lobbies SET status = 'in_game' WHERE id = $1`, lobbyID); err != nil {
		return "", time.Time{}, err
	}
	return gameID, deadline, bumpLobbyRevision(ctx, tx, lobbyID)
}

func (a *api) handleSubmitWavelengthTheme(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Left  string `json:"left"`
		Right string `json:"right"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	input.Left = strings.TrimSpace(input.Left)
	input.Right = strings.TrimSpace(input.Right)
	if (input.Left == "") != (input.Right == "") {
		writeError(w, http.StatusUnprocessableEntity, "incomplete_spectrum", "Fill in both poles, or neither to pass")
		return
	}
	if len([]rune(input.Left)) > wavelengthPoleMaxLength || len([]rune(input.Right)) > wavelengthPoleMaxLength {
		writeError(w, http.StatusUnprocessableEntity, "invalid_theme", "Each pole must be at most 40 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	game, err := lockActiveWavelengthGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "wavelength_not_started", "The Wavelength game has not started yet")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if game.Phase != "collecting_themes" {
		writeError(w, http.StatusConflict, "themes_locked", "Theme submissions are closed")
		return
	}

	var leftValue, rightValue *string
	if input.Left != "" {
		leftValue, rightValue = &input.Left, &input.Right
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO wavelength_theme_submissions (game_id, round_number, player_id, left_label, right_label)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (game_id, round_number, player_id)
		DO UPDATE SET left_label = $4, right_label = $5, submitted_at = now()
	`, game.ID, game.RoundNumber, player.ID, leftValue, rightValue); err != nil {
		a.internalError(w, r, err)
		return
	}
	rankingDeadline, err := tryAdvanceWavelengthThemeCollection(r.Context(), tx, game, player.LobbyID, false)
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
		a.scheduleWavelengthThemeRankingTimeout(player.Code, game.ID, game.RoundNumber, *rankingDeadline)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryAdvanceWavelengthThemeCollection opens the ranking of this round's
// spectra once every active player proposed one or passed (or when forced by
// the deadline). It returns the ranking deadline it set, nil when the phase
// did not move.
func tryAdvanceWavelengthThemeCollection(ctx context.Context, tx pgx.Tx, game wavelengthGameRow, lobbyID string, force bool) (*time.Time, error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, wavelengthThemeSubmissions, game.ID, game.RoundNumber)
		if err != nil {
			return nil, err
		}
		if len(awaiting) > 0 {
			return nil, nil
		}
	}
	deadline, err := themeRankingDeadline(ctx, tx, "wavelength_games", lobbyID, game.ID, game.RoundNumber > 1)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE wavelength_games SET phase = 'ranking_themes', theme_phase_deadline = $2 WHERE id = $1
	`, game.ID, deadline); err != nil {
		return nil, err
	}
	return &deadline, nil
}

func (a *api) handleRankWavelengthThemes(w http.ResponseWriter, r *http.Request) {
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

	game, err := lockActiveWavelengthGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "wavelength_not_started", "The Wavelength game has not started yet")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if game.Phase != "ranking_themes" {
		writeError(w, http.StatusConflict, "ranking_locked", "Theme ranking is not open")
		return
	}

	candidates, err := wavelengthThemeCandidates(r.Context(), tx, game.ID, game.RoundNumber)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if !rankingMatchesCandidates(input.Ranking, wavelengthSpectrumLabels(candidates)) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_ranking", "The ranking must contain every candidate theme exactly once")
		return
	}
	rankingJSON, err := json.Marshal(input.Ranking)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO wavelength_theme_rankings (game_id, round_number, voter_player_id, ranking)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (game_id, round_number, voter_player_id) DO UPDATE SET ranking = $4, submitted_at = now()
	`, game.ID, game.RoundNumber, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	roundID, started, err := tryConcludeWavelengthRanking(r.Context(), tx, game, player.LobbyID, candidates, false)
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
		a.scheduleWavelengthClueTimeout(player.Code, roundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryConcludeWavelengthRanking keeps only the most popular spectrum and
// starts the round on it once every active player ranked (or when forced).
func tryConcludeWavelengthRanking(
	ctx context.Context, tx pgx.Tx, game wavelengthGameRow, lobbyID string, candidates []wavelengthSpectrum, force bool,
) (roundID string, started bool, err error) {
	if !force {
		awaiting, err := themeStepAwaitingIDs(ctx, tx, lobbyID, wavelengthThemeRankings, game.ID, game.RoundNumber)
		if err != nil {
			return "", false, err
		}
		if len(awaiting) > 0 {
			return "", false, nil
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT ranking FROM wavelength_theme_rankings WHERE game_id = $1 AND round_number = $2
	`, game.ID, game.RoundNumber)
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

	winners := tallyThemeRanking(wavelengthSpectrumLabels(candidates), rankings)
	remaining, err := wavelengthRemainingPsychicCount(ctx, tx, game.ID, lobbyID)
	if err != nil {
		return "", false, err
	}
	selected, err := wavelengthWinningSpectra(winners, candidates, wavelengthSpectraToPlay(remaining))
	if err != nil {
		return "", false, err
	}
	for offset, spectrum := range selected {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wavelength_selected_spectra (game_id, round_number, left_label, right_label)
			VALUES ($1, $2, $3, $4)
		`, game.ID, game.RoundNumber+offset, spectrum.Left, spectrum.Right); err != nil {
			return "", false, err
		}
	}
	return startWavelengthRound(ctx, tx, game, lobbyID, selected[0])
}

// wavelengthSpectraToPlay is how many voted spectra the next rounds use:
// three, or fewer when fewer players are left to be the psychic.
func wavelengthSpectraToPlay(remainingPsychics int) int {
	return min(wavelengthSpectraPerVote, max(remainingPsychics, 1))
}

// wavelengthWinningSpectra maps the first count winning labels back to their
// spectra, in ranking order.
func wavelengthWinningSpectra(winners []string, candidates []wavelengthSpectrum, count int) ([]wavelengthSpectrum, error) {
	spectrumByKey := make(map[string]wavelengthSpectrum, len(candidates))
	for _, candidate := range candidates {
		spectrumByKey[strings.ToLower(strings.TrimSpace(candidate.label()))] = candidate
	}
	selected := make([]wavelengthSpectrum, 0, count)
	for _, winner := range winners {
		if len(selected) == count {
			break
		}
		spectrum, known := spectrumByKey[strings.ToLower(strings.TrimSpace(winner))]
		if !known {
			return nil, errors.New("wavelength ranking winner is not a candidate")
		}
		selected = append(selected, spectrum)
	}
	if len(selected) == 0 {
		return nil, errors.New("wavelength ranking produced no selected spectrum")
	}
	return selected, nil
}

// loadWavelengthSelectedSpectrum returns the spectrum voted for a round, if
// the last vote covered it.
func loadWavelengthSelectedSpectrum(ctx context.Context, db dbQuerier, gameID string, roundNumber int) (wavelengthSpectrum, bool, error) {
	var spectrum wavelengthSpectrum
	err := db.QueryRow(ctx, `
		SELECT left_label, right_label FROM wavelength_selected_spectra WHERE game_id = $1 AND round_number = $2
	`, gameID, roundNumber).Scan(&spectrum.Left, &spectrum.Right)
	if errors.Is(err, pgx.ErrNoRows) {
		return wavelengthSpectrum{}, false, nil
	}
	return spectrum, err == nil, err
}

func (a *api) scheduleWavelengthThemeSubmissionTimeout(code, gameID string, roundNumber int, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceWavelengthThemeSubmissionDeadline(code, gameID, roundNumber)
	})
}

// forceWavelengthThemeSubmissionDeadline only acts on the round it was
// scheduled for: the theme phases come back every round, so an old timer
// must never close a later round's collection early.
func (a *api) forceWavelengthThemeSubmissionDeadline(code, gameID string, roundNumber int) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin wavelength theme submission deadline transaction", "game_id", gameID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	game, lobbyID, err := lockWavelengthGame(ctx, tx, gameID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load wavelength game for theme submission deadline", "game_id", gameID, "error", err)
		}
		return
	}
	if game.Phase != "collecting_themes" || game.RoundNumber != roundNumber {
		return
	}
	rankingDeadline, err := tryAdvanceWavelengthThemeCollection(ctx, tx, game, lobbyID, true)
	if err != nil {
		a.logger.Error("unable to force wavelength theme collection deadline", "game_id", gameID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for wavelength theme submission deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit wavelength theme submission deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	a.scheduleWavelengthThemeRankingTimeout(code, gameID, roundNumber, *rankingDeadline)
}

func (a *api) scheduleWavelengthThemeRankingTimeout(code, gameID string, roundNumber int, deadline time.Time) {
	time.AfterFunc(time.Until(deadline), func() {
		a.forceWavelengthThemeRankingDeadline(code, gameID, roundNumber)
	})
}

func (a *api) forceWavelengthThemeRankingDeadline(code, gameID string, roundNumber int) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin wavelength theme ranking deadline transaction", "game_id", gameID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	game, lobbyID, err := lockWavelengthGame(ctx, tx, gameID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load wavelength game for theme ranking deadline", "game_id", gameID, "error", err)
		}
		return
	}
	if game.Phase != "ranking_themes" || game.RoundNumber != roundNumber {
		return
	}
	candidates, err := wavelengthThemeCandidates(ctx, tx, game.ID, game.RoundNumber)
	if err != nil {
		a.logger.Error("unable to load wavelength theme candidates for ranking deadline", "game_id", gameID, "error", err)
		return
	}
	roundID, started, err := tryConcludeWavelengthRanking(ctx, tx, game, lobbyID, candidates, true)
	if err != nil {
		a.logger.Error("unable to force wavelength theme ranking deadline", "game_id", gameID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for wavelength theme ranking deadline", "game_id", gameID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit wavelength theme ranking deadline transition", "game_id", gameID, "error", err)
		return
	}
	a.hub.publish(code)
	if started {
		a.scheduleWavelengthClueTimeout(code, roundID)
	}
}

// wavelengthThemeCandidates lists this round's proposals, topped up to three
// with default spectra. The defaults start at a point derived from the game
// and round, so every call returns the same list for a given round (the
// ranking is validated against it) while games and rounds still vary.
func wavelengthThemeCandidates(ctx context.Context, db dbQuerier, gameID string, roundNumber int) ([]wavelengthSpectrum, error) {
	rows, err := db.Query(ctx, `
		SELECT left_label, right_label
		FROM wavelength_theme_submissions
		WHERE game_id = $1 AND round_number = $2 AND left_label IS NOT NULL AND left_label <> ''
		ORDER BY submitted_at, player_id
	`, gameID, roundNumber)
	if err != nil {
		return nil, err
	}
	proposals := make([]wavelengthSpectrum, 0)
	for rows.Next() {
		var spectrum wavelengthSpectrum
		if err := rows.Scan(&spectrum.Left, &spectrum.Right); err != nil {
			rows.Close()
			return nil, err
		}
		proposals = append(proposals, spectrum)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	playedRows, err := db.Query(ctx, `SELECT left_label, right_label FROM wavelength_rounds WHERE game_id = $1`, gameID)
	if err != nil {
		return nil, err
	}
	played := make([]wavelengthSpectrum, 0)
	for playedRows.Next() {
		var spectrum wavelengthSpectrum
		if err := playedRows.Scan(&spectrum.Left, &spectrum.Right); err != nil {
			playedRows.Close()
			return nil, err
		}
		played = append(played, spectrum)
	}
	if err := playedRows.Err(); err != nil {
		playedRows.Close()
		return nil, err
	}
	playedRows.Close()

	return mergeWavelengthCandidates(proposals, played, wavelengthDefaultsOffset(gameID, roundNumber)), nil
}

// mergeWavelengthCandidates dedupes the proposals, then tops them up to three
// with defaults read from offset onwards, preferring the spectra this game
// has not played yet.
func mergeWavelengthCandidates(proposals, played []wavelengthSpectrum, offset int) []wavelengthSpectrum {
	candidates := make([]wavelengthSpectrum, 0, max(len(proposals), 3))
	seen := make(map[string]struct{})
	add := func(spectrum wavelengthSpectrum) {
		key := strings.ToLower(strings.TrimSpace(spectrum.label()))
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, spectrum)
	}
	for _, proposal := range proposals {
		add(proposal)
	}
	playedKeys := make(map[string]struct{}, len(played))
	for _, spectrum := range played {
		playedKeys[strings.ToLower(strings.TrimSpace(spectrum.label()))] = struct{}{}
	}
	defaults := defaultWavelengthSpectra()
	for _, allowPlayed := range []bool{false, true} {
		for step := 0; step < len(defaults) && len(candidates) < 3; step++ {
			fallback := defaults[(offset+step)%len(defaults)]
			if _, wasPlayed := playedKeys[strings.ToLower(fallback.label())]; wasPlayed && !allowPlayed {
				continue
			}
			add(fallback)
		}
	}
	return candidates
}

func wavelengthDefaultsOffset(gameID string, roundNumber int) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(gameID + ":" + strconv.Itoa(roundNumber)))
	return int(hash.Sum32() % uint32(len(defaultWavelengthSpectra())))
}

func wavelengthSpectrumLabels(spectra []wavelengthSpectrum) []string {
	labels := make([]string, len(spectra))
	for index, spectrum := range spectra {
		labels[index] = spectrum.label()
	}
	return labels
}

// --- rounds: psychic, clue, guesses, scoring, progression --------------------

// pickWavelengthPsychic draws, at random, an active player who has not been
// the psychic yet in this game. ok is false once everyone had their turn.
func pickWavelengthPsychic(activeIDs, pastPsychicIDs []string) (psychicID string, ok bool) {
	eligible := wavelengthEligiblePsychics(activeIDs, pastPsychicIDs)
	if len(eligible) == 0 {
		return "", false
	}
	return eligible[rand.Intn(len(eligible))], true
}

func wavelengthEligiblePsychics(activeIDs, pastPsychicIDs []string) []string {
	past := make(map[string]struct{}, len(pastPsychicIDs))
	for _, id := range pastPsychicIDs {
		past[id] = struct{}{}
	}
	eligible := make([]string, 0, len(activeIDs))
	for _, id := range activeIDs {
		if _, wasPsychic := past[id]; !wasPsychic {
			eligible = append(eligible, id)
		}
	}
	return eligible
}

// wavelengthPoints scores one needle by its distance to the target, using
// the 4-3-2 bands of the dial.
func wavelengthPoints(target, position int) int {
	distance := target - position
	if distance < 0 {
		distance = -distance
	}
	switch {
	case distance <= wavelengthBullseyeRadius:
		return wavelengthBullseyePoints
	case distance <= wavelengthInnerRadius:
		return wavelengthInnerPoints
	case distance <= wavelengthOuterRadius:
		return wavelengthOuterPoints
	default:
		return 0
	}
}

// wavelengthPsychicPoints rewards a clear clue: the psychic earns the rounded
// average of the points the guessers who answered scored.
func wavelengthPsychicPoints(guesserPoints []int) int {
	if len(guesserPoints) == 0 {
		return 0
	}
	total := 0
	for _, points := range guesserPoints {
		total += points
	}
	return int(math.Round(float64(total) / float64(len(guesserPoints))))
}

func wavelengthPastPsychicIDs(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT psychic_player_id FROM wavelength_rounds WHERE game_id = $1`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// startWavelengthRound draws the psychic and the secret target, then opens
// the clue phase. When nobody is left to be the psychic, it ends the game
// instead and started is false.
func startWavelengthRound(
	ctx context.Context, tx pgx.Tx, game wavelengthGameRow, lobbyID string, spectrum wavelengthSpectrum,
) (roundID string, started bool, err error) {
	activeIDs, err := activeLobbyPlayerIDs(ctx, tx, lobbyID)
	if err != nil {
		return "", false, err
	}
	pastPsychicIDs, err := wavelengthPastPsychicIDs(ctx, tx, game.ID)
	if err != nil {
		return "", false, err
	}
	psychicID, ok := pickWavelengthPsychic(activeIDs, pastPsychicIDs)
	if !ok {
		return "", false, completeWavelengthGame(ctx, tx, game.ID)
	}
	target := wavelengthTargetMin + rand.Intn(wavelengthTargetMax-wavelengthTargetMin+1)
	if err := tx.QueryRow(ctx, `
		INSERT INTO wavelength_rounds (game_id, round_number, left_label, right_label, psychic_player_id, target, phase_deadline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, game.ID, game.RoundNumber, spectrum.Left, spectrum.Right, psychicID, target, time.Now().Add(wavelengthClueWindow)).Scan(&roundID); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE wavelength_games SET phase = 'playing', theme_phase_deadline = NULL WHERE id = $1
	`, game.ID); err != nil {
		return "", false, err
	}
	return roundID, true, nil
}

func completeWavelengthGame(ctx context.Context, tx pgx.Tx, gameID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE wavelength_games SET phase = 'completed', theme_phase_deadline = NULL, completed_at = now() WHERE id = $1
	`, gameID)
	return err
}

func (a *api) handleConfirmWavelengthClue(w http.ResponseWriter, r *http.Request) {
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

	round, ok := a.loadPlayingWavelengthRound(w, r, tx, player)
	if !ok {
		return
	}
	if round.PsychicPlayerID != player.ID {
		writeError(w, http.StatusForbidden, "not_psychic", "Only this round's psychic can confirm the clue")
		return
	}
	if round.Phase != "clue" {
		writeError(w, http.StatusConflict, "wavelength_round_locked", "This step is no longer open")
		return
	}
	closed, err := openWavelengthGuessing(r.Context(), tx, round, player.LobbyID)
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
	if !closed {
		a.scheduleWavelengthGuessTimeout(player.Code, round.ID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// openWavelengthGuessing moves the round from the psychic's clue to the
// guesses. closed is true when no guesser is left, so the round went
// straight to its results.
func openWavelengthGuessing(ctx context.Context, tx pgx.Tx, round wavelengthRoundRow, lobbyID string) (closed bool, err error) {
	if _, err := tx.Exec(ctx, `
		UPDATE wavelength_rounds SET phase = 'guessing', phase_deadline = $2 WHERE id = $1
	`, round.ID, time.Now().Add(wavelengthGuessWindow)); err != nil {
		return false, err
	}
	return tryCloseWavelengthGuessing(ctx, tx, round, lobbyID, false)
}

func (a *api) scheduleWavelengthClueTimeout(code, roundID string) {
	time.AfterFunc(wavelengthClueWindow, func() {
		a.forceWavelengthRoundDeadline(code, roundID, "clue")
	})
}

func (a *api) scheduleWavelengthGuessTimeout(code, roundID string) {
	time.AfterFunc(wavelengthGuessWindow, func() {
		a.forceWavelengthRoundDeadline(code, roundID, "guessing")
	})
}

// forceWavelengthRoundDeadline closes the clue or guessing phase it was
// scheduled for, if the round is still in it.
func (a *api) forceWavelengthRoundDeadline(code, roundID, expectedPhase string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin wavelength round deadline transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var lobbyID string
	if err := tx.QueryRow(ctx, `
		SELECT game.lobby_id
		FROM wavelength_rounds AS round
		JOIN wavelength_games AS game ON game.id = round.game_id
		WHERE round.id = $1
		FOR UPDATE OF game
	`, roundID).Scan(&lobbyID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load wavelength round for deadline", "round_id", roundID, "error", err)
		}
		return
	}
	round, err := loadWavelengthRound(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to load wavelength round for deadline", "round_id", roundID, "error", err)
		return
	}
	if round.Phase != expectedPhase {
		return
	}
	scheduleGuessTimeout := false
	if expectedPhase == "clue" {
		closed, err := openWavelengthGuessing(ctx, tx, round, lobbyID)
		if err != nil {
			a.logger.Error("unable to open wavelength guessing after the clue deadline", "round_id", roundID, "error", err)
			return
		}
		scheduleGuessTimeout = !closed
	} else if _, err := tryCloseWavelengthGuessing(ctx, tx, round, lobbyID, true); err != nil {
		a.logger.Error("unable to close wavelength guessing window", "round_id", roundID, "error", err)
		return
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		a.logger.Error("unable to bump lobby revision for wavelength round deadline", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit wavelength round deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
	if scheduleGuessTimeout {
		a.scheduleWavelengthGuessTimeout(code, roundID)
	}
}

func (a *api) handleSubmitWavelengthGuess(w http.ResponseWriter, r *http.Request) {
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
	if input.Position == nil || *input.Position < 0 || *input.Position > 100 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_position", "Position must be between 0 and 100")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	round, ok := a.loadPlayingWavelengthRound(w, r, tx, player)
	if !ok {
		return
	}
	if round.PsychicPlayerID == player.ID {
		writeError(w, http.StatusForbidden, "psychic_cannot_guess", "The psychic already knows the target")
		return
	}
	if round.Phase != "guessing" {
		writeError(w, http.StatusConflict, "wavelength_round_locked", "This round no longer accepts guesses")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO wavelength_guesses (round_id, player_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (round_id, player_id) DO UPDATE SET position = $3, created_at = now()
	`, round.ID, player.ID, *input.Position); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tryCloseWavelengthGuessing(r.Context(), tx, round, player.LobbyID, false); err != nil {
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

// loadPlayingWavelengthRound locks the lobby's game and returns its current
// round, writing the error response itself when no round is being played.
func (a *api) loadPlayingWavelengthRound(w http.ResponseWriter, r *http.Request, tx pgx.Tx, player authenticatedPlayer) (wavelengthRoundRow, bool) {
	game, err := lockActiveWavelengthGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && game.Phase != "playing") {
		writeError(w, http.StatusConflict, "wavelength_not_playing", "No Wavelength round is currently in progress")
		return wavelengthRoundRow{}, false
	}
	if err != nil {
		a.internalError(w, r, err)
		return wavelengthRoundRow{}, false
	}
	round, err := loadCurrentWavelengthRound(r.Context(), tx, game.ID)
	if err != nil {
		a.internalError(w, r, err)
		return wavelengthRoundRow{}, false
	}
	return round, true
}

// tryCloseWavelengthGuessing scores the round and opens its results once
// every active guesser placed their needle (or when forced by the deadline).
func tryCloseWavelengthGuessing(ctx context.Context, tx pgx.Tx, round wavelengthRoundRow, lobbyID string, force bool) (closed bool, err error) {
	if !force {
		awaiting, err := wavelengthGuessAwaitingIDs(ctx, tx, lobbyID, round.ID, round.PsychicPlayerID)
		if err != nil {
			return false, err
		}
		if len(awaiting) > 0 {
			return false, nil
		}
	}
	if err := scoreWavelengthRound(ctx, tx, round); err != nil {
		return false, err
	}
	return true, nil
}

func scoreWavelengthRound(ctx context.Context, tx pgx.Tx, round wavelengthRoundRow) error {
	rows, err := tx.Query(ctx, `SELECT player_id, position FROM wavelength_guesses WHERE round_id = $1`, round.ID)
	if err != nil {
		return err
	}
	type guessRow struct {
		playerID string
		position int
	}
	guesses := make([]guessRow, 0)
	for rows.Next() {
		var guess guessRow
		if err := rows.Scan(&guess.playerID, &guess.position); err != nil {
			rows.Close()
			return err
		}
		guesses = append(guesses, guess)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	guesserPoints := make([]int, 0, len(guesses))
	for _, guess := range guesses {
		points := wavelengthPoints(round.Target, guess.position)
		guesserPoints = append(guesserPoints, points)
		if _, err := tx.Exec(ctx, `
			UPDATE wavelength_guesses SET points = $1 WHERE round_id = $2 AND player_id = $3
		`, points, round.ID, guess.playerID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE wavelength_rounds SET phase = 'results', psychic_points = $2 WHERE id = $1
	`, round.ID, wavelengthPsychicPoints(guesserPoints))
	return err
}

func (a *api) handleStartNextWavelengthRound(w http.ResponseWriter, r *http.Request) {
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

	game, err := lockActiveWavelengthGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && game.Phase != "playing") {
		writeError(w, http.StatusConflict, "wavelength_not_playing", "No Wavelength round is currently in progress")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentWavelengthRound(r.Context(), tx, game.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "results" {
		writeError(w, http.StatusConflict, "wavelength_round_locked", "This round has not finished yet")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE wavelength_rounds SET phase = 'completed', completed_at = now() WHERE id = $1
	`, round.ID); err != nil {
		a.internalError(w, r, err)
		return
	}

	remaining, err := wavelengthRemainingPsychicCount(r.Context(), tx, game.ID, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	// The next round plays the next spectrum of the last vote; once they are
	// all played, the group votes again for the following rounds.
	nextSpectrum, alreadyVoted, err := loadWavelengthSelectedSpectrum(r.Context(), tx, game.ID, game.RoundNumber+1)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	votingRoundNumber := 0
	startedRoundID := ""
	themeDeadline := time.Now().Add(themeStepWindow(themeProposalStep, false))
	switch {
	case remaining == 0:
		err = completeWavelengthGame(r.Context(), tx, game.ID)
	case alreadyVoted:
		game.RoundNumber++
		if _, err = tx.Exec(r.Context(), `
			UPDATE wavelength_games SET round_number = $2 WHERE id = $1
		`, game.ID, game.RoundNumber); err == nil {
			startedRoundID, _, err = startWavelengthRound(r.Context(), tx, game, player.LobbyID, nextSpectrum)
		}
	default:
		votingRoundNumber = game.RoundNumber + 1
		_, err = tx.Exec(r.Context(), `
			UPDATE wavelength_games
			SET phase = 'collecting_themes', round_number = $2, theme_phase_deadline = $3
			WHERE id = $1
		`, game.ID, votingRoundNumber, themeDeadline)
	}
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
	if votingRoundNumber > 0 {
		a.scheduleWavelengthThemeSubmissionTimeout(player.Code, game.ID, votingRoundNumber, themeDeadline)
	}
	if startedRoundID != "" {
		a.scheduleWavelengthClueTimeout(player.Code, startedRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleReplayWavelength(w http.ResponseWriter, r *http.Request) {
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

	game, err := lockActiveWavelengthGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && game.Phase != "completed" {
		writeError(w, http.StatusConflict, "wavelength_in_progress", "The current cycle has not finished yet")
		return
	}
	gameID, themeDeadline, err := createWavelengthGame(r.Context(), tx, player.LobbyID, false)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.scheduleWavelengthThemeSubmissionTimeout(player.Code, gameID, 1, themeDeadline)
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- shared row loaders ------------------------------------------------

type wavelengthGameRow struct {
	ID          string
	Phase       string
	RoundNumber int
}

func loadActiveWavelengthGame(ctx context.Context, db dbQuerier, lobbyID string) (wavelengthGameRow, error) {
	var game wavelengthGameRow
	err := db.QueryRow(ctx, `
		SELECT id, phase, round_number FROM wavelength_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
	`, lobbyID).Scan(&game.ID, &game.Phase, &game.RoundNumber)
	return game, err
}

// lockActiveWavelengthGame is loadActiveWavelengthGame for write paths: the
// row lock serializes every command and deadline of a lobby, so "everyone
// answered" checks never miss a concurrent answer.
func lockActiveWavelengthGame(ctx context.Context, tx pgx.Tx, lobbyID string) (wavelengthGameRow, error) {
	var game wavelengthGameRow
	err := tx.QueryRow(ctx, `
		SELECT id, phase, round_number FROM wavelength_games
		WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
		FOR UPDATE
	`, lobbyID).Scan(&game.ID, &game.Phase, &game.RoundNumber)
	return game, err
}

func lockWavelengthGame(ctx context.Context, tx pgx.Tx, gameID string) (game wavelengthGameRow, lobbyID string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id, phase, round_number, lobby_id FROM wavelength_games WHERE id = $1 FOR UPDATE
	`, gameID).Scan(&game.ID, &game.Phase, &game.RoundNumber, &lobbyID)
	return game, lobbyID, err
}

type wavelengthRoundRow struct {
	ID                 string
	RoundNumber        int
	LeftLabel          string
	RightLabel         string
	PsychicPlayerID    string
	PsychicDisplayName string
	Target             int
	Phase              string
	PhaseDeadline      time.Time
	PsychicPoints      *int
}

const wavelengthRoundColumns = `
	round.id, round.round_number, round.left_label, round.right_label,
	round.psychic_player_id, psychic.display_name, round.target, round.phase,
	round.phase_deadline, round.psychic_points
	FROM wavelength_rounds AS round
	JOIN players AS psychic ON psychic.id = round.psychic_player_id`

func scanWavelengthRound(row pgx.Row) (wavelengthRoundRow, error) {
	var round wavelengthRoundRow
	err := row.Scan(
		&round.ID, &round.RoundNumber, &round.LeftLabel, &round.RightLabel,
		&round.PsychicPlayerID, &round.PsychicDisplayName, &round.Target, &round.Phase,
		&round.PhaseDeadline, &round.PsychicPoints,
	)
	return round, err
}

func loadCurrentWavelengthRound(ctx context.Context, db dbQuerier, gameID string) (wavelengthRoundRow, error) {
	return scanWavelengthRound(db.QueryRow(ctx, `
		SELECT `+wavelengthRoundColumns+`
		WHERE round.game_id = $1
		ORDER BY round.round_number DESC
		LIMIT 1
	`, gameID))
}

func loadWavelengthRound(ctx context.Context, db dbQuerier, roundID string) (wavelengthRoundRow, error) {
	return scanWavelengthRound(db.QueryRow(ctx, `SELECT `+wavelengthRoundColumns+` WHERE round.id = $1`, roundID))
}

// wavelengthGuesserCount is how many active players must place a needle:
// everyone but the psychic.
func wavelengthGuesserCount(ctx context.Context, db dbQuerier, lobbyID, psychicPlayerID string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM players WHERE lobby_id = $1 AND is_active AND id <> $2
	`, lobbyID, psychicPlayerID).Scan(&count)
	return count, err
}

// wavelengthRemainingPsychicCount is how many active players have not been
// the psychic yet in this game, i.e. how many rounds are left to start.
func wavelengthRemainingPsychicCount(ctx context.Context, db dbQuerier, gameID, lobbyID string) (int, error) {
	activeIDs, err := activeLobbyPlayerIDs(ctx, db, lobbyID)
	if err != nil {
		return 0, err
	}
	pastPsychicIDs, err := wavelengthPastPsychicIDs(ctx, db, gameID)
	if err != nil {
		return 0, err
	}
	return len(wavelengthEligiblePsychics(activeIDs, pastPsychicIDs)), nil
}

func loadWavelengthReveal(ctx context.Context, db dbQuerier, roundID string) ([]wavelengthRevealEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT guess.player_id, player.display_name, guess.position, COALESCE(guess.points, 0)
		FROM wavelength_guesses AS guess
		JOIN players AS player ON player.id = guess.player_id
		WHERE guess.round_id = $1
		ORDER BY guess.points DESC NULLS LAST, player.display_name ASC
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reveal := make([]wavelengthRevealEntryView, 0)
	for rows.Next() {
		var entry wavelengthRevealEntryView
		if err := rows.Scan(&entry.PlayerID, &entry.DisplayName, &entry.Position, &entry.Points); err != nil {
			return nil, err
		}
		reveal = append(reveal, entry)
	}
	return reveal, rows.Err()
}

// loadWavelengthLeaderboard totals each player's guesser points and the
// points they earned as a psychic.
func loadWavelengthLeaderboard(ctx context.Context, db dbQuerier, gameID string) ([]fastBioLeaderboardEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT player.id, player.display_name,
		       COALESCE((
		         SELECT sum(guess.points) FROM wavelength_guesses AS guess
		         JOIN wavelength_rounds AS round ON round.id = guess.round_id
		         WHERE round.game_id = $1 AND guess.player_id = player.id
		       ), 0) + COALESCE((
		         SELECT sum(round.psychic_points) FROM wavelength_rounds AS round
		         WHERE round.game_id = $1 AND round.psychic_player_id = player.id
		       ), 0) AS total_score
		FROM players AS player
		WHERE player.lobby_id = (SELECT lobby_id FROM wavelength_games WHERE id = $1)
		  AND player.is_active
		ORDER BY total_score DESC, player.display_name ASC
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leaderboard := make([]fastBioLeaderboardEntryView, 0)
	for rows.Next() {
		var entry fastBioLeaderboardEntryView
		if err := rows.Scan(&entry.PlayerID, &entry.DisplayName, &entry.Score); err != nil {
			return nil, err
		}
		leaderboard = append(leaderboard, entry)
	}
	return leaderboard, rows.Err()
}

// loadWavelengthThemeProgress populates the deadline, the "X of Y" progress
// counters and the awaited players of the current round's theme step.
func (a *api) loadWavelengthThemeProgress(ctx context.Context, game wavelengthGameRow, lobbyID string, table themeStepTable, view *wavelengthStateView) error {
	var deadline *time.Time
	if err := a.pool.QueryRow(ctx, `
		SELECT theme_phase_deadline FROM wavelength_games WHERE id = $1
	`, game.ID).Scan(&deadline); err != nil {
		return err
	}
	view.ThemeDeadline = deadline
	progress, err := loadThemeStepProgress(ctx, a.pool, lobbyID, table, game.ID, game.RoundNumber)
	if err != nil {
		return err
	}
	view.ThemeProgressCount = progress.count
	view.ThemeProgressRequired = progress.required
	view.AwaitingPlayerIDs = progress.awaiting
	return nil
}

// loadWavelengthState builds the Wavelength view of the lobby state for the
// current player. The target stays hidden from everyone but the psychic
// until the round is scored.
func (a *api) loadWavelengthState(ctx context.Context, currentPlayer authenticatedPlayer) (wavelengthStateView, error) {
	game, err := loadActiveWavelengthGame(ctx, a.pool, currentPlayer.LobbyID)
	if err != nil {
		return wavelengthStateView{}, err
	}
	view := wavelengthStateView{ID: game.ID, Phase: game.Phase, RoundNumber: game.RoundNumber}

	if game.Phase == "completed" {
		leaderboard, err := loadWavelengthLeaderboard(ctx, a.pool, game.ID)
		if err != nil {
			return wavelengthStateView{}, err
		}
		view.Leaderboard = leaderboard
		return view, nil
	}

	var startedRounds int
	if err := a.pool.QueryRow(ctx, `
		SELECT count(*) FROM wavelength_rounds WHERE game_id = $1
	`, game.ID).Scan(&startedRounds); err != nil {
		return wavelengthStateView{}, err
	}
	remaining, err := wavelengthRemainingPsychicCount(ctx, a.pool, game.ID, currentPlayer.LobbyID)
	if err != nil {
		return wavelengthStateView{}, err
	}
	view.TotalRounds = startedRounds + remaining

	if game.Phase == "collecting_themes" || game.Phase == "ranking_themes" {
		view.SpectraToPlay = wavelengthSpectraToPlay(remaining)
	}

	switch game.Phase {
	case "collecting_themes":
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(
			  SELECT 1 FROM wavelength_theme_submissions WHERE game_id = $1 AND round_number = $2 AND player_id = $3
			)
		`, game.ID, game.RoundNumber, currentPlayer.ID).Scan(&view.ThemeSubmitted); err != nil {
			return wavelengthStateView{}, err
		}
		if err := a.loadWavelengthThemeProgress(ctx, game, currentPlayer.LobbyID, wavelengthThemeSubmissions, &view); err != nil {
			return wavelengthStateView{}, err
		}

	case "ranking_themes":
		view.ThemeSubmitted = true
		candidates, err := wavelengthThemeCandidates(ctx, a.pool, game.ID, game.RoundNumber)
		if err != nil {
			return wavelengthStateView{}, err
		}
		view.ThemeCandidates = wavelengthSpectrumLabels(candidates)
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(
			  SELECT 1 FROM wavelength_theme_rankings WHERE game_id = $1 AND round_number = $2 AND voter_player_id = $3
			)
		`, game.ID, game.RoundNumber, currentPlayer.ID).Scan(&view.ThemeRanked); err != nil {
			return wavelengthStateView{}, err
		}
		if err := a.loadWavelengthThemeProgress(ctx, game, currentPlayer.LobbyID, wavelengthThemeRankings, &view); err != nil {
			return wavelengthStateView{}, err
		}

	case "playing":
		view.ThemeSubmitted = true
		view.ThemeRanked = true
		view.IsLastRound = remaining == 0
		round, err := loadCurrentWavelengthRound(ctx, a.pool, game.ID)
		if err != nil {
			return wavelengthStateView{}, err
		}
		view.RoundPhase = round.Phase
		view.LeftLabel = round.LeftLabel
		view.RightLabel = round.RightLabel
		view.PsychicPlayerID = round.PsychicPlayerID
		view.PsychicDisplayName = round.PsychicDisplayName
		view.IsPsychic = round.PsychicPlayerID == currentPlayer.ID
		isScored := round.Phase == "results" || round.Phase == "completed"
		if view.IsPsychic || isScored {
			target := round.Target
			view.Target = &target
		}
		if round.Phase == "clue" || round.Phase == "guessing" {
			deadline := round.PhaseDeadline
			view.PhaseDeadline = &deadline
			requiredCount, err := wavelengthGuesserCount(ctx, a.pool, currentPlayer.LobbyID, round.PsychicPlayerID)
			if err != nil {
				return wavelengthStateView{}, err
			}
			awaiting, err := wavelengthGuessAwaitingIDs(ctx, a.pool, currentPlayer.LobbyID, round.ID, round.PsychicPlayerID)
			if err != nil {
				return wavelengthStateView{}, err
			}
			progress := newStepProgress(requiredCount, awaiting)
			view.SubmissionProgressRequired = progress.required
			view.SubmissionProgressCount = progress.count
			if round.Phase == "clue" {
				view.AwaitingPlayerIDs = []string{round.PsychicPlayerID}
			} else {
				view.AwaitingPlayerIDs = progress.awaiting
			}
		}

		var myPosition, myPoints *int
		err = a.pool.QueryRow(ctx, `
			SELECT position, points FROM wavelength_guesses WHERE round_id = $1 AND player_id = $2
		`, round.ID, currentPlayer.ID).Scan(&myPosition, &myPoints)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return wavelengthStateView{}, err
		}
		view.Submitted = myPosition != nil
		view.MyPosition = myPosition

		if isScored {
			reveal, err := loadWavelengthReveal(ctx, a.pool, round.ID)
			if err != nil {
				return wavelengthStateView{}, err
			}
			view.Reveal = reveal
			if round.PsychicPoints != nil {
				view.PsychicPoints = *round.PsychicPoints
			}
			if view.IsPsychic {
				view.RoundScore = view.PsychicPoints
			} else if myPoints != nil {
				view.RoundScore = *myPoints
			}
			if remaining > 0 {
				nextSpectrum, alreadyVoted, err := loadWavelengthSelectedSpectrum(ctx, a.pool, game.ID, round.RoundNumber+1)
				if err != nil {
					return wavelengthStateView{}, err
				}
				if alreadyVoted {
					view.NextSpectrumLabel = nextSpectrum.label()
				}
			}
		}
	}

	return view, nil
}
