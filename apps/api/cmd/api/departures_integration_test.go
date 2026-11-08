package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// These tests run against a live stack, like the other integration tests:
// they are skipped unless API_INTEGRATION_URL is set.

var integrationModeSegments = map[string]string{
	lobbyModeFastBio:       "fast-bio",
	lobbyModeZeroToHundred: "zero-to-100",
	lobbyModeSituation:     "situation",
	lobbyModeWavelength:    "wavelength",
	lobbyModeSecretRanking: "secret-ranking",
}

type integrationGame struct {
	baseURL  string
	code     string
	segment  string
	identity integrationSession
	players  []playerSessionResponse
}

func (game integrationGame) url(path string) string {
	return game.baseURL + "/api/v1/lobbies/" + game.code + path
}

func (game integrationGame) state(t *testing.T, token string) lobbyStateResponse {
	t.Helper()
	return integrationRequestJSON[lobbyStateResponse](t, game.identity, http.MethodGet, game.url("/state"), token, nil, http.StatusOK)
}

func (game integrationGame) themeBody(mode, label string) map[string]string {
	if mode == lobbyModeWavelength {
		return map[string]string{"left": label + " froid", "right": label + " chaud"}
	}
	return map[string]string{"theme": label}
}

// startIntegrationUncappedGame creates a lobby of playerCount connected
// players and starts its game. Sockets stay open until the test ends.
func startIntegrationUncappedGame(t *testing.T, mode string, playerCount int) (integrationGame, []*websocket.Conn) {
	t.Helper()
	baseURL := integrationBaseURL(t)
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	identity, host := createIntegrationModeLobby(t, baseURL, "Host-"+suffix, mode, 0)
	game := integrationGame{
		baseURL: baseURL, code: host.State.Code, segment: integrationModeSegments[mode],
		identity: identity, players: []playerSessionResponse{host},
	}
	for index := 1; index < playerCount; index++ {
		game.players = append(game.players, joinIntegrationLobby(t, baseURL, game.code, fmt.Sprintf("Player%d-%s", index+1, suffix)))
	}
	sockets := make([]*websocket.Conn, 0, playerCount)
	for _, player := range game.players {
		socket := openIntegrationWebSocket(t, baseURL, game.code, player.ReconnectToken)
		sockets = append(sockets, socket)
		t.Cleanup(func() { _ = socket.CloseNow() })
	}
	for _, player := range game.players {
		waitForPlayerConnection(t, baseURL, game.code, host.ReconnectToken, player.State.CurrentPlayerID, true)
	}
	integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPost, game.url("/"+game.segment+"/start"), host.ReconnectToken, nil, http.StatusOK)
	return game, sockets
}

func leaveIntegrationLobby(t *testing.T, game integrationGame, token string) {
	t.Helper()
	integrationRequestJSON[struct{}](t, game.identity, http.MethodPost, game.url("/players/me/leave"), token, nil, http.StatusNoContent)
}

