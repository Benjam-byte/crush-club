package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func (a *api) authenticatePlayerToken(ctx context.Context, code, rawToken string) (authenticatedPlayer, error) {
	if rawToken == "" {
		return authenticatedPlayer{}, errNotFound
	}
	tokenHash := sha256.Sum256([]byte(rawToken))
	var player authenticatedPlayer
	err := a.pool.QueryRow(ctx, `
		UPDATE players AS player
		SET last_seen_at = now(), updated_at = now()
		FROM lobbies AS lobby
		WHERE player.lobby_id = lobby.id
		  AND lobby.code = $1
		  AND lobby.status <> 'expired'
		  AND lobby.expires_at > now()
		  AND player.reconnect_token_hash = $2
		  AND player.excluded_at IS NULL
		RETURNING player.id, player.lobby_id, lobby.code, player.display_name, player.is_host
	`, strings.ToUpper(code), hex.EncodeToString(tokenHash[:])).Scan(
		&player.ID,
		&player.LobbyID,
		&player.Code,
		&player.DisplayName,
		&player.IsHost,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authenticatedPlayer{}, errNotFound
	}
	return player, err
}

func (a *api) requirePlayer(w http.ResponseWriter, r *http.Request) (authenticatedPlayer, bool) {
	player, err := a.authenticatePlayerToken(r.Context(), r.PathValue("code"), bearerToken(r))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusUnauthorized, "player_session_required", "A valid player session is required")
		return authenticatedPlayer{}, false
	}
	if err != nil {
		a.internalError(w, r, err)
		return authenticatedPlayer{}, false
	}
	return player, true
}

func (a *api) handleGetLobbyState(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *api) loadLobbyState(ctx context.Context, currentPlayer authenticatedPlayer) (lobbyStateResponse, error) {
	state := lobbyStateResponse{
		ServerTime:      time.Now().UTC(),
		CurrentPlayerID: currentPlayer.ID,
		Players:         make([]lobbyPlayerView, 0),
	}
	var snapshotJSON []byte
	err := a.pool.QueryRow(ctx, `
		SELECT code, status, mode, max_players, revision, game_config_snapshot
		FROM lobbies
		WHERE id = $1 AND status <> 'expired' AND expires_at > now()
	`, currentPlayer.LobbyID).Scan(
		&state.Code,
		&state.Status,
		&state.Mode,
		&state.MaxPlayers,
		&state.Revision,
		&snapshotJSON,
	)
	if err != nil {
		return lobbyStateResponse{}, err
	}
	if err := json.Unmarshal(snapshotJSON, &state.Questionnaire); err != nil {
		return lobbyStateResponse{}, err
	}
	hydrateQuestionnaireSnapshot(&state.Questionnaire)
	state.GameConfig = lobbyGameConfigSummary{
		ID:            state.Questionnaire.SourceConfigID,
		Name:          state.Questionnaire.Name,
		Version:       state.Questionnaire.SourceVersion,
		QuestionCount: len(state.Questionnaire.Questions),
	}

	rows, err := a.pool.Query(ctx, `
		SELECT player.id, player.display_name, player.is_host, player.ready_status,
		       player.disconnected_at, player.joined_at,
		       COALESCE((
		         SELECT array_agg(photo.id::text ORDER BY photo.position)
		         FROM player_photos AS photo WHERE photo.player_id = player.id
		       ), ARRAY[]::text[])
		FROM players AS player
		WHERE player.lobby_id = $1
		  AND (player.excluded_at IS NULL OR $2::text = 'completed')
		ORDER BY player.joined_at, player.id
	`, currentPlayer.LobbyID, state.Status)
	if err != nil {
		return lobbyStateResponse{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var player lobbyPlayerView
		if err := rows.Scan(
			&player.ID,
			&player.DisplayName,
			&player.IsHost,
			&player.ReadyStatus,
			&player.DisconnectedAt,
			&player.JoinedAt,
			&player.PhotoIDs,
		); err != nil {
			return lobbyStateResponse{}, err
		}
		player.Connected = a.hub != nil && a.hub.isOnline(state.Code, player.ID)
		if player.Connected {
			player.DisconnectedAt = nil
		} else if player.DisconnectedAt != nil && player.IsHost {
			deadline := player.DisconnectedAt.Add(reconnectGracePeriod)
			player.ReconnectDeadline = &deadline
		}
		player.CanExclude = canExcludeLobbyPlayer(player.IsHost, player.Connected, player.DisconnectedAt)
		state.Players = append(state.Players, player)
	}
	if err := rows.Err(); err != nil {
		return lobbyStateResponse{}, err
	}

	if state.Mode == lobbyModeFastBio {
		fastBioGame, err := a.loadFastBioState(ctx, currentPlayer)
		if errors.Is(err, pgx.ErrNoRows) {
			return state, nil
		}
		if err != nil {
			return lobbyStateResponse{}, err
		}
		state.FastBioGame = &fastBioGame
		return state, nil
	}

	if state.Mode == lobbyModeZeroToHundred {
		zeroToHundredGame, err := a.loadZeroToHundredState(ctx, currentPlayer)
		if errors.Is(err, pgx.ErrNoRows) {
			return state, nil
		}
		if err != nil {
			return lobbyStateResponse{}, err
		}
		state.ZeroToHundredGame = &zeroToHundredGame
		return state, nil
	}

	if state.Mode == lobbyModeSituation {
		situationGame, err := a.loadSituationState(ctx, currentPlayer)
		if errors.Is(err, pgx.ErrNoRows) {
			return state, nil
		}
		if err != nil {
			return lobbyStateResponse{}, err
		}
		state.SituationGame = &situationGame
		return state, nil
	}

	game, err := a.loadGameState(ctx, currentPlayer)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return lobbyStateResponse{}, err
	}
	state.Game = &game
	return state, nil
}

