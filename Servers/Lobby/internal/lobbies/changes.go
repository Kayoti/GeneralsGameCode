// This file holds POST Lobby/{id}: the changes a member or the host makes to a
// lobby, with the "who" rule, the range checks and the ready-state rules of
// SERVICE.md section 7.3.

package lobbies

import (
	"math"
	"time"
)

// Values of ELobbyUpdateField (CLIENT_HTTP.md 5.4). Fields 7, 9 and 10 have no
// effect and are not listed. Field 19 belongs to server lobbies.
const (
	FieldMap               = 0
	FieldMySide            = 1
	FieldMyColor           = 2
	FieldMyStartPos        = 3
	FieldMyTeam            = 4
	FieldStartingCash      = 5
	FieldLimitSuperweapons = 6
	FieldMyHasMap          = 8
	FieldKickUser          = 11
	FieldSetSlotState      = 12
	FieldAISide            = 13
	FieldAIColor           = 14
	FieldAITeam            = 15
	FieldAIStartPos        = 16
	FieldMaxCameraHeight   = 17
	FieldJoinability       = 18
)

// The documents give no upper bound for these choices. They follow from the
// eight slots, plus a generous bound for the number of player templates, and
// refuse absurd values.
const (
	maxSideIndex  = 63
	maxColorIndex = SlotCount - 1
	maxTeamIndex  = SlotCount - 1
)

// ChangeResult is what ApplyChange did.
type ChangeResult int

const (
	// ChangeRefused means the change had no effect.
	ChangeRefused ChangeResult = iota
	// ChangeApplied means the lobby changed; the caller sends message 6.
	ChangeApplied
	// ChangeKick means the host asked to remove a member; the caller removes
	// the user ApplyChange returned, as for a leave.
	ChangeKick
)

// ApplyChange applies one POST Lobby/{id} from the user (SERVICE.md section
// 7.3). Fields 1 to 4 and 8 are for the sender's own slot and are refused
// while a countdown locks the lobby; the others are for the host. Unknown
// fields and field 7 are refused. For ChangeKick the second result is the
// member to remove.
func (lobby *Lobby) ApplyChange(userID int64, request FieldRequest, now time.Time) (ChangeResult, int64) {
	index, isMember := lobby.slotOf(userID)
	if !isMember || request.Field == nil {
		return ChangeRefused, NoUser
	}
	switch *request.Field {
	case FieldMySide, FieldMyColor, FieldMyStartPos, FieldMyTeam, FieldMyHasMap:
		if lobby.IsLocked(now) {
			return ChangeRefused, NoUser
		}
		return resultOf(lobby.applyOwnField(index, request)), NoUser
	}
	if userID != lobby.owner {
		return ChangeRefused, NoUser
	}
	if *request.Field == FieldKickUser {
		return lobby.kickTarget(request)
	}
	return resultOf(lobby.applyHostField(request)), NoUser
}

// resultOf maps an accepted flag to ChangeApplied or ChangeRefused.
func resultOf(isApplied bool) ChangeResult {
	if isApplied {
		return ChangeApplied
	}
	return ChangeRefused
}

// clearOwnReady clears one member's ready state. The host stays ready.
func (lobby *Lobby) clearOwnReady(index int) {
	lobby.slots[index].isReady = false
	lobby.keepHostReady()
}