// waitForCloseStatus reads until the server closes the socket and returns
// its close code.
func waitForCloseStatus(t *testing.T, connection *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if _, _, err := connection.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

// dialRefusedWebSocket opens a lobby socket with token and returns the close
// code the server answers with.
func dialRefusedWebSocket(t *testing.T, baseURL, code, token string) websocket.StatusCode {
	t.Helper()
	websocketURL := strings.Replace(strings.Replace(baseURL, "https://", "wss://", 1), "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, websocketURL+"/ws/lobbies/"+code, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, websocketAuthentication{Type: "authenticate", ReconnectToken: token}); err != nil {
		t.Fatal(err)
	}
	return waitForCloseStatus(t, connection)
}

func requireErrorCode(t *testing.T, body map[string]map[string]string, want string) {
	t.Helper()
	if body["error"]["code"] != want {
		t.Fatalf("error = %#v, want code %s", body, want)
	}
}

func TestHostExcludesOnlineAndOfflinePlayers(t *testing.T) {
	baseURL := integrationBaseURL(t)
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	identity, host := createIntegrationLobby(t, baseURL, "ExcludeHost-"+suffix, 3)
	code := host.State.Code
	online := joinIntegrationLobby(t, baseURL, code, "OnlineGuest-"+suffix)
	offline := joinIntegrationLobby(t, baseURL, code, "OfflineGuest-"+suffix)
	excludeURL := func(playerID string) string {
		return baseURL + "/api/v1/lobbies/" + code + "/players/" + playerID + "/exclude"
	}

	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, identity, http.MethodPost, excludeURL(host.State.CurrentPlayerID), online.ReconnectToken, nil, http.StatusForbidden,
	), "host_required")
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, identity, http.MethodPost, excludeURL(host.State.CurrentPlayerID), host.ReconnectToken, nil, http.StatusUnprocessableEntity,
	), "cannot_exclude_self")

	// A player who dropped is still offered as a quick exclusion.
	offlineSocket := openIntegrationWebSocket(t, baseURL, code, offline.ReconnectToken)
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, offline.State.CurrentPlayerID, true)
	offlineSocket.CloseNow()
	// The hub drops the socket just before the disconnection is stored, so
	// the quick exclusion can show up a moment after "offline".
	offeredForExclusion := false
	for deadline := time.Now().Add(5 * time.Second); !offeredForExclusion && time.Now().Before(deadline); {
		disconnected := waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, offline.State.CurrentPlayerID, false)
		for _, player := range disconnected.Players {
			offeredForExclusion = offeredForExclusion || (player.ID == offline.State.CurrentPlayerID && player.CanExclude)
		}
		if !offeredForExclusion {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !offeredForExclusion {
		t.Fatal("a freshly disconnected player was not offered for exclusion")
	}
	integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPost, excludeURL(offline.State.CurrentPlayerID), host.ReconnectToken, nil, http.StatusOK)

	// A connected player can be removed too: their socket closes with the
	// kicked code and their session is refused with the reason from then on.
	onlineSocket := openIntegrationWebSocket(t, baseURL, code, online.ReconnectToken)
	t.Cleanup(func() { _ = onlineSocket.CloseNow() })
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, online.State.CurrentPlayerID, true)
	state := integrationRequestJSON[lobbyStateResponse](
		t, identity, http.MethodPost, excludeURL(online.State.CurrentPlayerID), host.ReconnectToken, nil, http.StatusOK,
	)
	if len(state.Players) != 1 || state.Players[0].ID != host.State.CurrentPlayerID {
		t.Fatalf("excluded players remain in the lobby: %#v", state.Players)
	}
	if status := waitForCloseStatus(t, onlineSocket); status != closePlayerKicked {
		t.Fatalf("kicked socket closed with %v, want %v", status, closePlayerKicked)
	}
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, identity, http.MethodGet, baseURL+"/api/v1/lobbies/"+code+"/state", online.ReconnectToken, nil, http.StatusUnauthorized,
	), "player_excluded")
	if status := dialRefusedWebSocket(t, baseURL, code, online.ReconnectToken); status != closePlayerKicked {
		t.Fatalf("reconnecting kicked player closed with %v, want %v", status, closePlayerKicked)
	}
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, identity, http.MethodPost, excludeURL(online.State.CurrentPlayerID), host.ReconnectToken, nil, http.StatusNotFound,
	), "player_not_found")
}

