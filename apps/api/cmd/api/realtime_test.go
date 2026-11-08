package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5"
)

func TestWebsocketSessionFailureDistinguishesDatabaseErrors(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		want websocket.StatusCode
	}{
		{name: "invalid session", err: errNotFound, want: closeInvalidSession},
		{name: "wrapped invalid session", err: fmt.Errorf("authenticate: %w", errNotFound), want: closeInvalidSession},
		{name: "database unavailable", err: errors.New("database unavailable"), want: websocket.StatusTryAgainLater},
		{name: "verification timeout", err: context.DeadlineExceeded, want: websocket.StatusTryAgainLater},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			status, _ := websocketSessionFailure(testCase.err)
			if status != testCase.want {
				t.Fatalf("status = %v, want %v", status, testCase.want)
			}
		})
	}
}

func snapshotFailureTestServer(t *testing.T, load func(context.Context, authenticatedPlayer) (lobbyStateResponse, error)) (*httptest.Server, *realtimeHub) {
	t.Helper()
	a := &api{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), instanceID: "test-instance"}
	hub := newRealtimeHub(a)
	hub.loadState = load
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := acceptLobbyWebSocket(w, r)
		if err != nil {
			return
		}
		connection.SetReadLimit(8 << 10)
		ctx, cancel := context.WithCancel(context.Background())
		client := &realtimeClient{
			id: "test-connection", player: authenticatedPlayer{ID: "test-player", Code: "ROOM"},
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
	return server, hub
}

