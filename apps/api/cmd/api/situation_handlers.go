package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func defaultSituationThemes() []string {
	return []string{
		"En entretien d'embauche avec le Père Fouettard",
		"Coincé·e dans un ascenseur avec sa belle-mère",
		"Animateur·rice d'un mariage improvisé",
		"Négociateur·rice face à un vendeur de tapis",
		"Perdu·e en pleine forêt sans réseau",
		"Invité·e surprise à une émission de télé-réalité",
	}
}

// --- lobby start & theme collection & ranking -------------------------------

func (a *api) handleStartSituationGame(w http.ResponseWriter, r *http.Request) {
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
	if mode != lobbyModeSituation {
		writeError(w, http.StatusConflict, "wrong_mode", "This lobby is not a Situation lobby")
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
		writeError(w, http.StatusConflict, "invalid_player_count", "Situation needs at least two players")
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
		INSERT INTO situation_games (lobby_id) VALUES ($1) RETURNING id
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

func (a *api) handleSubmitSituationTheme(w http.ResponseWriter, r *http.Request) {
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
	if len([]rune(input.Theme)) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_theme", "Situation must be at most 100 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, phase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "situation_not_started", "The Situation game has not started yet")
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
		INSERT INTO situation_theme_submissions (game_id, player_id, theme_label)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, player_id) DO UPDATE SET theme_label = $3, submitted_at = now()
	`, gameID, player.ID, themeValue); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := a.tryAdvanceSituationThemeCollection(r.Context(), tx, gameID, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) tryAdvanceSituationThemeCollection(ctx context.Context, tx pgx.Tx, gameID, lobbyID string) error {
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return err
	}
	var submittedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM situation_theme_submissions WHERE game_id = $1
	`, gameID).Scan(&submittedCount); err != nil {
		return err
	}
	if submittedCount < activeCount {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE situation_games SET phase = 'ranking_themes' WHERE id = $1`, gameID)
	return err
}

func (a *api) handleRankSituationThemes(w http.ResponseWriter, r *http.Request) {
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

	gameID, phase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "situation_not_started", "The Situation game has not started yet")
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

	candidates, err := situationThemeCandidates(r.Context(), tx, gameID)
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
		INSERT INTO situation_theme_rankings (game_id, voter_player_id, ranking)
		VALUES ($1, $2, $3)
		ON CONFLICT (game_id, voter_player_id) DO UPDATE SET ranking = $3, submitted_at = now()
	`, gameID, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	nextRoundID, shouldScheduleNext, err := a.tryConcludeSituationRanking(r.Context(), tx, gameID, player.LobbyID, candidates)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	if shouldScheduleNext {
		a.scheduleSituationProposalTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) tryConcludeSituationRanking(
	ctx context.Context, tx pgx.Tx, gameID, lobbyID string, candidates []string,
) (nextRoundID string, shouldScheduleNext bool, err error) {
	activeCount, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	if err != nil {
		return "", false, err
	}
	var votedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM situation_theme_rankings WHERE game_id = $1
	`, gameID).Scan(&votedCount); err != nil {
		return "", false, err
	}
	if votedCount < activeCount {
		return "", false, nil
	}

	rows, err := tx.Query(ctx, `SELECT ranking FROM situation_theme_rankings WHERE game_id = $1`, gameID)
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
		return "", false, errors.New("situation ranking produced no selected themes")
	}
	for index, themeLabel := range selectedThemes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO situation_selected_themes (game_id, position, theme_label) VALUES ($1, $2, $3)
		`, gameID, index+1, themeLabel); err != nil {
			return "", false, err
		}
	}

	roundID, err := startSituationRound(ctx, tx, gameID, 1, selectedThemes[0])
	if err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE situation_games SET phase = 'playing' WHERE id = $1`, gameID); err != nil {
		return "", false, err
	}
	return roundID, true, nil
}

func situationThemeCandidates(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label
		FROM situation_theme_submissions
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
	for _, fallback := range defaultSituationThemes() {
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

// --- rounds: proposals, duel bracket, review, ranking, scoring --------------

func startSituationRound(ctx context.Context, tx pgx.Tx, gameID string, roundNumber int, themeLabel string) (string, error) {
	var roundID string
	deadline := time.Now().Add(situationProposalWindow)
	err := tx.QueryRow(ctx, `
		INSERT INTO situation_rounds (game_id, round_number, theme_label, proposal_deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, gameID, roundNumber, themeLabel, deadline).Scan(&roundID)
	return roundID, err
}

