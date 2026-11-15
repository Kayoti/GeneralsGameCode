// Tests of the lobby model: create, join, every field change, ready state,
// leaving, the countdown lock, start and chat targets (SERVICE.md section 7).

package lobbies

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	hostID   int64 = 1001
	guestID  int64 = 1002
	thirdID  int64 = 1003
	seedUsed       = 12345
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// createRequest returns a valid create request for a lobby of maxPlayers.
func createRequest(maxPlayers int) CreateRequest {
	name := "My game"
	return CreateRequest{Name: &name, MapName: "Alpine Assault", MapPath: "alpine assault", MapOfficial: true, MaxPlayers: &maxPlayers}
}

// newLobby returns a lobby of maxPlayers hosted by hostID.
func newLobby(t *testing.T, maxPlayers int) *Lobby {
	t.Helper()
	lobby, err := NewLobby(1, hostID, createRequest(maxPlayers), seedUsed)
	if err != nil {
		t.Fatalf("NewLobby returned error: %v", err)
	}
	return lobby
}

// joined returns a lobby of maxPlayers with the guest in slot 1.
func joined(t *testing.T, maxPlayers int) *Lobby {
	t.Helper()
	lobby := newLobby(t, maxPlayers)
	if err := lobby.Join(guestID, JoinRequest{HasMap: true}, testNow); err != nil {
		t.Fatalf("Join returned error: %v", err)
	}
	return lobby
}

func intp(value int) *int       { return &value }
func int64p(value int64) *int64 { return &value }
func boolp(value bool) *bool    { return &value }
func stringp(value string) *string {
	return &value
}

// change applies a field request from the user and returns the result.
func change(lobby *Lobby, userID int64, request FieldRequest) ChangeResult {
	result, _ := lobby.ApplyChange(userID, request, testNow)
	return result
}

func TestNewLobbyHasEightSlotsAndAReadyHostInSlotZero(t *testing.T) {
	lobby := newLobby(t, 4)

	detail := lobby.DetailEntry(func(int64) string { return "Host" })
	if len(detail.Members) != SlotCount {
		t.Fatalf("members = %d, want %d", len(detail.Members), SlotCount)
	}
	host := detail.Members[0]
	if host.UserID != hostID || host.SlotState != SlotHuman || !host.IsReady || host.DisplayName != "Host" {
		t.Errorf("slot 0 = %+v, want the ready host", host)
	}
	for index, member := range detail.Members {
		if member.SlotIndex != index {
			t.Errorf("member %d has SlotIndex %d", index, member.SlotIndex)
		}
		if index == 0 {
			continue
		}
		wantState := SlotOpen
		if index >= 4 {
			wantState = SlotClosed
		}
		if member.UserID != NoUser || member.SlotState != wantState || member.DisplayName != "" ||
			member.Side != NoChoice || member.Color != NoChoice || member.Team != NoChoice || member.StartingPosition != NoChoice ||
			member.IsReady || member.HasMap || member.Port != 0 {
			t.Errorf("slot %d = %+v, want the empty entry in state %d", index, member, wantState)
		}
	}
	if detail.RNGSeed != seedUsed || detail.MatchID != 0 || detail.LobbyType != 0 || detail.Region != "" {
		t.Errorf("detail = %+v, want seed %d, match 0, type 0, empty region", detail.lobbyHeader, seedUsed)
	}
}

func TestNewLobbyRefusesInvalidRequests(t *testing.T) {
	cases := map[string]func(*CreateRequest){
		"missing name":    func(request *CreateRequest) { request.Name = nil },
		"blank name":      func(request *CreateRequest) { request.Name = stringp("  \t ") },
		"missing max":     func(request *CreateRequest) { request.MaxPlayers = nil },
		"one player":      func(request *CreateRequest) { request.MaxPlayers = intp(1) },
		"nine players":    func(request *CreateRequest) { request.MaxPlayers = intp(9) },
		"negative height": func(request *CreateRequest) { request.MaxCamHeight = -1 },
		"huge height":     func(request *CreateRequest) { request.MaxCamHeight = 70000 },
		"long password":   func(request *CreateRequest) { request.Password = strings.Repeat("a", maxPasswordCharacters+1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := createRequest(4)
			mutate(&request)
			_, err := NewLobby(1, hostID, request, seedUsed)
			var invalid *InvalidError
			if !errors.As(err, &invalid) {
				t.Errorf("NewLobby error = %v, want *InvalidError", err)
			}
		})
	}
}

