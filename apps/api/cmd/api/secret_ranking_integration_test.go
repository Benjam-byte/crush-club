package main

import (
	"net/http"
	"testing"
	"time"
)

// openIntegrationSecretRankingRound passes every theme proposal, ranks the
// default questions and returns the first round, open for bets.
func openIntegrationSecretRankingRound(t *testing.T, game integrationGame) *secretRankingStateView {
	t.Helper()
	host := game.players[0]
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/secret-ranking/themes"), player.ReconnectToken,
			map[string]string{"theme": ""}, http.StatusOK,
		)
	}
	candidates := game.state(t, host.ReconnectToken).SecretRankingGame.ThemeCandidates
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/secret-ranking/themes/rank"), player.ReconnectToken,
			map[string]any{"ranking": candidates}, http.StatusOK,
		)
	}
	round := game.state(t, host.ReconnectToken).SecretRankingGame
	if round.Phase != "playing" || round.RoundPhase != "secret" || round.RoundNumber != 1 || round.ThemeLabel != candidates[0] {
		t.Fatalf("the first round did not open for bets: %#v", round)
	}
	return round
}

func betIntegrationSecretRanking(t *testing.T, game integrationGame, player playerSessionResponse, amount int64) *secretRankingStateView {
	t.Helper()
	return integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/amount"), player.ReconnectToken,
		map[string]int64{"amount": amount}, http.StatusOK,
	).SecretRankingGame
}

func numberIntegrationSecretRanking(t *testing.T, game integrationGame, player playerSessionResponse, position int) *secretRankingStateView {
	t.Helper()
	return integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/position"), player.ReconnectToken,
		map[string]int{"position": position}, http.StatusOK,
	).SecretRankingGame
}

