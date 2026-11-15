// This file holds what the service does with the lobby messages a client
// sends over the WebSocket: 5, 10, 13, 23, 24, 25 and 26 (SERVICE.md sections
// 6.3 and 7.5). The sender is the connection's player; a lobby is found from
// the player, never from a value in the message.

package realtime

import (
	"encoding/json"
	"time"

	"servers-lobby/internal/lobbies"
)

// handleMarkReady handles message 5: the ready state of a lobby member.
func (hub *Hub) handleMarkReady(live *connection, data []byte) {
	var request markReadyRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Ready == nil {
		hub.logger.Debug("ignored a ready message with no valid ready", "userId", live.player.userID)
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	lobby := hub.lobbyOf(live.player)
	if lobby == nil {
		return
	}
	wasListed := lobby.IsListed()
	if lobby.SetReady(live.player.userID, *request.Ready, time.Now()) {
		hub.lobbyChanged(lobby, wasListed)
	}
}

// handleLobbyChat handles message 10 and sends message 11 to the members the
// lobby names. Text that is empty after cleaning is ignored.
func (hub *Hub) handleLobbyChat(live *connection, data []byte) {
	var request lobbyChatRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Message == nil {
		hub.logger.Debug("ignored a lobby chat with no valid message", "userId", live.player.userID)
		return
	}
	text := lobbies.CleanText(*request.Message, maxChatCharacters)
	if text == "" {
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sender := live.player
	lobby := hub.lobbyOf(sender)
	if lobby == nil {
		return
	}
	targets, isAnnouncement, ok := lobby.ChatTargets(sender.userID, request.Announcement, request.ShowAnnouncementToHost)
	if !ok {
		return
	}
	line := lobbyChatMessage{
		MsgID:                  msgLobbyChatFromServer,
		Message:                text,
		Action:                 request.Action,
		Announcement:           isAnnouncement,
		ShowAnnouncementToHost: isAnnouncement && request.ShowAnnouncementToHost,
		UserID:                 sender.userID,
	}
	for _, userID := range targets {
		if member, found := hub.players[userID]; found {
			hub.queueMessage(member, msgLobbyChatFromServer, line)
		}
	}
}

// handleCountdownStarted handles message 23: the host's countdown locks the lobby.
func (hub *Hub) handleCountdownStarted(live *connection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if lobby := hub.lobbyOf(live.player); lobby != nil {
		lobby.StartCountdown(live.player.userID, time.Now(), hub.settings.CountdownLock)
	}
}

// handleConnectivityCheck handles message 26. The check itself comes with
// player-to-player support; here a new 26 from the host ends the countdown lock
// (SERVICE.md section 6.3).
func (hub *Hub) handleConnectivityCheck(live *connection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if lobby := hub.lobbyOf(live.player); lobby != nil {
		lobby.EndCountdown(live.player.userID)
	}
}

// handleRemovePassword handles message 24.
func (hub *Hub) handleRemovePassword(live *connection) {
	hub.changePassword(live.player, "")
}

// handleChangePassword handles message 25. A missing or empty new_password is
// ignored; removing the password is message 24.
func (hub *Hub) handleChangePassword(live *connection, data []byte) {
	var request changePasswordRequest
	if err := json.Unmarshal(data, &request); err != nil || request.NewPassword == nil || *request.NewPassword == "" {
		hub.logger.Debug("ignored a password change with no valid password", "userId", live.player.userID)
		return
	}
	hub.changePassword(live.player, *request.NewPassword)
}

// changePassword sets or, for an empty password, removes the password of the
// sender's lobby when the sender is its host, and sends the notices.
func (hub *Hub) changePassword(sender *player, password string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	lobby := hub.lobbyOf(sender)
	if lobby == nil {
		return
	}
	wasListed := lobby.IsListed()
	if lobby.SetPassword(sender.userID, password) {
		hub.lobbyChanged(lobby, wasListed)
	}
}

// handleStartGame handles message 13 from the host (SERVICE.md section 7.5).
// When the lobby starts, the members get 6 and then 13, and the watchers get 7.
// Every human member gets 13, also one whose connection is down: that one is
// kept for the reconnect (SERVICE.md section 6.2).
func (hub *Hub) handleStartGame(live *connection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	lobby := hub.lobbyOf(live.player)
	if lobby == nil {
		return
	}
	wasListed := lobby.IsListed()
	if !lobby.Start(live.player.userID, hub.nextMatchID) {
		return
	}
	hub.nextMatchID++
	hub.lobbyChanged(lobby, wasListed)
	for _, userID := range lobby.Humans() {
		if member, found := hub.players[userID]; found {
			hub.queueHeldMessage(member, msgStartGame, startGameMessage{MsgID: msgStartGame})
		}
	}
}

// queueHeldMessage encodes a message and puts it in the outbox of target,
// where it waits for a player whose connection is down. The caller holds hub.mu.
func (hub *Hub) queueHeldMessage(target *player, msgID int, message any) {
	data, err := json.Marshal(message)
	if err != nil {
		hub.logger.Error("encode message", "msgId", msgID, "error", err)
		return
	}
	hub.enqueue(target, outgoing{msgID: msgID, data: data, held: true})
}
