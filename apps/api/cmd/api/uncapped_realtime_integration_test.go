package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type integrationThemeView struct {
	phase         string
	submitted     bool
	ranked        bool
	deadline      *time.Time
	progressCount int
	requiredCount int
	candidates    []string
}

func integrationThemeSelection(t *testing.T, mode string, state lobbyStateResponse) integrationThemeView {
	t.Helper()
	switch mode {
	case lobbyModeFastBio:
		if state.FastBioGame == nil {
			t.Fatal("missing Fast Bio state")
		}
		game := state.FastBioGame
		return integrationThemeView{
			phase: game.Phase, submitted: game.ThemeSubmitted, ranked: game.ThemeRanked,
			deadline: game.ThemeDeadline, progressCount: game.ThemeProgressCount,
			requiredCount: game.ThemeProgressRequired, candidates: game.ThemeCandidates,
		}
	case lobbyModeZeroToHundred:
		if state.ZeroToHundredGame == nil {
			t.Fatal("missing 0 to 100 state")
		}
		game := state.ZeroToHundredGame
		return integrationThemeView{
			phase: game.Phase, submitted: game.ThemeSubmitted, ranked: game.ThemeRanked,
			deadline: game.ThemeDeadline, progressCount: game.ThemeProgressCount,
			requiredCount: game.ThemeProgressRequired, candidates: game.ThemeCandidates,
		}
	case lobbyModeSituation:
		if state.SituationGame == nil {
			t.Fatal("missing Situation state")
		}
		game := state.SituationGame
		return integrationThemeView{
			phase: game.Phase, submitted: game.ThemeSubmitted, ranked: game.ThemeRanked,
			deadline: game.ThemeDeadline, progressCount: game.ThemeProgressCount,
			requiredCount: game.ThemeProgressRequired, candidates: game.ThemeCandidates,
		}
	default:
		t.Fatalf("unsupported uncapped mode %q", mode)
		return integrationThemeView{}
	}
}

func requireAdvancedRevision(t *testing.T, previous, next int64) {
	t.Helper()
	if next <= previous {
		t.Fatalf("revision did not advance: previous=%d next=%d", previous, next)
	}
}

