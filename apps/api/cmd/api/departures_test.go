package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestThemeStepWindow(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		step  themeStep
		first bool
		want  time.Duration
	}{
		{name: "first proposal", step: themeProposalStep, first: true, want: 2 * time.Minute},
		{name: "first ranking", step: themeRankingStep, first: true, want: 2 * time.Minute},
		{name: "later proposal", step: themeProposalStep, first: false, want: 40 * time.Second},
		{name: "later ranking", step: themeRankingStep, first: false, want: 40 * time.Second},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := themeStepWindow(testCase.step, testCase.first); got != testCase.want {
				t.Fatalf("themeStepWindow = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestActivePlayerMinimum(t *testing.T) {
	for mode, want := range map[string]int{
		lobbyModeFastBio:       2,
		lobbyModeZeroToHundred: 3,
		lobbyModeSituation:     2,
		lobbyModeWavelength:    2,
		lobbyModeSecretRanking: 3,
	} {
		if got := activePlayerMinimum(mode); got != want {
			t.Errorf("activePlayerMinimum(%s) = %d, want %d", mode, got, want)
		}
	}
}

func TestStepProgressFollowsAwaitedPlayers(t *testing.T) {
	progress := newStepProgress(4, []string{"bob"})
	if progress.count != 3 || progress.required != 4 || !slices.Equal(progress.awaiting, []string{"bob"}) {
		t.Fatalf("progress = %#v", progress)
	}
	// A required count below the awaited players never goes negative.
	if progress := newStepProgress(1, []string{"bob", "eve"}); progress.count != 0 {
		t.Fatalf("count = %d, want 0", progress.count)
	}
}

func TestSessionErrorCodeExplainsRemovals(t *testing.T) {
	for _, testCase := range []struct {
		err  error
		want string
	}{
		{err: playerRemovedError{reason: exclusionReasonKicked}, want: "player_excluded"},
		{err: playerRemovedError{reason: exclusionReasonRefused}, want: "admission_refused"},
		{err: playerRemovedError{reason: exclusionReasonLeft}, want: "player_session_required"},
		{err: errNotFound, want: "player_session_required"},
	} {
		if got := sessionErrorCode(testCase.err); got != testCase.want {
			t.Errorf("sessionErrorCode(%v) = %s, want %s", testCase.err, got, testCase.want)
		}
		if !errors.Is(testCase.err, errNotFound) {
			t.Errorf("%v no longer matches errNotFound", testCase.err)
		}
	}
}

func TestRemovedPlayerSnapshotClosesWithReason(t *testing.T) {
	for reason, want := range map[string]websocket.StatusCode{
		exclusionReasonKicked:  closePlayerKicked,
		exclusionReasonRefused: closeAdmissionRefused,
		exclusionReasonLeft:    closeInvalidSession,
	} {
		t.Run(reason, func(t *testing.T) {
			server, _ := snapshotFailureTestServer(t, func(context.Context, authenticatedPlayer) (lobbyStateResponse, error) {
				return lobbyStateResponse{}, playerRemovedError{reason: reason}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.CloseNow()
			_, _, err = connection.Read(ctx)
			if websocket.CloseStatus(err) != want {
				t.Fatalf("close status = %v (%v), want %v", websocket.CloseStatus(err), err, want)
			}
		})
	}
}

func TestClosePlayerOnlyClosesTheirSockets(t *testing.T) {
	a := &api{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), instanceID: "test-instance"}
	hub := newRealtimeHub(a)
	hub.loadState = func(_ context.Context, player authenticatedPlayer) (lobbyStateResponse, error) {
		return lobbyStateResponse{Code: player.Code, CurrentPlayerID: player.ID, Revision: 1}, nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := acceptLobbyWebSocket(w, r)
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		client := &realtimeClient{
			id: r.URL.Query().Get("player"), player: authenticatedPlayer{ID: r.URL.Query().Get("player"), Code: "ROOM"},
			conn: connection, ctx: ctx, cancel: cancel, send: make(chan struct{}, 1),
			events: make(chan websocketReactionEvent, 8), probes: make(chan websocketDiagnosticPong, 1),
		}
		hub.register(client)
		defer func() { cancel(); hub.unregister(client); connection.CloseNow() }()
		go hub.writeLoop(client)
		hub.trigger(client)
		_ = hub.readLoop(client)
	}))
	t.Cleanup(func() { hub.closeAll(); server.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(playerID string) *websocket.Conn {
		connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"?player="+playerID, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { connection.CloseNow() })
		var snapshot websocketSnapshot
		if err := wsjson.Read(ctx, connection, &snapshot); err != nil {
			t.Fatal(err)
		}
		return connection
	}
	kicked := dial("kicked")
	staying := dial("staying")

	hub.closePlayer("room", "kicked", closePlayerKicked, "removed by the host")
	if _, _, err := kicked.Read(ctx); websocket.CloseStatus(err) != closePlayerKicked {
		t.Fatalf("kicked socket close status = %v (%v)", websocket.CloseStatus(err), err)
	}
	hub.publish("ROOM")
	var snapshot websocketSnapshot
	if err := wsjson.Read(ctx, staying, &snapshot); err != nil {
		t.Fatalf("the other player lost their socket: %v", err)
	}
	if snapshot.State.CurrentPlayerID != "staying" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}