func TestNewLobbyCutsTheNameAt40CharactersAndPasswordNeedsThePassworedFlag(t *testing.T) {
	request := createRequest(4)
	request.Name = stringp(strings.Repeat("x", 60))
	request.Password = "secret"
	lobby, err := NewLobby(1, hostID, request, seedUsed)
	if err != nil {
		t.Fatalf("NewLobby returned error: %v", err)
	}
	header := lobby.ListEntry(func(int64) string { return "" }).lobbyHeader
	if len(header.Name) != MaxNameCharacters || header.IsPassworded {
		t.Errorf("name length %d, passworded %v; want 40 and false", len(header.Name), header.IsPassworded)
	}
	request.Passworded = true
	lobby, _ = NewLobby(1, hostID, request, seedUsed)
	if !lobby.ListEntry(func(int64) string { return "" }).IsPassworded {
		t.Error("a lobby created with passworded true and a password is not passworded")
	}
}

func TestJoinTakesTheLowestOpenSlotAndFailsInTheOrderOfTheSpecification(t *testing.T) {
	lobby := newLobby(t, 3)
	lobby.password = "pw"

	if err := lobby.Join(guestID, JoinRequest{Password: "bad"}, testNow); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: error = %v", err)
	}
	if err := lobby.Join(guestID, JoinRequest{Password: "pw"}, testNow); err != nil {
		t.Fatalf("right password: error = %v", err)
	}
	if index, _ := lobby.slotOf(guestID); index != 1 {
		t.Errorf("guest took slot %d, want 1", index)
	}
	lobby.slots[1] = emptySlot(SlotClosed)
	if err := lobby.Join(thirdID, JoinRequest{Password: "pw"}, testNow); err != nil {
		t.Fatalf("third join: error = %v", err)
	}
	if index, _ := lobby.slotOf(thirdID); index != 2 {
		t.Errorf("third took slot %d, want 2", index)
	}
	if err := lobby.Join(1004, JoinRequest{Password: "pw"}, testNow); !errors.Is(err, ErrLobbyFull) {
		t.Errorf("full lobby: error = %v, want ErrLobbyFull", err)
	}
}

func TestJoinIsRefusedWhileLockedAndWhenStarted(t *testing.T) {
	lobby := newLobby(t, 4)
	lobby.StartCountdown(hostID, testNow, 25*time.Second)
	lobby.password = "pw"

	if err := lobby.Join(guestID, JoinRequest{Password: "bad"}, testNow); !errors.Is(err, ErrLobbyLocked) {
		t.Errorf("locked lobby with a wrong password: error = %v, want ErrLobbyLocked", err)
	}
	if err := lobby.Join(guestID, JoinRequest{Password: "pw"}, testNow.Add(25*time.Second)); err != nil {
		t.Errorf("join after the lock ended: error = %v", err)
	}
	lobby.SetReady(guestID, true, testNow.Add(25*time.Second))
	if !lobby.Start(hostID, 1) {
		t.Fatal("Start was refused with everyone ready")
	}
	if err := lobby.Join(thirdID, JoinRequest{Password: "pw"}, testNow); !errors.Is(err, ErrLobbyNotFound) {
		t.Errorf("started lobby: error = %v, want ErrLobbyNotFound", err)
	}
}

