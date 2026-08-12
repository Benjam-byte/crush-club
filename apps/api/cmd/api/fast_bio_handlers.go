package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- theme collection & ranking -------------------------------------------

func (a *api) handleStartFastBioGame(w http.ResponseWriter, r *http.Request) {
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
	if mode != lobbyModeFastBio {
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a Fast Bio lobby")
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
		writeError(w, http.StatusConflict, "invalid_player_count", "Fast Bio needs at least two players")
		return
	}
	for _, id := range playerIDs {
		if !a.hub.isOnline(player.Code, id) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	var gameID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO fast_bio_games (lobby_id) VALUES ($1) RETURNING id
	`, player.LobbyID).Scan(&gameID); err != nil {
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
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleSubmitFastBioTheme(w http.ResponseWriter, r *http.Request) {
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
	if len([]rune(input.Theme)) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_theme", "Theme must be at most 60 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, phase, err := loadActiveFastBioGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "fast_bio_not_started", "The Fast Bio game has not started yet")
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
		INSERT INTO fast_bio_theme_submissions (game_id, player_id, theme_label)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, player_id) DO UPDATE SET theme_label = $3, submitted_at = now()
	`, gameID, player.ID, themeValue); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := a.tryAdvanceFastBioThemeCollection(r.Context(), tx, gameID, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryAdvanceFastBioThemeCollection moves the game into ranking_themes once
// every active player has either proposed a theme or explicitly passed.
func (a *api) tryAdvanceFastBioThemeCollection(ctx context.Context, tx pgx.Tx, gameID, lobbyID string) error {
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return err
	}
	var submittedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM fast_bio_theme_submissions WHERE game_id = $1
	`, gameID).Scan(&submittedCount); err != nil {
		return err
	}
	if submittedCount < activeCount {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE fast_bio_games SET phase = 'ranking_themes' WHERE id = $1`, gameID)
	return err
}

func (a *api) handleRankFastBioThemes(w http.ResponseWriter, r *http.Request) {
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

	gameID, phase, err := loadActiveFastBioGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "fast_bio_not_started", "The Fast Bio game has not started yet")
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

	candidates, err := fastBioThemeCandidates(r.Context(), tx, gameID)
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
		INSERT INTO fast_bio_theme_rankings (game_id, voter_player_id, ranking)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, voter_player_id) DO UPDATE SET ranking = $3, submitted_at = now()
	`, gameID, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	nextRoundID, shouldScheduleNext, err := a.tryConcludeFastBioRanking(r.Context(), tx, gameID, player.LobbyID, candidates)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if shouldScheduleNext {
		a.scheduleFastBioRoundTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// tryConcludeFastBioRanking tallies the vote once every active player has
// ranked the candidates, seeds the three selected themes, and starts round 1.
func (a *api) tryConcludeFastBioRanking(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, candidates []string,
) (nextRoundID string, shouldScheduleNext bool, err error) {
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return "", false, err
	}
	var votedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM fast_bio_theme_rankings WHERE game_id = $1
	`, gameID).Scan(&votedCount); err != nil {
		return "", false, err
	}
	if votedCount < activeCount {
		return "", false, nil
	}

	rows, err := tx.Query(ctx, `SELECT ranking FROM fast_bio_theme_rankings WHERE game_id = $1`, gameID)
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
		return "", false, errors.New("fast bio ranking produced no selected themes")
	}
	for index, themeLabel := range selectedThemes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fast_bio_selected_themes (game_id, position, theme_label) VALUES ($1, $2, $3)
		`, gameID, index+1, themeLabel); err != nil {
			return "", false, err
		}
	}

	roundID, err := a.startFastBioRound(ctx, tx, gameID, lobbyID, 1, selectedThemes[0])
	if err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE fast_bio_games SET phase = 'playing' WHERE id = $1`, gameID); err != nil {
		return "", false, err
	}
	return roundID, true, nil
}

