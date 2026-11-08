package main

import "context"

// The "awaiting" helpers list the active players a phase is still waiting
// for, in lobby order. They are both what the clients show ("on attend
// encore …") and what the handlers check before closing a phase, so a player
// who left is never waited for, and never counted as having answered.

func queryPlayerIDs(ctx context.Context, db dbQuerier, query string, args ...any) ([]string, error) {
	rows, err := db.Query(ctx, query, args...)
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

// themeStepTable names where one theme step's answers are stored.
type themeStepTable struct {
	name         string
	playerColumn string
	// perRound is true for Wavelength, which collects themes every round.
	perRound bool
}

var (
	fastBioThemeSubmissions       = themeStepTable{name: "fast_bio_theme_submissions", playerColumn: "player_id"}
	fastBioThemeRankings          = themeStepTable{name: "fast_bio_theme_rankings", playerColumn: "voter_player_id"}
	zeroToHundredThemeSubmissions = themeStepTable{name: "zero_to_100_theme_submissions", playerColumn: "player_id"}
	zeroToHundredThemeRankings    = themeStepTable{name: "zero_to_100_theme_rankings", playerColumn: "voter_player_id"}
	situationThemeSubmissions     = themeStepTable{name: "situation_theme_submissions", playerColumn: "player_id"}
	situationThemeRankings        = themeStepTable{name: "situation_theme_rankings", playerColumn: "voter_player_id"}
	wavelengthThemeSubmissions    = themeStepTable{name: "wavelength_theme_submissions", playerColumn: "player_id", perRound: true}
	wavelengthThemeRankings       = themeStepTable{name: "wavelength_theme_rankings", playerColumn: "voter_player_id", perRound: true}
	secretRankingThemeSubmissions = themeStepTable{name: "secret_ranking_theme_submissions", playerColumn: "player_id"}
	secretRankingThemeRankings    = themeStepTable{name: "secret_ranking_theme_rankings", playerColumn: "voter_player_id"}
)

// themeStepAwaitingIDs lists the active players who have not answered a
// theme step yet. roundNumber only matters for per-round tables.
func themeStepAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID string, table themeStepTable, gameID string, roundNumber int) ([]string, error) {
	query := `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM ` + table.name + ` AS answer
		    WHERE answer.game_id = $2 AND answer.` + table.playerColumn + ` = player.id`
	args := []any{lobbyID, gameID}
	if table.perRound {
		query += ` AND answer.round_number = $3`
		args = append(args, roundNumber)
	}
	query += `
		  )
		ORDER BY player.joined_at, player.id`
	return queryPlayerIDs(ctx, db, query, args...)
}

// stepProgress is a phase's "X/Y" counters, derived from the awaited players
// so both always agree.
type stepProgress struct {
	awaiting []string
	count    int
	required int
}

func newStepProgress(required int, awaiting []string) stepProgress {
	return stepProgress{awaiting: awaiting, count: max(required-len(awaiting), 0), required: required}
}

func loadThemeStepProgress(ctx context.Context, db dbQuerier, lobbyID string, table themeStepTable, gameID string, roundNumber int) (stepProgress, error) {
	required, err := activeLobbyPlayerCount(ctx, db, lobbyID)
	if err != nil {
		return stepProgress{}, err
	}
	awaiting, err := themeStepAwaitingIDs(ctx, db, lobbyID, table, gameID, roundNumber)
	if err != nil {
		return stepProgress{}, err
	}
	return newStepProgress(required, awaiting), nil
}

// fastBioSubmissionAwaitingIDs lists the active authors of a round who have
// not sent their bio yet. Players admitted mid-round have no target, so they
// are not waited for.
func fastBioSubmissionAwaitingIDs(ctx context.Context, db dbQuerier, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT author.id
		FROM fast_bio_assignments AS assignment
		JOIN players AS author ON author.id = assignment.author_player_id
		WHERE assignment.round_id = $1 AND author.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM fast_bio_proposals AS proposal
		    WHERE proposal.round_id = $1 AND proposal.author_player_id = author.id
		  )
		ORDER BY author.joined_at, author.id
	`, roundID)
}

func fastBioActiveAuthorCount(ctx context.Context, db dbQuerier, roundID string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM fast_bio_assignments AS assignment
		JOIN players AS author ON author.id = assignment.author_player_id
		WHERE assignment.round_id = $1 AND author.is_active
	`, roundID).Scan(&count)
	return count, err
}

