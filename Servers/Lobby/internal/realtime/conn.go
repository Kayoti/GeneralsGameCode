// This file holds one WebSocket connection: the loop that reads the client's
// messages and the loop that writes the messages waiting for it (SERVICE.md section 6.1).

package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// Limits on what a client may send (SERVICE.md sections 3 and 6.3).
const (
	// maxMessageBytes is the largest message the service reads; a larger one ends the connection.
	maxMessageBytes = 256 * 1024
	// maxMessagesPerSecond is how many messages count against a client in one
	// second. Messages 12 and 14 are not counted.
	maxMessagesPerSecond = 60
	// rateWindow is the period maxMessagesPerSecond counts over.
	rateWindow = time.Second
)

// closeReasonFlood is the close reason for a client that sends too much.
const closeReasonFlood = "too many messages"

// connection is one WebSocket of one player. Its context ends when the
// connection is dropped; reads and writes use it, so ending it closes the
// socket without a close frame.
type connection struct {
	hub    *Hub
	player *player
	socket *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
}

// wakeWriter tells the writer loop that its player's outbox has messages.
// It never blocks.
func (live *connection) wakeWriter() {
	select {
	case live.wake <- struct{}{}:
	default:
	}
}

// drop ends the connection at once, without a close frame. It never blocks.
func (live *connection) drop() {
	live.cancel()
}

// closeWithFrame sends a close frame, which makes the client log out, and
// ends the connection. It waits for the client's answer for a few seconds.
func (live *connection) closeWithFrame(code websocket.StatusCode, reason string) {
	_ = live.socket.Close(code, reason)
	live.cancel()
}

// rateLimiter counts the messages of one connection in a fixed window.
type rateLimiter struct {
	windowStart time.Time
	count       int
}

// allow counts one message at now and reports whether the client is still
// within maxMessagesPerSecond.
func (limiter *rateLimiter) allow(now time.Time) bool {
	if now.Sub(limiter.windowStart) >= rateWindow {
		limiter.windowStart = now
		limiter.count = 0
	}
	limiter.count++
	return limiter.count <= maxMessagesPerSecond
}

// readLoop reads and handles messages until the connection ends. It returns
// true when the client closed the connection with a close frame. JSON is
// accepted in text and binary frames. Silence for Settings.IdleTimeout ends
// the connection without a close frame.
func (live *connection) readLoop() bool {
	var limiter rateLimiter
	for {
		readCtx, stopTimer := context.WithTimeout(live.ctx, live.hub.settings.IdleTimeout)
		_, data, err := live.socket.Read(readCtx)
		stopTimer()
		if err != nil {
			return websocket.CloseStatus(err) != -1
		}
		if !live.withinRate(&limiter, data) {
			live.hub.logger.Info("client sent too many messages", "userId", live.player.userID)
			live.closeWithFrame(websocket.StatusPolicyViolation, closeReasonFlood)
			return false
		}
		live.hub.handleMessage(live, data)
	}
}

// withinRate counts a message against the connection's limit and reports
// whether the client is still within it. Messages 12 and 14 are not counted.
func (live *connection) withinRate(limiter *rateLimiter, data []byte) bool {
	var envelope inboundEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.MsgID != nil {
		if *envelope.MsgID == msgSignal || *envelope.MsgID == msgPing {
			return true
		}
	}
	return limiter.allow(time.Now())
}

// writeLoop writes the messages waiting in the player's outbox, as text
// frames, until the connection ends. A write that fails or takes longer than
// Settings.WriteTimeout ends the connection.
func (live *connection) writeLoop() {
	for {
		select {
		case <-live.ctx.Done():
			return
		case <-live.wake:
		}
		for _, message := range live.hub.takeOutbox(live) {
			writeCtx, stopTimer := context.WithTimeout(live.ctx, live.hub.settings.WriteTimeout)
			err := live.socket.Write(writeCtx, websocket.MessageText, message.data)
			stopTimer()
			if err != nil {
				live.hub.logger.Info("write failed", "userId", live.player.userID, "error", err)
				live.drop()
				return
			}
		}
	}
}

// takeOutbox removes and returns the messages waiting for the player, or
// nothing when live is no longer the player's connection.
func (hub *Hub) takeOutbox(live *connection) []outgoing {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if live.player.conn != live {
		return nil
	}
	waiting := live.player.outbox
	live.player.outbox = nil
	return waiting
}
