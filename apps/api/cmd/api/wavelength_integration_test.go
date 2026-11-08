package main

import (
	"net/http"
	"testing"
	"time"
)

// playIntegrationWavelengthRound has the psychic give the clue and everyone
// else place a needle, and returns the round's results.
func playIntegrationWavelengthRound(t *testing.T, game integrationGame) *wavelengthStateView {
	t.Helper()
	host := game.players[0]
	psychicID := game.state(t, host.ReconnectToken).WavelengthGame.PsychicPlayerID
	for _, player := range game.players {
		if player.State.CurrentPlayerID == psychicID {
			integrationRequestJSON[lobbyStateResponse](t, game.identity, http.MethodPost, game.url("/wavelength/clue"), player.ReconnectToken, nil, http.StatusOK)
		}
	}
	for _, player := range game.players {
		if player.State.CurrentPlayerID != psychicID {
			integrationRequestJSON[lobbyStateResponse](
				t, game.identity, http.MethodPost, game.url("/wavelength/guess"), player.ReconnectToken,
				map[string]int{"position": 50}, http.StatusOK,
			)
		}
	}
	return game.state(t, host.ReconnectToken).WavelengthGame
}

func TestWavelengthPlaysVotedSpectraBeforeVotingAgain(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeWavelength, 4)
	host := game.players[0]
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/wavelength/themes"), player.ReconnectToken,
			map[string]string{"left": "", "right": ""}, http.StatusOK,
		)
	}
	vote := game.state(t, host.ReconnectToken).WavelengthGame
	if vote.Phase != "ranking_themes" || vote.SpectraToPlay != 3 {
		t.Fatalf("the first vote should pick three spectra: phase=%s spectraToPlay=%d", vote.Phase, vote.SpectraToPlay)
	}
	candidates := vote.ThemeCandidates
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/wavelength/themes/rank"), player.ReconnectToken,
			map[string]any{"ranking": candidates}, http.StatusOK,
		)
	}

	for index := range 3 {
		round := game.state(t, host.ReconnectToken).WavelengthGame
		if round.Phase != "playing" || round.RoundPhase != "clue" || round.RoundNumber != index+1 {
			t.Fatalf("round %d did not open on its clue: %#v", index+1, round)
		}
		if label := round.LeftLabel + " ↔ " + round.RightLabel; label != candidates[index] {
			t.Fatalf("round %d plays %q, want the n°%d of the vote %q", index+1, label, index+1, candidates[index])
		}
		results := playIntegrationWavelengthRound(t, game)
		wantNext := ""
		if index < 2 {
			wantNext = candidates[index+1]
		}
		if results.RoundPhase != "results" || results.NextSpectrumLabel != wantNext {
			t.Fatalf("round %d results: phase=%s next=%q, want next=%q", index+1, results.RoundPhase, results.NextSpectrumLabel, wantNext)
		}
		integrationRequestJSON[lobbyStateResponse](t, game.identity, http.MethodPost, game.url("/wavelength/rounds/next"), host.ReconnectToken, nil, http.StatusOK)
	}

	state := game.state(t, host.ReconnectToken)
	nextVote := state.WavelengthGame
	if nextVote.Phase != "collecting_themes" || nextVote.RoundNumber != 4 || nextVote.SpectraToPlay != 1 {
		t.Fatalf("after the three voted spectra the group should vote for the last round: %#v", nextVote)
	}
	if window := nextVote.ThemeDeadline.Sub(state.ServerTime); window < 30*time.Second || window > 45*time.Second {
		t.Fatalf("second vote window = %v, want about 40 seconds", window)
	}
}