func TestOwnFieldsChangeOnlyTheSendersSlotAndClearItsReadyState(t *testing.T) {
	cases := map[string]FieldRequest{
		"side":      {Field: intp(FieldMySide), Side: intp(3), StartPos: intp(2)},
		"color":     {Field: intp(FieldMyColor), Color: intp(4)},
		"start pos": {Field: intp(FieldMyStartPos), MyStartPos: intp(2)},
		"team":      {Field: intp(FieldMyTeam), Team: intp(1)},
		"no map":    {Field: intp(FieldMyHasMap), HasMap: boolp(false)},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			lobby := joined(t, 4)
			lobby.SetReady(guestID, true, testNow)

			if change(lobby, guestID, request) != ChangeApplied {
				t.Fatal("the change was refused")
			}
			if lobby.slots[1].isReady {
				t.Error("the member is still ready")
			}
			if !lobby.slots[0].isReady {
				t.Error("the host is not ready")
			}
		})
	}
}

func TestOwnFieldValuesAreStored(t *testing.T) {
	lobby := joined(t, 4)
	change(lobby, guestID, FieldRequest{Field: intp(FieldMySide), Side: intp(SideObserver), StartPos: intp(NoChoice)})
	change(lobby, guestID, FieldRequest{Field: intp(FieldMyColor), Color: intp(5)})
	change(lobby, guestID, FieldRequest{Field: intp(FieldMyStartPos), MyStartPos: intp(3)})
	change(lobby, guestID, FieldRequest{Field: intp(FieldMyTeam), Team: intp(2)})

	guest := lobby.slots[1]
	if guest.side != SideObserver || guest.color != 5 || guest.startPos != 3 || guest.team != 2 {
		t.Errorf("guest slot = %+v", guest)
	}
}

func TestHasMapTrueKeepsTheReadyState(t *testing.T) {
	lobby := joined(t, 4)
	lobby.SetReady(guestID, true, testNow)

	change(lobby, guestID, FieldRequest{Field: intp(FieldMyHasMap), HasMap: boolp(true)})

	if !lobby.slots[1].isReady {
		t.Error("has_map true cleared the ready state")
	}
}

func TestOwnFieldChangesAreRefusedForBadValuesAndTakenChoices(t *testing.T) {
	lobby := joined(t, 4)
	change(lobby, hostID, FieldRequest{Field: intp(FieldMyColor), Color: intp(2)})
	change(lobby, hostID, FieldRequest{Field: intp(FieldMyStartPos), MyStartPos: intp(1)})
	lobby.SetReady(guestID, true, testNow)

	refused := map[string]FieldRequest{
		"color taken":         {Field: intp(FieldMyColor), Color: intp(2)},
		"color too high":      {Field: intp(FieldMyColor), Color: intp(8)},
		"color below random":  {Field: intp(FieldMyColor), Color: intp(-2)},
		"start pos taken":     {Field: intp(FieldMyStartPos), MyStartPos: intp(1)},
		"start pos past max":  {Field: intp(FieldMyStartPos), MyStartPos: intp(4)},
		"side too high":       {Field: intp(FieldMySide), Side: intp(64), StartPos: intp(NoChoice)},
		"side below observer": {Field: intp(FieldMySide), Side: intp(-3), StartPos: intp(NoChoice)},
		"side without start":  {Field: intp(FieldMySide), Side: intp(1)},
		"team too high":       {Field: intp(FieldMyTeam), Team: intp(8)},
		"missing value":       {Field: intp(FieldMyTeam)},
		"has map missing":     {Field: intp(FieldMyHasMap)},
	}
	for name, request := range refused {
		if change(lobby, guestID, request) != ChangeRefused {
			t.Errorf("%s: the change was accepted", name)
		}
	}
	if !lobby.slots[1].isReady {
		t.Error("a refused change cleared the ready state")
	}
	if change(lobby, guestID, FieldRequest{Field: intp(FieldMyColor), Color: intp(NoChoice)}) != ChangeApplied {
		t.Error("random colour was refused")
	}
}

