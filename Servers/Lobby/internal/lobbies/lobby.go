// Package lobbies holds the lobby model of SERVICE.md section 7: the data of a
// lobby, its eight slots, and the rules for creating, joining, changing and
// leaving it. A Lobby does no locking and sends nothing; the caller guards it
// and sends the notices (message 6 and 7), so one lock covers the lobbies and
// the players together.
package lobbies

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"
)

// Slot states of CLIENT_HTTP.md 5.2.
const (
	SlotOpen     = 0
	SlotClosed   = 1
	SlotEasyAI   = 2
	SlotMediumAI = 3
	SlotBrutalAI = 4
	SlotHuman    = 5
)

// Sizes and limits (SERVICE.md section 7.1). The documents set no limit on
// the map name, the map path or the password; those three are the service's
// own, chosen to refuse absurd input.
const (
	// SlotCount is the number of member entries of every lobby.
	SlotCount = 8
	// MinPlayers and MaxPlayers bound a lobby's MaxPlayers.
	MinPlayers = 2
	MaxPlayers = SlotCount
	// MaxNameCharacters is the longest lobby name.
	MaxNameCharacters = 40

	maxMapNameCharacters  = 128
	maxMapPathCharacters  = 260
	maxPasswordCharacters = 128
)

// Values of the per-player choices. NoChoice is "random" or "none" in the client.
const (
	NoChoice     = -1
	SideObserver = -2
	// NoUser is the UserID of a slot that holds no human (CLIENT_HTTP.md 5.2).
	NoUser int64 = -1
)

// Joinability values of field 18 (CLIENT_HTTP.md 5.4).
const (
	joinabilityPublic      = 0
	joinabilityFriendsOnly = 1
)

// The errors Join returns. Each has its HTTP status in SERVICE.md section 7.2.
var (
	ErrLobbyNotFound = errors.New("the lobby does not exist or has started")
	ErrLobbyLocked   = errors.New("the lobby is locked by a countdown")
	ErrWrongPassword = errors.New("the password does not match")
	ErrLobbyFull     = errors.New("the lobby has no free slot")
)

// InvalidError says why a create request was refused.
type InvalidError struct {
	Reason string
}

func (invalid *InvalidError) Error() string {
	return "invalid lobby: " + invalid.Reason
}

// slot is one of the eight member entries of a lobby.
type slot struct {
	userID    int64
	state     int
	isReady   bool
	side      int
	color     int
	team      int
	startPos  int
	hasMap    bool
	joinOrder uint64
}

// emptySlot returns the entry of a slot that holds nobody, in the given state.
func emptySlot(state int) slot {
	return slot{userID: NoUser, state: state, side: NoChoice, color: NoChoice, team: NoChoice, startPos: NoChoice}
}

// isHuman reports whether the slot holds a player.
func (entry slot) isHuman() bool {
	return entry.state == SlotHuman
}

// isAI reports whether the slot holds a computer player.
func (entry slot) isAI() bool {
	return isAIState(entry.state)
}

// isAIState reports whether a slot state is one of the three computer players.
func isAIState(state int) bool {
	return state >= SlotEasyAI && state <= SlotBrutalAI
}

// Lobby is one lobby. It is not safe for concurrent use.
type Lobby struct {
	id                int64
	owner             int64
	name              string
	mapName           string
	mapPath           string
	isMapOfficial     bool
	maxPlayers        int
	vanillaTeamsOnly  bool
	startingCash      uint32
	limitSuperweapons bool
	tracksStats       bool
	allowObservers    bool
	maxCameraHeight   uint16
	exeCRC            uint32
	iniCRC            uint32
	matchID           uint64
	rngSeed           int
	joinability       int
	password          string
	slots             [SlotCount]slot
	joinCounter       uint64
	lockedUntil       time.Time
	hasStarted        bool
}

