// This file holds the WebSocket message ids and the shapes of the messages
// this package reads and writes (CLIENT_REALTIME.md section 1.7, SERVICE.md section 6.3).

package realtime

// Message ids of CLIENT_REALTIME.md section 1.7. The two notices are exported
// because the lobby code sends them through Hub.Notify.
const (
	msgRoomChatFromClient  = 1
	msgRoomChatFromServer  = 2
	msgRoomChange          = 3
	msgRoomMemberList      = 4
	msgMarkReady           = 5
	msgPlayerNameChange    = 9
	msgLobbyChatFromClient = 10
	msgLobbyChatFromServer = 11
	msgSignal              = 12
	msgStartGame           = 13
	msgPing                = 14
	msgPong                = 15
	msgCountdownStarted    = 23
	msgRemovePassword      = 24
	msgChangePassword      = 25
	msgConnectivityCheck   = 26

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

// markReadyRequest is message 5. Ready is a pointer so that a missing key is told from false.
type markReadyRequest struct {
	Ready *bool `json:"ready"`
}

// lobbyChatRequest is message 10.
type lobbyChatRequest struct {
	Message                *string `json:"message"`
	Action                 bool    `json:"action"`
	Announcement           bool    `json:"announcement"`
	ShowAnnouncementToHost bool    `json:"show_announcement_to_host"`
}

// lobbyChatMessage is message 11 with every key the client reads.
type lobbyChatMessage struct {
	MsgID                  int    `json:"msg_id"`
	Message                string `json:"message"`
	Action                 bool   `json:"action"`
	Announcement           bool   `json:"announcement"`
	ShowAnnouncementToHost bool   `json:"show_announcement_to_host"`
	UserID                 int64  `json:"user_id"`
}

// changePasswordRequest is message 25.
type changePasswordRequest struct {
	NewPassword *string `json:"new_password"`
}

// startGameMessage is message 13 from the service (CLIENT_REALTIME.md 1.7). The
// screenshot address is always empty (SERVICE.md section 7.5).
type startGameMessage struct {
	MsgID         int    `json:"msg_id"`
	ScreenshotURL string `json:"screenshot_url"`
}