// fastBioThemeCandidates returns the distinct themes proposed by players
// (case-insensitively deduplicated, in submission order), backfilled with
// the default theme list until there are at least three to rank.
func fastBioThemeCandidates(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label
		FROM fast_bio_theme_submissions
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
	for _, fallback := range defaultFastBioThemes() {
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

func rankingMatchesCandidates(ranking, candidates []string) bool {
	if len(ranking) != len(candidates) || len(candidates) == 0 {
		return false
	}
	remaining := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		remaining[strings.ToLower(strings.TrimSpace(candidate))]++
	}
	for _, entry := range ranking {
		key := strings.ToLower(strings.TrimSpace(entry))
		if remaining[key] <= 0 {
			return false
		}
		remaining[key]--
	}
	return true
}

// tallyThemeRanking runs a Borda count over every submitted ranking
// (first place among N candidates earns N points, last place earns 1) and
// returns the top three candidates, most popular first. Ties break by the
// candidate's position in the input list (earliest submission first).
func tallyThemeRanking(candidates []string, rankings [][]string) []string {
	labelByKey := make(map[string]string, len(candidates))
	orderByKey := make(map[string]int, len(candidates))
	scoreByKey := make(map[string]int, len(candidates))
	for index, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate))
		labelByKey[key] = candidate
		orderByKey[key] = index
	}
	candidateCount := len(candidates)
	for _, ranking := range rankings {
		for position, entry := range ranking {
			key := strings.ToLower(strings.TrimSpace(entry))
			if _, known := labelByKey[key]; !known {
				continue
			}
			scoreByKey[key] += candidateCount - position
		}
	}
	keys := make([]string, 0, len(labelByKey))
	for key := range labelByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if scoreByKey[keys[i]] != scoreByKey[keys[j]] {
			return scoreByKey[keys[i]] > scoreByKey[keys[j]]
		}
		return orderByKey[keys[i]] < orderByKey[keys[j]]
	})
	if len(keys) > 3 {
		keys = keys[:3]
	}
	result := make([]string, len(keys))
	for index, key := range keys {
		result[index] = labelByKey[key]
	}
	return result
}

// --- rounds: assignment, submission, review, reactions ---------------------

// assignFastBioTargets builds a random derangement (nobody is assigned to
// themselves) over the given players using a shuffled circular permutation.
func assignFastBioTargets(playerIDs []string) (map[string]string, error) {
	if len(playerIDs) < 2 {
		return nil, errors.New("fast bio requires at least two players")
	}
	shuffled := make([]string, len(playerIDs))
	copy(shuffled, playerIDs)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	assignments := make(map[string]string, len(shuffled))
	for index, authorID := range shuffled {
		assignments[authorID] = shuffled[(index+1)%len(shuffled)]
	}
	return assignments, nil
}

// startFastBioRound creates a new round for the game's currently active
// lobby players, with a fresh random target assignment and a 2-minute
// submission window.
func (a *api) startFastBioRound(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, roundNumber int, themeLabel string,
) (string, error) {
	playerIDs, err := activeLobbyPlayerIDs(ctx, tx, lobbyID)
	if err != nil {
		return "", err
	}
	assignments, err := assignFastBioTargets(playerIDs)
	if err != nil {
		return "", err
	}
	var roundID string
	deadline := time.Now().Add(fastBioSubmissionWindow)
	if err := tx.QueryRow(ctx, `
		INSERT INTO fast_bio_rounds (game_id, round_number, theme_label, submission_deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, gameID, roundNumber, themeLabel, deadline).Scan(&roundID); err != nil {
		return "", err
	}
	for authorID, targetID := range assignments {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fast_bio_assignments (round_id, author_player_id, target_player_id)
			VALUES ($1, $2, $3)
		`, roundID, authorID, targetID); err != nil {
			return "", err
		}
	}
	return roundID, nil
}

func (a *api) scheduleFastBioRoundTimeout(code, roundID string) {
	time.AfterFunc(fastBioSubmissionWindow, func() {
		a.forceFastBioSubmissionDeadline(code, roundID)
	})
}