// NewLobby creates a lobby with the host in slot 0 (SERVICE.md section 7.2).
// It returns an *InvalidError when a field is not valid. rngSeed is the
// 31-bit seed chosen by the caller.
func NewLobby(id int64, hostID int64, request CreateRequest, rngSeed int) (*Lobby, error) {
	if request.Name == nil {
		return nil, &InvalidError{Reason: "name is missing"}
	}
	name := CleanText(*request.Name, MaxNameCharacters)
	if name == "" {
		return nil, &InvalidError{Reason: "name is empty"}
	}
	if request.MaxPlayers == nil || *request.MaxPlayers < MinPlayers || *request.MaxPlayers > MaxPlayers {
		return nil, &InvalidError{Reason: fmt.Sprintf("max_players must be %d to %d", MinPlayers, MaxPlayers)}
	}
	if request.MaxCamHeight < 0 || request.MaxCamHeight > math.MaxUint16 {
		return nil, &InvalidError{Reason: "max_cam_height is out of range"}
	}
	if utf8.RuneCountInString(request.Password) > maxPasswordCharacters {
		return nil, &InvalidError{Reason: "password is too long"}
	}
	lobby := &Lobby{
		id:               id,
		owner:            hostID,
		name:             name,
		mapName:          CleanText(request.MapName, maxMapNameCharacters),
		mapPath:          CleanText(request.MapPath, maxMapPathCharacters),
		isMapOfficial:    request.MapOfficial,
		vanillaTeamsOnly: request.VanillaTeams,
		startingCash:     request.StartingCash,
		tracksStats:      request.TrackStats,
		allowObservers:   request.AllowObservers,
		maxCameraHeight:  uint16(request.MaxCamHeight),
		exeCRC:           request.ExeCRC,
		iniCRC:           request.IniCRC,
		rngSeed:          rngSeed,
		joinability:      joinabilityPublic,
	}
	if request.Passworded {
		lobby.password = request.Password
	}
	for index := range lobby.slots {
		lobby.slots[index] = emptySlot(SlotClosed)
	}
	lobby.resizeSlots(*request.MaxPlayers)
	host := emptySlot(SlotHuman)
	host.userID = hostID
	host.isReady = true
	lobby.slots[0] = host
	lobby.joinCounter = 1
	return lobby, nil
}

// resizeSlots sets maxPlayers. Slots from maxPlayers up become closed, and a
// closed slot below it that lay above the old size becomes open. A slot that
// holds something is never changed; the callers have checked that none lies
// at or above maxPlayers.
func (lobby *Lobby) resizeSlots(maxPlayers int) {
	oldMax := lobby.maxPlayers
	for index := range lobby.slots {
		switch {
		case index >= maxPlayers:
			lobby.slots[index] = emptySlot(SlotClosed)
		case index >= oldMax && lobby.slots[index].state == SlotClosed:
			lobby.slots[index] = emptySlot(SlotOpen)
		}
	}
	lobby.maxPlayers = maxPlayers
}

// ID returns the lobby id.
func (lobby *Lobby) ID() int64 {
	return lobby.id
}

// Owner returns the host's user id.
func (lobby *Lobby) Owner() int64 {
	return lobby.owner
}

// HasStarted reports whether the match has started.
func (lobby *Lobby) HasStarted() bool {
	return lobby.hasStarted
}

// IsListed reports whether GET Lobbies shows the lobby: it has not started
// and is public (SERVICE.md section 5.1).
func (lobby *Lobby) IsListed() bool {
	return !lobby.hasStarted && lobby.joinability == joinabilityPublic
}

// IsLocked reports whether a countdown locks the lobby at now (SERVICE.md section 6.3).
func (lobby *Lobby) IsLocked(now time.Time) bool {
	return now.Before(lobby.lockedUntil)
}

// slotOf returns the slot index of a human member.
func (lobby *Lobby) slotOf(userID int64) (int, bool) {
	for index, entry := range lobby.slots {
		if entry.isHuman() && entry.userID == userID {
			return index, true
		}
	}
	return 0, false
}

// IsMember reports whether the user holds a human slot.
func (lobby *Lobby) IsMember(userID int64) bool {
	_, found := lobby.slotOf(userID)
	return found
}

// Humans returns the user ids of the human members in slot order.
func (lobby *Lobby) Humans() []int64 {
	humans := make([]int64, 0, SlotCount)
	for _, entry := range lobby.slots {
		if entry.isHuman() {
			humans = append(humans, entry.userID)
		}
	}
	return humans
}

// Join gives the user the lowest open slot. The checks are in the order of
// SERVICE.md section 7.2, and each failure is one of the Err values.
func (lobby *Lobby) Join(userID int64, request JoinRequest, now time.Time) error {
	if lobby.hasStarted {
		return ErrLobbyNotFound
	}
	if lobby.IsLocked(now) {
		return ErrLobbyLocked
	}
	if lobby.password != "" && subtle.ConstantTimeCompare([]byte(lobby.password), []byte(request.Password)) != 1 {
		return ErrWrongPassword
	}
	for index, entry := range lobby.slots {
		if entry.state != SlotOpen {
			continue
		}
		member := emptySlot(SlotHuman)
		member.userID = userID
		member.hasMap = request.HasMap
		member.joinOrder = lobby.joinCounter
		lobby.joinCounter++
		lobby.slots[index] = member
		return nil
	}
	return ErrLobbyFull
}

