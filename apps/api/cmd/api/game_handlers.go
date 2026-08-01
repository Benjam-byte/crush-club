package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/example/crush-club/apps/api/internal/scoring"
	"github.com/jackc/pgx/v5"
)

type startingPlayer struct {
	ID          string
	ReadyStatus string
	PhotoCount  int
}

type storedSubmission struct {
	ID              string
	PlayerID        string
	Tagline         string
	BioAnswers      map[string]string
	QuestionAnswers map[string]json.RawMessage
	LoverQuestionID string
}

type scoreLine struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	OfficialAnswer  any    `json:"officialAnswer"`
	PredictedAnswer any    `json:"predictedAnswer"`
	BaseScore       int    `json:"baseScore"`
	MaximumScore    int    `json:"maximumScore"`
	FinalScore      int    `json:"finalScore"`
	Exact           bool   `json:"exact"`
	IsLoverApplied  bool   `json:"isLoverApplied"`
}

func (a *api) handleStartMultiplayerGame(w http.ResponseWriter, r *http.Request) {
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
	var status string
	err = tx.QueryRow(r.Context(), `
		SELECT status FROM lobbies WHERE id = $1 FOR UPDATE
	`, player.LobbyID).Scan(&status)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if status == "in_game" || status == "completed" {
		state, stateErr := a.loadLobbyState(r.Context(), player)
		if stateErr != nil {
			a.internalError(w, r, stateErr)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}

	rows, err := tx.Query(r.Context(), `
		SELECT player.id, player.ready_status,
		       (SELECT count(*) FROM player_photos AS photo WHERE photo.player_id = player.id)
		FROM players AS player
		WHERE player.lobby_id = $1 AND player.excluded_at IS NULL
		ORDER BY player.joined_at, player.id
	`, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	playerList := make([]startingPlayer, 0, maximumPlayerCount)
	for rows.Next() {
		var candidate startingPlayer
		if err := rows.Scan(&candidate.ID, &candidate.ReadyStatus, &candidate.PhotoCount); err != nil {
			rows.Close()
			a.internalError(w, r, err)
			return
		}
		playerList = append(playerList, candidate)
	}
	rows.Close()
	if len(playerList) < minimumPlayerCount || len(playerList) > maximumPlayerCount {
		writeError(w, http.StatusConflict, "invalid_player_count", "A game needs between 2 and 10 players")
		return
	}
	for _, candidate := range playerList {
		if candidate.ReadyStatus != "ready" || candidate.PhotoCount != 4 {
			writeError(w, http.StatusConflict, "players_not_ready", "Every player must upload four photos before the game starts")
			return
		}
		if !a.hub.isOnline(player.Code, candidate.ID) {
			writeError(w, http.StatusConflict, "players_offline", "Every player must be connected before the game starts")
			return
		}
	}

	var gameID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO games (lobby_id, total_rounds)
		VALUES ($1, $2)
		RETURNING id
	`, player.LobbyID, len(playerList)).Scan(&gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	for seat, candidate := range playerList {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO game_participants (game_id, player_id, seat) VALUES ($1, $2, $3)
		`, gameID, candidate.ID, seat); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO game_rounds (game_id, round_number, subject_player_id)
		VALUES ($1, 1, $2)
	`, gameID, playerList[0].ID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies
		SET status = 'in_game', revision = revision + 1, updated_at = now()
		WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleSubmitCurrentRound(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input roundSubmissionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID, roundID, subjectPlayerID, roundStatus string
	err = tx.QueryRow(r.Context(), `
		SELECT game.id, round.id, round.subject_player_id, round.status
		FROM games AS game
		JOIN game_rounds AS round
		  ON round.game_id = game.id AND round.round_number = game.current_round_number
		JOIN game_participants AS participant
		  ON participant.game_id = game.id AND participant.player_id = $2 AND participant.is_active
		WHERE game.lobby_id = $1
		FOR UPDATE OF game, round
	`, player.LobbyID, player.ID).Scan(&gameID, &roundID, &subjectPlayerID, &roundStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "game_not_active", "This player is not active in the current game")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	kind := "prediction"
	if player.ID == subjectPlayerID {
		kind = "official"
	}
	snapshot, err := loadQuestionnaireSnapshot(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := validateRoundSubmission(snapshot, input, kind); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_submission", err.Error())
		return
	}
	input.Tagline = strings.TrimSpace(input.Tagline)
	bioJSON, _ := json.Marshal(input.BioAnswers)
	questionJSON, _ := json.Marshal(input.QuestionAnswers)

	var existing storedSubmission
	var existingBioJSON, existingQuestionJSON []byte
	err = tx.QueryRow(r.Context(), `
		SELECT id, COALESCE(tagline, ''), bio_answers, question_answers,
		       COALESCE(lover_question_id, '')
		FROM round_submissions
		WHERE round_id = $1 AND player_id = $2
	`, roundID, player.ID).Scan(
		&existing.ID,
		&existing.Tagline,
		&existingBioJSON,
		&existingQuestionJSON,
		&existing.LoverQuestionID,
	)
	if err == nil {
		_ = json.Unmarshal(existingBioJSON, &existing.BioAnswers)
		_ = json.Unmarshal(existingQuestionJSON, &existing.QuestionAnswers)
		if existing.Tagline != input.Tagline || existing.LoverQuestionID != input.LoverQuestionID ||
			!reflect.DeepEqual(existing.BioAnswers, input.BioAnswers) ||
			!rawAnswerMapsEqual(existing.QuestionAnswers, input.QuestionAnswers) {
			writeError(w, http.StatusConflict, "submission_locked", "The submitted profile is already locked")
			return
		}
		state, stateErr := a.loadLobbyState(r.Context(), player)
		if stateErr != nil {
			a.internalError(w, r, stateErr)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internalError(w, r, err)
		return
	}
	if roundStatus != "collecting_submissions" {
		writeError(w, http.StatusConflict, "submission_closed", "Submissions are closed for this round")
		return
	}
	var nullableTagline any
	if input.Tagline != "" {
		nullableTagline = input.Tagline
	}
	var nullableLover any
	if input.LoverQuestionID != "" {
		nullableLover = input.LoverQuestionID
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO round_submissions (
			round_id, player_id, kind, tagline, bio_answers, question_answers, lover_question_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, roundID, player.ID, kind, nullableTagline, bioJSON, questionJSON, nullableLover); err != nil {
		a.internalError(w, r, err)
		return
	}
	var requiredCount, submittedCount int
	if err := tx.QueryRow(r.Context(), `
		SELECT
		  (SELECT count(*) FROM game_participants WHERE game_id = $1 AND is_active),
		  (SELECT count(*)
		   FROM round_submissions AS submission
		   JOIN game_participants AS participant
		     ON participant.game_id = $1 AND participant.player_id = submission.player_id
		   WHERE submission.round_id = $2 AND participant.is_active)
	`, gameID, roundID).Scan(&requiredCount, &submittedCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if submittedCount == requiredCount {
		if _, err := tx.Exec(r.Context(), `
			UPDATE game_rounds SET status = 'reveal_and_vote' WHERE id = $1
		`, roundID); err != nil {
			a.internalError(w, r, err)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE games SET phase = 'reveal_and_vote', updated_at = now() WHERE id = $1
		`, gameID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleVoteCurrentRound(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var input roundVoteInput
	if !decodeRequest(w, r, &input) {
		return
	}
	if input.SubmissionID == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_vote", "A submission is required")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID, roundID, subjectID, status string
	err = tx.QueryRow(r.Context(), `
		SELECT game.id, round.id, round.subject_player_id, round.status
		FROM games AS game
		JOIN game_rounds AS round
		  ON round.game_id = game.id AND round.round_number = game.current_round_number
		WHERE game.lobby_id = $1
		FOR UPDATE OF game, round
	`, player.LobbyID).Scan(&gameID, &roundID, &subjectID, &status)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if player.ID != subjectID {
		writeError(w, http.StatusForbidden, "subject_required", "Only the round subject can vote")
		return
	}
	if status == "round_results" {
		state, stateErr := a.loadLobbyState(r.Context(), player)
		if stateErr != nil {
			a.internalError(w, r, stateErr)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if status != "reveal_and_vote" {
		writeError(w, http.StatusConflict, "vote_closed", "Voting is not open")
		return
	}
	var votedPlayerID string
	err = tx.QueryRow(r.Context(), `
		SELECT submission.player_id
		FROM round_submissions AS submission
		WHERE submission.id = $1 AND submission.round_id = $2 AND submission.kind = 'prediction'
	`, input.SubmissionID, roundID).Scan(&votedPlayerID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_vote", "The selected profile is not eligible")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	snapshot, err := loadQuestionnaireSnapshot(r.Context(), tx, player.LobbyID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	official, predictionList, err := loadStoredRoundSubmissions(r.Context(), tx, roundID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	for _, prediction := range predictionList {
		baseScore, loverAdjustment, taglineBonus, totalScore, exactCount, lines, scoreErr :=
			scorePrediction(snapshot, official, prediction, prediction.ID == input.SubmissionID)
		if scoreErr != nil {
			a.internalError(w, r, scoreErr)
			return
		}
		lineJSON, err := json.Marshal(lines)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE round_submissions
			SET base_score = $1, lover_adjustment = $2, tagline_bonus = $3,
			    total_score = $4, exact_count = $5, score_lines = $6
			WHERE id = $7
		`, baseScore, loverAdjustment, taglineBonus, totalScore, exactCount, lineJSON,
			prediction.ID); err != nil {
			a.internalError(w, r, err)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE game_participants
			SET cumulative_score = cumulative_score + $1,
			    exact_count = exact_count + $2,
			    tagline_bonus_count = tagline_bonus_count + CASE WHEN $3 > 0 THEN 1 ELSE 0 END
			WHERE game_id = $4 AND player_id = $5
		`, totalScore, exactCount, taglineBonus, gameID, prediction.PlayerID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE game_rounds
		SET voted_submission_id = $1, status = 'round_results', completed_at = now()
		WHERE id = $2
	`, input.SubmissionID, roundID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE games SET phase = 'round_results', updated_at = now() WHERE id = $1
	`, gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
	`, player.LobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleNextRound(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can close the current round")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var gameID, phase string
	var currentRound int
	err = tx.QueryRow(r.Context(), `
		SELECT id, phase, current_round_number FROM games WHERE lobby_id = $1 FOR UPDATE
	`, player.LobbyID).Scan(&gameID, &phase, &currentRound)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if phase == "between_rounds" || phase == "completed" {
		_ = tx.Rollback(r.Context())
		state, stateErr := a.loadLobbyState(r.Context(), player)
		if stateErr != nil {
			a.internalError(w, r, stateErr)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if phase != "round_results" {
		writeError(w, http.StatusConflict, "round_not_finished", "The current round is not finished")
		return
	}
	if err := moveToIntermissionOrComplete(r.Context(), tx, player.LobbyID, gameID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleStartNextRound(w http.ResponseWriter, r *http.Request) {
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
	var gameID, phase string
	var currentRound int
	err = tx.QueryRow(r.Context(), `
		SELECT id, phase, current_round_number FROM games WHERE lobby_id = $1 FOR UPDATE
	`, player.LobbyID).Scan(&gameID, &phase, &currentRound)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if phase == "collecting_submissions" || phase == "completed" {
		_ = tx.Rollback(r.Context())
		state, stateErr := a.loadLobbyState(r.Context(), player)
		if stateErr != nil {
			a.internalError(w, r, stateErr)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if phase != "between_rounds" {
		writeError(w, http.StatusConflict, "intermission_required", "Return to the lobby before starting the next round")
		return
	}

	rows, err := tx.Query(r.Context(), `
		SELECT player_id
		FROM game_participants
		WHERE game_id = $1 AND is_active
		ORDER BY seat
	`, gameID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	activePlayerIDs := make([]string, 0, maximumPlayerCount)
	for rows.Next() {
		var activePlayerID string
		if err := rows.Scan(&activePlayerID); err != nil {
			rows.Close()
			a.internalError(w, r, err)
			return
		}
		activePlayerIDs = append(activePlayerIDs, activePlayerID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.internalError(w, r, err)
		return
	}
	rows.Close()
	if len(activePlayerIDs) < minimumPlayerCount {
		if err := completeGame(r.Context(), tx, player.LobbyID, gameID); err != nil {
			a.internalError(w, r, err)
			return
		}
	} else {
		for _, activePlayerID := range activePlayerIDs {
			if !a.hub.isOnline(player.Code, activePlayerID) {
				writeError(w, http.StatusConflict, "players_offline", "Every active player must be connected before the next round starts")
				return
			}
		}
		nextSubjectID, subjectErr := findNextSubject(r.Context(), tx, gameID)
		if errors.Is(subjectErr, pgx.ErrNoRows) {
			if err := completeGame(r.Context(), tx, player.LobbyID, gameID); err != nil {
				a.internalError(w, r, err)
				return
			}
		} else if subjectErr != nil {
			a.internalError(w, r, subjectErr)
			return
		} else {
			nextRound := currentRound + 1
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO game_rounds (game_id, round_number, subject_player_id)
				VALUES ($1, $2, $3)
			`, gameID, nextRound, nextSubjectID); err != nil {
				a.internalError(w, r, err)
				return
			}
			if _, err := tx.Exec(r.Context(), `
				UPDATE games
				SET phase = 'collecting_submissions', current_round_number = $1,
				    total_rounds = (SELECT count(*) FROM game_rounds WHERE game_id = $2) +
				      (SELECT count(*) FROM game_participants AS participant
				       WHERE participant.game_id = $2 AND participant.is_active
				         AND NOT EXISTS (
				           SELECT 1 FROM game_rounds AS round
				           WHERE round.game_id = participant.game_id
				             AND round.subject_player_id = participant.player_id
				         )),
				    updated_at = now()
				WHERE id = $2
			`, nextRound, gameID); err != nil {
				a.internalError(w, r, err)
				return
			}
			if _, err := tx.Exec(r.Context(), `
				UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
			`, player.LobbyID); err != nil {
				a.internalError(w, r, err)
				return
			}
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleExcludePlayer(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the host can exclude a disconnected player")
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
	var disconnectedAt *time.Time
	var targetIsHost bool
	err = tx.QueryRow(r.Context(), `
		SELECT disconnected_at, is_host
		FROM players
		WHERE id = $1 AND lobby_id = $2 AND excluded_at IS NULL
		FOR UPDATE
	`, targetPlayerID, player.LobbyID).Scan(&disconnectedAt, &targetIsHost)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "player_not_found", "Player not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if targetIsHost || disconnectedAt == nil || time.Since(*disconnectedAt) < reconnectGracePeriod || a.hub.isOnline(player.Code, targetPlayerID) {
		writeError(w, http.StatusConflict, "reconnect_grace_active", "This player can only be excluded after 90 seconds offline")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE players SET excluded_at = now(), updated_at = now() WHERE id = $1
	`, targetPlayerID); err != nil {
		a.internalError(w, r, err)
		return
	}

	var lobbyStatus string
	if err := tx.QueryRow(r.Context(), `SELECT status FROM lobbies WHERE id = $1 FOR UPDATE`, player.LobbyID).Scan(&lobbyStatus); err != nil {
		a.internalError(w, r, err)
		return
	}
	if lobbyStatus == "in_game" {
		if err := excludeGameParticipant(r.Context(), tx, player.LobbyID, targetPlayerID); err != nil {
			a.internalError(w, r, err)
			return
		}
	} else {
		if _, err := tx.Exec(r.Context(), `
			UPDATE lobbies
			SET status = CASE
			      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL) < $2
			        THEN 'waiting_for_players'::lobby_status
			      WHEN NOT (SELECT bool_and(ready_status = 'ready') FROM players WHERE lobby_id = $1 AND excluded_at IS NULL)
			        THEN 'preparing_photos'::lobby_status
			      ELSE 'ready_to_start'::lobby_status
			    END,
			    revision = revision + 1, updated_at = now()
			WHERE id = $1
		`, player.LobbyID, minimumPlayerCount); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
}

func loadQuestionnaireSnapshot(ctx context.Context, tx pgx.Tx, lobbyID string) (questionnaireSnapshot, error) {
	var snapshotJSON []byte
	if err := tx.QueryRow(ctx, `SELECT game_config_snapshot FROM lobbies WHERE id = $1`, lobbyID).Scan(&snapshotJSON); err != nil {
		return questionnaireSnapshot{}, err
	}
	var snapshot questionnaireSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return questionnaireSnapshot{}, err
	}
	if len(snapshot.ProfileFields) == 0 {
		snapshot.ProfileFields = defaultProfileFields()
	}
	return snapshot, nil
}

func validateRoundSubmission(snapshot questionnaireSnapshot, input roundSubmissionInput, kind string) error {
	if len(input.BioAnswers) != len(snapshot.ProfileFields) {
		return validationError{"answer every profile field"}
	}
	for _, field := range snapshot.ProfileFields {
		value, exists := input.BioAnswers[field.ID]
		if !exists || !containsOption(field.Options, value) {
			return validationError{fmt.Sprintf("invalid answer for profile field %s", field.ID)}
		}
	}
	if len(input.QuestionAnswers) != len(snapshot.Questions) {
		return validationError{"answer every questionnaire question"}
	}
	for _, item := range snapshot.Questions {
		answer, exists := input.QuestionAnswers[item.ID]
		if !exists {
			return validationError{fmt.Sprintf("missing answer for question %s", item.ID)}
		}
		switch item.Type {
		case "integer_range":
			var value int
			if err := json.Unmarshal(answer, &value); err != nil || item.Minimum == nil || item.Maximum == nil || value < *item.Minimum || value > *item.Maximum {
				return validationError{fmt.Sprintf("invalid answer for question %s", item.ID)}
			}
		case "single_choice", "binary_choice":
			var value string
			if err := json.Unmarshal(answer, &value); err != nil || !containsOption(item.Options, value) {
				return validationError{fmt.Sprintf("invalid answer for question %s", item.ID)}
			}
		default:
			return validationError{fmt.Sprintf("unsupported question type %s", item.Type)}
		}
	}
	if kind == "official" {
		if strings.TrimSpace(input.Tagline) != "" || input.LoverQuestionID != "" {
			return validationError{"the subject cannot submit a tagline or LOVER"}
		}
		return nil
	}
	taglineLength := len([]rune(strings.TrimSpace(input.Tagline)))
	if taglineLength < 1 || taglineLength > 100 {
		return validationError{"tagline must contain between 1 and 100 characters"}
	}
	loverQuestion := findQuestion(snapshot.Questions, input.LoverQuestionID)
	if loverQuestion == nil || !loverQuestion.LoverEligible {
		return validationError{"select one eligible LOVER question"}
	}
	return nil
}

func containsOption(options []questionOption, value string) bool {
	for _, option := range options {
		if option.ID == value {
			return true
		}
	}
	return false
}

func findQuestion(questionList []question, questionID string) *question {
	for index := range questionList {
		if questionList[index].ID == questionID {
			return &questionList[index]
		}
	}
	return nil
}

func rawAnswerMapsEqual(first, second map[string]json.RawMessage) bool {
	if len(first) != len(second) {
		return false
	}
	for key, firstValue := range first {
		secondValue, exists := second[key]
		if !exists || !bytes.Equal(bytes.TrimSpace(firstValue), bytes.TrimSpace(secondValue)) {
			return false
		}
	}
	return true
}

func loadStoredRoundSubmissions(ctx context.Context, tx pgx.Tx, roundID string) (storedSubmission, []storedSubmission, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, player_id, kind, COALESCE(tagline, ''), bio_answers,
		       question_answers, COALESCE(lover_question_id, '')
		FROM round_submissions WHERE round_id = $1 ORDER BY submitted_at, id
	`, roundID)
	if err != nil {
		return storedSubmission{}, nil, err
	}
	defer rows.Close()
	var official storedSubmission
	predictions := make([]storedSubmission, 0)
	for rows.Next() {
		var submission storedSubmission
		var kind string
		var bioJSON, questionJSON []byte
		if err := rows.Scan(
			&submission.ID,
			&submission.PlayerID,
			&kind,
			&submission.Tagline,
			&bioJSON,
			&questionJSON,
			&submission.LoverQuestionID,
		); err != nil {
			return storedSubmission{}, nil, err
		}
		if err := json.Unmarshal(bioJSON, &submission.BioAnswers); err != nil {
			return storedSubmission{}, nil, err
		}
		if err := json.Unmarshal(questionJSON, &submission.QuestionAnswers); err != nil {
			return storedSubmission{}, nil, err
		}
		if kind == "official" {
			official = submission
		} else {
			predictions = append(predictions, submission)
		}
	}
	if err := rows.Err(); err != nil {
		return storedSubmission{}, nil, err
	}
	if official.ID == "" || len(predictions) == 0 {
		return storedSubmission{}, nil, errors.New("round submissions are incomplete")
	}
	return official, predictions, nil
}

func scorePrediction(
	snapshot questionnaireSnapshot,
	official storedSubmission,
	prediction storedSubmission,
	selectedTagline bool,
) (int, int, int, int, int, []scoreLine, error) {
	baseTotal := 0
	loverAdjustment := 0
	exactCount := 0
	lineList := make([]scoreLine, 0, len(snapshot.ProfileFields)+len(snapshot.Questions))
	for _, field := range snapshot.ProfileFields {
		officialValue := official.BioAnswers[field.ID]
		predictedValue := prediction.BioAnswers[field.ID]
		exact := officialValue == predictedValue
		baseScore := 0
		if exact {
			baseScore = 10
			exactCount++
		}
		baseTotal += baseScore
		lineList = append(lineList, scoreLine{
			ID: field.ID, Label: field.Label, OfficialAnswer: officialValue,
			PredictedAnswer: predictedValue, BaseScore: baseScore, MaximumScore: 10,
			FinalScore: baseScore, Exact: exact,
		})
	}
	for _, item := range snapshot.Questions {
		officialRaw := official.QuestionAnswers[item.ID]
		predictedRaw := prediction.QuestionAnswers[item.ID]
		var officialValue, predictedValue any
		baseScore := 0
		exact := false
		switch item.Type {
		case "integer_range":
			var officialInteger, predictedInteger int
			if err := json.Unmarshal(officialRaw, &officialInteger); err != nil {
				return 0, 0, 0, 0, 0, nil, err
			}
			if err := json.Unmarshal(predictedRaw, &predictedInteger); err != nil {
				return 0, 0, 0, 0, 0, nil, err
			}
			officialValue, predictedValue = officialInteger, predictedInteger
			exact = officialInteger == predictedInteger
			baseScore = scoring.IntegerRangeScore(officialInteger, predictedInteger, *item.Minimum, *item.Maximum, item.MaximumScore)
		case "single_choice", "binary_choice":
			var officialChoice, predictedChoice string
			if err := json.Unmarshal(officialRaw, &officialChoice); err != nil {
				return 0, 0, 0, 0, 0, nil, err
			}
			if err := json.Unmarshal(predictedRaw, &predictedChoice); err != nil {
				return 0, 0, 0, 0, 0, nil, err
			}
			officialValue, predictedValue = officialChoice, predictedChoice
			exact = officialChoice == predictedChoice
			baseScore = scoring.SingleChoiceScore(officialChoice, predictedChoice, item.MaximumScore)
		default:
			return 0, 0, 0, 0, 0, nil, fmt.Errorf("unsupported question type %s", item.Type)
		}
		isLover := prediction.LoverQuestionID == item.ID
		finalScore := scoring.FinalScore(scoring.AnswerScore{
			BaseScore: baseScore, MaximumScore: item.MaximumScore,
			Exact: exact, LoverSelected: isLover,
		})
		baseTotal += baseScore
		loverAdjustment += finalScore - baseScore
		if exact {
			exactCount++
		}
		lineList = append(lineList, scoreLine{
			ID: item.ID, Label: item.Label, OfficialAnswer: officialValue,
			PredictedAnswer: predictedValue, BaseScore: baseScore,
			MaximumScore: item.MaximumScore, FinalScore: finalScore,
			Exact: exact, IsLoverApplied: isLover,
		})
	}
	taglineBonus := 0
	if selectedTagline {
		taglineBonus = bestTaglineBonus
	}
	return baseTotal, loverAdjustment, taglineBonus, baseTotal + loverAdjustment + taglineBonus, exactCount, lineList, nil
}

func findNextSubject(ctx context.Context, tx pgx.Tx, gameID string) (string, error) {
	var nextSubjectID string
	err := tx.QueryRow(ctx, `
		SELECT participant.player_id
		FROM game_participants AS participant
		WHERE participant.game_id = $1 AND participant.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM game_rounds AS round
		    WHERE round.game_id = participant.game_id
		      AND round.subject_player_id = participant.player_id
		  )
		ORDER BY participant.seat
		LIMIT 1
	`, gameID).Scan(&nextSubjectID)
	return nextSubjectID, err
}

func completeGame(ctx context.Context, tx pgx.Tx, lobbyID, gameID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE games SET phase = 'completed', completed_at = now(), updated_at = now() WHERE id = $1
	`, gameID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE lobbies SET status = 'completed', revision = revision + 1, updated_at = now() WHERE id = $1
	`, lobbyID)
	return err
}

func updateGameRoundCount(ctx context.Context, tx pgx.Tx, gameID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE games
		SET total_rounds = (SELECT count(*) FROM game_rounds WHERE game_id = $1) +
		      (SELECT count(*) FROM game_participants AS participant
		       WHERE participant.game_id = $1 AND participant.is_active
		         AND NOT EXISTS (
		           SELECT 1 FROM game_rounds AS round
		           WHERE round.game_id = participant.game_id
		             AND round.subject_player_id = participant.player_id
		         )),
		    updated_at = now()
		WHERE id = $1
	`, gameID)
	return err
}

func moveToIntermissionOrComplete(ctx context.Context, tx pgx.Tx, lobbyID, gameID string) error {
	var activeCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM game_participants WHERE game_id = $1 AND is_active`, gameID).Scan(&activeCount); err != nil {
		return err
	}
	_, err := findNextSubject(ctx, tx, gameID)
	if activeCount < minimumPlayerCount || errors.Is(err, pgx.ErrNoRows) {
		return completeGame(ctx, tx, lobbyID, gameID)
	}
	if err != nil {
		return err
	}
	if err := updateGameRoundCount(ctx, tx, gameID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE games
		SET phase = 'between_rounds', updated_at = now()
		WHERE id = $1
	`, gameID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
	`, lobbyID); err != nil {
		return err
	}
	return nil
}

func excludeGameParticipant(ctx context.Context, tx pgx.Tx, lobbyID, targetPlayerID string) error {
	var gameID, phase, roundID, subjectID string
	var currentRound int
	err := tx.QueryRow(ctx, `
		SELECT game.id, game.phase, game.current_round_number, round.id, round.subject_player_id
		FROM games AS game
		JOIN game_rounds AS round
		  ON round.game_id = game.id AND round.round_number = game.current_round_number
		WHERE game.lobby_id = $1
		FOR UPDATE OF game, round
	`, lobbyID).Scan(&gameID, &phase, &currentRound, &roundID, &subjectID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE game_participants SET is_active = false WHERE game_id = $1 AND player_id = $2
	`, gameID, targetPlayerID); err != nil {
		return err
	}
	var activeCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM game_participants WHERE game_id = $1 AND is_active`, gameID).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount < minimumPlayerCount {
		if _, err := tx.Exec(ctx, `
			UPDATE games SET phase = 'completed', completed_at = now(), updated_at = now() WHERE id = $1
		`, gameID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE lobbies SET status = 'completed', revision = revision + 1, updated_at = now() WHERE id = $1
		`, lobbyID)
		return err
	}
	if phase == "between_rounds" {
		_, nextSubjectErr := findNextSubject(ctx, tx, gameID)
		if errors.Is(nextSubjectErr, pgx.ErrNoRows) {
			return completeGame(ctx, tx, lobbyID, gameID)
		}
		if nextSubjectErr != nil {
			return nextSubjectErr
		}
	}
	if targetPlayerID == subjectID && (phase == "collecting_submissions" || phase == "reveal_and_vote") {
		if _, err := tx.Exec(ctx, `
			UPDATE game_rounds SET status = 'skipped', completed_at = now() WHERE id = $1
		`, roundID); err != nil {
			return err
		}
		return moveToIntermissionOrComplete(ctx, tx, lobbyID, gameID)
	}
	if phase == "collecting_submissions" {
		var requiredCount, submittedCount int
		if err := tx.QueryRow(ctx, `
			SELECT
			  (SELECT count(*) FROM game_participants WHERE game_id = $1 AND is_active),
			  (SELECT count(*)
			   FROM round_submissions AS submission
			   JOIN game_participants AS participant
			     ON participant.game_id = $1 AND participant.player_id = submission.player_id
			   WHERE submission.round_id = $2 AND participant.is_active)
		`, gameID, roundID).Scan(&requiredCount, &submittedCount); err != nil {
			return err
		}
		if requiredCount == submittedCount {
			if _, err := tx.Exec(ctx, `
				UPDATE game_rounds SET status = 'reveal_and_vote' WHERE id = $1
			`, roundID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE games SET phase = 'reveal_and_vote', updated_at = now() WHERE id = $1
			`, gameID); err != nil {
				return err
			}
		}
	}
	if err = updateGameRoundCount(ctx, tx, gameID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
	`, lobbyID)
	return err
}