func TestOwnFieldsAreRefusedWhileTheCountdownLocksTheLobby(t *testing.T) {
	lobby := joined(t, 4)
	lobby.StartCountdown(hostID, testNow, 25*time.Second)

	for _, request := range []FieldRequest{
		{Field: intp(FieldMyTeam), Team: intp(1)},
		{Field: intp(FieldMyHasMap), HasMap: boolp(false)},
	} {
		if result, _ := lobby.ApplyChange(guestID, request, testNow); result != ChangeRefused {
			t.Errorf("request %v was accepted while locked", *request.Field)
		}
		if result, _ := lobby.ApplyChange(guestID, request, testNow.Add(25*time.Second)); result != ChangeApplied {
			t.Errorf("request %v was refused after the lock ended", *request.Field)
		}
	}
}

func TestReadyIsRefusedWhileLocked(t *testing.T) {
	lobby := joined(t, 4)
	lobby.StartCountdown(hostID, testNow, 25*time.Second)

	if lobby.SetReady(guestID, true, testNow) {
		t.Error("ready was accepted while locked")
	}
	if !lobby.SetReady(guestID, true, testNow.Add(25*time.Second)) {
		t.Error("ready was refused after the lock ended")
	}
}

func TestHostFieldsAreRefusedForMembers(t *testing.T) {
	lobby := joined(t, 4)
	requests := []FieldRequest{
		{Field: intp(FieldMap), Map: stringp("m"), MapPath: stringp("m"), MapOfficial: boolp(true), MaxPlayers: intp(3)},
		{Field: intp(FieldStartingCash), StartingCash: int64p(5000)},
		{Field: intp(FieldLimitSuperweapons), LimitSuperweapons: boolp(true)},
		{Field: intp(FieldKickUser), UserID: int64p(hostID)},
		{Field: intp(FieldSetSlotState), SlotIndex: intp(2), SlotState: intp(SlotClosed)},
		{Field: intp(FieldMaxCameraHeight), MaxCameraHeight: intp(300)},
		{Field: intp(FieldJoinability), Joinability: intp(1)},
	}
	for _, request := range requests {
		if change(lobby, guestID, request) != ChangeRefused {
			t.Errorf("field %d from a member was accepted", *request.Field)
		}
	}
}

func TestHostFieldsChangeTheLobbyAndClearEveryOthersReadyState(t *testing.T) {
	cases := map[string]FieldRequest{
		"map":           {Field: intp(FieldMap), Map: stringp("Tournament"), MapPath: stringp("tournament"), MapOfficial: boolp(false), MaxPlayers: intp(6)},
		"starting cash": {Field: intp(FieldStartingCash), StartingCash: int64p(20000)},
		"superweapons":  {Field: intp(FieldLimitSuperweapons), LimitSuperweapons: boolp(true)},
		"slot state":    {Field: intp(FieldSetSlotState), SlotIndex: intp(2), SlotState: intp(SlotEasyAI)},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			lobby := joined(t, 4)
			lobby.SetReady(guestID, true, testNow)

			if change(lobby, hostID, request) != ChangeApplied {
				t.Fatal("the change was refused")
			}
			if lobby.slots[1].isReady {
				t.Error("the member is still ready")
			}
			if !lobby.slots[0].isReady {
				t.Error("the host is not ready")
			}
		})
	}
}

func TestHostFieldValuesAreStored(t *testing.T) {
	lobby := joined(t, 4)
	change(lobby, hostID, FieldRequest{Field: intp(FieldMap), Map: stringp("Tournament\x07"), MapPath: stringp("tournament"), MapOfficial: boolp(false), MaxPlayers: intp(6)})
	change(lobby, hostID, FieldRequest{Field: intp(FieldStartingCash), StartingCash: int64p(20000)})
	change(lobby, hostID, FieldRequest{Field: intp(FieldLimitSuperweapons), LimitSuperweapons: boolp(true)})
	change(lobby, hostID, FieldRequest{Field: intp(FieldMaxCameraHeight), MaxCameraHeight: intp(450)})

	header := lobby.DetailEntry(func(int64) string { return "" }).lobbyHeader
	if header.MapName != "Tournament" || header.MapPath != "tournament" || header.IsMapOfficial || header.MaxPlayers != 6 ||
		header.StartingCash != 20000 || !header.IsLimitSuperweapons || header.MaximumCameraHeight != 450 {
		t.Errorf("header = %+v", header)
	}
	if lobby.slots[5].state != SlotOpen || lobby.slots[6].state != SlotClosed {
		t.Errorf("slot 5 state %d, slot 6 state %d; want open and closed", lobby.slots[5].state, lobby.slots[6].state)
	}
}

