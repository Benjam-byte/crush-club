package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

type integrationSession struct {
	client     *http.Client
	hostCookie *http.Cookie
}

func integrationBaseURL(t *testing.T) string {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("API_INTEGRATION_URL"), "/")
	if baseURL == "" {
		t.Skip("API_INTEGRATION_URL is not set")
	}
	return baseURL
}

func newIntegrationHost(t *testing.T, baseURL string) integrationSession {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/host-session", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || len(response.Cookies()) == 0 {
		t.Fatalf("host session status = %d, cookies = %d", response.StatusCode, len(response.Cookies()))
	}
	return integrationSession{client: client, hostCookie: response.Cookies()[0]}
}

func integrationRequestJSON[T any](
	t *testing.T,
	session integrationSession,
	method, url, token string,
	body any,
	wantStatus int,
) T {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if session.hostCookie != nil {
		request.AddCookie(session.hostCookie)
	}
	response, err := session.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("%s %s status = %d, want %d: %s", method, url, response.StatusCode, wantStatus, responseBody)
	}
	var result T
	if wantStatus != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatalf("decode %s %s: %v", method, url, err)
		}
	}
	return result
}

func createIntegrationLobby(t *testing.T, baseURL, displayName string, maxPlayers int) (integrationSession, playerSessionResponse) {
	t.Helper()
	host := newIntegrationHost(t, baseURL)
	configList := integrationRequestJSON[[]gameConfig](
		t, host, http.MethodGet, baseURL+"/api/v1/game-configs", "", nil, http.StatusOK,
	)
	if len(configList) == 0 {
		t.Fatal("no visible game configuration")
	}
	created := integrationRequestJSON[playerSessionResponse](
		t,
		host,
		http.MethodPost,
		baseURL+"/api/v1/lobbies",
		"",
		map[string]any{
			"displayName":  displayName,
			"maxPlayers":   maxPlayers,
			"gameConfigId": configList[0].ID,
		},
		http.StatusCreated,
	)
	return host, created
}

func uploadIntegrationPhotos(
	t *testing.T,
	baseURL, code, token string,
	client *http.Client,
) lobbyStateResponse {
	t.Helper()
	pngData, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	for index := range 4 {
		part, err := writer.CreateFormFile("photos", fmt.Sprintf("photo-%d.png", index+1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(pngData); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/players/me/photos",
		&requestBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("photo upload status = %d: %s", response.StatusCode, responseBody)
	}
	var state lobbyStateResponse
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func integrationGetBytes(t *testing.T, url, token string, wantStatus int) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d: %s", url, response.StatusCode, wantStatus, responseBody)
	}
	return responseBody
}

func openIntegrationWebSocket(
	t *testing.T,
	baseURL, code, token string,
) *websocket.Conn {
	t.Helper()
	websocketURL := strings.Replace(baseURL, "https://", "wss://", 1)
	websocketURL = strings.Replace(websocketURL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, websocketURL+"/ws/lobbies/"+code, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, connection, websocketAuthentication{
		Type: "authenticate", ReconnectToken: token,
	}); err != nil {
		connection.CloseNow()
		t.Fatal(err)
	}
	var snapshot websocketSnapshot
	if err := wsjson.Read(ctx, connection, &snapshot); err != nil {
		connection.CloseNow()
		t.Fatal(err)
	}
	if snapshot.Type != "state.snapshot" || snapshot.State.Code != code {
		connection.CloseNow()
		t.Fatalf("unexpected initial websocket snapshot: %#v", snapshot)
	}
	return connection
}

func waitForSnapshotRevision(
	t *testing.T,
	connection *websocket.Conn,
	revision int64,
) lobbyStateResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var snapshot websocketSnapshot
		if err := wsjson.Read(ctx, connection, &snapshot); err != nil {
			t.Fatalf("wait for websocket revision %d: %v", revision, err)
		}
		if snapshot.Type == "state.snapshot" && snapshot.State.Revision >= revision {
			return snapshot.State
		}
	}
}