// TestUncappedModeRealtimeRevisionFlow exercises the player-visible states
// consumed by the Angular confirmation screens. It is skipped with the other
// integration tests unless API_INTEGRATION_URL points at a running stack.
func TestUncappedModeRealtimeRevisionFlow(t *testing.T) {
	baseURL := integrationBaseURL(t)
	testCases := []struct {
		mode        string
		playerCount int
		apiSegment  string
	}{
		{mode: lobbyModeFastBio, playerCount: 2, apiSegment: "fast-bio"},
		{mode: lobbyModeZeroToHundred, playerCount: 3, apiSegment: "zero-to-100"},
		{mode: lobbyModeSituation, playerCount: 2, apiSegment: "situation"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.mode, func(t *testing.T) {
			suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
			hostIdentity, host := createIntegrationModeLobby(
				t, baseURL, "Host-"+suffix, testCase.mode, 0,
			)
			players := []playerSessionResponse{host}
			for index := 1; index < testCase.playerCount; index++ {
				players = append(players, joinIntegrationLobby(
					t, baseURL, host.State.Code, fmt.Sprintf("Player-%d-%s", index+1, suffix),
				))
			}

			sockets := make([]*websocket.Conn, 0, len(players))
			for _, player := range players {
				socket := openIntegrationWebSocket(t, baseURL, host.State.Code, player.ReconnectToken)
				sockets = append(sockets, socket)
				t.Cleanup(func() { _ = socket.CloseNow() })
			}
			for _, player := range players {
				waitForPlayerConnection(
					t, baseURL, host.State.Code, host.ReconnectToken,
					player.State.CurrentPlayerID, true,
				)
			}

			state := integrationRequestJSON[lobbyStateResponse](
				t, hostIdentity, http.MethodGet,
				baseURL+"/api/v1/lobbies/"+host.State.Code+"/state",
				host.ReconnectToken, nil, http.StatusOK,
			)
			previousRevision := state.Revision
			state = integrationRequestJSON[lobbyStateResponse](
				t, hostIdentity, http.MethodPost,
				baseURL+"/api/v1/lobbies/"+host.State.Code+"/"+testCase.apiSegment+"/start",
				host.ReconnectToken, nil, http.StatusOK,
			)
			requireAdvancedRevision(t, previousRevision, state.Revision)
			initialTheme := integrationThemeSelection(t, testCase.mode, state)
			if initialTheme.phase != "collecting_themes" || initialTheme.deadline == nil ||
				initialTheme.progressCount != 0 || initialTheme.requiredCount != len(players) {
				t.Fatalf("unexpected initial theme state: %#v", initialTheme)
			}

			for index, player := range players {
				previousRevision = state.Revision
				state = integrationRequestJSON[lobbyStateResponse](
					t, hostIdentity, http.MethodPost,
					baseURL+"/api/v1/lobbies/"+host.State.Code+"/"+testCase.apiSegment+"/themes",
					player.ReconnectToken,
					map[string]string{"theme": fmt.Sprintf("Theme %d %s", index+1, suffix)},
					http.StatusOK,
				)
				requireAdvancedRevision(t, previousRevision, state.Revision)
				view := integrationThemeSelection(t, testCase.mode, state)
				if index < len(players)-1 {
					if view.phase != "collecting_themes" || !view.submitted ||
						view.progressCount != index+1 || view.requiredCount != len(players) {
						t.Fatalf("theme submission was not reflected immediately: %#v", view)
					}
				} else if view.phase != "ranking_themes" || view.deadline == nil ||
					!view.deadline.After(*initialTheme.deadline) || len(view.candidates) < 3 {
					t.Fatalf("theme collection did not open a fresh ranking phase: %#v", view)
				}
				observer := sockets[(index+1)%len(sockets)]
				snapshot := waitForSnapshotRevision(t, observer, state.Revision)
				snapshotView := integrationThemeSelection(t, testCase.mode, snapshot)
				if snapshotView.phase != view.phase || snapshotView.progressCount != view.progressCount {
					t.Fatalf("websocket theme progress mismatch: response=%#v snapshot=%#v", view, snapshotView)
				}
			}

			rankingState := integrationThemeSelection(t, testCase.mode, state)
			rankingDeadline := rankingState.deadline
			candidates := append([]string(nil), rankingState.candidates...)
			for index, player := range players {
				previousRevision = state.Revision
				state = integrationRequestJSON[lobbyStateResponse](
					t, hostIdentity, http.MethodPost,
					baseURL+"/api/v1/lobbies/"+host.State.Code+"/"+testCase.apiSegment+"/themes/rank",
					player.ReconnectToken,
					map[string]any{"ranking": candidates},
					http.StatusOK,
				)
				requireAdvancedRevision(t, previousRevision, state.Revision)
				view := integrationThemeSelection(t, testCase.mode, state)
				if index < len(players)-1 {
					if view.phase != "ranking_themes" || !view.ranked ||
						view.progressCount != index+1 || view.requiredCount != len(players) {
						t.Fatalf("theme ranking was not reflected immediately: %#v", view)
					}
				} else if view.phase != "playing" {
					t.Fatalf("final theme ranking did not start the first round: %#v", view)
				}
				observer := sockets[(index+1)%len(sockets)]
				snapshot := waitForSnapshotRevision(t, observer, state.Revision)
				if snapshot.Revision < state.Revision {
					t.Fatalf("websocket revision=%d, want at least %d", snapshot.Revision, state.Revision)
				}
			}

			previousRevision = state.Revision
			submitFirstRoundAction(
				t, testCase.mode, baseURL, hostIdentity, players, rankingDeadline, &state,
			)
			requireAdvancedRevision(t, previousRevision, state.Revision)
			snapshot := waitForSnapshotRevision(t, sockets[1], state.Revision)
			requireRoundProgressBroadcast(t, testCase.mode, snapshot)
			if testCase.mode == lobbyModeFastBio {
				requireFastBioReactionRevision(t, baseURL, hostIdentity, players, sockets, &state)
			}
		})
	}
}