// applyOwnField handles fields 1, 2, 3, 4 and 8 for the member in slot index.
// A change of side, colour, position or team clears the member's ready state;
// so does field 8 with has_map false.
func (lobby *Lobby) applyOwnField(index int, request FieldRequest) bool {
	switch *request.Field {
	case FieldMySide:
		if request.Side == nil || request.StartPos == nil {
			return false
		}
		if !isValidSide(*request.Side, SideObserver) || !lobby.isFreeStartPos(*request.StartPos, index) {
			return false
		}
		lobby.slots[index].side = *request.Side
		lobby.slots[index].startPos = *request.StartPos
	case FieldMyColor:
		if request.Color == nil || !lobby.isFreeColor(*request.Color, index) {
			return false
		}
		lobby.slots[index].color = *request.Color
	case FieldMyStartPos:
		if request.MyStartPos == nil || !lobby.isFreeStartPos(*request.MyStartPos, index) {
			return false
		}
		lobby.slots[index].startPos = *request.MyStartPos
	case FieldMyTeam:
		if request.Team == nil || !isValidTeam(*request.Team) {
			return false
		}
		lobby.slots[index].team = *request.Team
	case FieldMyHasMap:
		if request.HasMap == nil {
			return false
		}
		lobby.slots[index].hasMap = *request.HasMap
		if *request.HasMap {
			return true
		}
	}
	lobby.clearOwnReady(index)
	return true
}

// applyHostField handles the host fields other than the kick: 0, 5, 6, 12 to
// 18. Fields 0, 5, 6 and 12 to 16 clear every other member's ready state.
func (lobby *Lobby) applyHostField(request FieldRequest) bool {
	var isApplied bool
	var clearsReady bool
	switch *request.Field {
	case FieldMap:
		isApplied, clearsReady = lobby.applyMap(request), true
	case FieldStartingCash:
		isApplied, clearsReady = lobby.applyStartingCash(request), true
	case FieldLimitSuperweapons:
		isApplied, clearsReady = request.LimitSuperweapons != nil, true
		if isApplied {
			lobby.limitSuperweapons = *request.LimitSuperweapons
		}
	case FieldSetSlotState:
		isApplied, clearsReady = lobby.applySlotState(request), true
	case FieldAISide, FieldAIColor, FieldAITeam, FieldAIStartPos:
		isApplied, clearsReady = lobby.applyAIField(request), true
	case FieldMaxCameraHeight:
		isApplied = lobby.applyMaxCameraHeight(request)
	case FieldJoinability:
		isApplied = lobby.applyJoinability(request)
	}
	if isApplied && clearsReady {
		lobby.clearReadyExceptHost()
	}
	return isApplied
}

// applyMap handles field 0. max_players may not leave a slot that holds a
// human or an AI at or above it; that also keeps it from being below the
// number of such slots (SERVICE.md section 7.3).
func (lobby *Lobby) applyMap(request FieldRequest) bool {
	if request.Map == nil || request.MapPath == nil || request.MapOfficial == nil || request.MaxPlayers == nil {
		return false
	}
	maxPlayers := *request.MaxPlayers
	if maxPlayers < MinPlayers || maxPlayers > MaxPlayers {
		return false
	}
	for index := maxPlayers; index < SlotCount; index++ {
		if lobby.slots[index].isHuman() || lobby.slots[index].isAI() {
			return false
		}
	}
	lobby.mapName = CleanText(*request.Map, maxMapNameCharacters)
	lobby.mapPath = CleanText(*request.MapPath, maxMapPathCharacters)
	lobby.isMapOfficial = *request.MapOfficial
	lobby.resizeSlots(maxPlayers)
	return true
}

// applyStartingCash handles field 5.
func (lobby *Lobby) applyStartingCash(request FieldRequest) bool {
	if request.StartingCash == nil || *request.StartingCash < 0 || *request.StartingCash > math.MaxUint32 {
		return false
	}
	lobby.startingCash = uint32(*request.StartingCash)
	return true
}

// applySlotState handles field 12: the host opens, closes or fills with an
// AI a slot that holds no human. Slots at or above max_players stay closed.
func (lobby *Lobby) applySlotState(request FieldRequest) bool {
	if request.SlotIndex == nil || request.SlotState == nil {
		return false
	}
	index, state := *request.SlotIndex, *request.SlotState
	if index < 0 || index >= lobby.maxPlayers || lobby.slots[index].isHuman() {
		return false
	}
	if state != SlotOpen && state != SlotClosed && !isAIState(state) {
		return false
	}
	lobby.slots[index] = emptySlot(state)
	return true
}

