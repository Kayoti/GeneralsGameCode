// This file holds what the Hub does with lobbies: the calls behind the HTTP
// lobby endpoints and the notices that follow every change (SERVICE.md
// sections 5.1, 6.3 and 7). The lobby rules themselves are in the lobbies
// package; here they run under hub.mu together with the players.

package realtime

import (
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"servers-lobby/internal/lobbies"
)

// noLobby is the lobby id of a player who is in no lobby. Lobby ids start at 1 (SERVICE.md section 3).
const noLobby int64 = 0

// firstLobbyID and firstMatchID are the first ids handed out (SERVICE.md section 3).
const (
	firstLobbyID int64  = 1
	firstMatchID uint64 = 1
)

// rngSeedLimit is one more than the largest lobby RNGSeed, a 31-bit number (SERVICE.md section 7.1).
const rngSeedLimit = math.MaxInt32

// ErrNotConnected is returned for an account that has no WebSocket place on
// the hub. A lobby member is always a player of the hub, because the place
// decides when the member is removed (SERVICE.md section 7.4).
var ErrNotConnected = errors.New("the account has no WebSocket connection")

// newRNGSeed returns a random 31-bit number for a lobby (SERVICE.md section 7.1).
func newRNGSeed() int {
	return int(rand.Int32N(rngSeedLimit))
}

// nameOf returns the display name of a player, or empty for an unknown id.
// The caller holds hub.mu.
func (hub *Hub) nameOf(userID int64) string {
	if found, ok := hub.players[userID]; ok {
		return found.name
	}
	return ""
}

// lobbyOf returns the lobby the player is in, or nil. The caller holds hub.mu.
func (hub *Hub) lobbyOf(target *player) *lobbies.Lobby {
	if target.lobbyID == noLobby {
		return nil
	}
	return hub.lobbies[target.lobbyID]
}

// notifyMembers sends message 6 to every human member of the lobby. The caller holds hub.mu.
func (hub *Hub) notifyMembers(lobby *lobbies.Lobby) {
	for _, userID := range lobby.Humans() {
		if member, found := hub.players[userID]; found {
			hub.notifyLocked(member, MsgLobbyCurrentLobbyUpdate)
		}
	}
}

// notifyWatchers sends message 7 to everyone who is in a room and not in a
// lobby, which is who looks at the list (SERVICE.md section 6.3). The caller holds hub.mu.
func (hub *Hub) notifyWatchers() {
	for _, watcher := range hub.players {
		if watcher.roomID != roomNone && watcher.lobbyID == noLobby {
			hub.notifyLocked(watcher, MsgLobbyListUpdate)
		}
	}
}

// lobbyChanged sends the notices that follow a change: 6 to the members, and
// 7 to the watchers when the lobby is on the list now or was before the
// change. The caller holds hub.mu.
func (hub *Hub) lobbyChanged(lobby *lobbies.Lobby, wasListed bool) {
	hub.notifyMembers(lobby)
	if wasListed || lobby.IsListed() {
		hub.notifyWatchers()
	}
}

// leaveLobby takes a player out of their lobby as SERVICE.md section 7.4
// says: the slot opens, an empty lobby is deleted, the remaining members get
// 6 and the watchers get 7. The caller holds hub.mu.
func (hub *Hub) leaveLobby(target *player) {
	lobby := hub.lobbyOf(target)
	target.lobbyID = noLobby
	if lobby == nil {
		return
	}
	wasListed := lobby.IsListed()
	if lobby.Remove(target.userID) {
		delete(hub.lobbies, lobby.ID())
	} else {
		hub.notifyMembers(lobby)
	}
	if wasListed {
		hub.notifyWatchers()
	}
}