func TestMaxPlayersBelowTheOccupiedSlotsIsRefused(t *testing.T) {
	lobby := joined(t, 6)
	change(lobby, hostID, FieldRequest{Field: intp(FieldSetSlotState), SlotIndex: intp(2), SlotState: intp(SlotBrutalAI)})
	resize := func(maxPlayers int) ChangeResult {
		return change(lobby, hostID, FieldRequest{Field: intp(FieldMap), Map: stringp("m"), MapPath: stringp("m"), MapOfficial: boolp(true), MaxPlayers: intp(maxPlayers)})
	}

	if resize(2) != ChangeRefused {
		t.Error("max_players 2 was accepted with an AI in slot 2")
	}
	if resize(1) != ChangeRefused || resize(9) != ChangeRefused {
		t.Error("max_players outside 2 to 8 was accepted")
	}
	if resize(3) != ChangeApplied {
		t.Error("max_players 3 was refused although slots 0 to 2 are the only ones taken")
	}
}

func TestStartingCashAndCameraHeightRanges(t *testing.T) {
	lobby := newLobby(t, 4)

	for name, request := range map[string]FieldRequest{
		"negative cash":     {Field: intp(FieldStartingCash), StartingCash: int64p(-1)},
		"cash over uint32":  {Field: intp(FieldStartingCash), StartingCash: int64p(1 << 32)},
		"negative height":   {Field: intp(FieldMaxCameraHeight), MaxCameraHeight: intp(-1)},
		"height over 16bit": {Field: intp(FieldMaxCameraHeight), MaxCameraHeight: intp(65536)},
		"joinability 2":     {Field: intp(FieldJoinability), Joinability: intp(2)},
		"map without path":  {Field: intp(FieldMap), Map: stringp("m"), MaxPlayers: intp(4)},
	} {
		if change(lobby, hostID, request) != ChangeRefused {
			t.Errorf("%s was accepted", name)
		}
	}
	if change(lobby, hostID, FieldRequest{Field: intp(FieldStartingCash), StartingCash: int64p(1<<32 - 1)}) != ChangeApplied {
		t.Error("the largest uint32 cash was refused")
	}
}

func TestJoinabilityFriendsOnlyTakesTheLobbyOffTheList(t *testing.T) {
	lobby := newLobby(t, 4)
	if !lobby.IsListed() {
		t.Fatal("a new lobby is not listed")
	}

	change(lobby, hostID, FieldRequest{Field: intp(FieldJoinability), Joinability: intp(1)})
	if lobby.IsListed() {
		t.Error("a friends-only lobby is listed")
	}
}

func TestSlotStateOnlyForSlotsWithoutAHuman(t *testing.T) {
	lobby := joined(t, 4)
	set := func(index int, state int) ChangeResult {
		return change(lobby, hostID, FieldRequest{Field: intp(FieldSetSlotState), SlotIndex: intp(index), SlotState: intp(state)})
	}

	if set(1, SlotClosed) != ChangeRefused {
		t.Error("a human slot was changed by field 12")
	}
	if set(2, SlotHuman) != ChangeRefused || set(2, 6) != ChangeRefused || set(2, -1) != ChangeRefused {
		t.Error("an invalid slot state was accepted")
	}
	if set(4, SlotOpen) != ChangeRefused || set(-1, SlotOpen) != ChangeRefused {
		t.Error("a slot at or above max_players, or below 0, was changed")
	}
	if set(2, SlotMediumAI) != ChangeApplied || lobby.slots[2].state != SlotMediumAI {
		t.Error("slot 2 did not become a medium AI")
	}
	if set(2, SlotClosed) != ChangeApplied || lobby.slots[2].state != SlotClosed {
		t.Error("slot 2 did not become closed")
	}
}