func (a *api) scheduleSituationProposalTimeout(code, roundID string) {
	time.AfterFunc(situationProposalWindow, func() {
		a.forceSituationProposalDeadline(code, roundID)
	})
}

func (a *api) forceSituationProposalDeadline(code, roundID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin situation proposal deadline transaction", "round_id", roundID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var phase string
	if err := tx.QueryRow(ctx, `SELECT phase FROM situation_rounds WHERE id = $1 FOR UPDATE`, roundID).Scan(&phase); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load situation round for proposal deadline", "round_id", roundID, "error", err)
		}
		return
	}
	if phase != "proposing" {
		return
	}
	duelIDs, err := advanceSituationBracket(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to close situation proposal window", "round_id", roundID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit situation proposal deadline transition", "round_id", roundID, "error", err)
		return
	}
	a.hub.publish(code)
	for _, duelID := range duelIDs {
		a.scheduleSituationDuelTimeout(code, duelID)
	}
}

func (a *api) handleSubmitSituationProposal(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		ChosenPlayerID string `json:"chosenPlayerId"`
		Reason         string `json:"reason"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ChosenPlayerID == "" || input.ChosenPlayerID == player.ID {
		writeError(w, http.StatusUnprocessableEntity, "invalid_choice", "Choose another player")
		return
	}
	if len([]rune(input.Reason)) < 1 || len([]rune(input.Reason)) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_reason", "Reason must contain between 1 and 100 characters")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	gameID, gamePhase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "situation_not_playing", "No situation round is currently open for proposals")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentSituationRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "proposing" {
		writeError(w, http.StatusConflict, "situation_round_locked", "This round no longer accepts proposals")
		return
	}

	var chosenIsActive bool
	if err := tx.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM players WHERE id = $1 AND lobby_id = $2 AND excluded_at IS NULL)
	`, input.ChosenPlayerID, player.LobbyID).Scan(&chosenIsActive); err != nil {
		a.internalError(w, r, err)
		return
	}
	if !chosenIsActive {
		writeError(w, http.StatusUnprocessableEntity, "invalid_choice", "Choose an active player")
		return
	}

	var proposalID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO situation_proposals (round_id, author_player_id, chosen_player_id, reason)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, round.ID, player.ID, input.ChosenPlayerID, input.Reason).Scan(&proposalID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(w, http.StatusConflict, "already_submitted", "You already submitted a proposal for this round")
			return
		}
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO situation_group_members (round_id, player_id, proposal_id) VALUES ($1, $2, $3)
	`, round.ID, player.ID, proposalID); err != nil {
		a.internalError(w, r, err)
		return
	}

	activeCount, err := activeLobbyPlayerCount(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	var submittedCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(*) FROM situation_proposals WHERE round_id = $1
	`, round.ID).Scan(&submittedCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	var duelIDs []string
	if submittedCount >= activeCount {
		duelIDs, err = advanceSituationBracket(r.Context(), tx, round.ID)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	for _, duelID := range duelIDs {
		a.scheduleSituationDuelTimeout(player.Code, duelID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusCreated)
}

// advanceSituationBracket looks at the currently alive proposals for a round
// and either opens the reveal carousel (4 or fewer remain) or schedules the
// next wave of the elimination tree: every duel in a wave is created at once
// and runs in parallel (not one after another), and any group left over
// (odd count, or the wave doesn't need to use every group to reach exactly
// four) simply carries over untouched to the next wave — no duel row means
// no work for that group this round, i.e. a waiting screen on the client.
// situationDuelsForWave picks how many duels to run in a wave given
// aliveCount surviving proposals: enough to make real progress (pair up as
// many as possible), but never more than needed to land on exactly
// situationFinalistCount survivors — so the tree converges to precisely 4
// without ever overshooting, whatever aliveCount started at.
func situationDuelsForWave(aliveCount int) int {
	duelsThisWave := aliveCount / 2
	if remaining := aliveCount - situationFinalistCount; remaining < duelsThisWave {
		duelsThisWave = remaining
	}
	if duelsThisWave < 0 {
		return 0
	}
	return duelsThisWave
}

func advanceSituationBracket(ctx context.Context, tx pgx.Tx, roundID string) ([]string, error) {
	aliveProposalIDs, err := loadAliveSituationProposalIDs(ctx, tx, roundID)
	if err != nil {
		return nil, err
	}
	if len(aliveProposalIDs) <= situationFinalistCount {
		_, err := tx.Exec(ctx, `
			UPDATE situation_rounds SET phase = 'revealing', review_index = 0 WHERE id = $1
		`, roundID)
		return nil, err
	}

	var nextWave int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(max(wave_number), 0) + 1 FROM situation_duels WHERE round_id = $1
	`, roundID).Scan(&nextWave); err != nil {
		return nil, err
	}

	shuffled := make([]string, len(aliveProposalIDs))
	copy(shuffled, aliveProposalIDs)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	duelsThisWave := situationDuelsForWave(len(shuffled))

	duelIDs := make([]string, 0, duelsThisWave)
	for i := 0; i < duelsThisWave; i++ {
		proposalA := shuffled[2*i]
		proposalB := shuffled[2*i+1]
		representativeA, err := randomSituationGroupMember(ctx, tx, roundID, proposalA)
		if err != nil {
			return nil, err
		}
		representativeB, err := randomSituationGroupMember(ctx, tx, roundID, proposalB)
		if err != nil {
			return nil, err
		}
		var duelID string
		deadline := time.Now().Add(situationDuelWindow)
		if err := tx.QueryRow(ctx, `
			INSERT INTO situation_duels (
				round_id, wave_number, proposal_a_id, proposal_b_id,
				representative_a_player_id, representative_b_player_id, deadline
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, roundID, nextWave, proposalA, proposalB, representativeA, representativeB, deadline).Scan(&duelID); err != nil {
			return nil, err
		}
		duelIDs = append(duelIDs, duelID)
	}
	if duelsThisWave > 0 {
		if _, err := tx.Exec(ctx, `UPDATE situation_rounds SET phase = 'dueling' WHERE id = $1`, roundID); err != nil {
			return nil, err
		}
	}
	return duelIDs, nil
}

func randomSituationGroupMember(ctx context.Context, tx pgx.Tx, roundID, proposalID string) (string, error) {
	var playerID string
	err := tx.QueryRow(ctx, `
		SELECT player_id FROM situation_group_members
		WHERE round_id = $1 AND proposal_id = $2
		ORDER BY random()
		LIMIT 1
	`, roundID, proposalID).Scan(&playerID)
	return playerID, err
}

// tryAdvanceSituationWave checks whether every duel of the round's current
// wave has been resolved (or was never created, i.e. a fresh round) and, if
// so, advances the bracket (next wave, or the reveal carousel).
func tryAdvanceSituationWave(ctx context.Context, tx pgx.Tx, roundID string) ([]string, error) {
	var currentWave int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(max(wave_number), 0) FROM situation_duels WHERE round_id = $1
	`, roundID).Scan(&currentWave); err != nil {
		return nil, err
	}
	if currentWave > 0 {
		var unresolvedCount int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM situation_duels WHERE round_id = $1 AND wave_number = $2 AND resolved_at IS NULL
		`, roundID, currentWave).Scan(&unresolvedCount); err != nil {
			return nil, err
		}
		if unresolvedCount > 0 {
			return nil, nil
		}
	}
	return advanceSituationBracket(ctx, tx, roundID)
}

