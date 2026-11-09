package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestMultiplayerClassicReplayKeepsGroupAndResetsGame(t *testing.T) {
	baseURL := integrationBaseURL(t)
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	identity, host := createIntegrationLobby(t, baseURL, "ReplayHost-"+suffix, 2)
	code := host.State.Code
	lobbyURL := baseURL + "/api/v1/lobbies/" + code
	guest := joinIntegrationLobby(t, baseURL, code, "ReplayGuest-"+suffix)
	uploadIntegrationPhotos(t, baseURL, code, host.ReconnectToken, identity.client)
	uploadIntegrationPhotos(t, baseURL, code, guest.ReconnectToken, identity.client)
	hostSocket := openIntegrationWebSocket(t, baseURL, code, host.ReconnectToken)
	defer hostSocket.CloseNow()
	guestSocket := openIntegrationWebSocket(t, baseURL, code, guest.ReconnectToken)
	defer guestSocket.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, guest.State.CurrentPlayerID, true)
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, host.State.CurrentPlayerID, true)

	integrationRequestJSON[map[string]any](t, identity, http.MethodPost, lobbyURL+"/replay", guest.ReconnectToken, nil, http.StatusForbidden)
	integrationRequestJSON[map[string]any](t, identity, http.MethodPost, lobbyURL+"/replay", host.ReconnectToken, nil, http.StatusConflict)
	state := integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPost, lobbyURL+"/start", host.ReconnectToken, nil, http.StatusOK)
	oldGameID := state.Game.ID
	for round := 1; round <= 2; round++ {
		photoID := integrationSubjectPhotoID(t, state)
		subjectToken, cupidToken := host.ReconnectToken, guest.ReconnectToken
		if round == 2 {
			subjectToken, cupidToken = guest.ReconnectToken, host.ReconnectToken
		}
		integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPut, lobbyURL+"/rounds/current/submission", subjectToken, validIntegrationSubmission(state.Questionnaire, photoID, false), http.StatusOK)
		state = integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPut, lobbyURL+"/rounds/current/submission", cupidToken, validIntegrationSubmission(state.Questionnaire, photoID, true), http.StatusOK)
		integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPut, lobbyURL+"/rounds/current/vote", subjectToken, roundVoteInput{SubmissionID: state.Game.Submissions[0].ID}, http.StatusOK)
		state = integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPost, lobbyURL+"/rounds/current/next", host.ReconnectToken, nil, http.StatusOK)
		if round == 1 {
			state = integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPost, lobbyURL+"/rounds/next/start", host.ReconnectToken, nil, http.StatusOK)
		}
	}
	if state.Game.Phase != "completed" {
		t.Fatal("expected a completed game")
	}
	integrationRequestJSON[map[string]any](t, identity, http.MethodPost, lobbyURL+"/replay", guest.ReconnectToken, nil, http.StatusForbidden)
	// Failure must leave the old results and photos intact.
	guestSocket.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, guest.State.CurrentPlayerID, false)
	integrationRequestJSON[map[string]any](t, identity, http.MethodPost, lobbyURL+"/replay", host.ReconnectToken, nil, http.StatusConflict)
	unchanged := integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodGet, lobbyURL+"/state", host.ReconnectToken, nil, http.StatusOK)
	if unchanged.Game.ID != oldGameID || unchanged.Game.Phase != "completed" || len(unchanged.Game.Leaderboard) != 2 {
		t.Fatal("failed replay changed final results")
	}
	guestSocket = openIntegrationWebSocket(t, baseURL, code, guest.ReconnectToken)
	defer guestSocket.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, guest.State.CurrentPlayerID, true)

	// Two taps/concurrent requests must create one game, never reset the new one.
	type response struct {
		status int
		state  lobbyStateResponse
		err    error
	}
	responses := make(chan response, 2)
	for i := 0; i < 2; i++ {
		go func() {
			request, err := http.NewRequest(http.MethodPost, lobbyURL+"/replay", nil)
			if err != nil {
				responses <- response{err: err}
				return
			}
			request.Header.Set("Authorization", "Bearer "+host.ReconnectToken)
			result, err := identity.client.Do(request)
			if err != nil {
				responses <- response{err: err}
				return
			}
			defer result.Body.Close()
			reply := response{status: result.StatusCode}
			if result.StatusCode == http.StatusOK {
				reply.err = json.NewDecoder(result.Body).Decode(&reply.state)
			}
			responses <- reply
		}()
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		reply := <-responses
		if reply.err != nil {
			t.Fatal(reply.err)
		}
		switch reply.status {
		case http.StatusOK:
			successes++
			state = reply.state
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("replay status = %d", reply.status)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	if state.Code != code || state.CurrentPlayerID != host.State.CurrentPlayerID || state.Game.ID == oldGameID || state.Game.Phase != "collecting_submissions" || state.Game.RoundNumber != 1 || state.Game.TotalRounds != 2 || state.Game.SubmittedCount != 0 || len(state.Game.Submissions) != 0 || len(state.Game.RoundResults) != 0 {
		t.Fatalf("invalid replay state: %#v", state.Game)
	}
	if len(state.Players) != 2 {
		t.Fatal("replay lost players")
	}
	for _, player := range state.Players {
		if len(player.PhotoIDs) != 4 || player.ReadyStatus != "ready" {
			t.Fatal("replay lost photos or readiness")
		}
	}
	guestState := waitForSnapshotRevision(t, guestSocket, state.Revision)
	if guestState.Game.ID != state.Game.ID || guestState.Game.Submitted || guestState.CurrentPlayerID != guest.State.CurrentPlayerID {
		t.Fatal("guest did not follow the rematch with the same session")
	}
	// Submissions work again with the same player token and retained photographs.
	state = integrationRequestJSON[lobbyStateResponse](t, identity, http.MethodPut, lobbyURL+"/rounds/current/submission", host.ReconnectToken, validIntegrationSubmission(state.Questionnaire, integrationSubjectPhotoID(t, state), false), http.StatusOK)
	if state.Game.SubmittedCount != 1 {
		t.Fatal("the rematch could not accept its first answer")
	}

	_, other := createIntegrationModeLobby(t, baseURL, "OtherMode-"+suffix, lobbyModeFastBio, 2)
	integrationRequestJSON[map[string]any](t, identity, http.MethodPost, baseURL+"/api/v1/lobbies/"+other.State.Code+"/replay", other.ReconnectToken, nil, http.StatusConflict)
}