func (a *api) forceFastBioSubmissionDeadline(code, roundID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin fast bio deadline transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase string
	if err := tx.QueryRow(ctx, `SELECT phase FROM fast_bio_rounds WHERE id = $1 FOR UPDATE`, roundID).Scan(&phase); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load fast bio round for deadline", "round_id", roundID, "error", err)
		}
		return
	}
	if phase != "submitting" {
		return
	}
	nextRoundID, shouldScheduleNext, err := a.transitionFastBioRoundOutOfSubmitting(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to close fast bio submission window", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit fast bio deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
	if shouldScheduleNext {
		a.scheduleFastBioRoundTimeout(code, nextRoundID)
	}
}

// transitionFastBioRoundOutOfSubmitting moves a round out of the submitting
// phase once its deadline is reached or everyone has submitted. If nobody
// submitted anything there is nothing to review, so the round is finished
// immediately instead of opening an empty review carousel.
func (a *api) transitionFastBioRoundOutOfSubmitting(
	ctx context.Context, tx pgx.Tx, roundID string,
) (nextRoundID string, shouldScheduleNext bool, err error) {
	var proposalCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM fast_bio_proposals WHERE round_id = $1`, roundID).Scan(&proposalCount); err != nil {
		return "", false, err
	}
	if proposalCount == 0 {
		return a.finishFastBioRound(ctx, tx, roundID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fast_bio_rounds SET phase = 'reviewing', review_index = 0 WHERE id = $1
	`, roundID); err != nil {
		return "", false, err
	}
	return "", false, nil
}