func waitForPlayerConnection(
	t *testing.T,
	baseURL, code, token, playerID string,
	connected bool,
) lobbyStateResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := integrationRequestJSON[lobbyStateResponse](
			t,
			integrationSession{client: &http.Client{Timeout: 5 * time.Second}},
			http.MethodGet,
			baseURL+"/api/v1/lobbies/"+code+"/state",
			token,
			nil,
			http.StatusOK,
		)
		for _, player := range state.Players {
			if player.ID == playerID && player.Connected == connected {
				return state
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("player %s did not reach connected=%v", playerID, connected)
	return lobbyStateResponse{}
}

func validIntegrationSubmission(snapshot questionnaireSnapshot, prediction bool) roundSubmissionInput {
	input := roundSubmissionInput{
		BioAnswers:      make(map[string]string, len(snapshot.ProfileFields)),
		QuestionAnswers: make(map[string]json.RawMessage, len(snapshot.Questions)),
	}
	for _, field := range snapshot.ProfileFields {
		input.BioAnswers[field.ID] = field.Options[0].ID
	}
	for _, item := range snapshot.Questions {
		if item.Type == "integer_range" {
			input.QuestionAnswers[item.ID] = json.RawMessage(fmt.Sprintf("%d", *item.Minimum))
		} else {
			encoded, _ := json.Marshal(item.Options[0].ID)
			input.QuestionAnswers[item.ID] = encoded
		}
		if prediction && input.LoverQuestionID == "" && item.LoverEligible {
			input.LoverQuestionID = item.ID
		}
	}
	if prediction {
		input.Tagline = "Une accroche vraiment irrésistible"
	}
	return input
}

func TestMultiplayerHTTPFlow(t *testing.T) {
	baseURL := integrationBaseURL(t)
	nameSuffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	hostName := "Host-" + nameSuffix
	hostIdentity, host := createIntegrationLobby(t, baseURL, hostName, 2)
	code := host.State.Code

	integrationRequestJSON[map[string]any](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/players",
		"",
		map[string]string{"displayName": strings.ToLower(hostName)},
		http.StatusConflict,
	)
	guest := integrationRequestJSON[playerSessionResponse](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/players",
		"",
		map[string]string{"displayName": "Guest-" + nameSuffix},
		http.StatusCreated,
	)
	integrationRequestJSON[map[string]any](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/players",
		"",
		map[string]string{"displayName": "Overflow-" + nameSuffix},
		http.StatusConflict,
	)
	integrationRequestJSON[map[string]any](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodGet,
		baseURL+"/api/v1/lobbies/"+code+"/state",
		"",
		nil,
		http.StatusUnauthorized,
	)

	hostState := uploadIntegrationPhotos(t, baseURL, code, host.ReconnectToken, hostIdentity.client)
	guestState := uploadIntegrationPhotos(t, baseURL, code, guest.ReconnectToken, hostIdentity.client)
	if len(hostState.Players) != 2 || len(guestState.Players) != 2 {
		t.Fatalf("players are not synchronized: host=%d guest=%d", len(hostState.Players), len(guestState.Players))
	}
	photoID := guestState.Players[0].PhotoIDs[0]
	photoURL := baseURL + "/api/v1/lobbies/" + code + "/photos/" + photoID
	integrationGetBytes(t, photoURL, "", http.StatusUnauthorized)
	if photoData := integrationGetBytes(t, photoURL, guest.ReconnectToken, http.StatusOK); len(photoData) == 0 {
		t.Fatal("an authenticated lobby member received an empty photo")
	}
	_, otherLobby := createIntegrationLobby(t, baseURL, "Other-"+nameSuffix, 2)
	integrationGetBytes(
		t,
		baseURL+"/api/v1/lobbies/"+otherLobby.State.Code+"/photos/"+photoID,
		otherLobby.ReconnectToken,
		http.StatusNotFound,
	)

	hostSocketOne := openIntegrationWebSocket(t, baseURL, code, host.ReconnectToken)
	hostSocketTwo := openIntegrationWebSocket(t, baseURL, code, host.ReconnectToken)
	guestSocket := openIntegrationWebSocket(t, baseURL, code, guest.ReconnectToken)
	defer hostSocketOne.CloseNow()
	defer hostSocketTwo.CloseNow()
	defer guestSocket.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, host.State.CurrentPlayerID, true)
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, guest.State.CurrentPlayerID, true)
	hostSocketOne.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, host.State.CurrentPlayerID, true)
	hostSocketTwo.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, host.State.CurrentPlayerID, false)
	hostReconnectedSocket := openIntegrationWebSocket(t, baseURL, code, host.ReconnectToken)
	defer hostReconnectedSocket.CloseNow()
	waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, host.State.CurrentPlayerID, true)

	state := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost, baseURL+"/api/v1/lobbies/"+code+"/start",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if state.Game == nil || state.Game.RoundNumber != 1 || state.Game.SubjectPlayerID != host.State.CurrentPlayerID {
		t.Fatalf("unexpected first round: %#v", state.Game)
	}
	guestSnapshot := waitForSnapshotRevision(t, guestSocket, state.Revision)
	if guestSnapshot.Game == nil || guestSnapshot.Game.RoundNumber != 1 {
		t.Fatal("the start mutation was not broadcast to the guest websocket")
	}
	officialInput := validIntegrationSubmission(state.Questionnaire, false)
	predictionInput := validIntegrationSubmission(state.Questionnaire, true)
	state = integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/submission",
		host.ReconnectToken, officialInput, http.StatusOK,
	)
	if state.Game == nil || state.Game.OfficialSubmission != nil || len(state.Game.Submissions) != 0 {
		t.Fatal("official answers leaked during collection")
	}
	guestCollectionState := integrationRequestJSON[lobbyStateResponse](
		t, integrationSession{client: hostIdentity.client}, http.MethodGet,
		baseURL+"/api/v1/lobbies/"+code+"/state", guest.ReconnectToken, nil, http.StatusOK,
	)
	if guestCollectionState.Game == nil || guestCollectionState.Game.OfficialSubmission != nil ||
		len(guestCollectionState.Game.Submissions) != 0 {
		t.Fatal("official answers leaked to another player during collection")
	}
	state = integrationRequestJSON[lobbyStateResponse](
		t, integrationSession{client: hostIdentity.client}, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/submission",
		guest.ReconnectToken, predictionInput, http.StatusOK,
	)
	if state.Game == nil || state.Game.Phase != "reveal_and_vote" || len(state.Game.Submissions) != 1 {
		t.Fatalf("round did not reveal: %#v", state.Game)
	}
	firstPredictionID := state.Game.Submissions[0].ID
	state = integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/vote",
		host.ReconnectToken, roundVoteInput{SubmissionID: firstPredictionID}, http.StatusOK,
	)
	if state.Game == nil || state.Game.Phase != "round_results" || len(state.Game.RoundResults) != 1 {
		t.Fatalf("first round did not score: %#v", state.Game)
	}
	state = integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/next",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if state.Game == nil || state.Game.Phase != "between_rounds" || state.Game.RoundNumber != 1 ||
		state.Game.NextSubjectPlayerID != guest.State.CurrentPlayerID {
		t.Fatalf("game did not enter the intermission: %#v", state.Game)
	}
	integrationRequestJSON[map[string]any](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/players",
		"",
		map[string]string{"displayName": "Intermission-" + nameSuffix},
		http.StatusConflict,
	)
	state = integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/next/start",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if state.Game == nil || state.Game.Phase != "collecting_submissions" ||
		state.Game.RoundNumber != 2 || state.Game.SubjectPlayerID != guest.State.CurrentPlayerID {
		t.Fatalf("roles did not reverse: %#v", state.Game)
	}
	integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/submission",
		host.ReconnectToken, predictionInput, http.StatusOK,
	)
	state = integrationRequestJSON[lobbyStateResponse](
		t, integrationSession{client: hostIdentity.client}, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/submission",
		guest.ReconnectToken, officialInput, http.StatusOK,
	)
	if state.Game == nil || len(state.Game.Submissions) != 1 || state.Game.Submissions[0].PlayerID != "" {
		t.Fatal("the target received a non-anonymous prediction")
	}
	state = integrationRequestJSON[lobbyStateResponse](
		t, integrationSession{client: hostIdentity.client}, http.MethodPut,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/vote",
		guest.ReconnectToken, roundVoteInput{SubmissionID: state.Game.Submissions[0].ID}, http.StatusOK,
	)
	state = integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/rounds/current/next",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if state.Status != "completed" || state.Game == nil || state.Game.Phase != "completed" || len(state.Game.Leaderboard) != 2 {
		t.Fatalf("game did not complete with a persistent leaderboard: %#v", state)
	}
	refreshed := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodGet, baseURL+"/api/v1/lobbies/"+code+"/state",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if refreshed.Game == nil || refreshed.Game.Phase != "completed" || len(refreshed.Game.Leaderboard) != 2 {
		t.Fatal("completed game was not persisted")
	}
	integrationRequestJSON[map[string]any](
		t,
		integrationSession{client: &http.Client{Timeout: 10 * time.Second}},
		http.MethodPost,
		baseURL+"/api/v1/lobbies/"+code+"/players",
		"",
		map[string]string{"displayName": "Late-" + nameSuffix},
		http.StatusConflict,
	)
}