func TestAIFieldsOnlyForAISlots(t *testing.T) {
	lobby := joined(t, 4)
	change(lobby, hostID, FieldRequest{Field: intp(FieldSetSlotState), SlotIndex: intp(2), SlotState: intp(SlotEasyAI)})
	lobby.SetReady(guestID, true, testNow)

	for _, request := range []FieldRequest{
		{Field: intp(FieldAISide), Slot: intp(2), Side: intp(4), StartPos: intp(3)},
		{Field: intp(FieldAIColor), Slot: intp(2), Color: intp(6)},
		{Field: intp(FieldAITeam), Slot: intp(2), Team: intp(1)},
		{Field: intp(FieldAIStartPos), Slot: intp(2), StartPos: intp(2)},
	} {
		if change(lobby, hostID, request) != ChangeApplied {
			t.Errorf("field %d was refused", *request.Field)
		}
	}
	ai := lobby.slots[2]
	if ai.side != 4 || ai.color != 6 || ai.team != 1 || ai.startPos != 2 {
		t.Errorf("AI slot = %+v", ai)
	}
	if lobby.slots[1].isReady {
		t.Error("an AI change did not clear the member's ready state")
	}
	change(lobby, hostID, FieldRequest{Field: intp(FieldMyStartPos), MyStartPos: intp(0)})
	for name, request := range map[string]FieldRequest{
		"human slot":     {Field: intp(FieldAITeam), Slot: intp(1), Team: intp(1)},
		"open slot":      {Field: intp(FieldAITeam), Slot: intp(3), Team: intp(1)},
		"slot too high":  {Field: intp(FieldAITeam), Slot: intp(8), Team: intp(1)},
		"missing slot":   {Field: intp(FieldAITeam), Team: intp(1)},
		"AI observer":    {Field: intp(FieldAISide), Slot: intp(2), Side: intp(SideObserver), StartPos: intp(NoChoice)},
		"taken position": {Field: intp(FieldAIStartPos), Slot: intp(2), StartPos: intp(0)},
	} {
		if change(lobby, hostID, request) != ChangeRefused {
			t.Errorf("%s was accepted", name)
		}
	}
	if change(lobby, guestID, FieldRequest{Field: intp(FieldAITeam), Slot: intp(2), Team: intp(1)}) != ChangeRefused {
		t.Error("an AI change by a member was accepted")
	}
}

func TestKickIsForTheHostAndOnlyForOtherMembers(t *testing.T) {
	lobby := joined(t, 4)

	if result, _ := lobby.ApplyChange(guestID, FieldRequest{Field: intp(FieldKickUser), UserID: int64p(hostID)}, testNow); result != ChangeRefused {
		t.Error("a member kicked the host")
	}
	for name, target := range map[string]int64{"the host": hostID, "a stranger": 9999, "nobody": NoUser} {
		if result, _ := lobby.ApplyChange(hostID, FieldRequest{Field: intp(FieldKickUser), UserID: int64p(target)}, testNow); result != ChangeRefused {
			t.Errorf("kicking %s was accepted", name)
		}
	}
	result, kicked := lobby.ApplyChange(hostID, FieldRequest{Field: intp(FieldKickUser), UserID: int64p(guestID)}, testNow)
	if result != ChangeKick || kicked != guestID {
		t.Errorf("kick = %v, %d; want ChangeKick, %d", result, kicked, guestID)
	}
}

