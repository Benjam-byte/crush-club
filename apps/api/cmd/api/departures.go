package main

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
)

// When an admitted player leaves or is kicked during a game, the game must
// stop waiting for them. Classic games are updated inside the departure's
// transaction (excludeGameParticipant). The uncapped modes are re-evaluated
// right after it commits, in their own transaction that locks the game row
// first, like their deadline handlers: below the mode's minimum the game
// ends, otherwise any phase that was only waiting for the departed player
// moves on.

// activePlayerMinimum is the number of active players a mode needs to go on.
func activePlayerMinimum(mode string) int {
	switch mode {
	case lobbyModeZeroToHundred:
		return zeroToHundredMinPlayerCount
	case lobbyModeSecretRanking:
		return secretRankingMinPlayerCount
	}
	return minimumPlayerCount
}

func (a *api) settleGameAfterDeparture(code, lobbyID, mode string) {
	for attempt := 1; attempt <= 2; attempt++ {
		err := a.settleGameOnce(code, lobbyID, mode)
		if err == nil {
			return
		}
		a.logger.Error("unable to settle the game after a departure",
			"lobby_code", code, "mode", mode, "attempt", attempt, "error", err)
	}
}

func (a *api) settleGameOnce(code, lobbyID, mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var timers phaseTimers
	switch mode {
	case lobbyModeFastBio:
		timers, err = a.settleFastBioGame(ctx, tx, code, lobbyID)
	case lobbyModeZeroToHundred:
		timers, err = a.settleZeroToHundredGame(ctx, tx, code, lobbyID)
	case lobbyModeSituation:
		timers, err = a.settleSituationGame(ctx, tx, code, lobbyID)
	case lobbyModeWavelength:
		timers, err = a.settleWavelengthGame(ctx, tx, code, lobbyID)
	case lobbyModeSecretRanking:
		timers, err = a.settleSecretRankingGame(ctx, tx, code, lobbyID)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if err := bumpLobbyRevision(ctx, tx, lobbyID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	a.hub.publish(code)
	timers.start()
	return nil
}

// hasEnoughActivePlayers reports whether the lobby can still play mode.
func hasEnoughActivePlayers(ctx context.Context, tx pgx.Tx, lobbyID, mode string) (bool, error) {
	count, err := activeLobbyPlayerCount(ctx, tx, lobbyID)
	return count >= activePlayerMinimum(mode), err
}

// abortUncappedGame ends a game that no longer has enough players. Its open
// round and the game itself are completed, so pending deadline timers find
// nothing left to do and everyone lands on the final leaderboard. Both table
// names are hardcoded per mode (never user input).
func abortUncappedGame(ctx context.Context, tx pgx.Tx, roundsTable, gamesTable, gameID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE `+roundsTable+` SET phase = 'completed', completed_at = COALESCE(completed_at, now())
		WHERE game_id = $1 AND phase <> 'completed'
	`, gameID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE `+gamesTable+` SET phase = 'completed', completed_at = now() WHERE id = $1
	`, gameID)
	return err
}

func (a *api) settleFastBioGame(ctx context.Context, tx pgx.Tx, code, lobbyID string) (phaseTimers, error) {
	gameID, phase, err := lockActiveFastBioGame(ctx, tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && phase == "completed") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enough, err := hasEnoughActivePlayers(ctx, tx, lobbyID, lobbyModeFastBio)
	if err != nil {
		return nil, err
	}
	if !enough {
		return nil, abortUncappedGame(ctx, tx, "fast_bio_rounds", "fast_bio_games", gameID)
	}

	var timers phaseTimers
	switch phase {
	case "collecting_themes":
		rankingDeadline, err := a.tryAdvanceFastBioThemeCollection(ctx, tx, gameID, lobbyID, false)
		if err != nil {
			return nil, err
		}
		if rankingDeadline != nil {
			timers.add(func() { a.scheduleFastBioThemeRankingTimeout(code, gameID, *rankingDeadline) })
		}
	case "ranking_themes":
		candidates, err := fastBioThemeCandidates(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		roundID, started, err := a.tryConcludeFastBioRanking(ctx, tx, gameID, lobbyID, candidates, false)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleFastBioRoundTimeout(code, roundID) })
		}
	case "playing":
		round, err := loadCurrentFastBioRound(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		// Reviews are paced by the host, whose "next" gate already ignores
		// departed players: only the submissions can be waiting on them.
		if round.Phase != "submitting" {
			return nil, nil
		}
		awaiting, err := fastBioSubmissionAwaitingIDs(ctx, tx, round.ID)
		if err != nil || len(awaiting) > 0 {
			return nil, err
		}
		nextRoundID, started, err := a.transitionFastBioRoundOutOfSubmitting(ctx, tx, round.ID)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleFastBioRoundTimeout(code, nextRoundID) })
		}
	}
	return timers, nil
}