// finishFastBioRound marks a round completed and either starts the next
// round (new random assignment, next selected theme, fresh 2-minute window)
// or, if it was the third and last one, completes the whole Fast Bio cycle.
func (a *api) finishFastBioRound(ctx context.Context, tx pgx.Tx, roundID string) (nextRoundID string, shouldScheduleNext bool, err error) {
	var gameID, lobbyID string
	var roundNumber int
	if err := tx.QueryRow(ctx, `
		SELECT round.game_id, round.round_number, game.lobby_id
		FROM fast_bio_rounds AS round
		JOIN fast_bio_games AS game ON game.id = round.game_id
		WHERE round.id = $1
	`, roundID).Scan(&gameID, &roundNumber, &lobbyID); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE fast_bio_rounds SET phase = 'completed', completed_at = now() WHERE id = $1
	`, roundID); err != nil {
		return "", false, err
	}
	if roundNumber >= fastBioRoundCount {
		_, err := tx.Exec(ctx, `
			UPDATE fast_bio_games SET phase = 'completed', completed_at = now() WHERE id = $1
		`, gameID)
		return "", false, err
	}
	var nextThemeLabel string
	if err := tx.QueryRow(ctx, `
		SELECT theme_label FROM fast_bio_selected_themes WHERE game_id = $1 AND position = $2
	`, gameID, roundNumber+1).Scan(&nextThemeLabel); err != nil {
		return "", false, err
	}
	newRoundID, err := a.startFastBioRound(ctx, tx, gameID, lobbyID, roundNumber+1, nextThemeLabel)
	if err != nil {
		return "", false, err
	}
	return newRoundID, true, nil
}

func (a *api) handleSubmitFastBioProposal(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	photo, ok := a.stageSinglePhoto(w, r)
	if !ok {
		return
	}
	defer removeTemporaryPhotos([]validatedPhoto{photo})

	bio := ""
	if r.MultipartForm != nil {
		if values := r.MultipartForm.Value["bio"]; len(values) > 0 {
			bio = strings.TrimSpace(values[0])
		}
	}
	if len([]rune(bio)) < 1 || len([]rune(bio)) > 280 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_bio", "Bio must contain between 1 and 280 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, gamePhase, err := loadActiveFastBioGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "fast_bio_not_playing", "No Fast Bio round is currently open for submissions")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentFastBioRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "submitting" {
		writeError(w, http.StatusConflict, "fast_bio_round_locked", "This round no longer accepts submissions")
		return
	}
	var targetPlayerID string
	err = tx.QueryRow(r.Context(), `
		SELECT target_player_id FROM fast_bio_assignments WHERE round_id = $1 AND author_player_id = $2
	`, round.ID, player.ID).Scan(&targetPlayerID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "not_assigned", "You were not assigned a target for this round")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO fast_bio_proposals (
			round_id, author_player_id, target_player_id, storage_key, content_type, width, height, size_bytes, bio
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, round.ID, player.ID, targetPlayerID, photo.storageKey, photo.contentType, photo.width, photo.height, photo.size, bio); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(w, http.StatusConflict, "already_submitted", "You already submitted a proposal for this round")
			return
		}
		a.internalError(w, r, err)
		return
	}

	var requiredCount, submittedCount int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM fast_bio_assignments WHERE round_id = $1`, round.ID).Scan(&requiredCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM fast_bio_proposals WHERE round_id = $1`, round.ID).Scan(&submittedCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	var nextRoundID string
	var shouldScheduleNext bool
	if submittedCount >= requiredCount {
		nextRoundID, shouldScheduleNext, err = a.transitionFastBioRoundOutOfSubmitting(r.Context(), tx, round.ID)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	movedPaths, err := a.moveStagedPhotos([]validatedPhoto{photo})
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		removePhotoPaths(movedPaths)
		a.internalError(w, r, err)
		return
	}
	if shouldScheduleNext {
		a.scheduleFastBioRoundTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusCreated)
}

func (a *api) handleGetFastBioProposalPhoto(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var storageKey, contentType string
	var submittedAt time.Time
	err := a.pool.QueryRow(r.Context(), `
		SELECT proposal.storage_key, proposal.content_type, proposal.submitted_at
		FROM fast_bio_proposals AS proposal
		JOIN fast_bio_rounds AS round ON round.id = proposal.round_id
		JOIN fast_bio_games AS game ON game.id = round.game_id
		WHERE proposal.id = $1 AND game.lobby_id = $2
	`, r.PathValue("proposalID"), player.LobbyID).Scan(&storageKey, &contentType, &submittedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "photo_not_found", "Photo not found")
		return
	}
	if filepath.Base(storageKey) != storageKey {
		a.internalError(w, r, errors.New("unsafe photo storage key"))
		return
	}
	file, err := os.Open(filepath.Join(a.photoStoragePath, storageKey))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "photo_not_found", "Photo not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", submittedAt, io.NewSectionReader(file, 0, fileInfo.Size()))
}

func (a *api) handleAdvanceFastBioReview(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can navigate the review")
		return
	}
	var input struct {
		Direction string `json:"direction"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	if input.Direction != "next" && input.Direction != "previous" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_direction", "direction must be next or previous")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, gamePhase, err := loadActiveFastBioGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "fast_bio_not_playing", "No Fast Bio round is currently being reviewed")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentFastBioRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "reviewing" {
		writeError(w, http.StatusConflict, "fast_bio_round_locked", "This round is not being reviewed")
		return
	}
	var proposalCount int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM fast_bio_proposals WHERE round_id = $1`, round.ID).Scan(&proposalCount); err != nil {
		a.internalError(w, r, err)
		return
	}

	var nextRoundID string
	var shouldScheduleNext bool
	switch {
	case input.Direction == "previous":
		if round.ReviewIndex > 0 {
			if _, err := tx.Exec(r.Context(), `
				UPDATE fast_bio_rounds SET review_index = review_index - 1 WHERE id = $1
			`, round.ID); err != nil {
				a.internalError(w, r, err)
				return
			}
		}
	case round.ReviewIndex >= proposalCount-1:
		nextRoundID, shouldScheduleNext, err = a.finishFastBioRound(r.Context(), tx, round.ID)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
	default:
		if _, err := tx.Exec(r.Context(), `
			UPDATE fast_bio_rounds SET review_index = review_index + 1 WHERE id = $1
		`, round.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if shouldScheduleNext {
		a.scheduleFastBioRoundTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleReactToFastBioProposal(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		Emoji string `json:"emoji"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	points, validEmoji := fastBioReactionPoints[input.Emoji]
	if !validEmoji {
		writeError(w, http.StatusUnprocessableEntity, "invalid_emoji", "emoji must be one of the four reaction options")
		return
	}
	proposalID := r.PathValue("proposalID")

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var authorPlayerID, roundID string
	err = tx.QueryRow(r.Context(), `
		SELECT proposal.author_player_id, proposal.round_id
		FROM fast_bio_proposals AS proposal
		JOIN fast_bio_rounds AS round ON round.id = proposal.round_id
		JOIN fast_bio_games AS game ON game.id = round.game_id
		WHERE proposal.id = $1 AND game.lobby_id = $2
	`, proposalID, player.LobbyID).Scan(&authorPlayerID, &roundID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "proposal_not_found", "Proposal not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if authorPlayerID == player.ID {
		writeError(w, http.StatusConflict, "cannot_react_to_own_proposal", "You cannot react to your own proposal")
		return
	}

	rows, err := tx.Query(r.Context(), `
		SELECT id FROM fast_bio_proposals WHERE round_id = $1 ORDER BY submitted_at, id
	`, roundID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	currentIndex, position := -1, 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			a.internalError(w, r, err)
			return
		}
		if id == proposalID {
			currentIndex = position
		}
		position++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.internalError(w, r, err)
		return
	}
	rows.Close()

	var roundPhase string
	var reviewIndex int
	if err := tx.QueryRow(r.Context(), `
		SELECT phase, review_index FROM fast_bio_rounds WHERE id = $1
	`, roundID).Scan(&roundPhase, &reviewIndex); err != nil {
		a.internalError(w, r, err)
		return
	}
	if roundPhase != "reviewing" || currentIndex != reviewIndex {
		writeError(w, http.StatusConflict, "proposal_not_active", "This proposal is not currently being reviewed")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO fast_bio_reactions (proposal_id, voter_player_id, emoji, points)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (proposal_id, voter_player_id) DO UPDATE SET emoji = $3, points = $4, created_at = now()
	`, proposalID, player.ID, input.Emoji, points); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.broadcastReaction(player.Code, websocketReactionEvent{
		Type:           "reaction.broadcast",
		ProposalID:     proposalID,
		Emoji:          input.Emoji,
		AuthorPlayerID: authorPlayerID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleReplayFastBio(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can start a new Fast Bio cycle")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	_, gamePhase, err := loadActiveFastBioGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && gamePhase != "completed" {
		writeError(w, http.StatusConflict, "fast_bio_in_progress", "The current Fast Bio cycle has not finished yet")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO fast_bio_games (lobby_id) VALUES ($1)
	`, player.LobbyID); err != nil {
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
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

// --- shared row loaders ------------------------------------------------

func activeLobbyPlayerIDs(ctx context.Context, db dbQuerier, lobbyID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id FROM players WHERE lobby_id = $1 AND excluded_at IS NULL ORDER BY joined_at, id
	`, lobbyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	playerIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		playerIDs = append(playerIDs, id)
	}
	return playerIDs, rows.Err()
}

func activeLobbyPlayerCount(ctx context.Context, db dbQuerier, lobbyID string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL
	`, lobbyID).Scan(&count)
	return count, err
}