func TestFieldSevenUnknownFieldsAndFieldNineteenHaveNoEffect(t *testing.T) {
	lobby := joined(t, 4)
	before := lobby.DetailEntry(func(int64) string { return "" })

	for _, field := range []int{7, 9, 10, 19, 20, -1, 99} {
		if change(lobby, hostID, FieldRequest{Field: intp(field)}) != ChangeRefused {
			t.Errorf("field %d was accepted", field)
		}
	}
	if change(lobby, hostID, FieldRequest{}) != ChangeRefused {
		t.Error("a request with no field was accepted")
	}
	if change(lobby, thirdID, FieldRequest{Field: intp(FieldMyTeam), Team: intp(1)}) != ChangeRefused {
		t.Error("a change from a non-member was accepted")
	}
	after := lobby.DetailEntry(func(int64) string { return "" })
	if before.lobbyHeader != after.lobbyHeader || len(before.Members) != len(after.Members) {
		t.Error("the lobby changed")
	}
	for index := range before.Members {
		if before.Members[index] != after.Members[index] {
			t.Errorf("member %d changed", index)
		}
	}
}

func TestHostIsAlwaysReady(t *testing.T) {
	lobby := newLobby(t, 4)

	lobby.SetReady(hostID, false, testNow)
	if !lobby.slots[0].isReady {
		t.Error("the host could clear its ready state")
	}
	change(lobby, hostID, FieldRequest{Field: intp(FieldMyTeam), Team: intp(1)})
	if !lobby.slots[0].isReady {
		t.Error("a change of the host's own field cleared its ready state")
	}
}

func TestRemoveOpensTheSlotAndTheEarliestMemberBecomesHost(t *testing.T) {
	lobby := joined(t, 4)
	if err := lobby.Join(thirdID, JoinRequest{}, testNow); err != nil {
		t.Fatalf("Join returned error: %v", err)
	}
	lobby.slots[1], lobby.slots[2] = lobby.slots[2], lobby.slots[1]

	if lobby.Remove(hostID) {
		t.Fatal("the lobby was reported empty with two members left")
	}
	if lobby.slots[0].state != SlotOpen || lobby.slots[0].userID != NoUser {
		t.Errorf("slot 0 = %+v, want open", lobby.slots[0])
	}
	if lobby.Owner() != guestID {
		t.Errorf("owner = %d, want %d, who joined before %d", lobby.Owner(), guestID, thirdID)
	}
	if index, _ := lobby.slotOf(guestID); !lobby.slots[index].isReady {
		t.Error("the new host is not ready")
	}
}

func TestRemoveOfTheLastHumanReportsAnEmptyLobbyEvenWithAIsLeft(t *testing.T) {
	lobby := newLobby(t, 4)
	change(lobby, hostID, FieldRequest{Field: intp(FieldSetSlotState), SlotIndex: intp(1), SlotState: intp(SlotEasyAI)})

	if lobby.Remove(thirdID) {
		t.Error("a stranger leaving emptied the lobby")
	}
	if !lobby.Remove(hostID) {
		t.Error("the last human leaving did not empty the lobby")
	}
}

func TestOwnerIsKeptWhenTheHostLeavesAStartedLobby(t *testing.T) {
	lobby := joined(t, 4)
	lobby.SetReady(guestID, true, testNow)
	if !lobby.Start(hostID, 7) {
		t.Fatal("Start was refused")
	}

	lobby.Remove(hostID)

	if lobby.Owner() != hostID {
		t.Errorf("owner = %d, want the owner at the start, %d", lobby.Owner(), hostID)
	}
	if !lobby.Remove(guestID) {
		t.Error("the lobby was not reported empty when every member of a started lobby left")
	}
}

func TestStartNeedsTheHostAndEveryHumanReady(t *testing.T) {
	lobby := joined(t, 4)

	if lobby.Start(hostID, 1) {
		t.Fatal("started with a member not ready")
	}
	if lobby.Start(guestID, 1) {
		t.Fatal("a member started the lobby")
	}
	lobby.SetReady(guestID, true, testNow)
	if lobby.Start(guestID, 1) {
		t.Fatal("a member started the lobby although all were ready")
	}
	lobby.StartCountdown(hostID, testNow, 25*time.Second)
	if !lobby.Start(hostID, 9) {
		t.Fatal("the host could not start with everyone ready")
	}
	if lobby.IsListed() || !lobby.HasStarted() || lobby.IsLocked(testNow) {
		t.Error("a started lobby must be off the list and unlocked")
	}
	if lobby.DetailEntry(func(int64) string { return "" }).MatchID != 9 {
		t.Error("MatchID was not set")
	}
	if lobby.Start(hostID, 10) {
		t.Error("a lobby started twice")
	}
}