func (a *api) settleZeroToHundredGame(ctx context.Context, tx pgx.Tx, code, lobbyID string) (phaseTimers, error) {
	gameID, phase, err := lockActiveZeroToHundredGame(ctx, tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && phase == "completed") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enough, err := hasEnoughActivePlayers(ctx, tx, lobbyID, lobbyModeZeroToHundred)
	if err != nil {
		return nil, err
	}
	if !enough {
		return nil, abortUncappedGame(ctx, tx, "zero_to_100_rounds", "zero_to_100_games", gameID)
	}

	var timers phaseTimers
	switch phase {
	case "collecting_themes":
		rankingDeadline, err := a.tryAdvanceZeroToHundredThemeCollection(ctx, tx, gameID, lobbyID, false)
		if err != nil {
			return nil, err
		}
		if rankingDeadline != nil {
			timers.add(func() { a.scheduleZeroToHundredThemeRankingTimeout(code, gameID, *rankingDeadline) })
		}
	case "ranking_themes":
		candidates, err := zeroToHundredThemeCandidates(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		roundID, started, err := a.tryConcludeZeroToHundredRanking(ctx, tx, gameID, lobbyID, candidates, false)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleZeroToHundredRoundTimeout(code, roundID) })
		}
	case "playing":
		round, err := loadCurrentZeroToHundredRound(ctx, tx, gameID)
		if err != nil || round.Phase != "guessing" {
			return nil, err
		}
		awaiting, err := zeroToHundredGuessAwaitingIDs(ctx, tx, lobbyID, round.ID)
		if err != nil || len(awaiting) > 0 {
			return nil, err
		}
		if err := a.closeZeroToHundredGuessing(ctx, tx, round.ID); err != nil {
			return nil, err
		}
	}
	return timers, nil
}

func (a *api) settleSituationGame(ctx context.Context, tx pgx.Tx, code, lobbyID string) (phaseTimers, error) {
	gameID, phase, err := lockActiveSituationGame(ctx, tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && phase == "completed") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enough, err := hasEnoughActivePlayers(ctx, tx, lobbyID, lobbyModeSituation)
	if err != nil {
		return nil, err
	}
	if !enough {
		if _, err := tx.Exec(ctx, `
			UPDATE situation_duels SET resolved_at = now()
			WHERE resolved_at IS NULL AND round_id IN (SELECT id FROM situation_rounds WHERE game_id = $1)
		`, gameID); err != nil {
			return nil, err
		}
		return nil, abortUncappedGame(ctx, tx, "situation_rounds", "situation_games", gameID)
	}

	var timers phaseTimers
	scheduleDuels := func(duelIDs []string) {
		for _, duelID := range duelIDs {
			timers.add(func() { a.scheduleSituationDuelTimeout(code, duelID) })
		}
	}
	switch phase {
	case "collecting_themes":
		rankingDeadline, err := a.tryAdvanceSituationThemeCollection(ctx, tx, gameID, lobbyID, false)
		if err != nil {
			return nil, err
		}
		if rankingDeadline != nil {
			timers.add(func() { a.scheduleSituationThemeRankingTimeout(code, gameID, *rankingDeadline) })
		}
	case "ranking_themes":
		candidates, err := situationThemeCandidates(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		roundID, started, err := a.tryConcludeSituationRanking(ctx, tx, gameID, lobbyID, candidates, false)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleSituationProposalTimeout(code, roundID) })
		}
	case "playing":
		round, err := loadCurrentSituationRound(ctx, tx, gameID)
		if err != nil {
			return nil, err
		}
		switch round.Phase {
		case "proposing":
			awaiting, err := situationProposalAwaitingIDs(ctx, tx, lobbyID, round.ID)
			if err != nil || len(awaiting) > 0 {
				return nil, err
			}
			duelIDs, err := advanceSituationBracket(ctx, tx, round.ID)
			if err != nil {
				return nil, err
			}
			scheduleDuels(duelIDs)
		case "dueling":
			duelIDs, err := replaceDepartedSituationRepresentatives(ctx, tx, round.ID)
			if err != nil {
				return nil, err
			}
			scheduleDuels(duelIDs)
		case "ranking":
			if _, err := tryConcludeSituationFinalRanking(ctx, tx, lobbyID, round.ID); err != nil {
				return nil, err
			}
		}
	}
	return timers, nil
}