func canExcludeLobbyPlayer(isHost, connected bool, disconnectedAt *time.Time) bool {
	return !isHost && !connected && disconnectedAt != nil
}

func (a *api) loadGameState(ctx context.Context, currentPlayer authenticatedPlayer) (gameStateView, error) {
	var game gameStateView
	var roundID string
	err := a.pool.QueryRow(ctx, `
		SELECT game.id, game.phase, game.current_round_number, game.total_rounds,
		       round.id, round.subject_player_id
		FROM games AS game
		JOIN game_rounds AS round
		  ON round.game_id = game.id AND round.round_number = game.current_round_number
		WHERE game.lobby_id = $1
	`, currentPlayer.LobbyID).Scan(
		&game.ID,
		&game.Phase,
		&game.RoundNumber,
		&game.TotalRounds,
		&roundID,
		&game.SubjectPlayerID,
	)
	if err != nil {
		return gameStateView{}, err
	}
	if currentPlayer.ID == game.SubjectPlayerID {
		game.Role = "subject"
	} else {
		game.Role = "cupid"
	}
	if err := a.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM game_participants
			WHERE game_id = $1 AND player_id = $2 AND is_active
		)
	`, game.ID, currentPlayer.ID).Scan(&game.IsParticipant); err != nil {
		return gameStateView{}, err
	}
	if game.Phase == "round_results" || game.Phase == "between_rounds" {
		err = a.pool.QueryRow(ctx, `
			SELECT participant.player_id
			FROM game_participants AS participant
			WHERE participant.game_id = $1 AND participant.is_active
			  AND NOT EXISTS (
			    SELECT 1 FROM game_rounds AS candidate
			    WHERE candidate.game_id = participant.game_id
			      AND candidate.subject_player_id = participant.player_id
			  )
			ORDER BY participant.seat
			LIMIT 1
		`, game.ID).Scan(&game.NextSubjectPlayerID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return gameStateView{}, err
		}
	}

	err = a.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM game_participants WHERE game_id = $1 AND is_active),
		  (SELECT count(*)
		   FROM round_submissions AS submission
		   JOIN game_participants AS participant
		     ON participant.game_id = $1 AND participant.player_id = submission.player_id
		   WHERE submission.round_id = $2 AND participant.is_active),
		  EXISTS(SELECT 1 FROM round_submissions WHERE round_id = $2 AND player_id = $3)
	`, game.ID, roundID, currentPlayer.ID).Scan(
		&game.RequiredCount,
		&game.SubmittedCount,
		&game.Submitted,
	)
	if err != nil {
		return gameStateView{}, err
	}

	if game.Phase == "reveal_and_vote" || game.Phase == "round_results" || game.Phase == "completed" {
		subjectIsVoting := game.Phase == "reveal_and_vote" && currentPlayer.ID == game.SubjectPlayerID
		revealAuthors := game.Phase == "round_results" || game.Phase == "completed" ||
			(game.Phase == "reveal_and_vote" && currentPlayer.ID != game.SubjectPlayerID)
		submissionList, err := a.loadRoundSubmissionViews(ctx, roundID, revealAuthors)
		if err != nil {
			return gameStateView{}, err
		}
		for index := range submissionList {
			submission := submissionList[index]
			if submission.PlayerID == game.SubjectPlayerID {
				if !subjectIsVoting {
					game.OfficialSubmission = &submission
				}
				continue
			}
			if subjectIsVoting {
				submission.BioAnswers = map[string]any{}
				submission.QuestionAnswers = map[string]any{}
				submission.LoverQuestionID = ""
			}
			if !revealAuthors {
				submission.PlayerID = ""
				submission.AuthorName = ""
			}
			game.Submissions = append(game.Submissions, submission)
		}
	}

	if game.Phase == "round_results" || game.Phase == "between_rounds" || game.Phase == "completed" {
		if game.Phase != "between_rounds" {
			results, err := a.loadRoundResults(ctx, roundID)
			if err != nil {
				return gameStateView{}, err
			}
			game.RoundResults = results
		}
		leaderboard, err := a.loadLeaderboard(ctx, game.ID, roundID)
		if err != nil {
			return gameStateView{}, err
		}
		game.Leaderboard = leaderboard
	}

	return game, nil
}

