package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type realtimeHub struct {
	api     *api
	mu      sync.RWMutex
	clients map[string]map[*realtimeClient]struct{}
}

type realtimeClient struct {
	player authenticatedPlayer
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	send   chan struct{}
}

type websocketAuthentication struct {
	Type           string `json:"type"`
	ReconnectToken string `json:"reconnectToken"`
}

type websocketSnapshot struct {
	Type  string             `json:"type"`
	State lobbyStateResponse `json:"state"`
}

func newRealtimeHub(apiServer *api) *realtimeHub {
	return &realtimeHub{
		api:     apiServer,
		clients: make(map[string]map[*realtimeClient]struct{}),
	}
}

func (a *api) handleLobbyWebSocket(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		a.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	connection.SetReadLimit(8 << 10)

	authContext, cancelAuthentication := context.WithTimeout(context.Background(), 5*time.Second)
	var authentication websocketAuthentication
	err = wsjson.Read(authContext, connection, &authentication)
	cancelAuthentication()
	if err != nil || authentication.Type != "authenticate" {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication required")
		return
	}
	player, err := a.authenticatePlayerToken(context.Background(), strings.ToUpper(r.PathValue("code")), authentication.ReconnectToken)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid player session")
		return
	}

	connectionContext, cancelConnection := context.WithCancel(context.Background())
	client := &realtimeClient{
		player: player,
		conn:   connection,
		ctx:    connectionContext,
		cancel: cancelConnection,
		send:   make(chan struct{}, 1),
	}
	firstConnection := a.hub.register(client)
	if firstConnection {
		a.hub.markConnected(player)
	}
	defer func() {
		cancelConnection()
		lastConnection := a.hub.unregister(client)
		if lastConnection {
			a.hub.markDisconnected(player)
		}
		_ = connection.Close(websocket.StatusNormalClosure, "")
	}()

	go a.hub.writeLoop(client)
	go a.hub.pingLoop(client)
	a.hub.trigger(client)

	for {
		_, _, err := connection.Read(connectionContext)
		if err != nil {
			return
		}
	}
}

func (h *realtimeHub) register(client *realtimeClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.clients[client.player.Code]
	if room == nil {
		room = make(map[*realtimeClient]struct{})
		h.clients[client.player.Code] = room
	}
	firstConnection := true
	for existing := range room {
		if existing.player.ID == client.player.ID {
			firstConnection = false
			break
		}
	}
	room[client] = struct{}{}
	return firstConnection
}

func (h *realtimeHub) unregister(client *realtimeClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.clients[client.player.Code]
	if room == nil {
		return false
	}
	delete(room, client)
	lastConnection := true
	for existing := range room {
		if existing.player.ID == client.player.ID {
			lastConnection = false
			break
		}
	}
	if len(room) == 0 {
		delete(h.clients, client.player.Code)
	}
	return lastConnection
}

func (h *realtimeHub) trigger(client *realtimeClient) {
	select {
	case client.send <- struct{}{}:
	default:
	}
}

func (h *realtimeHub) publish(code string) {
	h.mu.RLock()
	room := h.clients[strings.ToUpper(code)]
	clientList := make([]*realtimeClient, 0, len(room))
	for client := range room {
		clientList = append(clientList, client)
	}
	h.mu.RUnlock()
	for _, client := range clientList {
		h.trigger(client)
	}
}

func (h *realtimeHub) writeLoop(client *realtimeClient) {
	for {
		select {
		case <-client.ctx.Done():
			return
		case <-client.send:
			requestContext, cancel := context.WithTimeout(client.ctx, 10*time.Second)
			state, err := h.api.loadLobbyState(requestContext, client.player)
			if err == nil {
				err = wsjson.Write(requestContext, client.conn, websocketSnapshot{
					Type:  "state.snapshot",
					State: state,
				})
			}
			cancel()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					h.api.logger.Debug("websocket snapshot failed", "player_id", client.player.ID, "error", err)
				}
				client.cancel()
				return
			}
		}
	}
}

func (h *realtimeHub) pingLoop(client *realtimeClient) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-client.ctx.Done():
			return
		case <-ticker.C:
			pingContext, cancel := context.WithTimeout(client.ctx, 10*time.Second)
			err := client.conn.Ping(pingContext)
			cancel()
			if err != nil {
				client.cancel()
				return
			}
			_, _ = h.api.pool.Exec(context.Background(), `
				UPDATE players SET last_seen_at = now(), updated_at = now() WHERE id = $1
			`, client.player.ID)
		}
	}
}