func loadActiveFastBioGame(ctx context.Context, db dbQuerier, lobbyID string) (id, phase string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, phase FROM fast_bio_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

type fastBioRoundRow struct {
	ID                 string
	RoundNumber        int
	ThemeLabel         string
	Phase              string
	SubmissionDeadline time.Time
	ReviewIndex        int
}

func loadCurrentFastBioRound(ctx context.Context, db dbQuerier, gameID string) (fastBioRoundRow, error) {
	var round fastBioRoundRow
	err := db.QueryRow(ctx, `
		SELECT id, round_number, theme_label, phase, submission_deadline, review_index
		FROM fast_bio_rounds
		WHERE game_id = $1
		ORDER BY round_number DESC
		LIMIT 1
	`, gameID).Scan(
		&round.ID, &round.RoundNumber, &round.ThemeLabel, &round.Phase, &round.SubmissionDeadline, &round.ReviewIndex,
	)
	return round, err
}

func loadFastBioSelectedThemes(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label FROM fast_bio_selected_themes WHERE game_id = $1 ORDER BY position
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	themes := make([]string, 0, fastBioRoundCount)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		themes = append(themes, label)
	}
	return themes, rows.Err()
}

func (a *api) loadFastBioProposalAt(ctx context.Context, roundID string, index int) (fastBioProposalView, error) {
	var proposal fastBioProposalView
	err := a.pool.QueryRow(ctx, `
		SELECT proposal.id, proposal.author_player_id, author.display_name,
		       proposal.target_player_id, target.display_name, proposal.bio
		FROM fast_bio_proposals AS proposal
		JOIN players AS author ON author.id = proposal.author_player_id
		JOIN players AS target ON target.id = proposal.target_player_id
		WHERE proposal.round_id = $1
		ORDER BY proposal.submitted_at, proposal.id
		OFFSET $2 LIMIT 1
	`, roundID, index).Scan(
		&proposal.ID, &proposal.AuthorPlayerID, &proposal.AuthorDisplayName,
		&proposal.TargetPlayerID, &proposal.TargetDisplayName, &proposal.Bio,
	)
	if err != nil {
		return fastBioProposalView{}, err
	}
	proposal.PhotoID = proposal.ID

	rows, err := a.pool.Query(ctx, `
		SELECT emoji, count(*), COALESCE(sum(points), 0) FROM fast_bio_reactions WHERE proposal_id = $1 GROUP BY emoji
	`, proposal.ID)
	if err != nil {
		return fastBioProposalView{}, err
	}
	defer rows.Close()
	reactions := make([]fastBioReactionCountView, 0, 4)
	totalPoints := 0
	for rows.Next() {
		var emoji string
		var count, points int
		if err := rows.Scan(&emoji, &count, &points); err != nil {
			return fastBioProposalView{}, err
		}
		reactions = append(reactions, fastBioReactionCountView{Emoji: emoji, Count: count})
		totalPoints += points
	}
	if err := rows.Err(); err != nil {
		return fastBioProposalView{}, err
	}
	proposal.Reactions = reactions
	proposal.TotalPoints = totalPoints
	return proposal, nil
}