func submitFirstRoundAction(
	t *testing.T,
	mode, baseURL string,
	hostIdentity integrationSession,
	players []playerSessionResponse,
	rankingDeadline *time.Time,
	state *lobbyStateResponse,
) {
	t.Helper()
	code := players[0].State.Code
	switch mode {
	case lobbyModeFastBio:
		if state.FastBioGame == nil || state.FastBioGame.SubmissionDeadline == nil ||
			rankingDeadline == nil || !state.FastBioGame.SubmissionDeadline.After(*rankingDeadline) {
			t.Fatalf("Fast Bio round did not receive a fresh deadline: %#v", state.FastBioGame)
		}
		*state = submitIntegrationFastBioProposal(
			t, baseURL, code, players[0].ReconnectToken, hostIdentity.client,
		)
		if state.FastBioGame == nil || !state.FastBioGame.Submitted ||
			state.FastBioGame.SubmissionProgressCount != 1 {
			t.Fatalf("Fast Bio proposal was not acknowledged: %#v", state.FastBioGame)
		}
	case lobbyModeZeroToHundred:
		if state.ZeroToHundredGame == nil || state.ZeroToHundredGame.SubmissionDeadline == nil ||
			rankingDeadline == nil || !state.ZeroToHundredGame.SubmissionDeadline.After(*rankingDeadline) {
			t.Fatalf("0 to 100 round did not receive a fresh deadline: %#v", state.ZeroToHundredGame)
		}
		positions := make(map[string]int, len(state.ZeroToHundredGame.Nominees))
		for index, nominee := range state.ZeroToHundredGame.Nominees {
			positions[nominee.PlayerID] = 20 + index*20
		}
		*state = integrationRequestJSON[lobbyStateResponse](
			t, hostIdentity, http.MethodPost,
			baseURL+"/api/v1/lobbies/"+code+"/zero-to-100/guesses",
			players[0].ReconnectToken, map[string]any{"positions": positions}, http.StatusOK,
		)
		if state.ZeroToHundredGame == nil || !state.ZeroToHundredGame.Submitted ||
			state.ZeroToHundredGame.SubmissionProgressCount != 1 {
			t.Fatalf("0 to 100 positions were not acknowledged: %#v", state.ZeroToHundredGame)
		}
	case lobbyModeSituation:
		if state.SituationGame == nil || state.SituationGame.ProposalDeadline == nil ||
			rankingDeadline == nil || !state.SituationGame.ProposalDeadline.After(*rankingDeadline) {
			t.Fatalf("Situation round did not receive a fresh deadline: %#v", state.SituationGame)
		}
		*state = integrationRequestJSON[lobbyStateResponse](
			t, hostIdentity, http.MethodPost,
			baseURL+"/api/v1/lobbies/"+code+"/situation/proposal",
			players[0].ReconnectToken,
			map[string]string{
				"chosenPlayerId": players[1].State.CurrentPlayerID,
				"reason":         "Parce que ce joueur garde son calme",
			},
			http.StatusCreated,
		)
		if state.SituationGame == nil || !state.SituationGame.Submitted ||
			state.SituationGame.SubmissionProgressCount != 1 {
			t.Fatalf("Situation proposal was not acknowledged: %#v", state.SituationGame)
		}
	}
}

func requireRoundProgressBroadcast(t *testing.T, mode string, snapshot lobbyStateResponse) {
	t.Helper()
	switch mode {
	case lobbyModeFastBio:
		if snapshot.FastBioGame == nil || snapshot.FastBioGame.SubmissionProgressCount != 1 {
			t.Fatalf("Fast Bio proposal progress was not broadcast: %#v", snapshot.FastBioGame)
		}
	case lobbyModeZeroToHundred:
		if snapshot.ZeroToHundredGame == nil || snapshot.ZeroToHundredGame.SubmissionProgressCount != 1 {
			t.Fatalf("0 to 100 progress was not broadcast: %#v", snapshot.ZeroToHundredGame)
		}
	case lobbyModeSituation:
		if snapshot.SituationGame == nil || snapshot.SituationGame.SubmissionProgressCount != 1 {
			t.Fatalf("Situation proposal progress was not broadcast: %#v", snapshot.SituationGame)
		}
	}
}