func TestLateJoinerNeedsHostApproval(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeWavelength, 2)
	host, guest := game.players[0], game.players[1]
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)

	late := joinIntegrationLobby(t, game.baseURL, game.code, "Late-"+suffix)
	if late.State.CurrentPlayerAdmission != "pending" || late.State.WavelengthGame != nil {
		t.Fatalf("a late joiner was let into the game: admission=%q game=%v", late.State.CurrentPlayerAdmission, late.State.WavelengthGame)
	}
	hostState := game.state(t, host.ReconnectToken)
	if len(hostState.PendingPlayers) != 1 || hostState.PendingPlayers[0].ID != late.State.CurrentPlayerID || len(hostState.Players) != 2 {
		t.Fatalf("pending request not shown apart: players=%d pending=%#v", len(hostState.Players), hostState.PendingPlayers)
	}
	if theme := integrationThemeSelection(t, lobbyModeWavelength, hostState); theme.requiredCount != 2 {
		t.Fatalf("a pending player is waited for: %#v", theme)
	}
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, game.url("/wavelength/themes"), late.ReconnectToken,
		game.themeBody(lobbyModeWavelength, "Tôt"), http.StatusForbidden,
	), "admission_pending")

	admitURL := game.url("/players/" + late.State.CurrentPlayerID + "/admit")
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, admitURL, guest.ReconnectToken, nil, http.StatusForbidden,
	), "host_required")
	hostState = integrationRequestJSON[lobbyStateResponse](t, game.identity, http.MethodPost, admitURL, host.ReconnectToken, nil, http.StatusOK)
	if len(hostState.Players) != 3 || len(hostState.PendingPlayers) != 0 {
		t.Fatalf("admitted player not in the game: players=%d pending=%d", len(hostState.Players), len(hostState.PendingPlayers))
	}
	if theme := integrationThemeSelection(t, lobbyModeWavelength, hostState); theme.requiredCount != 3 {
		t.Fatalf("the admitted player is not expected yet: %#v", theme)
	}
	lateState := game.state(t, late.ReconnectToken)
	if lateState.CurrentPlayerAdmission != "admitted" || lateState.WavelengthGame == nil {
		t.Fatalf("admitted player still kept out: %#v", lateState.CurrentPlayerAdmission)
	}
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodPost, admitURL, host.ReconnectToken, nil, http.StatusConflict,
	), "player_not_pending")

	refused := joinIntegrationLobby(t, game.baseURL, game.code, "Refused-"+suffix)
	refusedSocket := openIntegrationWebSocket(t, game.baseURL, game.code, refused.ReconnectToken)
	t.Cleanup(func() { _ = refusedSocket.CloseNow() })
	integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/players/"+refused.State.CurrentPlayerID+"/refuse"), host.ReconnectToken, nil, http.StatusOK,
	)
	if status := waitForCloseStatus(t, refusedSocket); status != closeAdmissionRefused {
		t.Fatalf("refused socket closed with %v, want %v", status, closeAdmissionRefused)
	}
	requireErrorCode(t, integrationRequestJSON[map[string]map[string]string](
		t, game.identity, http.MethodGet, game.url("/state"), refused.ReconnectToken, nil, http.StatusUnauthorized,
	), "admission_refused")
}

func TestDepartureAdvancesUncappedPhase(t *testing.T) {
	for _, testCase := range []struct {
		mode        string
		playerCount int
	}{
		{mode: lobbyModeFastBio, playerCount: 3},
		{mode: lobbyModeZeroToHundred, playerCount: 4},
		{mode: lobbyModeSituation, playerCount: 3},
		{mode: lobbyModeWavelength, playerCount: 3},
		{mode: lobbyModeSecretRanking, playerCount: 4},
	} {
		t.Run(testCase.mode, func(t *testing.T) {
			game, _ := startIntegrationUncappedGame(t, testCase.mode, testCase.playerCount)
			host := game.players[0]
			straggler := game.players[len(game.players)-1]
			for index, player := range game.players[:len(game.players)-1] {
				integrationRequestJSON[lobbyStateResponse](
					t, game.identity, http.MethodPost, game.url("/"+game.segment+"/themes"), player.ReconnectToken,
					game.themeBody(testCase.mode, fmt.Sprintf("Thème %d", index+1)), http.StatusOK,
				)
			}
			state := game.state(t, host.ReconnectToken)
			if !slices.Equal(state.AwaitingPlayerIDs, []string{straggler.State.CurrentPlayerID}) {
				t.Fatalf("awaiting = %v, want only the straggler %s", state.AwaitingPlayerIDs, straggler.State.CurrentPlayerID)
			}

			leaveIntegrationLobby(t, game, straggler.ReconnectToken)
			state = game.state(t, host.ReconnectToken)
			theme := integrationThemeSelection(t, testCase.mode, state)
			if theme.phase != "ranking_themes" || theme.requiredCount != testCase.playerCount-1 {
				t.Fatalf("the game still waits for the player who left: %#v", theme)
			}
			if len(state.AwaitingPlayerIDs) != testCase.playerCount-1 {
				t.Fatalf("awaiting after the ranking opened = %v", state.AwaitingPlayerIDs)
			}
		})
	}
}