// CreateLobby creates a lobby with the account as host and returns its id
// (PUT Lobbies). An account already in a lobby leaves it first. It returns
// ErrNotConnected for an account with no WebSocket and an *lobbies.InvalidError
// for a request that is not valid; neither changes anything.
func (hub *Hub) CreateLobby(userID int64, request lobbies.CreateRequest) (int64, error) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	host, found := hub.players[userID]
	if !found {
		return noLobby, ErrNotConnected
	}
	lobby, err := lobbies.NewLobby(hub.nextLobbyID, userID, request, newRNGSeed())
	if err != nil {
		return noLobby, err
	}
	hub.leaveLobby(host)
	hub.nextLobbyID++
	hub.lobbies[lobby.ID()] = lobby
	host.lobbyID = lobby.ID()
	hub.notifyWatchers()
	return lobby.ID(), nil
}

// ListLobbies returns the lobbies that are public and have not started, in id order (GET Lobbies).
func (hub *Hub) ListLobbies() []lobbies.ListLobby {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	listed := make([]lobbies.ListLobby, 0, len(hub.lobbies))
	for _, lobby := range hub.lobbies {
		if lobby.IsListed() {
			listed = append(listed, lobby.ListEntry(hub.nameOf))
		}
	}
	sort.Slice(listed, func(first, second int) bool {
		return listed[first].LobbyID < listed[second].LobbyID
	})
	return listed
}

// LobbyDetail returns a lobby to one of its members (GET Lobby/{id}). The
// bool is false when there is no such lobby or the account is not in it.
func (hub *Hub) LobbyDetail(userID int64, lobbyID int64) (lobbies.DetailLobby, bool) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	lobby, found := hub.lobbies[lobbyID]
	if !found || !lobby.IsMember(userID) {
		return lobbies.DetailLobby{}, false
	}
	return lobby.DetailEntry(hub.nameOf), true
}

// JoinLobby puts the account in a lobby (PUT Lobby/{id}). It returns one of
// the lobbies.Err values for the failures of SERVICE.md section 7.2, or
// ErrNotConnected. An account already in a lobby leaves it when the join
// succeeds; one already in this lobby changes nothing.
func (hub *Hub) JoinLobby(userID int64, lobbyID int64, request lobbies.JoinRequest) error {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	joiner, found := hub.players[userID]
	if !found {
		return ErrNotConnected
	}
	lobby, found := hub.lobbies[lobbyID]
	if !found {
		return lobbies.ErrLobbyNotFound
	}
	if lobby.IsMember(userID) {
		return nil
	}
	wasListed := lobby.IsListed()
	if err := lobby.Join(userID, request, time.Now()); err != nil {
		return err
	}
	hub.leaveLobby(joiner)
	joiner.lobbyID = lobbyID
	hub.lobbyChanged(lobby, wasListed)
	return nil
}

// ChangeLobby applies one change of POST Lobby/{id}. A refused change has no
// effect and sends nothing. A kick removes the member as a leave does, and
// the kicked player gets 6 so that its client finds itself missing.
func (hub *Hub) ChangeLobby(userID int64, lobbyID int64, request lobbies.FieldRequest) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	lobby, found := hub.lobbies[lobbyID]
	if !found {
		return
	}
	wasListed := lobby.IsListed()
	result, kickedID := lobby.ApplyChange(userID, request, time.Now())
	switch result {
	case lobbies.ChangeApplied:
		hub.lobbyChanged(lobby, wasListed)
	case lobbies.ChangeKick:
		hub.kickFromLobby(kickedID)
	}
}

// kickFromLobby removes a member on the host's order and tells the member's
// client with message 6. The caller holds hub.mu.
func (hub *Hub) kickFromLobby(userID int64) {
	kicked, found := hub.players[userID]
	if !found {
		return
	}
	hub.notifyLocked(kicked, MsgLobbyCurrentLobbyUpdate)
	hub.leaveLobby(kicked)
}

// LeaveLobby takes the account out of the lobby (DELETE Lobby/{id}). An
// account that is not in that lobby changes nothing.
func (hub *Hub) LeaveLobby(userID int64, lobbyID int64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	leaver, found := hub.players[userID]
	if !found || leaver.lobbyID != lobbyID {
		return
	}
	hub.leaveLobby(leaver)
}