// aiSlotIndex returns the slot a field 13 to 16 request names, which must hold an AI.
func (lobby *Lobby) aiSlotIndex(request FieldRequest) (int, bool) {
	if request.Slot == nil || *request.Slot < 0 || *request.Slot >= SlotCount {
		return 0, false
	}
	return *request.Slot, lobby.slots[*request.Slot].isAI()
}

// applyAIField handles fields 13 to 16 for the AI slot named by the request.
func (lobby *Lobby) applyAIField(request FieldRequest) bool {
	index, isAI := lobby.aiSlotIndex(request)
	if !isAI {
		return false
	}
	switch *request.Field {
	case FieldAISide:
		if request.Side == nil || request.StartPos == nil {
			return false
		}
		if !isValidSide(*request.Side, NoChoice) || !lobby.isFreeStartPos(*request.StartPos, index) {
			return false
		}
		lobby.slots[index].side = *request.Side
		lobby.slots[index].startPos = *request.StartPos
	case FieldAIColor:
		if request.Color == nil || !lobby.isFreeColor(*request.Color, index) {
			return false
		}
		lobby.slots[index].color = *request.Color
	case FieldAITeam:
		if request.Team == nil || !isValidTeam(*request.Team) {
			return false
		}
		lobby.slots[index].team = *request.Team
	case FieldAIStartPos:
		if request.StartPos == nil || !lobby.isFreeStartPos(*request.StartPos, index) {
			return false
		}
		lobby.slots[index].startPos = *request.StartPos
	}
	return true
}

// applyMaxCameraHeight handles field 17.
func (lobby *Lobby) applyMaxCameraHeight(request FieldRequest) bool {
	if request.MaxCameraHeight == nil || *request.MaxCameraHeight < 0 || *request.MaxCameraHeight > math.MaxUint16 {
		return false
	}
	lobby.maxCameraHeight = uint16(*request.MaxCameraHeight)
	return true
}

// applyJoinability handles field 18.
func (lobby *Lobby) applyJoinability(request FieldRequest) bool {
	if request.Joinability == nil {
		return false
	}
	if *request.Joinability != joinabilityPublic && *request.Joinability != joinabilityFriendsOnly {
		return false
	}
	lobby.joinability = *request.Joinability
	return true
}

// kickTarget checks field 11. The host cannot kick itself; to leave it
// leaves. The caller removes the returned member.
func (lobby *Lobby) kickTarget(request FieldRequest) (ChangeResult, int64) {
	if request.UserID == nil || *request.UserID == lobby.owner || !lobby.IsMember(*request.UserID) {
		return ChangeRefused, NoUser
	}
	return ChangeKick, *request.UserID
}

// isValidSide reports whether side is a template index, random, or (when
// minimum is SideObserver) observer.
func isValidSide(side int, minimum int) bool {
	return side >= minimum && side <= maxSideIndex
}

// isValidTeam reports whether team is none or a team index.
func isValidTeam(team int) bool {
	return team >= NoChoice && team <= maxTeamIndex
}

// isFreeColor reports whether colour is random or a valid colour that no
// other slot has taken.
func (lobby *Lobby) isFreeColor(color int, ownIndex int) bool {
	if color < NoChoice || color > maxColorIndex {
		return false
	}
	for index, entry := range lobby.slots {
		if index != ownIndex && color != NoChoice && entry.color == color {
			return false
		}
	}
	return true
}

// isFreeStartPos reports whether position is random or a position below
// max_players that no other slot has taken.
func (lobby *Lobby) isFreeStartPos(position int, ownIndex int) bool {
	if position < NoChoice || position >= lobby.maxPlayers {
		return false
	}
	for index, entry := range lobby.slots {
		if index != ownIndex && position != NoChoice && entry.startPos == position {
			return false
		}
	}
	return true
}