func waitForIntegrationSecretRankingPhase(t *testing.T, game integrationGame, roundPhase string, timeout time.Duration) lobbyStateResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		state := game.state(t, game.players[0].ReconnectToken)
		if state.SecretRankingGame != nil && state.SecretRankingGame.RoundPhase == roundPhase {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("round phase %q not reached: %#v", roundPhase, state.SecretRankingGame)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestSecretRankingPlaysARoundOnTheTable(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeSecretRanking, 3)
	ana, ben, cleo := game.players[0], game.players[1], game.players[2]
	openIntegrationSecretRankingRound(t, game)

	invalid := integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/amount"), ana.ReconnectToken,
		map[string]int64{"amount": -1}, http.StatusUnprocessableEntity,
	)
	requireErrorCode(t, invalid, "invalid_amount")

	if mine := betIntegrationSecretRanking(t, game, ana, 500); mine.MyAmount == nil || *mine.MyAmount != 500 || !mine.Submitted {
		t.Fatalf("Ana's bet was not acknowledged: %#v", mine)
	}
	if other := game.state(t, ben.ReconnectToken).SecretRankingGame; other.MyAmount != nil || other.Submitted || len(other.Reveal) > 0 {
		t.Fatalf("Ben sees a bet that is not his: %#v", other)
	}
	betIntegrationSecretRanking(t, game, ben, 50)
	table := betIntegrationSecretRanking(t, game, cleo, 5000)
	if table.RoundPhase != "placing" || table.ParticipantCount != 3 || table.MyAmount == nil || *table.MyAmount != 5000 {
		t.Fatalf("the phones did not go to the table: %#v", table)
	}

	// Two phones on #1: no lock.
	numberIntegrationSecretRanking(t, game, ana, 1)
	if duplicate := numberIntegrationSecretRanking(t, game, ben, 1); duplicate.LockAt != nil || duplicate.NumberedCount != 2 {
		t.Fatalf("a duplicate number armed the lock: %#v", duplicate)
	}
	numberIntegrationSecretRanking(t, game, cleo, 3)
	if armed := numberIntegrationSecretRanking(t, game, ben, 2); armed.LockAt == nil {
		t.Fatalf("1, 2, 3 did not arm the lock: %#v", armed)
	}
	// A tap during the countdown cancels it; numbering again re-arms it.
	if cancelled := numberIntegrationSecretRanking(t, game, cleo, 1); cancelled.LockAt != nil {
		t.Fatalf("breaking the numbers did not cancel the lock: %#v", cancelled)
	}
	invalidPosition := integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/position"), cleo.ReconnectToken,
		map[string]int{"position": 4}, http.StatusUnprocessableEntity,
	)
	requireErrorCode(t, invalidPosition, "invalid_table_position")
	if rearmed := numberIntegrationSecretRanking(t, game, cleo, 3); rearmed.LockAt == nil {
		t.Fatalf("numbering again did not re-arm the lock: %#v", rearmed)
	}

	revealing := waitForIntegrationSecretRankingPhase(t, game, "revealing", secretRankingLockDelay+5*time.Second)
	reveal := revealing.SecretRankingGame
	if len(reveal.Reveal) != 3 || reveal.RevealAt == nil || reveal.RevealEndsAt == nil || reveal.RevealStepMs != 1400 {
		t.Fatalf("the reveal is incomplete: %#v", reveal)
	}
	// Line: Ana 500 (#1), Ben 50 (#2), Cléo 5000 (#3). True order: Ben, Ana, Cléo.
	wantLine := []struct {
		playerID   string
		amount     int64
		wellPlaced bool
	}{
		{ana.State.CurrentPlayerID, 500, false},
		{ben.State.CurrentPlayerID, 50, false},
		{cleo.State.CurrentPlayerID, 5000, true},
	}
	for index, want := range wantLine {
		got := reveal.Reveal[index]
		if got.Position != index+1 || got.PlayerID != want.playerID || got.Amount != want.amount || got.WellPlaced != want.wellPlaced {
			t.Fatalf("reveal[%d] = %#v, want %#v", index, got, want)
		}
	}
	if reveal.WellPlacedCount != 1 || reveal.ParticipantCount != 3 {
		t.Fatalf("score = %d/%d, want 1/3", reveal.WellPlacedCount, reveal.ParticipantCount)
	}

	early := integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/debrief"), ben.ReconnectToken, nil, http.StatusConflict,
	)
	requireErrorCode(t, early, "reveal_in_progress")

	time.Sleep(reveal.RevealEndsAt.Sub(revealing.ServerTime) + 200*time.Millisecond)
	debrief := integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/debrief"), ben.ReconnectToken, nil, http.StatusOK,
	).SecretRankingGame
	if debrief.RoundPhase != "results" || len(debrief.Reveal) != 3 {
		t.Fatalf("the debrief did not open: %#v", debrief)
	}

	notHost := integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/rounds/next"), ben.ReconnectToken, nil, http.StatusForbidden,
	)
	requireErrorCode(t, notHost, "host_required")
	next := integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/secret-ranking/rounds/next"), ana.ReconnectToken, nil, http.StatusOK,
	).SecretRankingGame
	if next.RoundPhase != "secret" || next.RoundNumber != 2 || next.MyAmount != nil || next.Submitted {
		t.Fatalf("the second round did not open fresh: %#v", next)
	}
}

func TestSecretRankingDepartureRenumbersTheTable(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeSecretRanking, 4)
	host, leaver := game.players[0], game.players[3]
	openIntegrationSecretRankingRound(t, game)
	for index, player := range game.players {
		betIntegrationSecretRanking(t, game, player, int64(100*(index+1)))
	}
	numberIntegrationSecretRanking(t, game, host, 1)
	numberIntegrationSecretRanking(t, game, game.players[1], 2)

	leaveIntegrationLobby(t, game, leaver.ReconnectToken)
	table := game.state(t, host.ReconnectToken).SecretRankingGame
	if table.RoundPhase != "placing" || table.ParticipantCount != 3 || table.NumberedCount != 0 ||
		table.PositionsResetAt == nil || table.MyPosition != nil {
		t.Fatalf("the table was not renumbered after a departure: %#v", table)
	}
	for _, seat := range table.Seats {
		if seat.PlayerID == leaver.State.CurrentPlayerID {
			t.Fatalf("the departed player's phone is still on the table: %#v", table.Seats)
		}
	}
}