func TestSnapshotLoadFailureKeepsSocketAndDiagnosticProbeAlive(t *testing.T) {
	var attempts atomic.Int32
	server, hub := snapshotFailureTestServer(t, func(_ context.Context, player authenticatedPlayer) (lobbyStateResponse, error) {
		if attempts.Add(1) == 1 {
			return lobbyStateResponse{}, errors.New("temporary database failure")
		}
		return lobbyStateResponse{Code: player.Code, CurrentPlayerID: player.ID, Revision: 7}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var unavailable websocketStateUnavailable
	if err := wsjson.Read(ctx, connection, &unavailable); err != nil {
		t.Fatal(err)
	}
	if unavailable.Type != "state.unavailable" || unavailable.RetryInMS != 1000 {
		t.Fatalf("unexpected retry notice: %#v", unavailable)
	}
	for range 100 {
		hub.publish("ROOM")
	}
	if err := wsjson.Write(ctx, connection, websocketDiagnosticPing{Type: "diagnostic.ping", ID: "probe-1"}); err != nil {
		t.Fatal(err)
	}
	var pong websocketDiagnosticPong
	if err := wsjson.Read(ctx, connection, &pong); err != nil {
		t.Fatal(err)
	}
	if pong.Type != "diagnostic.pong" || pong.ID != "probe-1" || pong.PlayerID != "test-player" || pong.ServerInstanceID != "test-instance" {
		t.Fatalf("diagnostic probe failed during a database outage: %#v", pong)
	}
	var snapshot websocketSnapshot
	if err := wsjson.Read(ctx, connection, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "state.snapshot" || snapshot.State.Revision != 7 || snapshot.ConnectionID != "test-connection" {
		t.Fatalf("snapshot was not recovered on the same connection: %#v", snapshot)
	}
	if attempts.Load() != 2 {
		t.Fatalf("database loads = %d, want 2 despite repeated broadcasts", attempts.Load())
	}
	if !hub.isOnline("ROOM", "test-player") {
		t.Fatal("transient failure disconnected the player")
	}
}

func TestMissingLobbyStopsSnapshotRetries(t *testing.T) {
	server, _ := snapshotFailureTestServer(t, func(context.Context, authenticatedPlayer) (lobbyStateResponse, error) {
		return lobbyStateResponse{}, pgx.ErrNoRows
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	_, _, err = connection.Read(ctx)
	if websocket.CloseStatus(err) != closeLobbyUnavailable {
		t.Fatalf("missing lobby closure: %v", err)
	}
}

func TestLobbyStateResponsesIdentifyInstanceEvenWhenUnauthorized(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := newAPI(nil, logger, false)
	b := newAPI(nil, logger, false)
	if a.instanceID == "" || a.instanceID == b.instanceID {
		t.Fatal("server instances do not have distinct identifiers")
	}
	response := httptest.NewRecorder()
	a.handleGetLobbyState(response, httptest.NewRequest(http.MethodGet, "/api/v1/lobbies/ROOM/state", nil))
	if response.Code != http.StatusUnauthorized || response.Header().Get("X-Crush-Club-Server") != a.instanceID || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("diagnostic response status=%d headers=%v", response.Code, response.Header())
	}
}

func TestLobbyWebSocketCompressionIsNegotiatedPerConnection(t *testing.T) {
	serverErrors := make(chan error, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := acceptLobbyWebSocket(w, r)
		if err != nil {
			serverErrors <- err
			return
		}
		defer connection.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		messageType, payload, err := connection.Read(ctx)
		if err == nil {
			err = connection.Write(ctx, messageType, payload)
		}
		serverErrors <- err
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/lobbies/ROOM"
	testCases := []struct {
		query       string
		compression bool
	}{
		{query: "?compression=off", compression: false},
		{query: "?compression=on", compression: true},
		{query: "", compression: false},
	}
	connections := make([]*websocket.Conn, 0, len(testCases))
	for _, testCase := range testCases {
		connection, response, err := websocket.Dial(ctx, url+testCase.query, &websocket.DialOptions{
			CompressionMode: websocket.CompressionContextTakeover,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = connection.CloseNow() })
		negotiated := strings.Contains(response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
		if negotiated != testCase.compression {
			t.Fatalf("compression for %q = %v, want %v", testCase.query, negotiated, testCase.compression)
		}
		connections = append(connections, connection)
	}
	for index, connection := range connections {
		payload := strings.Repeat(fmt.Sprintf("player-%d lobby snapshot ", index), 128)
		if err := connection.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		_, received, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if string(received) != payload {
			t.Fatal("a message was corrupted or sent to another connection")
		}
	}
	for range connections {
		if err := <-serverErrors; err != nil {
			t.Fatal(err)
		}
	}
}

func newTestRealtimeClient(t *testing.T, playerID string) *realtimeClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &realtimeClient{
		player: authenticatedPlayer{ID: playerID, Code: "ROOM"},
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan struct{}, 1),
	}
}

func TestRealtimeHubConcurrentReconnectsKeepPlayersSeparate(t *testing.T) {
	hub := newRealtimeHub(nil)
	players := []*realtimeClient{
		newTestRealtimeClient(t, "player-one"),
		newTestRealtimeClient(t, "player-two"),
	}
	for _, client := range players {
		if !hub.register(client) {
			t.Fatal("a distinct player's initial connection was not registered separately")
		}
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for index, initial := range players {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			current := initial
			for range 100 {
				replacement := newTestRealtimeClient(t, initial.player.ID)
				if hub.register(replacement) {
					t.Error("a replacement socket was mistaken for a new player")
				}
				if hub.unregister(current) {
					t.Error("closing an old socket marked a reconnected player offline")
				}
				current.cancel()
				for _, playerID := range []string{"player-one", "player-two"} {
					if !hub.isOnline("ROOM", playerID) {
						t.Errorf("reconnecting %s disconnected %s", initial.player.ID, playerID)
					}
				}
				if replacement.ctx.Err() != nil {
					t.Error("closing an old socket canceled its replacement")
				}
				hub.publish("ROOM")
				current = replacement
			}
			players[index] = current
		}()
	}
	close(start)
	workers.Wait()

	if count := len(hub.onlinePlayerIDs("ROOM")); count != 2 {
		t.Fatalf("online player count after concurrent reconnects = %d, want 2", count)
	}
	for _, client := range players {
		select {
		case <-client.send:
		default:
			t.Errorf("player %s stopped receiving lobby updates", client.player.ID)
		}
	}
	if !hub.unregister(players[0]) {
		t.Fatal("closing the first player's last socket was not detected")
	}
	players[0].cancel()
	if !hub.isOnline("ROOM", "player-two") || players[1].ctx.Err() != nil {
		t.Fatal("disconnecting the first player affected the second player")
	}
	if !hub.unregister(players[1]) {
		t.Fatal("closing the second player's last socket was not detected")
	}
}

func TestRealtimeHubSharedSessionDoesNotEvictExistingSockets(t *testing.T) {
	hub := newRealtimeHub(nil)
	first := newTestRealtimeClient(t, "shared-player")
	second := newTestRealtimeClient(t, "shared-player")
	other := newTestRealtimeClient(t, "other-player")
	if !hub.register(first) || !hub.register(other) {
		t.Fatal("distinct players were not registered separately")
	}
	if hub.register(second) {
		t.Fatal("two sockets with the same player ID were counted as distinct players")
	}
	hub.publish("ROOM")
	for _, client := range []*realtimeClient{first, second, other} {
		if client.ctx.Err() != nil {
			t.Fatal("sharing a session canceled an existing socket")
		}
		select {
		case <-client.send:
		default:
			t.Fatal("sharing a session stopped updates to an existing socket")
		}
	}
	if hub.unregister(first) {
		t.Fatal("closing one shared-session socket marked the player offline")
	}
	first.cancel()
	if !hub.isOnline("ROOM", "shared-player") || second.ctx.Err() != nil {
		t.Fatal("closing one socket affected the other shared-session socket")
	}
	if !hub.unregister(second) {
		t.Fatal("closing the last shared-session socket was not detected")
	}
	second.cancel()
	if hub.isOnline("ROOM", "shared-player") || !hub.isOnline("ROOM", "other-player") || other.ctx.Err() != nil {
		t.Fatal("shared-session disconnection affected the wrong player")
	}
	hub.unregister(other)
}
