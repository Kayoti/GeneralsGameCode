// This file holds the chat rooms: joining, leaving and the member list (SERVICE.md section 6.3).

package realtime

import (
	"encoding/json"
	"sort"
)

// roomErrorUnknown is the text for a room change to a room that does not exist.
const roomErrorUnknown = "That room does not exist."

// roomExists reports whether a room with the id is configured.
func (hub *Hub) roomExists(roomID int) bool {
	return roomID >= 1 && roomID <= len(hub.rooms)
}

// memberList returns the players of a room, in user id order, as message 4
// entries. A roomNone has no members. It never returns nil, so the JSON array
// is always present. The caller holds hub.mu.
func (hub *Hub) memberList(roomID int) []memberEntry {
	members := make([]memberEntry, 0, len(hub.roomMembers[roomID]))
	for _, member := range hub.roomMembers[roomID] {
		members = append(members, memberEntry{UserID: member.userID, Name: member.name})
	}
	sort.Slice(members, func(first, second int) bool {
		return members[first].UserID < members[second].UserID
	})
	return members
}

// queueMessage encodes a message and puts it in the outbox of target. The
// caller holds hub.mu.
func (hub *Hub) queueMessage(target *player, msgID int, message any) {
	data, err := json.Marshal(message)
	if err != nil {
		hub.logger.Error("encode message", "msgId", msgID, "error", err)
		return
	}
	hub.enqueue(target, outgoing{msgID: msgID, data: data})
}

// broadcastMemberList sends the member list of a room to everyone in it
// except the player given, who may be nil. The message carries request id 0.
// The caller holds hub.mu.
func (hub *Hub) broadcastMemberList(roomID int, except *player) {
	message := memberListMessage{
		MsgID:           msgRoomMemberList,
		Members:         hub.memberList(roomID),
		SelectedRoomID:  roomID,
		EffectiveRoomID: roomID,
		RejectedRoomID:  roomNone,
	}
	for _, member := range hub.roomMembers[roomID] {
		if member != except {
			hub.queueMessage(member, msgRoomMemberList, message)
		}
	}
}

// leaveRoom takes a player out of the room they are in and sends the room's
// remaining players a new member list. The caller holds hub.mu.
func (hub *Hub) leaveRoom(target *player) {
	previous := target.roomID
	if previous == roomNone {
		return
	}
	delete(hub.roomMembers[previous], target.userID)
	target.roomID = roomNone
	hub.broadcastMemberList(previous, nil)
}

// changeRoom handles message 3: room -1 leaves the current room, a room id
// moves the player there. The answer (4) repeats the request id and goes to
// the player; everyone else in the new room gets the same list with request
// id 0. A room that does not exist is rejected in the answer and the player
// stays where they are.
func (hub *Hub) changeRoom(target *player, roomID int, requestID uint64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if roomID != roomNone && !hub.roomExists(roomID) {
		hub.queueMessage(target, msgRoomMemberList, memberListMessage{
			MsgID:              msgRoomMemberList,
			Members:            hub.memberList(target.roomID),
			SelectedRoomID:     roomID,
			EffectiveRoomID:    target.roomID,
			RejectedRoomID:     roomID,
			RoomSelectionError: roomErrorUnknown,
			RequestID:          requestID,
		})
		return
	}
	if roomID != target.roomID {
		hub.leaveRoom(target)
	}
	if roomID != roomNone && roomID != target.roomID {
		hub.roomMembers[roomID][target.userID] = target
		target.roomID = roomID
	}
	hub.broadcastMemberList(target.roomID, target)
	hub.queueMessage(target, msgRoomMemberList, memberListMessage{
		MsgID:           msgRoomMemberList,
		Members:         hub.memberList(target.roomID),
		SelectedRoomID:  roomID,
		EffectiveRoomID: target.roomID,
		RejectedRoomID:  roomNone,
		RequestID:       requestID,
	})
}