func loadFastBioLeaderboard(ctx context.Context, db dbQuerier, gameID string) ([]fastBioLeaderboardEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT author.id, author.display_name,
		       COALESCE(sum(reaction.points), 0) AS total_score,
		       COALESCE(sum(reaction.points) FILTER (WHERE round.round_number = $2), 0) AS round_score
		FROM players AS author
		LEFT JOIN fast_bio_proposals AS proposal ON proposal.author_player_id = author.id
		LEFT JOIN fast_bio_rounds AS round ON round.id = proposal.round_id AND round.game_id = $1
		LEFT JOIN fast_bio_reactions AS reaction ON reaction.proposal_id = proposal.id AND round.id IS NOT NULL
		WHERE author.lobby_id = (SELECT lobby_id FROM fast_bio_games WHERE id = $1)
		  AND author.excluded_at IS NULL
		GROUP BY author.id, author.display_name
		ORDER BY total_score DESC, author.display_name ASC
	`, gameID, fastBioRoundCount)
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

// loadFastBioState builds the Fast Bio view of the lobby state for the
// current player, mirroring loadGameState's role for the classic mode.
func (a *api) loadFastBioState(ctx context.Context, currentPlayer authenticatedPlayer) (fastBioStateView, error) {
	gameID, gamePhase, err := loadActiveFastBioGame(ctx, a.pool, currentPlayer.LobbyID)
	if err != nil {
		return fastBioStateView{}, err
	}
	view := fastBioStateView{ID: gameID, Phase: gamePhase}

	switch gamePhase {
	case "collecting_themes":
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM fast_bio_theme_submissions WHERE game_id = $1 AND player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return fastBioStateView{}, err
		}
		view.ThemeSubmitted = exists

	case "ranking_themes":
		view.ThemeSubmitted = true
		candidates, err := fastBioThemeCandidates(ctx, a.pool, gameID)
		if err != nil {
			return fastBioStateView{}, err
		}
		view.ThemeCandidates = candidates
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM fast_bio_theme_rankings WHERE game_id = $1 AND voter_player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return fastBioStateView{}, err
		}
		view.ThemeRanked = exists

	case "playing":
		view.ThemeSubmitted = true
		view.ThemeRanked = true
		selectedThemes, err := loadFastBioSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return fastBioStateView{}, err
		}
		view.SelectedThemes = selectedThemes

		round, err := loadCurrentFastBioRound(ctx, a.pool, gameID)
		if err != nil {
			return fastBioStateView{}, err
		}
		view.RoundNumber = round.RoundNumber
		view.TotalRounds = fastBioRoundCount
		view.RoundPhase = round.Phase
		view.ThemeLabel = round.ThemeLabel
		if round.Phase == "submitting" {
			deadline := round.SubmissionDeadline
			view.SubmissionDeadline = &deadline
		}

		var targetID, targetName string
		err = a.pool.QueryRow(ctx, `
			SELECT assignment.target_player_id, target.display_name
			FROM fast_bio_assignments AS assignment
			JOIN players AS target ON target.id = assignment.target_player_id
			WHERE assignment.round_id = $1 AND assignment.author_player_id = $2
		`, round.ID, currentPlayer.ID).Scan(&targetID, &targetName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fastBioStateView{}, err
		}
		view.TargetPlayerID = targetID
		view.TargetDisplayName = targetName

		var submitted bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM fast_bio_proposals WHERE round_id = $1 AND author_player_id = $2)
		`, round.ID, currentPlayer.ID).Scan(&submitted); err != nil {
			return fastBioStateView{}, err
		}
		view.Submitted = submitted

		if round.Phase == "reviewing" || round.Phase == "completed" {
			var proposalCount int
			if err := a.pool.QueryRow(ctx, `
				SELECT count(*) FROM fast_bio_proposals WHERE round_id = $1
			`, round.ID).Scan(&proposalCount); err != nil {
				return fastBioStateView{}, err
			}
			view.ProposalCount = proposalCount
			view.ReviewIndex = round.ReviewIndex
			view.IsHostReview = currentPlayer.IsHost

			if round.Phase == "reviewing" && proposalCount > 0 && round.ReviewIndex < proposalCount {
				proposal, err := a.loadFastBioProposalAt(ctx, round.ID, round.ReviewIndex)
				if err != nil {
					return fastBioStateView{}, err
				}
				view.CurrentProposal = &proposal
				var myEmoji string
				err = a.pool.QueryRow(ctx, `
					SELECT emoji FROM fast_bio_reactions WHERE proposal_id = $1 AND voter_player_id = $2
				`, proposal.ID, currentPlayer.ID).Scan(&myEmoji)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return fastBioStateView{}, err
				}
				view.MyReactionEmoji = myEmoji
			}
		}

	case "completed":
		leaderboard, err := loadFastBioLeaderboard(ctx, a.pool, gameID)
		if err != nil {
			return fastBioStateView{}, err
		}
		view.Leaderboard = leaderboard
		selectedThemes, err := loadFastBioSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return fastBioStateView{}, err
		}
		view.SelectedThemes = selectedThemes
	}

	return view, nil
}