func (h *realtimeHub) isOnline(code, playerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[strings.ToUpper(code)] {
		if client.player.ID == playerID {
			return true
		}
	}
	return false
}

func (h *realtimeHub) onlinePlayerIDs(code string) map[string]struct{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[string]struct{})
	for client := range h.clients[strings.ToUpper(code)] {
		result[client.player.ID] = struct{}{}
	}
	return result
}

func (h *realtimeHub) markConnected(player authenticatedPlayer) {
	_, err := h.api.pool.Exec(context.Background(), `
		WITH connected AS (
		  UPDATE players
		  SET disconnected_at = NULL, last_seen_at = now(), updated_at = now()
		  WHERE id = $1
		)
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $2
	`, player.ID, player.LobbyID)
	if err != nil {
		h.api.logger.Error("unable to mark player connected", "player_id", player.ID, "error", err)
	}
	h.publish(player.Code)
}

func (h *realtimeHub) markDisconnected(player authenticatedPlayer) {
	disconnectedAt := time.Now().UTC()
	_, err := h.api.pool.Exec(context.Background(), `
		WITH disconnected AS (
		  UPDATE players
		  SET disconnected_at = $1, last_seen_at = $1, updated_at = now()
		  WHERE id = $2 AND excluded_at IS NULL
		)
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $3
	`, disconnectedAt, player.ID, player.LobbyID)
	if err != nil {
		h.api.logger.Error("unable to mark player disconnected", "player_id", player.ID, "error", err)
		return
	}
	h.publish(player.Code)
	time.AfterFunc(reconnectGracePeriod, func() {
		h.transferDisconnectedHost(player.Code, player.ID, disconnectedAt)
	})
}

func (h *realtimeHub) transferDisconnectedHost(code, playerID string, disconnectedAt time.Time) {
	if h.isOnline(code, playerID) {
		return
	}
	onlineIDs := h.onlinePlayerIDs(code)
	if len(onlineIDs) == 0 {
		return
	}
	rows, err := h.api.pool.Query(context.Background(), `
		SELECT player.id
		FROM players AS player
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE lobby.code = $1 AND player.excluded_at IS NULL
		ORDER BY player.joined_at, player.id
	`, code)
	if err != nil {
		h.api.logger.Error("unable to select host successor", "lobby_code", code, "error", err)
		return
	}
	var successorID string
	for rows.Next() {
		var candidateID string
		if scanErr := rows.Scan(&candidateID); scanErr != nil {
			err = scanErr
			break
		}
		if _, connected := onlineIDs[candidateID]; connected && candidateID != playerID {
			successorID = candidateID
			break
		}
	}
	rows.Close()
	if err != nil || successorID == "" {
		return
	}
	tag, err := h.api.pool.Exec(context.Background(), `
		WITH previous_host AS (
		  UPDATE players AS player
		  SET is_host = false, updated_at = now()
		  FROM lobbies AS lobby
		  WHERE player.id = $1 AND player.lobby_id = lobby.id
		    AND lobby.code = $2 AND player.is_host
		    AND player.disconnected_at = $3
		    AND player.disconnected_at <= now() - interval '90 seconds'
		  RETURNING lobby.id
		), next_host AS (
		  UPDATE players SET is_host = true, updated_at = now()
		  WHERE id = $4 AND lobby_id IN (SELECT id FROM previous_host)
		  RETURNING lobby_id
		)
		UPDATE lobbies
		SET host_player_id = $4, revision = revision + 1, updated_at = now()
		WHERE id IN (SELECT lobby_id FROM next_host)
	`, playerID, code, disconnectedAt, successorID)
	if err != nil {
		h.api.logger.Error("unable to transfer lobby host", "lobby_code", code, "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		h.publish(code)
	}
}

func (h *realtimeHub) closeAll() {
	h.mu.RLock()
	clients := make([]*realtimeClient, 0)
	for _, room := range h.clients {
		for client := range room {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()
	for _, client := range clients {
		client.cancel()
		_ = client.conn.Close(websocket.StatusServiceRestart, "server restarting")
	}
}