// Remove takes a human out of the lobby (SERVICE.md section 7.4). The slot
// becomes open. It returns true when no human is left, so the caller deletes
// the lobby. When the host left a lobby that has not started, the human who
// joined earliest becomes the owner and is ready. A user who is not a member
// changes nothing.
func (lobby *Lobby) Remove(userID int64) (isEmpty bool) {
	index, found := lobby.slotOf(userID)
	if !found {
		return false
	}
	lobby.slots[index] = emptySlot(SlotOpen)
	if len(lobby.Humans()) == 0 {
		return true
	}
	if userID == lobby.owner && !lobby.hasStarted {
		lobby.promoteEarliestMember()
	}
	return false
}

// promoteEarliestMember makes the human who joined first the owner, and ready.
// The lobby has at least one human.
func (lobby *Lobby) promoteEarliestMember() {
	earliest := -1
	for index, entry := range lobby.slots {
		if entry.isHuman() && (earliest < 0 || entry.joinOrder < lobby.slots[earliest].joinOrder) {
			earliest = index
		}
	}
	lobby.owner = lobby.slots[earliest].userID
	lobby.slots[earliest].isReady = true
}

// keepHostReady sets the host's ready state; the host is always ready (SERVICE.md section 7.1).
func (lobby *Lobby) keepHostReady() {
	if index, found := lobby.slotOf(lobby.owner); found {
		lobby.slots[index].isReady = true
	}
}

// clearReadyExceptHost clears the ready state of every member but the host.
func (lobby *Lobby) clearReadyExceptHost() {
	for index := range lobby.slots {
		lobby.slots[index].isReady = false
	}
	lobby.keepHostReady()
}

// SetReady sets the ready state of a member (message 5). It returns false,
// and changes nothing, for a user who is not a member and while a countdown
// locks the lobby. The host stays ready whatever it sends.
func (lobby *Lobby) SetReady(userID int64, isReady bool, now time.Time) bool {
	index, found := lobby.slotOf(userID)
	if !found || lobby.IsLocked(now) {
		return false
	}
	lobby.slots[index].isReady = isReady
	lobby.keepHostReady()
	return true
}

// StartCountdown locks the lobby for lockDuration from now (message 23). It
// returns false unless the user is the host of a lobby that has not started.
func (lobby *Lobby) StartCountdown(userID int64, now time.Time, lockDuration time.Duration) bool {
	if userID != lobby.owner || lobby.hasStarted || !lobby.IsMember(userID) {
		return false
	}
	lobby.lockedUntil = now.Add(lockDuration)
	return true
}

// EndCountdown ends the lock early (SERVICE.md section 6.3: a new message 26).
// It returns false unless the user is the host.
func (lobby *Lobby) EndCountdown(userID int64) bool {
	if userID != lobby.owner || !lobby.IsMember(userID) {
		return false
	}
	lobby.lockedUntil = time.Time{}
	return true
}

// Start marks the lobby as started with the given match id (SERVICE.md
// section 7.5 steps 1 and 2). It returns false, and changes nothing, unless
// the user is the host, the lobby has not started and every human is ready.
func (lobby *Lobby) Start(userID int64, matchID uint64) bool {
	if userID != lobby.owner || lobby.hasStarted || !lobby.IsMember(userID) {
		return false
	}
	for _, entry := range lobby.slots {
		if entry.isHuman() && !entry.isReady {
			return false
		}
	}
	lobby.matchID = matchID
	lobby.hasStarted = true
	lobby.lockedUntil = time.Time{}
	return true
}

// SetPassword sets the password, or removes it when password is empty
// (messages 24 and 25). It returns false unless the user is the host and the
// password is not too long.
func (lobby *Lobby) SetPassword(userID int64, password string) bool {
	if userID != lobby.owner || !lobby.IsMember(userID) || utf8.RuneCountInString(password) > maxPasswordCharacters {
		return false
	}
	lobby.password = password
	return true
}

// ChatTargets returns who receives a lobby chat line from the sender
// (SERVICE.md section 6.3, messages 10 and 11) and whether it goes out as an
// announcement. The announcement flag is kept only for the host. An
// announcement reaches the host only when showToHost is true. ok is false when
// the sender is not a member.
func (lobby *Lobby) ChatTargets(senderID int64, announcement bool, showToHost bool) (targets []int64, isAnnouncement bool, ok bool) {
	if !lobby.IsMember(senderID) {
		return nil, false, false
	}
	isAnnouncement = announcement && senderID == lobby.owner
	for _, userID := range lobby.Humans() {
		if isAnnouncement && userID == lobby.owner && !showToHost {
			continue
		}
		targets = append(targets, userID)
	}
	return targets, isAnnouncement, true
}