func (a *api) loadRoundSubmissionViews(ctx context.Context, roundID string, revealAuthors bool) ([]roundSubmissionView, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT submission.id, submission.player_id, player.display_name,
		       COALESCE(submission.tagline, ''), submission.bio_answers,
		       submission.question_answers, COALESCE(submission.lover_question_id, ''),
		       submission.submitted_at
		FROM round_submissions AS submission
		JOIN players AS player ON player.id = submission.player_id
		WHERE submission.round_id = $1
		ORDER BY submission.submitted_at, submission.id
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]roundSubmissionView, 0)
	for rows.Next() {
		var view roundSubmissionView
		var bioJSON, questionJSON []byte
		if err := rows.Scan(
			&view.ID,
			&view.PlayerID,
			&view.AuthorName,
			&view.Tagline,
			&bioJSON,
			&questionJSON,
			&view.LoverQuestionID,
			&view.SubmittedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(bioJSON, &view.BioAnswers); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(questionJSON, &view.QuestionAnswers); err != nil {
			return nil, err
		}
		if !revealAuthors {
			view.AuthorName = ""
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

func (a *api) loadRoundResults(ctx context.Context, roundID string) ([]roundResultView, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT submission.player_id, player.display_name, submission.base_score,
		       submission.lover_adjustment, submission.tagline_bonus,
		       submission.total_score, submission.exact_count, submission.score_lines
		FROM round_submissions AS submission
		JOIN players AS player ON player.id = submission.player_id
		WHERE submission.round_id = $1 AND submission.kind = 'prediction'
		ORDER BY submission.total_score DESC, submission.exact_count DESC, submission.submitted_at
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]roundResultView, 0)
	for rows.Next() {
		var view roundResultView
		if err := rows.Scan(
			&view.PlayerID,
			&view.DisplayName,
			&view.BaseScore,
			&view.LoverAdjustment,
			&view.TaglineBonus,
			&view.TotalScore,
			&view.ExactCount,
			&view.ScoreLines,
		); err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

func (a *api) loadLeaderboard(ctx context.Context, gameID, roundID string) ([]leaderboardEntryView, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT participant.player_id, player.display_name, participant.cumulative_score,
		       COALESCE(submission.total_score, 0), participant.exact_count,
		       participant.tagline_bonus_count
		FROM game_participants AS participant
		JOIN players AS player ON player.id = participant.player_id
		LEFT JOIN round_submissions AS submission
		  ON submission.round_id = $2 AND submission.player_id = participant.player_id
		 AND submission.kind = 'prediction'
		WHERE participant.game_id = $1
		ORDER BY participant.cumulative_score DESC, participant.exact_count DESC,
		         participant.seat
	`, gameID, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]leaderboardEntryView, 0)
	for rows.Next() {
		var view leaderboardEntryView
		if err := rows.Scan(
			&view.PlayerID,
			&view.DisplayName,
			&view.Score,
			&view.RoundScore,
			&view.ExactCount,
			&view.TaglineBonusCount,
		); err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, rows.Err()
}