func TestFourPlayerRoundIntermissions(t *testing.T) {
	baseURL := integrationBaseURL(t)
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	hostIdentity, host := createIntegrationLobby(t, baseURL, "FourHost-"+suffix, 4)
	code := host.State.Code
	sessionList := []playerSessionResponse{host}
	for index := 2; index <= 4; index++ {
		sessionList = append(sessionList, integrationRequestJSON[playerSessionResponse](
			t,
			integrationSession{client: hostIdentity.client},
			http.MethodPost,
			baseURL+"/api/v1/lobbies/"+code+"/players",
			"",
			map[string]string{"displayName": fmt.Sprintf("FourPlayer-%s-%d", suffix, index)},
			http.StatusCreated,
		))
	}
	for _, playerSession := range sessionList {
		uploadIntegrationPhotos(t, baseURL, code, playerSession.ReconnectToken, hostIdentity.client)
	}

	socketList := make([]*websocket.Conn, 0, len(sessionList))
	for _, playerSession := range sessionList {
		connection := openIntegrationWebSocket(t, baseURL, code, playerSession.ReconnectToken)
		socketList = append(socketList, connection)
		defer connection.CloseNow()
		waitForPlayerConnection(t, baseURL, code, host.ReconnectToken, playerSession.State.CurrentPlayerID, true)
	}

	state := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodPost, baseURL+"/api/v1/lobbies/"+code+"/start",
		host.ReconnectToken, nil, http.StatusOK,
	)
	cumulativeScoreByPlayerID := make(map[string]int, len(sessionList))
	for roundIndex, expectedSubject := range sessionList {
		roundNumber := roundIndex + 1
		if state.Game == nil || state.Game.Phase != "collecting_submissions" ||
			state.Game.RoundNumber != roundNumber || state.Game.TotalRounds != 4 ||
			state.Game.SubjectPlayerID != expectedSubject.State.CurrentPlayerID {
			t.Fatalf("round %d did not use arrival order: %#v", roundNumber, state.Game)
		}

		for playerIndex, playerSession := range sessionList {
			input := validIntegrationSubmission(state.Questionnaire, playerIndex != roundIndex)
			state = integrationRequestJSON[lobbyStateResponse](
				t, integrationSession{client: hostIdentity.client}, http.MethodPut,
				baseURL+"/api/v1/lobbies/"+code+"/rounds/current/submission",
				playerSession.ReconnectToken, input, http.StatusOK,
			)
		}
		targetState := integrationRequestJSON[lobbyStateResponse](
			t, integrationSession{client: hostIdentity.client}, http.MethodGet,
			baseURL+"/api/v1/lobbies/"+code+"/state",
			expectedSubject.ReconnectToken, nil, http.StatusOK,
		)
		if targetState.Game == nil || targetState.Game.Phase != "reveal_and_vote" ||
			len(targetState.Game.Submissions) != 3 {
			t.Fatalf("round %d did not collect one submission per player: %#v", roundNumber, targetState.Game)
		}
		for _, submission := range targetState.Game.Submissions {
			if submission.PlayerID != "" || submission.AuthorName != "" {
				t.Fatalf("round %d exposed a prediction author before the vote", roundNumber)
			}
		}
		state = integrationRequestJSON[lobbyStateResponse](
			t, integrationSession{client: hostIdentity.client}, http.MethodPut,
			baseURL+"/api/v1/lobbies/"+code+"/rounds/current/vote",
			expectedSubject.ReconnectToken,
			roundVoteInput{SubmissionID: targetState.Game.Submissions[0].ID},
			http.StatusOK,
		)
		if state.Game == nil || state.Game.Phase != "round_results" ||
			len(state.Game.RoundResults) != 3 || len(state.Game.Leaderboard) != 4 {
			t.Fatalf("round %d did not produce complete results: %#v", roundNumber, state.Game)
		}
		if roundIndex < len(sessionList)-1 && state.Game.NextSubjectPlayerID != sessionList[roundIndex+1].State.CurrentPlayerID {
			t.Fatalf("round %d next subject = %s, want %s", roundNumber, state.Game.NextSubjectPlayerID, sessionList[roundIndex+1].State.CurrentPlayerID)
		}
		if roundIndex == len(sessionList)-1 && state.Game.NextSubjectPlayerID != "" {
			t.Fatalf("last round unexpectedly has next subject %s", state.Game.NextSubjectPlayerID)
		}
		for _, result := range state.Game.RoundResults {
			cumulativeScoreByPlayerID[result.PlayerID] += result.TotalScore
		}
		for _, entry := range state.Game.Leaderboard {
			if entry.Score != cumulativeScoreByPlayerID[entry.PlayerID] {
				t.Fatalf("round %d cumulative score for %s = %d, want %d", roundNumber, entry.PlayerID, entry.Score, cumulativeScoreByPlayerID[entry.PlayerID])
			}
			expectedRoundScore := 0
			for _, result := range state.Game.RoundResults {
				if result.PlayerID == entry.PlayerID {
					expectedRoundScore = result.TotalScore
				}
			}
			if entry.RoundScore != expectedRoundScore {
				t.Fatalf("round %d round score for %s = %d, want %d", roundNumber, entry.PlayerID, entry.RoundScore, expectedRoundScore)
			}
		}

		state = integrationRequestJSON[lobbyStateResponse](
			t, hostIdentity, http.MethodPost,
			baseURL+"/api/v1/lobbies/"+code+"/rounds/current/next",
			host.ReconnectToken, nil, http.StatusOK,
		)
		if roundIndex == len(sessionList)-1 {
			if state.Game == nil || state.Game.Phase != "completed" || state.Status != "completed" {
				t.Fatalf("fourth round did not complete the game: %#v", state)
			}
			break
		}
		if state.Game == nil || state.Game.Phase != "between_rounds" ||
			state.Game.RoundNumber != roundNumber || state.Game.NextSubjectPlayerID != sessionList[roundIndex+1].State.CurrentPlayerID {
			t.Fatalf("round %d did not return to the intermission lobby: %#v", roundNumber, state.Game)
		}

		if roundIndex == 0 {
			integrationRequestJSON[lobbyStateResponse](
				t, hostIdentity, http.MethodPost,
				baseURL+"/api/v1/lobbies/"+code+"/rounds/current/next",
				host.ReconnectToken, nil, http.StatusOK,
			)
		}
		if roundIndex == 0 {
			startGate := make(chan struct{})
			statusList := make(chan int, 2)
			for range 2 {
				go func() {
					<-startGate
					request, requestErr := http.NewRequest(
						http.MethodPost,
						baseURL+"/api/v1/lobbies/"+code+"/rounds/next/start",
						nil,
					)
					if requestErr != nil {
						statusList <- 0
						return
					}
					request.Header.Set("Authorization", "Bearer "+host.ReconnectToken)
					response, responseErr := (&http.Client{Timeout: 10 * time.Second}).Do(request)
					if responseErr != nil {
						statusList <- 0
						return
					}
					response.Body.Close()
					statusList <- response.StatusCode
				}()
			}
			close(startGate)
			for range 2 {
				if status := <-statusList; status != http.StatusOK {
					t.Fatalf("concurrent next-round start status = %d, want %d", status, http.StatusOK)
				}
			}
			state = integrationRequestJSON[lobbyStateResponse](
				t, hostIdentity, http.MethodGet,
				baseURL+"/api/v1/lobbies/"+code+"/state",
				host.ReconnectToken, nil, http.StatusOK,
			)
		} else {
			state = integrationRequestJSON[lobbyStateResponse](
				t, hostIdentity, http.MethodPost,
				baseURL+"/api/v1/lobbies/"+code+"/rounds/next/start",
				host.ReconnectToken, nil, http.StatusOK,
			)
		}
		if state.Game == nil || state.Game.RoundNumber != roundNumber+1 {
			t.Fatalf("next round request was not idempotent: %#v", state.Game)
		}
	}

	refreshed := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodGet, baseURL+"/api/v1/lobbies/"+code+"/state",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if refreshed.Game == nil || refreshed.Game.Phase != "completed" || len(refreshed.Game.Leaderboard) != 4 {
		t.Fatalf("final four-player result was not persisted: %#v", refreshed.Game)
	}
	for _, entry := range refreshed.Game.Leaderboard {
		if entry.Score != cumulativeScoreByPlayerID[entry.PlayerID] {
			t.Fatalf("final score for %s = %d, want %d", entry.PlayerID, entry.Score, cumulativeScoreByPlayerID[entry.PlayerID])
		}
	}

	_, replay := createIntegrationLobby(t, baseURL, "ReplayHost-"+suffix, 4)
	if replay.State.Code == code || len(replay.State.Players) != 1 || len(replay.State.Players[0].PhotoIDs) != 0 {
		t.Fatalf("replay reused the old lobby or photos: %#v", replay.State)
	}
}