func TestSingleHumanWithAIsCanStart(t *testing.T) {
	lobby := newLobby(t, 4)
	change(lobby, hostID, FieldRequest{Field: intp(FieldSetSlotState), SlotIndex: intp(1), SlotState: intp(SlotEasyAI)})

	if !lobby.Start(hostID, 1) {
		t.Error("one human with an AI could not start")
	}
}

func TestCountdownIsHostOnlyAndEndsByTimeOrByEndCountdown(t *testing.T) {
	lobby := joined(t, 4)

	if lobby.StartCountdown(guestID, testNow, 25*time.Second) || lobby.IsLocked(testNow) {
		t.Fatal("a member started the countdown")
	}
	if !lobby.StartCountdown(hostID, testNow, 25*time.Second) {
		t.Fatal("the host could not start the countdown")
	}
	if !lobby.IsLocked(testNow.Add(24 * time.Second)) {
		t.Error("the lobby is unlocked before 25 seconds")
	}
	if lobby.IsLocked(testNow.Add(25 * time.Second)) {
		t.Error("the lobby is still locked after 25 seconds")
	}
	lobby.StartCountdown(hostID, testNow, 25*time.Second)
	if lobby.EndCountdown(guestID) || !lobby.IsLocked(testNow) {
		t.Error("a member ended the countdown")
	}
	if !lobby.EndCountdown(hostID) || lobby.IsLocked(testNow) {
		t.Error("the host could not end the countdown")
	}
}

func TestPasswordIsHostOnly(t *testing.T) {
	lobby := joined(t, 4)

	if lobby.SetPassword(guestID, "x") {
		t.Error("a member changed the password")
	}
	if !lobby.SetPassword(hostID, "new") || !lobby.ListEntry(func(int64) string { return "" }).IsPassworded {
		t.Error("the host could not set a password")
	}
	if lobby.SetPassword(hostID, strings.Repeat("a", maxPasswordCharacters+1)) {
		t.Error("a password that is too long was accepted")
	}
	if !lobby.SetPassword(hostID, "") || lobby.ListEntry(func(int64) string { return "" }).IsPassworded {
		t.Error("the host could not remove the password")
	}
}

func TestChatTargets(t *testing.T) {
	lobby := joined(t, 4)

	targets, isAnnouncement, ok := lobby.ChatTargets(guestID, true, true)
	if !ok || isAnnouncement || len(targets) != 2 {
		t.Errorf("announcement from a member: %v %v %v; want both members, not an announcement", targets, isAnnouncement, ok)
	}
	targets, isAnnouncement, _ = lobby.ChatTargets(hostID, true, false)
	if !isAnnouncement || len(targets) != 1 || targets[0] != guestID {
		t.Errorf("announcement hidden from the host: %v %v", targets, isAnnouncement)
	}
	targets, isAnnouncement, _ = lobby.ChatTargets(hostID, true, true)
	if !isAnnouncement || len(targets) != 2 {
		t.Errorf("announcement shown to the host: %v %v", targets, isAnnouncement)
	}
	targets, isAnnouncement, _ = lobby.ChatTargets(hostID, false, false)
	if isAnnouncement || len(targets) != 2 {
		t.Errorf("ordinary chat: %v %v", targets, isAnnouncement)
	}
	if _, _, ok := lobby.ChatTargets(thirdID, false, false); ok {
		t.Error("a stranger could chat")
	}
}

func TestCleanText(t *testing.T) {
	if got := CleanText("  a\x00b\tc  ", 10); got != "abc" {
		t.Errorf("CleanText = %q, want abc", got)
	}
	if got := CleanText("héllo wörld", 5); got != "héllo" {
		t.Errorf("CleanText = %q, want héllo", got)
	}
}