// fastBioReactionAwaitingIDs lists the active players, author excepted, who
// have not reacted to the proposal being reviewed.
func fastBioReactionAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, proposalID, authorPlayerID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active AND player.id <> $3
		  AND NOT EXISTS (
		    SELECT 1 FROM fast_bio_reactions AS reaction
		    WHERE reaction.proposal_id = $2 AND reaction.voter_player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, proposalID, authorPlayerID)
}

func zeroToHundredGuessAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM zero_to_100_guesses AS guess
		    WHERE guess.round_id = $2 AND guess.guesser_player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, roundID)
}

func situationProposalAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM situation_proposals AS proposal
		    WHERE proposal.round_id = $2 AND proposal.author_player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, roundID)
}

// situationDuelAwaitingIDs lists the representatives of the open duels who
// have not voted yet. Two who voted differently are not awaited: the duel's
// deadline decides between them.
func situationDuelAwaitingIDs(ctx context.Context, db dbQuerier, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id
		FROM situation_duels AS duel
		CROSS JOIN LATERAL (VALUES
		  (duel.representative_a_player_id, duel.vote_a_proposal_id),
		  (duel.representative_b_player_id, duel.vote_b_proposal_id)
		) AS side(player_id, vote_proposal_id)
		JOIN players AS player ON player.id = side.player_id
		WHERE duel.round_id = $1 AND duel.resolved_at IS NULL
		  AND side.vote_proposal_id IS NULL AND player.is_active
		ORDER BY player.joined_at, player.id
	`, roundID)
}

func situationRankingAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM situation_final_rankings AS ranking
		    WHERE ranking.round_id = $2 AND ranking.voter_player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, roundID)
}

// wavelengthGuessAwaitingIDs lists the active players, psychic excepted, who
// have not placed their needle yet.
func wavelengthGuessAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, roundID, psychicPlayerID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active AND player.id <> $3
		  AND NOT EXISTS (
		    SELECT 1 FROM wavelength_guesses AS guess
		    WHERE guess.round_id = $2 AND guess.player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, roundID, psychicPlayerID)
}

// secretRankingAmountAwaitingIDs lists the active players who have not bet
// in the round yet.
func secretRankingAmountAwaitingIDs(ctx context.Context, db dbQuerier, lobbyID, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id FROM players AS player
		WHERE player.lobby_id = $1 AND player.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM secret_ranking_entries AS entry
		    WHERE entry.round_id = $2 AND entry.player_id = player.id
		  )
		ORDER BY player.joined_at, player.id
	`, lobbyID, roundID)
}

// secretRankingPositionAwaitingIDs lists the active participants whose phone
// has no number yet. Players who did not bet are not on the table.
func secretRankingPositionAwaitingIDs(ctx context.Context, db dbQuerier, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT player.id
		FROM secret_ranking_entries AS entry
		JOIN players AS player ON player.id = entry.player_id
		WHERE entry.round_id = $1 AND entry.position IS NULL AND player.is_active
		ORDER BY player.joined_at, player.id
	`, roundID)
}

// classicSubmissionAwaitingIDs lists the active participants of a classic
// round who have not locked their profile yet.
func classicSubmissionAwaitingIDs(ctx context.Context, db dbQuerier, gameID, roundID string) ([]string, error) {
	return queryPlayerIDs(ctx, db, `
		SELECT participant.player_id
		FROM game_participants AS participant
		WHERE participant.game_id = $1 AND participant.is_active
		  AND NOT EXISTS (
		    SELECT 1 FROM round_submissions AS submission
		    WHERE submission.round_id = $2 AND submission.player_id = participant.player_id
		  )
		ORDER BY participant.seat
	`, gameID, roundID)
}