func TestMultiplayerCapacityIsAtomic(t *testing.T) {
	baseURL := integrationBaseURL(t)
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	hostIdentity, host := createIntegrationLobby(t, baseURL, "CapacityHost-"+suffix, maximumPlayerCount)

	var waitGroup sync.WaitGroup
	statusList := make(chan int, 12)
	for index := range 12 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			body, _ := json.Marshal(map[string]string{"displayName": fmt.Sprintf("Concurrent-%s-%d", suffix, index)})
			request, _ := http.NewRequest(
				http.MethodPost,
				baseURL+"/api/v1/lobbies/"+host.State.Code+"/players",
				bytes.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
			if err != nil {
				statusList <- 0
				return
			}
			response.Body.Close()
			statusList <- response.StatusCode
		}()
	}
	waitGroup.Wait()
	close(statusList)
	createdCount := 0
	conflictCount := 0
	for status := range statusList {
		switch status {
		case http.StatusCreated:
			createdCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected concurrent join status: %d", status)
		}
	}
	if createdCount != 9 || conflictCount != 3 {
		t.Fatalf("concurrent joins: created=%d conflicts=%d, want 9 and 3", createdCount, conflictCount)
	}
	state := integrationRequestJSON[lobbyStateResponse](
		t, hostIdentity, http.MethodGet,
		baseURL+"/api/v1/lobbies/"+host.State.Code+"/state",
		host.ReconnectToken, nil, http.StatusOK,
	)
	if len(state.Players) != maximumPlayerCount {
		t.Fatalf("lobby contains %d players, want %d", len(state.Players), maximumPlayerCount)
	}
}
