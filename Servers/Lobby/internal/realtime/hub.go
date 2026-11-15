// Package realtime holds the WebSocket side of the lobby service (SERVICE.md
// section 6): the live connection of each logged-in account, the place it
// keeps for 45 seconds after a drop, the chat rooms and the notices to a client.
//
// All state shared between connections is guarded by Hub.mu. Nothing is
// written to a socket while the lock is held: messages are put in the
// player's outbox and each connection has its own goroutine that writes them.
package realtime

import (
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Defaults of Settings (SERVICE.md sections 6.1 and 6.2).
const (
	defaultIdleTimeout   = 30 * time.Second
	defaultReconnectHold = 45 * time.Second
	// defaultWriteTimeout is not set by the documents. It stops a client that
	// does not read from holding a writer goroutine for ever.
	defaultWriteTimeout = 10 * time.Second
)

// maxOutboxMessages is how many messages may wait for one client. A client
// that falls further behind is dropped. The documents do not set it.
const maxOutboxMessages = 512

// Settings holds the values of a Hub. The timeouts are fields so that tests can shorten them.
type Settings struct {
	// Rooms are the room names; the ids are 1, 2, ... in this order (SERVICE.md section 5.1).
	Rooms []string
	// IdleTimeout drops a connection that sends nothing for this long.
	IdleTimeout time.Duration
	// ReconnectHold is how long a dropped player's place is kept.
	ReconnectHold time.Duration
	// WriteTimeout limits one write to a client.
	WriteTimeout time.Duration
}

// DefaultSettings returns the settings of SERVICE.md with the given room names.
func DefaultSettings(rooms []string) Settings {
	return Settings{
		Rooms:         rooms,
		IdleTimeout:   defaultIdleTimeout,
		ReconnectHold: defaultReconnectHold,
		WriteTimeout:  defaultWriteTimeout,
	}
}

// Accounts is what the Hub needs from the account rules. *accounts.Service implements it.
type Accounts interface {
	// DisplayName returns the stored name of an account; the bool is false when there is none.
	DisplayName(userID int64) (string, bool, error)
	// ChangeDisplayName sets the name after checking the rules of SERVICE.md section 4.
	ChangeDisplayName(userID int64, name string) error
}

// Room is a chat room that a client can be in (CLIENT_HTTP.md 4.11).
type Room struct {
	ID   int
	Name string
}

// outgoing is a message waiting to be written to a client. A notice is a
// message with no fields: it is kept while the player is disconnected, and
// only one identical notice waits at a time (SERVICE.md section 6.1).
type outgoing struct {
	msgID  int
	data   []byte
	notice bool
}

// player is an account that has a place on the service: the live connection,
// or the place held for it after a drop. Every field is guarded by Hub.mu.
type player struct {
	userID int64
	name   string
	roomID int
	conn   *connection
	outbox []outgoing

	holdTimer      *time.Timer
	holdGeneration uint64
}

// Hub holds the players and the rooms. It is safe for concurrent use.
type Hub struct {
	settings Settings
	accounts Accounts
	logger   *slog.Logger
	rooms    []Room

	mu          sync.Mutex
	players     map[int64]*player
	roomMembers map[int]map[int64]*player
}

// NewHub returns a Hub with the given settings. Room ids are given to
// settings.Rooms in order, starting at 1.
func NewHub(settings Settings, accounts Accounts, logger *slog.Logger) *Hub {
	rooms := make([]Room, 0, len(settings.Rooms))
	roomMembers := make(map[int]map[int64]*player, len(settings.Rooms))
	for index, name := range settings.Rooms {
		id := index + 1
		rooms = append(rooms, Room{ID: id, Name: name})
		roomMembers[id] = make(map[int64]*player)
	}
	return &Hub{
		settings:    settings,
		accounts:    accounts,
		logger:      logger,
		rooms:       rooms,
		players:     make(map[int64]*player),
		roomMembers: roomMembers,
	}
}

// Rooms returns the rooms in id order.
func (hub *Hub) Rooms() []Room {
	return append([]Room(nil), hub.rooms...)
}

// Notify queues a message that has no fields besides msg_id, such as
// MsgLobbyCurrentLobbyUpdate or MsgLobbyListUpdate, for a player. When an
// identical notice is already waiting for that player, nothing is queued, so
// the client never gets the same notice twice in a row (SERVICE.md section
// 6.1). A player who is disconnected keeps the notice until the place is
// released. An unknown player is ignored.
func (hub *Hub) Notify(userID int64, msgID int) {
	data, err := json.Marshal(noticeMessage{MsgID: msgID})
	if err != nil {
		hub.logger.Error("encode notice", "msgId", msgID, "error", err)
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	target, found := hub.players[userID]
	if !found {
		return
	}
	hub.enqueue(target, outgoing{msgID: msgID, data: data, notice: true})
}

// enqueue puts a message in a player's outbox and wakes the writer. A
// message that is not a notice is dropped while the player has no connection.
// A client whose outbox is full is dropped. The caller holds hub.mu.
func (hub *Hub) enqueue(target *player, message outgoing) {
	if target.conn == nil && !message.notice {
		return
	}
	if message.notice && hasWaitingNotice(target.outbox, message.msgID) {
		return
	}
	if len(target.outbox) >= maxOutboxMessages {
		hub.logger.Warn("client is too far behind", "userId", target.userID)
		if target.conn != nil {
			target.conn.drop()
		}
		return
	}
	target.outbox = append(target.outbox, message)
	if target.conn != nil {
		target.conn.wakeWriter()
	}
}

// hasWaitingNotice reports whether the outbox already holds the notice msgID.
func hasWaitingNotice(outbox []outgoing, msgID int) bool {
	for _, waiting := range outbox {
		if waiting.notice && waiting.msgID == msgID {
			return true
		}
	}
	return false
}

// keepOnlyNotices removes from the outbox every message that is not a notice.
// A message written for a connection that has ended is not sent to the next
// one (SERVICE.md section 6.2 keeps only notices). The caller holds hub.mu.
func keepOnlyNotices(target *player) {
	kept := target.outbox[:0]
	for _, waiting := range target.outbox {
		if waiting.notice {
			kept = append(kept, waiting)
		}
	}
	target.outbox = kept
}

// sortNotices puts the waiting notices in id order, so a reconnecting client
// gets 6 before 7 (SERVICE.md section 6.2). The caller holds hub.mu.
func sortNotices(target *player) {
	sort.SliceStable(target.outbox, func(first, second int) bool {
		return target.outbox[first].msgID < target.outbox[second].msgID
	})
}
