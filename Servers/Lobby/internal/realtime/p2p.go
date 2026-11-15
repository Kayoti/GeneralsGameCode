// This file holds the player-to-player connection setup of an Online lobby
// (SERVICE.md section 8.2): telling two players to connect, relaying their
// signals, and telling the others when a player leaves. The service never
// reads a signal; it only passes it on between two members of one lobby.

package realtime

import (
	"strconv"

	"servers-lobby/internal/lobbies"
)

// signalPrefix, signalMiddle and signalSuffix are the parts of message 12
// around the two values that change. The payload goes between the last two
// unchanged, so it is copied once, from the message that arrived to the
// outbox of the target (CLIENT_REALTIME.md 1.7 row 12).
const (
	signalPrefix = `{"msg_id":12,"target_user_id":`
	signalMiddle = `,"payload":`
	signalSuffix = `}`
)

// signalOverheadBytes is the room the fixed parts of message 12 and a user id need.
const signalOverheadBytes = len(signalPrefix) + len(signalMiddle) + len(signalSuffix) + 20

// sendStartSignalling queues message 17 to receiver, telling it to connect to
// peerID. The caller holds hub.mu.
func (hub *Hub) sendStartSignalling(receiver *player, lobbyID int64, peerID int64) {
	hub.queueMessage(receiver, msgStartSignalling, startSignallingMessage{
		MsgID:   msgStartSignalling,
		LobbyID: lobbyID,
		UserID:  peerID,
	})
}

// startPairSignalling sends 17 to both players of a pair: to first with the
// id of second, then to second with the id of first (SERVICE.md section 8.2).
// The caller holds hub.mu.
func (hub *Hub) startPairSignalling(lobbyID int64, first *player, second *player) {
	hub.sendStartSignalling(first, lobbyID, second.userID)
	hub.sendStartSignalling(second, lobbyID, first.userID)
}

// startJoinerSignalling starts the connection of a new member to every other
// human of the lobby. It runs after message 6 has been queued. The caller holds hub.mu.
func (hub *Hub) startJoinerSignalling(lobbyID int64, joiner *player, humans []int64) {
	for _, userID := range humans {
		if userID == joiner.userID {
			continue
		}
		if member, found := hub.players[userID]; found {
			hub.startPairSignalling(lobbyID, joiner, member)
		}
	}
}

// disconnectFromLeaver sends message 18 for the player who left to every
// human still in the lobby. The caller holds hub.mu.
func (hub *Hub) disconnectFromLeaver(lobbyID int64, leaverID int64, humans []int64) {
	for _, userID := range humans {
		if member, found := hub.players[userID]; found {
			hub.queueMessage(member, msgDisconnectPlayer, disconnectPlayerMessage{
				MsgID:   msgDisconnectPlayer,
				LobbyID: lobbyID,
				UserID:  leaverID,
			})
		}
	}
}

// lobbyMate returns the sender's lobby and the player with id targetID when
// that player is another human member of the same lobby. The id comes from
// the client, so both are checked as members. The caller holds hub.mu.
func (hub *Hub) lobbyMate(sender *player, targetID int64) (*lobbies.Lobby, *player, bool) {
	lobby := hub.lobbyOf(sender)
	target, found := hub.players[targetID]
	if !found || lobby == nil || target == sender || !lobby.IsMember(sender.userID) || !lobby.IsMember(target.userID) {
		return nil, nil, false
	}
	return lobby, target, true
}

// handleSignal handles message 12: the payload goes to target_user_id, only
// when sender and target are two different members of one lobby, with
// target_user_id changed to the sender. Anything else is dropped. A target
// whose connection is down, or whose outbox is full, misses the signal; the
// connection library sends it again.
func (hub *Hub) handleSignal(live *connection, envelope inboundEnvelope) {
	if envelope.TargetUserID == nil || len(envelope.Payload) == 0 || envelope.Payload[0] != '[' {
		hub.logger.Debug("ignored a signal with no valid target or payload", "userId", live.player.userID)
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sender := live.player
	_, target, ok := hub.lobbyMate(sender, *envelope.TargetUserID)
	if !ok {
		hub.logger.Debug("dropped a signal to a player outside the lobby", "userId", sender.userID)
		return
	}
	if len(target.outbox) >= maxOutboxMessages {
		hub.logger.Debug("dropped a signal for a client that is behind", "userId", target.userID)
		return
	}
	data := make([]byte, 0, len(envelope.Payload)+signalOverheadBytes)
	data = append(data, signalPrefix...)
	data = strconv.AppendInt(data, sender.userID, 10)
	data = append(data, signalMiddle...)
	data = append(data, envelope.Payload...)
	data = append(data, signalSuffix...)
	hub.enqueue(target, outgoing{msgID: msgSignal, data: data})
}

// handleRequestSignalling handles message 19: a member asks to connect to
// another again, so both are told with 17, when both are in the same lobby.
func (hub *Hub) handleRequestSignalling(live *connection, envelope inboundEnvelope) {
	if envelope.TargetUserID == nil {
		hub.logger.Debug("ignored a signalling request with no target", "userId", live.player.userID)
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sender := live.player
	lobby, target, ok := hub.lobbyMate(sender, *envelope.TargetUserID)
	if !ok {
		hub.logger.Debug("ignored a signalling request for a player outside the lobby", "userId", sender.userID)
		return
	}
	hub.startPairSignalling(lobby.ID(), sender, target)
}