func TestDepartureBelowMinimumEndsGameAndReplayIsShorter(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeWavelength, 2)
	host, guest := game.players[0], game.players[1]
	first := game.state(t, host.ReconnectToken)
	firstWindow := first.WavelengthGame.ThemeDeadline.Sub(first.ServerTime)
	if firstWindow < 100*time.Second || firstWindow > 125*time.Second {
		t.Fatalf("first theme window = %v, want about 2 minutes", firstWindow)
	}

	leaveIntegrationLobby(t, game, guest.ReconnectToken)
	state := game.state(t, host.ReconnectToken)
	if state.WavelengthGame == nil || state.WavelengthGame.Phase != "completed" {
		t.Fatalf("a game below its minimum did not end: %#v", state.WavelengthGame)
	}

	replayed := integrationRequestJSON[lobbyStateResponse](
		t, game.identity, http.MethodPost, game.url("/wavelength/replay"), host.ReconnectToken, nil, http.StatusOK,
	)
	replayWindow := replayed.WavelengthGame.ThemeDeadline.Sub(replayed.ServerTime)
	if replayWindow < 30*time.Second || replayWindow > 45*time.Second {
		t.Fatalf("replay theme window = %v, want about 40 seconds", replayWindow)
	}
}

func TestWavelengthPsychicDepartureRestartsRound(t *testing.T) {
	game, _ := startIntegrationUncappedGame(t, lobbyModeWavelength, 3)
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/wavelength/themes"), player.ReconnectToken,
			map[string]string{"left": "", "right": ""}, http.StatusOK,
		)
	}
	candidates := game.state(t, game.players[0].ReconnectToken).WavelengthGame.ThemeCandidates
	for _, player := range game.players {
		integrationRequestJSON[lobbyStateResponse](
			t, game.identity, http.MethodPost, game.url("/wavelength/themes/rank"), player.ReconnectToken,
			map[string]any{"ranking": candidates}, http.StatusOK,
		)
	}
	round := game.state(t, game.players[0].ReconnectToken).WavelengthGame
	if round.Phase != "playing" || round.RoundPhase != "clue" {
		t.Fatalf("round did not start: %#v", round)
	}
	psychicID := round.PsychicPlayerID
	var psychic playerSessionResponse
	var observer playerSessionResponse
	for _, player := range game.players {
		if player.State.CurrentPlayerID == psychicID {
			psychic = player
		} else {
			observer = player
		}
	}

	leaveIntegrationLobby(t, game, psychic.ReconnectToken)
	restarted := game.state(t, observer.ReconnectToken).WavelengthGame
	if restarted.Phase != "playing" || restarted.RoundPhase != "clue" || restarted.RoundNumber != round.RoundNumber {
		t.Fatalf("round did not restart in place: %#v", restarted)
	}
	if restarted.PsychicPlayerID == psychicID || restarted.PsychicPlayerID == "" {
		t.Fatalf("the departed psychic still leads the round: %s", restarted.PsychicPlayerID)
	}
	if restarted.LeftLabel != round.LeftLabel || restarted.RightLabel != round.RightLabel {
		t.Fatalf("the spectrum changed: %s/%s, want %s/%s", restarted.LeftLabel, restarted.RightLabel, round.LeftLabel, round.RightLabel)
	}
}
