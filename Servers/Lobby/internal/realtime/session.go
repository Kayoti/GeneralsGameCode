// This file holds the life of a player's place on the service: the upgrade
// request, reconnecting within the hold time, replacing a connection, and
// releasing the place (SERVICE.md sections 6.1 and 6.2).

package realtime

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// reconnectHeader is the upgrade request header that tells a reconnect from a new session.
const reconnectHeader = "is-reconnect"

// statusNoHeldPlace is the answer to a reconnect when no place is held
// (SERVICE.md section 6.2). The client then connects as a new session.
const statusNoHeldPlace = http.StatusResetContent

// closeReasonReplaced is sent to a connection that another login replaces.
// It is a fixed text because a close reason must not carry dynamic data.
const closeReasonReplaced = "replaced by a new connection"

// ServeWS upgrades the request to a WebSocket for the account userID and
// serves it until the connection ends. The caller has already checked the
// session token. A reconnect with no held place is answered with HTTP 205; a
// new session releases any place held before (SERVICE.md section 6.2).
func (hub *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID int64) {
	isReconnect := strings.EqualFold(r.Header.Get(reconnectHeader), "true")
	name, found, err := hub.accounts.DisplayName(userID)
	if err != nil {
		hub.logger.Error("read name for connection", "userId", userID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	held, ok := hub.beginSession(userID, name, isReconnect)
	if !ok {
		w.WriteHeader(statusNoHeldPlace)
		return
	}
	socket, err := websocket.Accept(w, r, nil)
	if err != nil {
		hub.logger.Info("upgrade failed", "userId", userID, "error", err)
		hub.holdPlace(held)
		return
	}
	hub.serveConnection(held, socket)
}

// beginSession finds or creates the player for an upgrade request. A
// reconnect takes over the held place; the bool is false when there is none.
// A new session releases any place held before and starts with no room.
// The returned player has no connection yet and its hold timer is stopped or running.
func (hub *Hub) beginSession(userID int64, name string, isReconnect bool) (*player, bool) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	existing, found := hub.players[userID]
	if isReconnect {
		if !found {
			return nil, false
		}
		hub.stopHold(existing)
		return existing, true
	}
	if found {
		hub.releasePlace(existing)
	}
	created := &player{userID: userID, name: name, roomID: roomNone, lobbyID: noLobby}
	hub.players[userID] = created
	return created, true
}

// serveConnection attaches the socket to the player, serves it until it
// ends, and then holds or releases the player's place.
func (hub *Hub) serveConnection(target *player, socket *websocket.Conn) {
	socket.SetReadLimit(maxMessageBytes)
	ctx, cancel := context.WithCancel(context.Background())
	live := &connection{hub: hub, player: target, socket: socket, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	if !hub.attach(live) {
		live.closeWithFrame(websocket.StatusPolicyViolation, closeReasonReplaced)
		return
	}
	go live.writeLoop()
	clientClosed := live.readLoop()
	cancel()
	hub.detach(live, clientClosed)
}

// attach makes live the player's connection. A connection that was live is
// closed with a close frame, which tells its client to log out. Notices that
// waited while the player had no connection are queued in id order. It returns
// false, and changes nothing, when the player's place was released since the
// upgrade began.
func (hub *Hub) attach(live *connection) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	target := live.player
	if hub.players[target.userID] != target {
		return false
	}
	hub.stopHold(target)
	if previous := target.conn; previous != nil {
		go previous.closeWithFrame(websocket.StatusPolicyViolation, closeReasonReplaced)
	}
	keepHeldMessages(target)
	sortHeldMessages(target)
	target.conn = live
	if len(target.outbox) > 0 {
		live.wakeWriter()
	}
	return true
}

// detach handles the end of a connection. When the client closed it on
// purpose, which the client does only to log out, the place is released at
// once. Otherwise it is held for Settings.ReconnectHold. A connection that
// has been replaced changes nothing.
func (hub *Hub) detach(ended *connection, clientClosed bool) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	target := ended.player
	if target.conn != ended {
		return
	}
	target.conn = nil
	keepHeldMessages(target)
	if clientClosed {
		hub.releasePlace(target)
		return
	}
	hub.startHold(target)
}

// holdPlace starts the hold timer of a player that has no connection.
func (hub *Hub) holdPlace(target *player) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.players[target.userID] == target && target.conn == nil {
		hub.startHold(target)
	}
}

// startHold starts the timer that releases the place of a player who has no
// connection. The caller holds hub.mu.
func (hub *Hub) startHold(target *player) {
	hub.stopHold(target)
	generation := target.holdGeneration
	target.holdTimer = time.AfterFunc(hub.settings.ReconnectHold, func() {
		hub.expireHold(target, generation)
	})
}

// stopHold stops the hold timer of a player. A timer that has already fired
// and is waiting for the lock finds the changed generation and does nothing.
// The caller holds hub.mu.
func (hub *Hub) stopHold(target *player) {
	target.holdGeneration++
	if target.holdTimer != nil {
		target.holdTimer.Stop()
		target.holdTimer = nil
	}
}

// expireHold releases the place of a player whose hold ran out, unless the
// player came back or the timer was replaced meanwhile.
func (hub *Hub) expireHold(target *player, generation uint64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.players[target.userID] != target || target.conn != nil || target.holdGeneration != generation {
		return
	}
	hub.releasePlace(target)
}

// releasePlace removes a player from the service: the live connection, if any,
// is closed with a close frame, the player leaves their lobby (SERVICE.md
// section 7.4) and their room, and the place is forgotten. The caller holds hub.mu.
func (hub *Hub) releasePlace(target *player) {
	hub.stopHold(target)
	if live := target.conn; live != nil {
		target.conn = nil
		go live.closeWithFrame(websocket.StatusPolicyViolation, closeReasonReplaced)
	}
	hub.leaveLobby(target)
	hub.leaveRoom(target)
	if hub.players[target.userID] == target {
		delete(hub.players, target.userID)
	}
}
