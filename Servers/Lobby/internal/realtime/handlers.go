// This file holds what the service does with each message a client sends
// (SERVICE.md section 6.3). Every message is hostile input: its type and
// range are checked here, and the sender is the connection's player, never a
// value from the message.

package realtime

import (
	"encoding/json"
	"errors"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/lobbies"
)

// nameRefusalPrefix starts the chat line that tells a player their name change was refused.
const nameRefusalPrefix = "Name not changed: "

// nameSaveFailure is the reason given when the name could not be stored.
const nameSaveFailure = "the service could not save it, try again later"

// handleMessage reads one message from a client and does what its msg_id asks.
// A message that is not a JSON object with an integer msg_id, and a msg_id
// the service does not handle, are ignored and logged at debug level.
func (hub *Hub) handleMessage(live *connection, data []byte) {
	var envelope inboundEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.MsgID == nil {
		hub.logger.Debug("ignored a message with no msg_id", "userId", live.player.userID, "bytes", len(data))
		return
	}
	switch *envelope.MsgID {
	case msgPing:
		hub.answerPing(live)
	case msgRoomChange:
		hub.handleRoomChange(live, data)
	case msgRoomChatFromClient:
		hub.handleRoomChat(live, data)
	case msgPlayerNameChange:
		hub.handleNameChange(live, data)
	case msgMarkReady:
		hub.handleMarkReady(live, data)
	case msgLobbyChatFromClient:
		hub.handleLobbyChat(live, data)
	case msgStartGame:
		hub.handleStartGame(live)
	case msgCountdownStarted:
		hub.handleCountdownStarted(live)
	case msgRemovePassword:
		hub.handleRemovePassword(live)
	case msgChangePassword:
		hub.handleChangePassword(live, data)
	case msgConnectivityCheck:
		hub.handleConnectivityCheck(live)
	default:
		hub.logger.Debug("ignored a message", "userId", live.player.userID, "msgId", *envelope.MsgID)
	}
}

// answerPing answers message 14 with message 15.
func (hub *Hub) answerPing(live *connection) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.queueMessage(live.player, msgPong, noticeMessage{MsgID: msgPong})
}

// handleRoomChange handles message 3. A message without an integer room is ignored.
func (hub *Hub) handleRoomChange(live *connection, data []byte) {
	var request roomChangeRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Room == nil {
		hub.logger.Debug("ignored a room change with no valid room", "userId", live.player.userID)
		return
	}
	hub.changeRoom(live.player, *request.Room, request.RequestID)
}

// handleRoomChat handles message 1: the text goes to everyone in the sender's
// room, the sender included. A sender who is in no room, and text that is
// empty after cleaning, are ignored.
func (hub *Hub) handleRoomChat(live *connection, data []byte) {
	var request roomChatRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Message == nil {
		hub.logger.Debug("ignored a room chat with no valid message", "userId", live.player.userID)
		return
	}
	text := lobbies.CleanText(*request.Message, maxChatCharacters)
	if text == "" {
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sender := live.player
	if sender.roomID == roomNone {
		return
	}
	line := roomChatMessage{
		MsgID:   msgRoomChatFromServer,
		Message: formatChatLine(sender.name, text, request.Action),
		Action:  request.Action,
	}
	for _, member := range hub.roomMembers[sender.roomID] {
		hub.queueMessage(member, msgRoomChatFromServer, line)
	}
}

// formatChatLine puts the sender's name in front of the text, because message
// 2 carries only one string (CLIENT_REALTIME.md 1.7). An action reads "name text".
func formatChatLine(name string, text string, isAction bool) string {
	if isAction {
		return name + " " + text
	}
	return name + ": " + text
}

// handleNameChange handles message 9. The rules and the storing are in the
// accounts package. On success the room gets a new member list; a refused name
// is answered with one room chat line to the sender only.
func (hub *Hub) handleNameChange(live *connection, data []byte) {
	var request nameChangeRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Name == nil {
		hub.logger.Debug("ignored a name change with no valid name", "userId", live.player.userID)
		return
	}
	sender := live.player
	err := hub.accounts.ChangeDisplayName(sender.userID, *request.Name)
	var refusal *accounts.NameRefusal
	switch {
	case errors.As(err, &refusal):
		hub.sendChatLineTo(sender, nameRefusalPrefix+refusal.Reason)
	case err != nil:
		hub.logger.Error("change name", "userId", sender.userID, "error", err)
		hub.sendChatLineTo(sender, nameRefusalPrefix+nameSaveFailure)
	default:
		hub.applyNewName(sender, *request.Name)
	}
}

// applyNewName records a stored name on the player, sends the member list of
// the player's room, and tells the members of the player's lobby with 6 and
// the list watchers with 7 (SERVICE.md section 6.3).
func (hub *Hub) applyNewName(target *player, name string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	target.name = name
	if target.roomID != roomNone {
		hub.broadcastMemberList(target.roomID, nil)
	}
	if lobby := hub.lobbyOf(target); lobby != nil {
		hub.lobbyChanged(lobby, lobby.IsListed())
	}
}

// sendChatLineTo sends a room chat line from the service to one player.
func (hub *Hub) sendChatLineTo(target *player, text string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.queueMessage(target, msgRoomChatFromServer, roomChatMessage{MsgID: msgRoomChatFromServer, Message: text})
}