// replaceDepartedSituationRepresentatives hands each open duel defended by a
// departed player to another active backer of the same proposal, whose vote
// starts blank. A proposal nobody is left to defend forfeits its duel. It
// returns the duels of the next wave, when a forfeit completed the current
// one.
func replaceDepartedSituationRepresentatives(ctx context.Context, tx pgx.Tx, roundID string) ([]string, error) {
	type openDuel struct {
		id                   string
		proposalA, proposalB string
		activeA, activeB     bool
		replacedA, replacedB string
	}
	rows, err := tx.Query(ctx, `
		SELECT duel.id, duel.proposal_a_id, duel.proposal_b_id, side_a.is_active, side_b.is_active
		FROM situation_duels AS duel
		JOIN players AS side_a ON side_a.id = duel.representative_a_player_id
		JOIN players AS side_b ON side_b.id = duel.representative_b_player_id
		WHERE duel.round_id = $1 AND duel.resolved_at IS NULL
		FOR UPDATE OF duel
	`, roundID)
	if err != nil {
		return nil, err
	}
	duels := make([]openDuel, 0)
	for rows.Next() {
		var duel openDuel
		if err := rows.Scan(&duel.id, &duel.proposalA, &duel.proposalB, &duel.activeA, &duel.activeB); err != nil {
			rows.Close()
			return nil, err
		}
		if !duel.activeA || !duel.activeB {
			duels = append(duels, duel)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	resolvedAny := false
	for _, duel := range duels {
		if !duel.activeA {
			duel.replacedA, err = randomSituationGroupMember(ctx, tx, roundID, duel.proposalA)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		}
		if !duel.activeB {
			duel.replacedB, err = randomSituationGroupMember(ctx, tx, roundID, duel.proposalB)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		}
		forfeitA := !duel.activeA && duel.replacedA == ""
		forfeitB := !duel.activeB && duel.replacedB == ""
		if forfeitA || forfeitB {
			winner := duel.proposalA
			if forfeitA && (!forfeitB || rand.Intn(2) == 1) {
				winner = duel.proposalB
			}
			if _, err := resolveSituationDuel(ctx, tx, duel.id, winner); err != nil {
				return nil, err
			}
			resolvedAny = true
			continue
		}
		if duel.replacedA != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE situation_duels SET representative_a_player_id = $2, vote_a_proposal_id = NULL WHERE id = $1
			`, duel.id, duel.replacedA); err != nil {
				return nil, err
			}
		}
		if duel.replacedB != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE situation_duels SET representative_b_player_id = $2, vote_b_proposal_id = NULL WHERE id = $1
			`, duel.id, duel.replacedB); err != nil {
				return nil, err
			}
		}
	}
	if !resolvedAny {
		return nil, nil
	}
	return tryAdvanceSituationWave(ctx, tx, roundID)
}

func (a *api) settleWavelengthGame(ctx context.Context, tx pgx.Tx, code, lobbyID string) (phaseTimers, error) {
	game, err := lockActiveWavelengthGame(ctx, tx, lobbyID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && game.Phase == "completed") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	enough, err := hasEnoughActivePlayers(ctx, tx, lobbyID, lobbyModeWavelength)
	if err != nil {
		return nil, err
	}
	if !enough {
		if _, err := tx.Exec(ctx, `
			UPDATE wavelength_rounds SET phase = 'completed', completed_at = COALESCE(completed_at, now())
			WHERE game_id = $1 AND phase <> 'completed'
		`, game.ID); err != nil {
			return nil, err
		}
		return nil, completeWavelengthGame(ctx, tx, game.ID)
	}

	var timers phaseTimers
	switch game.Phase {
	case "collecting_themes":
		rankingDeadline, err := tryAdvanceWavelengthThemeCollection(ctx, tx, game, lobbyID, false)
		if err != nil {
			return nil, err
		}
		if rankingDeadline != nil {
			timers.add(func() {
				a.scheduleWavelengthThemeRankingTimeout(code, game.ID, game.RoundNumber, *rankingDeadline)
			})
		}
	case "ranking_themes":
		candidates, err := wavelengthThemeCandidates(ctx, tx, game.ID, game.RoundNumber)
		if err != nil {
			return nil, err
		}
		roundID, started, err := tryConcludeWavelengthRanking(ctx, tx, game, lobbyID, candidates, false)
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleWavelengthClueTimeout(code, roundID) })
		}
	case "playing":
		round, err := loadCurrentWavelengthRound(ctx, tx, game.ID)
		if err != nil {
			return nil, err
		}
		if round.Phase != "clue" && round.Phase != "guessing" {
			return nil, nil
		}
		var psychicIsActive bool
		if err := tx.QueryRow(ctx, `SELECT is_active FROM players WHERE id = $1`, round.PsychicPlayerID).Scan(&psychicIsActive); err != nil {
			return nil, err
		}
		if psychicIsActive {
			if round.Phase == "guessing" {
				_, err := tryCloseWavelengthGuessing(ctx, tx, round, lobbyID, false)
				return nil, err
			}
			return nil, nil
		}
		// Without its psychic the round cannot be played: it starts over on
		// the same spectrum with a new psychic and target. The old round's
		// timers find no row any more and do nothing.
		if _, err := tx.Exec(ctx, `DELETE FROM wavelength_rounds WHERE id = $1`, round.ID); err != nil {
			return nil, err
		}
		roundID, started, err := startWavelengthRound(ctx, tx, game, lobbyID, wavelengthSpectrum{
			Left: round.LeftLabel, Right: round.RightLabel,
		})
		if err != nil {
			return nil, err
		}
		if started {
			timers.add(func() { a.scheduleWavelengthClueTimeout(code, roundID) })
		}
	}
	return timers, nil
}
