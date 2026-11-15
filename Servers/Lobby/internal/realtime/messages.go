// This file holds the WebSocket message ids and the shapes of the messages
// this package reads and writes (CLIENT_REALTIME.md section 1.7, SERVICE.md section 6.3).

package realtime

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Message ids of CLIENT_REALTIME.md section 1.7. The two notices are exported
// because the lobby code sends them through Hub.Notify.
const (
	msgRoomChatFromClient = 1
	msgRoomChatFromServer = 2
	msgRoomChange         = 3
	msgRoomMemberList     = 4
	msgSignal             = 12
	msgPlayerNameChange   = 9
	msgPing               = 14
	msgPong               = 15

	// MsgLobbyCurrentLobbyUpdate tells a lobby member to fetch the lobby again.
	MsgLobbyCurrentLobbyUpdate = 6
	// MsgLobbyListUpdate tells a client in a room to fetch the lobby list again.
	MsgLobbyListUpdate = 7
)

// roomNone is the room id on the wire for "no room" (message 3, room -1).
const roomNone = -1

// maxChatCharacters is the longest room chat text; more is cut off (SERVICE.md section 6.3).
const maxChatCharacters = 300

// inboundEnvelope reads the msg_id every message carries. A message without
// an integer msg_id is ignored.
type inboundEnvelope struct {
	MsgID *int `json:"msg_id"`
}

// roomChangeRequest is message 3. Room is a pointer so that a missing key is
// told from room 0.
type roomChangeRequest struct {
	Room      *int   `json:"room"`
	RequestID uint64 `json:"request_id"`
}

// roomChatRequest is message 1.
type roomChatRequest struct {
	Message *string `json:"message"`
	Action  bool    `json:"action"`
}

// nameChangeRequest is message 9.
type nameChangeRequest struct {
	Name *string `json:"name"`
}

// noticeMessage is a message that has no fields besides its id: 6, 7 and 15.
type noticeMessage struct {
	MsgID int `json:"msg_id"`
}

// roomChatMessage is message 2 with every key the client reads.
type roomChatMessage struct {
	MsgID      int    `json:"msg_id"`
	Message    string `json:"message"`
	Action     bool   `json:"action"`
	Admin      bool   `json:"admin"`
	NameChange bool   `json:"name_change"`
}

// memberEntry is one player of message 4. The key spelling is the client's.
type memberEntry struct {
	UserID  int64  `json:"UserID"`
	Name    string `json:"Name"`
	IsAdmin bool   `json:"IsAdmin"`
}

// memberListMessage is message 4 with every key the client reads. The
// room ids are on the wire form: roomNone when there is no room.
type memberListMessage struct {
	MsgID              int           `json:"msg_id"`
	Members            []memberEntry `json:"members"`
	SelectedRoomID     int           `json:"selected_room_id"`
	EffectiveRoomID    int           `json:"effective_room_id"`
	RejectedRoomID     int           `json:"rejected_room_id"`
	RoomSelectionError string        `json:"room_selection_error"`
	RequestID          uint64        `json:"request_id"`
}

// cleanText prepares player text (SERVICE.md section 3): control characters
// are removed, the ends are trimmed, and at most maxCharacters characters are
// kept. Invalid UTF-8 has already been replaced when the JSON was decoded.
func cleanText(text string, maxCharacters int) string {
	withoutControls := strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, text)
	trimmed := strings.TrimSpace(withoutControls)
	if utf8.RuneCountInString(trimmed) <= maxCharacters {
		return trimmed
	}
	kept := []rune(trimmed)[:maxCharacters]
	return string(kept)
}