func resolveSituationDuel(ctx context.Context, tx pgx.Tx, duelID, winnerProposalID string) (roundID string, err error) {
	var proposalA, proposalB string
	if err := tx.QueryRow(ctx, `
		SELECT round_id, proposal_a_id, proposal_b_id FROM situation_duels WHERE id = $1 FOR UPDATE
	`, duelID).Scan(&roundID, &proposalA, &proposalB); err != nil {
		return "", err
	}
	loserProposalID := proposalB
	if winnerProposalID == proposalB {
		loserProposalID = proposalA
	}
	if _, err := tx.Exec(ctx, `
		UPDATE situation_duels SET winner_proposal_id = $1, resolved_at = now() WHERE id = $2
	`, winnerProposalID, duelID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE situation_proposals SET eliminated_at = now() WHERE id = $1
	`, loserProposalID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE situation_group_members SET proposal_id = $1 WHERE round_id = $2 AND proposal_id = $3
	`, winnerProposalID, roundID, loserProposalID); err != nil {
		return "", err
	}
	return roundID, nil
}

func (a *api) handleVoteSituationDuel(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input struct {
		ProposalID string `json:"proposalId"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	duelID := r.PathValue("duelID")

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var proposalA, proposalB, representativeA, representativeB string
	var voteA, voteB *string
	err = tx.QueryRow(r.Context(), `
		SELECT proposal_a_id, proposal_b_id, representative_a_player_id, representative_b_player_id,
		       vote_a_proposal_id, vote_b_proposal_id
		FROM situation_duels
		WHERE id = $1 AND resolved_at IS NULL
		FOR UPDATE
	`, duelID).Scan(&proposalA, &proposalB, &representativeA, &representativeB, &voteA, &voteB)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "duel_not_active", "This duel is not open for voting")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if player.ID != representativeA && player.ID != representativeB {
		writeError(w, http.StatusForbidden, "not_a_representative", "You are not part of this duel")
		return
	}
	if input.ProposalID != proposalA && input.ProposalID != proposalB {
		writeError(w, http.StatusUnprocessableEntity, "invalid_choice", "Choose one of the two proposals shown")
		return
	}

	if player.ID == representativeA {
		if _, err := tx.Exec(r.Context(), `UPDATE situation_duels SET vote_a_proposal_id = $1 WHERE id = $2`, input.ProposalID, duelID); err != nil {
			a.internalError(w, r, err)
			return
		}
		voteA = &input.ProposalID
	} else {
		if _, err := tx.Exec(r.Context(), `UPDATE situation_duels SET vote_b_proposal_id = $1 WHERE id = $2`, input.ProposalID, duelID); err != nil {
			a.internalError(w, r, err)
			return
		}
		voteB = &input.ProposalID
	}

	var duelIDs []string
	if voteA != nil && voteB != nil && *voteA == *voteB {
		roundID, err := resolveSituationDuel(r.Context(), tx, duelID, *voteA)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		duelIDs, err = tryAdvanceSituationWave(r.Context(), tx, roundID)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	for _, id := range duelIDs {
		a.scheduleSituationDuelTimeout(player.Code, id)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) scheduleSituationDuelTimeout(code, duelID string) {
	time.AfterFunc(situationDuelWindow, func() {
		a.forceSituationDuelDeadline(code, duelID)
	})
}

func (a *api) forceSituationDuelDeadline(code, duelID string) {
	ctx := context.Background()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		a.logger.Error("unable to begin situation duel deadline transaction", "duel_id", duelID, "error", err)
		return
	}
	defer tx.Rollback(ctx)

	var proposalA, proposalB string
	var resolved bool
	err = tx.QueryRow(ctx, `
		SELECT proposal_a_id, proposal_b_id, resolved_at IS NOT NULL FROM situation_duels WHERE id = $1 FOR UPDATE
	`, duelID).Scan(&proposalA, &proposalB, &resolved)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Error("unable to load situation duel for deadline", "duel_id", duelID, "error", err)
		}
		return
	}
	if resolved {
		return
	}
	winner := proposalA
	if rand.Intn(2) == 1 {
		winner = proposalB
	}
	roundID, err := resolveSituationDuel(ctx, tx, duelID, winner)
	if err != nil {
		a.logger.Error("unable to resolve situation duel at deadline", "duel_id", duelID, "error", err)
		return
	}
	duelIDs, err := tryAdvanceSituationWave(ctx, tx, roundID)
	if err != nil {
		a.logger.Error("unable to advance situation bracket after duel deadline", "duel_id", duelID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.logger.Error("unable to commit situation duel deadline transition", "duel_id", duelID, "error", err)
		return
	}
	a.hub.publish(code)
	for _, id := range duelIDs {
		a.scheduleSituationDuelTimeout(code, id)
	}
}

func (a *api) handleAdvanceSituationReview(w http.ResponseWriter, r *http.Request) {
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

	gameID, gamePhase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "situation_not_playing", "No situation round is currently being reviewed")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentSituationRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "revealing" {
		writeError(w, http.StatusConflict, "situation_round_locked", "This round is not being reviewed")
		return
	}
	finalistCount, err := countAliveSituationProposals(r.Context(), tx, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}

	switch {
	case input.Direction == "previous":
		if round.ReviewIndex > 0 {
			if _, err := tx.Exec(r.Context(), `
				UPDATE situation_rounds SET review_index = review_index - 1 WHERE id = $1
			`, round.ID); err != nil {
				a.internalError(w, r, err)
				return
			}
		}
	case round.ReviewIndex >= finalistCount-1:
		if _, err := tx.Exec(r.Context(), `UPDATE situation_rounds SET phase = 'ranking' WHERE id = $1`, round.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
	default:
		if _, err := tx.Exec(r.Context(), `
			UPDATE situation_rounds SET review_index = review_index + 1 WHERE id = $1
		`, round.ID); err != nil {
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

func (a *api) handleSubmitSituationRanking(w http.ResponseWriter, r *http.Request) {
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

	gameID, gamePhase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "situation_not_playing", "No situation round is open for ranking")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentSituationRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "ranking" {
		writeError(w, http.StatusConflict, "situation_round_locked", "Ranking is not open for this round")
		return
	}

	candidateIDs, err := loadAliveSituationProposalIDs(r.Context(), tx, round.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if !rankingMatchesCandidates(input.Ranking, candidateIDs) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_ranking", "The ranking must contain every finalist exactly once")
		return
	}
	rankingJSON, err := json.Marshal(input.Ranking)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO situation_final_rankings (round_id, voter_player_id, ranking)
		VALUES ($1, $2, $3)
		ON CONFLICT (round_id, voter_player_id) DO UPDATE SET ranking = $3, submitted_at = now()
	`, round.ID, player.ID, rankingJSON); err != nil {
		a.internalError(w, r, err)
		return
	}

	activeCount, err := activeLobbyPlayerCount(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	var votedCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT count(*) FROM situation_final_rankings WHERE round_id = $1
	`, round.ID).Scan(&votedCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if votedCount >= activeCount {
		if err := scoreSituationRound(r.Context(), tx, round.ID, candidateIDs); err != nil {
			a.internalError(w, r, err)
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE situation_rounds SET phase = 'results' WHERE id = $1`, round.ID); err != nil {
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

// scoreSituationRound runs a Borda count (same weighting as tallyThemeRanking:
// weight = candidate count - position) over every submitted ranking, then
// credits every player currently backing a given finalist (its author, plus
// anyone absorbed into its group by winning duels) with that finalist's
// total points for the round.
func scoreSituationRound(ctx context.Context, tx pgx.Tx, roundID string, candidateIDs []string) error {
	rows, err := tx.Query(ctx, `SELECT ranking FROM situation_final_rankings WHERE round_id = $1`, roundID)
	if err != nil {
		return err
	}
	rankings := make([][]string, 0)
	for rows.Next() {
		var rankingJSON []byte
		if err := rows.Scan(&rankingJSON); err != nil {
			rows.Close()
			return err
		}
		var ranking []string
		if err := json.Unmarshal(rankingJSON, &ranking); err != nil {
			rows.Close()
			return err
		}
		rankings = append(rankings, ranking)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	pointsByProposal := tallyProposalPoints(candidateIDs, rankings)
	for proposalID, points := range pointsByProposal {
		memberRows, err := tx.Query(ctx, `
			SELECT player_id FROM situation_group_members WHERE round_id = $1 AND proposal_id = $2
		`, roundID, proposalID)
		if err != nil {
			return err
		}
		memberIDs := make([]string, 0)
		for memberRows.Next() {
			var playerID string
			if err := memberRows.Scan(&playerID); err != nil {
				memberRows.Close()
				return err
			}
			memberIDs = append(memberIDs, playerID)
		}
		if err := memberRows.Err(); err != nil {
			memberRows.Close()
			return err
		}
		memberRows.Close()
		for _, playerID := range memberIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO situation_round_scores (round_id, player_id, points)
				VALUES ($1, $2, $3)
				ON CONFLICT (round_id, player_id) DO UPDATE SET points = $3
			`, roundID, playerID, points); err != nil {
				return err
			}
		}
	}
	return nil
}

// tallyProposalPoints is the scoring twin of tallyThemeRanking: same Borda
// weighting, but it returns every candidate's total rather than just the
// top three, since here every finalist needs a persisted score.
func tallyProposalPoints(candidates []string, rankings [][]string) map[string]int {
	points := make(map[string]int, len(candidates))
	candidateSet := make(map[string]struct{}, len(candidates))
	for _, id := range candidates {
		points[id] = 0
		candidateSet[id] = struct{}{}
	}
	weight := len(candidates)
	for _, ranking := range rankings {
		for position, id := range ranking {
			if _, ok := candidateSet[id]; !ok {
				continue
			}
			points[id] += weight - position
		}
	}
	return points
}

func (a *api) handleStartNextSituationRound(w http.ResponseWriter, r *http.Request) {
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

	gameID, gamePhase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && gamePhase != "playing") {
		writeError(w, http.StatusConflict, "situation_not_playing", "No situation round is currently in progress")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	round, err := loadCurrentSituationRound(r.Context(), tx, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if round.Phase != "results" {
		writeError(w, http.StatusConflict, "situation_round_locked", "This round has not finished yet")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE situation_rounds SET phase = 'completed', completed_at = now() WHERE id = $1
	`, round.ID); err != nil {
		a.internalError(w, r, err)
		return
	}

	var nextRoundID string
	shouldScheduleNext := false
	if round.RoundNumber >= situationRoundCount {
		if _, err := tx.Exec(r.Context(), `
			UPDATE situation_games SET phase = 'completed', completed_at = now() WHERE id = $1
		`, gameID); err != nil {
			a.internalError(w, r, err)
			return
		}
	} else {
		var nextThemeLabel string
		if err := tx.QueryRow(r.Context(), `
			SELECT theme_label FROM situation_selected_themes WHERE game_id = $1 AND position = $2
		`, gameID, round.RoundNumber+1).Scan(&nextThemeLabel); err != nil {
			a.internalError(w, r, err)
			return
		}
		nextRoundID, err = startSituationRound(r.Context(), tx, gameID, round.RoundNumber+1, nextThemeLabel)
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
		a.scheduleSituationProposalTimeout(player.Code, nextRoundID)
	}
	a.writeLobbyStateResponse(w, r, player, http.StatusOK)
}

func (a *api) handleReplaySituation(w http.ResponseWriter, r *http.Request) {
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

	_, gamePhase, err := loadActiveSituationGame(r.Context(), tx, player.LobbyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if err == nil && gamePhase != "completed" {
		writeError(w, http.StatusConflict, "situation_in_progress", "The current Situation cycle has not finished yet")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO situation_games (lobby_id) VALUES ($1)
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

func loadActiveSituationGame(ctx context.Context, db dbQuerier, lobbyID string) (id, phase string, err error) {
	err = db.QueryRow(ctx, `
		SELECT id, phase FROM situation_games WHERE lobby_id = $1 ORDER BY created_at DESC LIMIT 1
	`, lobbyID).Scan(&id, &phase)
	return id, phase, err
}

type situationRoundRow struct {
	ID               string
	RoundNumber      int
	ThemeLabel       string
	Phase            string
	ProposalDeadline time.Time
	ReviewIndex      int
}

func loadCurrentSituationRound(ctx context.Context, db dbQuerier, gameID string) (situationRoundRow, error) {
	var round situationRoundRow
	err := db.QueryRow(ctx, `
		SELECT id, round_number, theme_label, phase, proposal_deadline, review_index
		FROM situation_rounds
		WHERE game_id = $1
		ORDER BY round_number DESC
		LIMIT 1
	`, gameID).Scan(&round.ID, &round.RoundNumber, &round.ThemeLabel, &round.Phase, &round.ProposalDeadline, &round.ReviewIndex)
	return round, err
}

func loadSituationSelectedThemes(ctx context.Context, db dbQuerier, gameID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT theme_label FROM situation_selected_themes WHERE game_id = $1 ORDER BY position
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	themes := make([]string, 0, situationRoundCount)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		themes = append(themes, label)
	}
	return themes, rows.Err()
}

func countAliveSituationProposals(ctx context.Context, db dbQuerier, roundID string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM situation_proposals WHERE round_id = $1 AND eliminated_at IS NULL
	`, roundID).Scan(&count)
	return count, err
}

func loadAliveSituationProposalIDs(ctx context.Context, db dbQuerier, roundID string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id FROM situation_proposals WHERE round_id = $1 AND eliminated_at IS NULL ORDER BY submitted_at, id
	`, roundID)
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

func loadSituationProposalByID(ctx context.Context, db dbQuerier, proposalID string) (situationProposalView, error) {
	var view situationProposalView
	err := db.QueryRow(ctx, `
		SELECT proposal.id, proposal.author_player_id, author.display_name,
		       proposal.chosen_player_id, chosen.display_name, proposal.reason
		FROM situation_proposals AS proposal
		JOIN players AS author ON author.id = proposal.author_player_id
		JOIN players AS chosen ON chosen.id = proposal.chosen_player_id
		WHERE proposal.id = $1
	`, proposalID).Scan(
		&view.ID, &view.AuthorPlayerID, &view.AuthorDisplayName, &view.ChosenPlayerID, &view.ChosenDisplayName, &view.Reason,
	)
	return view, err
}

func loadSituationProposalAt(ctx context.Context, db dbQuerier, roundID string, index int) (situationProposalView, error) {
	var view situationProposalView
	err := db.QueryRow(ctx, `
		SELECT proposal.id, proposal.author_player_id, author.display_name,
		       proposal.chosen_player_id, chosen.display_name, proposal.reason
		FROM situation_proposals AS proposal
		JOIN players AS author ON author.id = proposal.author_player_id
		JOIN players AS chosen ON chosen.id = proposal.chosen_player_id
		WHERE proposal.round_id = $1 AND proposal.eliminated_at IS NULL
		ORDER BY proposal.submitted_at, proposal.id
		OFFSET $2 LIMIT 1
	`, roundID, index).Scan(
		&view.ID, &view.AuthorPlayerID, &view.AuthorDisplayName, &view.ChosenPlayerID, &view.ChosenDisplayName, &view.Reason,
	)
	return view, err
}

func loadAliveSituationProposalViews(ctx context.Context, db dbQuerier, roundID string) ([]situationProposalView, error) {
	rows, err := db.Query(ctx, `
		SELECT proposal.id, proposal.author_player_id, author.display_name,
		       proposal.chosen_player_id, chosen.display_name, proposal.reason
		FROM situation_proposals AS proposal
		JOIN players AS author ON author.id = proposal.author_player_id
		JOIN players AS chosen ON chosen.id = proposal.chosen_player_id
		WHERE proposal.round_id = $1 AND proposal.eliminated_at IS NULL
		ORDER BY proposal.submitted_at, proposal.id
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := make([]situationProposalView, 0)
	for rows.Next() {
		var view situationProposalView
		if err := rows.Scan(
			&view.ID, &view.AuthorPlayerID, &view.AuthorDisplayName, &view.ChosenPlayerID, &view.ChosenDisplayName, &view.Reason,
		); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

// loadMySituationDuel returns the current player's unresolved duel, if any.
// A nil result (no error) means they have no duel to play right now — the
// client shows a waiting screen until the current wave finishes.
func loadMySituationDuel(ctx context.Context, db dbQuerier, roundID, playerID string) (*situationDuelView, error) {
	var duelID, representativeA, representativeB, proposalAID, proposalBID string
	var voteA, voteB *string
	var deadline time.Time
	err := db.QueryRow(ctx, `
		SELECT id, representative_a_player_id, representative_b_player_id,
		       vote_a_proposal_id, vote_b_proposal_id, proposal_a_id, proposal_b_id, deadline
		FROM situation_duels
		WHERE round_id = $1 AND resolved_at IS NULL
		  AND (representative_a_player_id = $2 OR representative_b_player_id = $2)
		ORDER BY wave_number DESC
		LIMIT 1
	`, roundID, playerID).Scan(&duelID, &representativeA, &representativeB, &voteA, &voteB, &proposalAID, &proposalBID, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	proposalA, err := loadSituationProposalByID(ctx, db, proposalAID)
	if err != nil {
		return nil, err
	}
	proposalB, err := loadSituationProposalByID(ctx, db, proposalBID)
	if err != nil {
		return nil, err
	}

	duel := situationDuelView{
		ID:        duelID,
		ProposalA: proposalA,
		ProposalB: proposalB,
		Deadline:  deadline,
	}
	var myVote, opponentVote *string
	var opponentID string
	if playerID == representativeA {
		myVote, opponentVote = voteA, voteB
		opponentID = representativeB
	} else {
		myVote, opponentVote = voteB, voteA
		opponentID = representativeA
	}
	if myVote != nil {
		duel.MyVoteProposalID = *myVote
	}
	duel.OpponentHasVoted = opponentVote != nil
	if err := db.QueryRow(ctx, `SELECT display_name FROM players WHERE id = $1`, opponentID).Scan(&duel.OpponentDisplayName); err != nil {
		return nil, err
	}
	return &duel, nil
}

func loadSituationLeaderboard(ctx context.Context, db dbQuerier, gameID string) ([]fastBioLeaderboardEntryView, error) {
	rows, err := db.Query(ctx, `
		SELECT author.id, author.display_name,
		       COALESCE(sum(score.points) FILTER (WHERE round.id IS NOT NULL), 0) AS total_score,
		       COALESCE(sum(score.points) FILTER (WHERE round.round_number = $2), 0) AS round_score
		FROM players AS author
		LEFT JOIN situation_round_scores AS score ON score.player_id = author.id
		LEFT JOIN situation_rounds AS round ON round.id = score.round_id AND round.game_id = $1
		WHERE author.lobby_id = (SELECT lobby_id FROM situation_games WHERE id = $1)
		  AND author.excluded_at IS NULL
		GROUP BY author.id, author.display_name
		ORDER BY total_score DESC, author.display_name ASC
	`, gameID, situationRoundCount)
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

// loadSituationState builds the Situation view of the lobby state for the
// current player, mirroring loadFastBioState/loadZeroToHundredState.
func (a *api) loadSituationState(ctx context.Context, currentPlayer authenticatedPlayer) (situationStateView, error) {
	gameID, gamePhase, err := loadActiveSituationGame(ctx, a.pool, currentPlayer.LobbyID)
	if err != nil {
		return situationStateView{}, err
	}
	view := situationStateView{ID: gameID, Phase: gamePhase}

	switch gamePhase {
	case "collecting_themes":
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM situation_theme_submissions WHERE game_id = $1 AND player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return situationStateView{}, err
		}
		view.ThemeSubmitted = exists

	case "ranking_themes":
		view.ThemeSubmitted = true
		candidates, err := situationThemeCandidates(ctx, a.pool, gameID)
		if err != nil {
			return situationStateView{}, err
		}
		view.ThemeCandidates = candidates
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM situation_theme_rankings WHERE game_id = $1 AND voter_player_id = $2)
		`, gameID, currentPlayer.ID).Scan(&exists); err != nil {
			return situationStateView{}, err
		}
		view.ThemeRanked = exists

	case "playing":
		view.ThemeSubmitted = true
		view.ThemeRanked = true
		selectedThemes, err := loadSituationSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return situationStateView{}, err
		}
		view.SelectedThemes = selectedThemes

		round, err := loadCurrentSituationRound(ctx, a.pool, gameID)
		if err != nil {
			return situationStateView{}, err
		}
		view.RoundNumber = round.RoundNumber
		view.TotalRounds = situationRoundCount
		view.RoundPhase = round.Phase
		view.ThemeLabel = round.ThemeLabel

		switch round.Phase {
		case "proposing":
			deadline := round.ProposalDeadline
			view.ProposalDeadline = &deadline
			var submitted bool
			if err := a.pool.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM situation_proposals WHERE round_id = $1 AND author_player_id = $2)
			`, round.ID, currentPlayer.ID).Scan(&submitted); err != nil {
				return situationStateView{}, err
			}
			view.Submitted = submitted

		case "dueling":
			duel, err := loadMySituationDuel(ctx, a.pool, round.ID, currentPlayer.ID)
			if err != nil {
				return situationStateView{}, err
			}
			view.CurrentDuel = duel

		case "revealing":
			proposalCount, err := countAliveSituationProposals(ctx, a.pool, round.ID)
			if err != nil {
				return situationStateView{}, err
			}
			view.ProposalCount = proposalCount
			view.ReviewIndex = round.ReviewIndex
			view.IsHostReview = currentPlayer.IsHost
			if proposalCount > 0 && round.ReviewIndex < proposalCount {
				proposal, err := loadSituationProposalAt(ctx, a.pool, round.ID, round.ReviewIndex)
				if err != nil {
					return situationStateView{}, err
				}
				view.CurrentProposal = &proposal
			}

		case "ranking":
			candidates, err := loadAliveSituationProposalViews(ctx, a.pool, round.ID)
			if err != nil {
				return situationStateView{}, err
			}
			view.RankingCandidates = candidates
			var submitted bool
			if err := a.pool.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM situation_final_rankings WHERE round_id = $1 AND voter_player_id = $2)
			`, round.ID, currentPlayer.ID).Scan(&submitted); err != nil {
				return situationStateView{}, err
			}
			view.RankingSubmitted = submitted

		case "results", "completed":
			var roundScore int
			if err := a.pool.QueryRow(ctx, `
				SELECT COALESCE(points, 0) FROM situation_round_scores WHERE round_id = $1 AND player_id = $2
			`, round.ID, currentPlayer.ID).Scan(&roundScore); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return situationStateView{}, err
			}
			view.RoundScore = roundScore
		}

	case "completed":
		leaderboard, err := loadSituationLeaderboard(ctx, a.pool, gameID)
		if err != nil {
			return situationStateView{}, err
		}
		view.Leaderboard = leaderboard
		selectedThemes, err := loadSituationSelectedThemes(ctx, a.pool, gameID)
		if err != nil {
			return situationStateView{}, err
		}
		view.SelectedThemes = selectedThemes
	}

	return view, nil
}