func requireFastBioReactionRevision(
	t *testing.T,
	baseURL string,
	hostIdentity integrationSession,
	players []playerSessionResponse,
	sockets []*websocket.Conn,
	state *lobbyStateResponse,
) {
	t.Helper()
	previousRevision := state.Revision
	*state = submitIntegrationFastBioProposal(
		t, baseURL, players[0].State.Code, players[1].ReconnectToken, hostIdentity.client,
	)
	requireAdvancedRevision(t, previousRevision, state.Revision)
	if state.FastBioGame == nil || state.FastBioGame.RoundPhase != "reviewing" ||
		state.FastBioGame.CurrentProposal == nil {
		t.Fatalf("Fast Bio did not enter review after every proposal: %#v", state.FastBioGame)
	}
	if state.FastBioGame.ReactionProgressCount != 0 ||
		state.FastBioGame.ReactionProgressRequired != len(players)-1 {
		t.Fatalf("unexpected initial Fast Bio reaction progress: %#v", state.FastBioGame)
	}
	waitForSnapshotRevision(t, sockets[0], state.Revision)

	proposal := state.FastBioGame.CurrentProposal
	blocked := integrationRequestJSON[struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+players[0].State.Code+"/fast-bio/review/advance",
		players[0].ReconnectToken,
		map[string]string{"direction": "next"},
		http.StatusConflict,
	)
	if blocked.Error.Code != "reactions_pending" {
		t.Fatalf("advance before every reaction error = %q, want reactions_pending", blocked.Error.Code)
	}
	unchanged := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodGet,
		baseURL+"/api/v1/lobbies/"+players[0].State.Code+"/state",
		players[0].ReconnectToken, nil, http.StatusOK,
	)
	if unchanged.Revision != state.Revision || unchanged.FastBioGame == nil ||
		unchanged.FastBioGame.ReviewIndex != state.FastBioGame.ReviewIndex {
		t.Fatalf("blocked Fast Bio advance changed state: before=%#v after=%#v", state.FastBioGame, unchanged.FastBioGame)
	}

	reactorIndex := 0
	if proposal.AuthorPlayerID == players[0].State.CurrentPlayerID {
		reactorIndex = 1
	}
	observerIndex := (reactorIndex + 1) % len(sockets)
	previousRevision = state.Revision
	integrationRequestJSON[struct{}](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+players[0].State.Code+"/fast-bio/proposals/"+proposal.ID+"/react",
		players[reactorIndex].ReconnectToken,
		map[string]string{"emoji": fastBioReactionHeart},
		http.StatusNoContent,
	)
	snapshot := waitForSnapshotRevision(t, sockets[observerIndex], previousRevision+1)
	requireAdvancedRevision(t, previousRevision, snapshot.Revision)
	if snapshot.FastBioGame == nil || snapshot.FastBioGame.CurrentProposal == nil {
		t.Fatalf("Fast Bio reaction snapshot has no current proposal: %#v", snapshot.FastBioGame)
	}
	if snapshot.FastBioGame.ReactionProgressCount != len(players)-1 ||
		snapshot.FastBioGame.ReactionProgressRequired != len(players)-1 {
		t.Fatalf("Fast Bio reaction progress was not broadcast: %#v", snapshot.FastBioGame)
	}
	foundReaction := false
	for _, reaction := range snapshot.FastBioGame.CurrentProposal.Reactions {
		if reaction.Emoji == fastBioReactionHeart && reaction.Count == 1 {
			foundReaction = true
			break
		}
	}
	if !foundReaction {
		t.Fatalf("Fast Bio reaction count was not broadcast: %#v", snapshot.FastBioGame.CurrentProposal.Reactions)
	}

	previousRevision = snapshot.Revision
	advanced := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+players[0].State.Code+"/fast-bio/review/advance",
		players[0].ReconnectToken,
		map[string]string{"direction": "next"},
		http.StatusOK,
	)
	requireAdvancedRevision(t, previousRevision, advanced.Revision)
	if advanced.FastBioGame == nil || advanced.FastBioGame.RoundPhase != "reviewing" ||
		advanced.FastBioGame.ReviewIndex != 1 {
		t.Fatalf("Fast Bio did not advance after every reaction: %#v", advanced.FastBioGame)
	}
	*state = advanced
}
