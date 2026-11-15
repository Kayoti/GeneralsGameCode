// Tests of the lobbies on the WebSocket side: the notices after every change,
// lobby chat, ready, the countdown lock, match start, held places and the
// ways a player leaves (SERVICE.md sections 6.2, 6.3 and 7). The HTTP calls
// are made on the Hub directly; the api package tests them over HTTP.

package realtime

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"servers-lobby/internal/lobbies"
)

// lobbyWait is how long the countdown lock lasts in these tests.
const lobbyWait = 300 * time.Millisecond

// lobbySettings are shortSettings with a short countdown lock and a longer hold.
func lobbySettings() Settings {
	settings := shortSettings()
	settings.CountdownLock = lobbyWait
	settings.ReconnectHold = time.Second
	settings.IdleTimeout = 5 * time.Second
	return settings
}

// testPlayer is a connected account of a test, already in room 1.
type testPlayer struct {
	id     int64
	socket *client
}

// newPlayers connects n accounts and puts each in room 1. The room messages
// that arrive are read, so each socket starts with nothing waiting.
func newPlayers(t *testing.T, service *testService, count int) []*testPlayer {
	t.Helper()
	players := make([]*testPlayer, 0, count)
	for range count {
		id := service.newAccount()
		socket := service.connect(id)
		joinRoom(t, socket)
		players = append(players, &testPlayer{id: id, socket: socket})
	}
	for _, member := range players {
		settle(t, member.socket)
	}
	return players
}

// settle sends a ping and returns every message that arrived before the pong,
// so the messages the service queued before the call are all returned.
func settle(t *testing.T, socket *client) []map[string]any {
	t.Helper()
	send(t, socket, map[string]any{"msg_id": 14})
	var before []map[string]any
	for {
		message := receive(t, socket)
		if message["msg_id"] == float64(15) {
			return before
		}
		before = append(before, message)
	}
}

// sendSync sends a message and waits until the service has handled it, which
// it knows when the pong of a ping sent right after arrives. It returns what
// reached the sender meanwhile.
func sendSync(t *testing.T, socket *client, message map[string]any) []map[string]any {
	t.Helper()
	send(t, socket, message)
	return settle(t, socket)
}

// ids returns the msg_id of each message.
func ids(messages []map[string]any) []int {
	result := make([]int, 0, len(messages))
	for _, message := range messages {
		result = append(result, int(message["msg_id"].(float64)))
	}
	return result
}

// requireIDs fails unless the messages that arrived before the pong have exactly these ids in order.
func requireIDs(t *testing.T, socket *client, want ...int) []map[string]any {
	t.Helper()
	messages := settle(t, socket)
	got := ids(messages)
	if len(got) != len(want) {
		t.Fatalf("messages %v, want ids %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("messages %v, want ids %v", got, want)
		}
	}
	return messages
}

// createRequest returns a valid create request for a lobby of maxPlayers.
func createRequest(maxPlayers int) lobbies.CreateRequest {
	name := "Game"
	return lobbies.CreateRequest{Name: &name, MapName: "Map", MapPath: "map", MaxPlayers: &maxPlayers}
}

// mustCreate creates a lobby hosted by the player and returns its id.
func mustCreate(t *testing.T, service *testService, host *testPlayer, maxPlayers int) int64 {
	t.Helper()
	lobbyID, err := service.hub.CreateLobby(host.id, createRequest(maxPlayers))
	if err != nil {
		t.Fatalf("CreateLobby returned error: %v", err)
	}
	return lobbyID
}

// mustJoin puts the player in the lobby.
func mustJoin(t *testing.T, service *testService, joiner *testPlayer, lobbyID int64) {
	t.Helper()
	if err := service.hub.JoinLobby(joiner.id, lobbyID, lobbies.JoinRequest{HasMap: true}); err != nil {
		t.Fatalf("JoinLobby returned error: %v", err)
	}
}

// memberOf returns the detail entry of a slot, as the lobby shows it to the player.
func memberOf(t *testing.T, service *testService, viewer *testPlayer, lobbyID int64, slotIndex int) lobbies.DetailMember {
	t.Helper()
	detail, found := service.hub.LobbyDetail(viewer.id, lobbyID)
	if !found {
		t.Fatalf("player %d does not see lobby %d", viewer.id, lobbyID)
	}
	return detail.Members[slotIndex]
}

func TestCreateTellsTheWatchersAndJoinTellsMembersAndWatchers(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, watcher := players[0], players[1], players[2]

	lobbyID := mustCreate(t, service, host, 4)
	requireIDs(t, host.socket)
	requireIDs(t, guest.socket, MsgLobbyListUpdate)
	requireIDs(t, watcher.socket, MsgLobbyListUpdate)

	mustJoin(t, service, guest, lobbyID)
	requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireIDs(t, watcher.socket, MsgLobbyListUpdate)
}

func TestEveryAcceptedChangeSendsSixToMembersAndARefusedOneSendsNothing(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)
	settle(t, guest.socket)
	field := lobbies.FieldMyTeam
	team := 2

	service.hub.ChangeLobby(guest.id, lobbyID, lobbies.FieldRequest{Field: &field, Team: &team})
	requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate)
	requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate)
	if memberOf(t, service, host, lobbyID, 1).Team != team {
		t.Error("the team was not stored")
	}

	badTeam := 99
	service.hub.ChangeLobby(guest.id, lobbyID, lobbies.FieldRequest{Field: &field, Team: &badTeam})
	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
}

func TestReadyMessageSetsReadyAndSendsSix(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)
	settle(t, guest.socket)

	sendSync(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate)
	if !memberOf(t, service, host, lobbyID, 1).IsReady {
		t.Error("the guest is not ready")
	}

	sendSync(t, host.socket, map[string]any{"msg_id": 5, "ready": false})
	if !memberOf(t, service, host, lobbyID, 0).IsReady {
		t.Error("the host could clear its ready state")
	}
	sendSync(t, guest.socket, map[string]any{"msg_id": 5})
	sendSync(t, guest.socket, map[string]any{"msg_id": 5, "ready": "yes"})
	if !memberOf(t, service, host, lobbyID, 1).IsReady {
		t.Error("a ready message with no valid ready changed the state")
	}
}

func TestMessagesFromAPlayerInNoLobbyAreIgnored(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, loner := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	settle(t, host.socket)
	settle(t, loner.socket)

	for _, message := range []map[string]any{
		{"msg_id": 5, "ready": true},
		{"msg_id": 10, "message": "hi", "action": false, "announcement": false, "show_announcement_to_host": false},
		{"msg_id": 13},
		{"msg_id": 23},
		{"msg_id": 24},
		{"msg_id": 25, "new_password": "x"},
		{"msg_id": 26},
	} {
		send(t, loner.socket, message)
	}
	settle(t, loner.socket)
	requireIDs(t, host.socket)
	if detail, _ := service.hub.LobbyDetail(host.id, lobbyID); detail.IsPassworded || detail.MatchID != 0 {
		t.Error("a message from a non-member changed the lobby")
	}
}

func TestLobbyChatReachesMembersWithEveryKeyAndAnnouncementRules(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, outsider := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	for _, member := range players {
		settle(t, member.socket)
	}

	send(t, guest.socket, map[string]any{"msg_id": 10, "message": "hello", "action": true, "announcement": true, "show_announcement_to_host": true})
	for _, member := range []*testPlayer{host, guest} {
		message := requireIDs(t, member.socket, 11)[0]
		requireKeys(t, message, "message", "action", "announcement", "show_announcement_to_host", "user_id")
		if message["message"] != "hello" || message["action"] != true || message["announcement"] != false ||
			message["show_announcement_to_host"] != false || message["user_id"] != float64(guest.id) {
			t.Errorf("message = %v", message)
		}
	}

	send(t, host.socket, map[string]any{"msg_id": 10, "message": "start in 5", "action": false, "announcement": true, "show_announcement_to_host": false})
	requireIDs(t, host.socket)
	message := requireIDs(t, guest.socket, 11)[0]
	if message["announcement"] != true || message["show_announcement_to_host"] != false {
		t.Errorf("announcement = %v", message)
	}

	send(t, host.socket, map[string]any{"msg_id": 10, "message": "again", "action": false, "announcement": true, "show_announcement_to_host": true})
	requireIDs(t, host.socket, 11)
	requireIDs(t, guest.socket, 11)
	requireIDs(t, outsider.socket)
}

func TestLobbyChatIsCutAt300CharactersAndEmptyTextIsIgnored(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 1)
	mustCreate(t, service, players[0], 4)
	settle(t, players[0].socket)

	send(t, players[0].socket, map[string]any{"msg_id": 10, "message": string(make([]rune, 0)) + "  \t "})
	send(t, players[0].socket, map[string]any{"msg_id": 10, "message": strings.Repeat("a", 400)})
	message := requireIDs(t, players[0].socket, 11)[0]
	if got := len([]rune(message["message"].(string))); got != 300 {
		t.Errorf("message has %d characters, want 300", got)
	}
}

func TestPasswordMessagesChangeAndRemoveThePassword(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, other := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	settle(t, host.socket)
	settle(t, guest.socket)

	if got := ids(sendSync(t, host.socket, map[string]any{"msg_id": 25, "new_password": "secret"})); len(got) != 1 || got[0] != MsgLobbyCurrentLobbyUpdate {
		t.Fatalf("the host got %v after changing the password, want one 6", got)
	}
	if err := service.hub.JoinLobby(guest.id, lobbyID, lobbies.JoinRequest{Password: "wrong"}); !errors.Is(err, lobbies.ErrWrongPassword) {
		t.Fatalf("join with a wrong password: error = %v", err)
	}
	if err := service.hub.JoinLobby(guest.id, lobbyID, lobbies.JoinRequest{Password: "secret"}); err != nil {
		t.Fatalf("join with the right password: error = %v", err)
	}
	settle(t, host.socket)

	send(t, guest.socket, map[string]any{"msg_id": 24})
	send(t, guest.socket, map[string]any{"msg_id": 25, "new_password": "mine"})
	settle(t, guest.socket)
	if err := service.hub.JoinLobby(other.id, lobbyID, lobbies.JoinRequest{Password: "mine"}); !errors.Is(err, lobbies.ErrWrongPassword) {
		t.Fatalf("a member changed the password: error = %v", err)
	}
	send(t, host.socket, map[string]any{"msg_id": 25, "new_password": ""})
	send(t, host.socket, map[string]any{"msg_id": 25})
	send(t, host.socket, map[string]any{"msg_id": 24})
	settle(t, host.socket)
	if err := service.hub.JoinLobby(other.id, lobbyID, lobbies.JoinRequest{}); err != nil {
		t.Fatalf("join after the password was removed: error = %v", err)
	}
}

func TestCountdownLocksJoinsAndMemberChangesAndEndsAfterTheLockTime(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, late := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)

	send(t, guest.socket, map[string]any{"msg_id": 23})
	settle(t, guest.socket)
	if err := service.hub.JoinLobby(late.id, lobbyID, lobbies.JoinRequest{}); err != nil {
		t.Fatalf("a countdown from a member locked the lobby: %v", err)
	}
	service.hub.LeaveLobby(late.id, lobbyID)

	send(t, host.socket, map[string]any{"msg_id": 23})
	settle(t, host.socket)
	if err := service.hub.JoinLobby(late.id, lobbyID, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrLobbyLocked) {
		t.Fatalf("join while locked: error = %v", err)
	}
	send(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, guest.socket)
	if memberOf(t, service, host, lobbyID, 1).IsReady {
		t.Error("a member became ready while the lobby was locked")
	}

	time.Sleep(lobbyWait + 50*time.Millisecond)
	if err := service.hub.JoinLobby(late.id, lobbyID, lobbies.JoinRequest{}); err != nil {
		t.Fatalf("join after the lock ended: %v", err)
	}
}

func TestANewMessage26EndsTheCountdownLock(t *testing.T) {
	settings := lobbySettings()
	settings.CountdownLock = time.Minute
	service := newTestService(t, settings)
	players := newPlayers(t, service, 2)
	host, late := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)

	send(t, host.socket, map[string]any{"msg_id": 23})
	settle(t, host.socket)
	send(t, late.socket, map[string]any{"msg_id": 26})
	settle(t, late.socket)
	if err := service.hub.JoinLobby(late.id, lobbyID, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrLobbyLocked) {
		t.Fatalf("a 26 from a non-member ended the lock: error = %v", err)
	}
	send(t, host.socket, map[string]any{"msg_id": 26})
	settle(t, host.socket)
	if err := service.hub.JoinLobby(late.id, lobbyID, lobbies.JoinRequest{}); err != nil {
		t.Fatalf("join after the host's 26: error = %v", err)
	}
}

func TestStartIsIgnoredUntilEveryHumanIsReady(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)
	settle(t, guest.socket)

	send(t, host.socket, map[string]any{"msg_id": 13})
	send(t, guest.socket, map[string]any{"msg_id": 13})
	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)

	send(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, host.socket)
	settle(t, guest.socket)
	send(t, guest.socket, map[string]any{"msg_id": 13})
	requireIDs(t, guest.socket)
	requireIDs(t, host.socket)
	if detail, _ := service.hub.LobbyDetail(host.id, lobbyID); detail.MatchID != 0 {
		t.Error("a member started the match")
	}
}

func TestStartSendsSixThenThirteenToEveryHumanAndLeavesTheList(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, watcher := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	sendSync(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	for _, member := range players {
		settle(t, member.socket)
	}

	send(t, host.socket, map[string]any{"msg_id": 13})
	for _, member := range []*testPlayer{host, guest} {
		messages := requireIDs(t, member.socket, MsgLobbyCurrentLobbyUpdate, 13)
		if messages[1]["screenshot_url"] != "" {
			t.Errorf("screenshot_url = %v, want an empty string", messages[1]["screenshot_url"])
		}
	}
	requireIDs(t, watcher.socket, MsgLobbyListUpdate)
	if detail, _ := service.hub.LobbyDetail(host.id, lobbyID); detail.MatchID != 1 {
		t.Errorf("MatchID = %d, want 1", detail.MatchID)
	}
	if len(service.hub.ListLobbies()) != 0 {
		t.Error("a started lobby is still listed")
	}
	if err := service.hub.JoinLobby(watcher.id, lobbyID, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrLobbyNotFound) {
		t.Errorf("join after the start: error = %v", err)
	}
	send(t, host.socket, map[string]any{"msg_id": 13})
	requireIDs(t, host.socket)
}

func TestHeldPlaceKeepsPendingSixSevenAndThirteenForTheReconnect(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, watcher := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	sendSync(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	for _, member := range players {
		settle(t, member.socket)
	}
	guest.socket.CloseNow()
	watcher.socket.CloseNow()
	service.waitUntilHeld(guest.id)
	service.waitUntilHeld(watcher.id)

	send(t, host.socket, map[string]any{"msg_id": 10, "message": "lost while away", "announcement": false})
	sendSync(t, host.socket, map[string]any{"msg_id": 13})

	guestBack, _, err := service.dial(guest.id, true)
	if err != nil {
		t.Fatalf("reconnect of the guest failed: %v", err)
	}
	messages := requireIDs(t, guestBack, MsgLobbyCurrentLobbyUpdate, 13)
	if messages[1]["screenshot_url"] != "" {
		t.Errorf("held 13 = %v", messages[1])
	}
	watcherBack, _, err := service.dial(watcher.id, true)
	if err != nil {
		t.Fatalf("reconnect of the watcher failed: %v", err)
	}
	requireIDs(t, watcherBack, MsgLobbyListUpdate)
}

func TestExpiredHoldRemovesThePlayerFromItsLobby(t *testing.T) {
	settings := lobbySettings()
	settings.ReconnectHold = lobbyWait
	service := newTestService(t, settings)
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)
	guest.socket.CloseNow()
	service.waitUntilHeld(guest.id)

	if memberOf(t, service, host, lobbyID, 1).UserID != guest.id {
		t.Fatal("the guest lost its slot while its place was held")
	}
	waitFor(t, "the held place to run out", func() bool {
		return memberOf(t, service, host, lobbyID, 1).UserID == lobbies.NoUser
	})
	requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer, msgRoomMemberList)
}

// waitFor polls until condition is true or testWait passes.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLoggingOutRemovesThePlayerFromItsLobbyAndAnEmptyLobbyIsDeleted(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)

	guest.socket.Close(1000, "logout")
	waitFor(t, "the guest to leave", func() bool {
		return memberOf(t, service, host, lobbyID, 1).UserID == lobbies.NoUser
	})
	host.socket.Close(1000, "logout")
	waitFor(t, "the lobby to be deleted", func() bool {
		service.hub.mu.Lock()
		defer service.hub.mu.Unlock()
		return len(service.hub.lobbies) == 0
	})
}

func TestHostLeavingPassesTheLobbyToTheEarliestJoiner(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 4)
	host, second, third, watcher := players[0], players[1], players[2], players[3]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, second, lobbyID)
	mustJoin(t, service, third, lobbyID)
	for _, member := range players {
		settle(t, member.socket)
	}

	service.hub.LeaveLobby(host.id, lobbyID)

	detail, _ := service.hub.LobbyDetail(second.id, lobbyID)
	if detail.Owner != second.id || !detail.Members[1].IsReady || detail.Members[0].SlotState != lobbies.SlotOpen {
		t.Errorf("owner %d, slot 1 %+v, slot 0 %+v; want the second player as ready owner and slot 0 open", detail.Owner, detail.Members[1], detail.Members[0])
	}
	requireIDs(t, second.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer)
	requireIDs(t, third.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer)
	requireIDs(t, host.socket, MsgLobbyListUpdate)
	requireIDs(t, watcher.socket, MsgLobbyListUpdate)
	if _, found := service.hub.LobbyDetail(host.id, lobbyID); found {
		t.Error("the leaver still sees the lobby")
	}
}

func TestLastMemberLeavingDeletesTheLobbyAndTellsTheWatchers(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, watcher := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	settle(t, watcher.socket)

	service.hub.LeaveLobby(host.id, lobbyID)

	requireIDs(t, watcher.socket, MsgLobbyListUpdate)
	if len(service.hub.ListLobbies()) != 0 {
		t.Error("the empty lobby is still listed")
	}
	if err := service.hub.JoinLobby(watcher.id, lobbyID, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrLobbyNotFound) {
		t.Errorf("join of a deleted lobby: error = %v", err)
	}
}

func TestKickRemovesTheMemberAndTellsItsClient(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	settle(t, host.socket)
	settle(t, guest.socket)
	field, target := lobbies.FieldKickUser, guest.id

	service.hub.ChangeLobby(host.id, lobbyID, lobbies.FieldRequest{Field: &field, UserID: &target})

	requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer)
	requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate, MsgLobbyListUpdate)
	if _, found := service.hub.LobbyDetail(guest.id, lobbyID); found {
		t.Error("the kicked player still sees the lobby")
	}
	service.hub.ChangeLobby(guest.id, lobbyID, lobbies.FieldRequest{Field: &field, UserID: &target})
}

func TestAPlayerIsInOneLobbyAtATime(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	first, second, third := players[0], players[1], players[2]
	firstLobby := mustCreate(t, service, first, 4)
	secondLobby := mustCreate(t, service, second, 4)
	mustJoin(t, service, third, firstLobby)

	mustJoin(t, service, third, secondLobby)

	if _, found := service.hub.LobbyDetail(third.id, firstLobby); found {
		t.Error("the player is still in the first lobby")
	}
	if !memberIs(t, service, second, secondLobby, 1, third.id) {
		t.Error("the player is not in the second lobby")
	}
	if _, err := service.hub.CreateLobby(first.id, createRequest(4)); err != nil {
		t.Fatalf("CreateLobby returned error: %v", err)
	}
	if _, found := service.hub.LobbyDetail(first.id, firstLobby); found {
		t.Error("creating a lobby did not remove the host from its old one")
	}
	if len(service.hub.ListLobbies()) != 2 {
		t.Errorf("%d lobbies listed, want 2 (the first was deleted when its host left)", len(service.hub.ListLobbies()))
	}
}

// memberIs reports whether the slot of the lobby holds the user.
func memberIs(t *testing.T, service *testService, viewer *testPlayer, lobbyID int64, slotIndex int, userID int64) bool {
	t.Helper()
	return memberOf(t, service, viewer, lobbyID, slotIndex).UserID == userID
}

func TestAFailedJoinKeepsThePlayerInItsLobby(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	hostLobby := mustCreate(t, service, host, 4)
	guestLobby := mustCreate(t, service, guest, 4)

	if err := service.hub.JoinLobby(host.id, 999, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrLobbyNotFound) {
		t.Fatalf("join of an unknown lobby: error = %v", err)
	}
	service.hub.hubLobbyForTest(guestLobby, func(lobby *lobbies.Lobby) { lobby.SetPassword(guest.id, "pw") })
	if err := service.hub.JoinLobby(host.id, guestLobby, lobbies.JoinRequest{}); !errors.Is(err, lobbies.ErrWrongPassword) {
		t.Fatalf("join with no password: error = %v", err)
	}
	if _, found := service.hub.LobbyDetail(host.id, hostLobby); !found {
		t.Error("a failed join removed the player from its lobby")
	}
}

// hubLobbyForTest runs change on a lobby under the hub lock.
func (hub *Hub) hubLobbyForTest(lobbyID int64, change func(*lobbies.Lobby)) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	change(hub.lobbies[lobbyID])
}

func TestNameChangeInALobbySendsSixToMembersAndSevenToWatchers(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, watcher := players[0], players[1], players[2]
	lobbyID := mustCreate(t, service, host, 4)
	mustJoin(t, service, guest, lobbyID)
	for _, member := range players {
		settle(t, member.socket)
	}

	sendSync(t, guest.socket, map[string]any{"msg_id": 9, "name": "Renamed Guest"})

	messages := settle(t, host.socket)
	if got := ids(messages); len(got) == 0 || got[len(got)-1] != MsgLobbyCurrentLobbyUpdate {
		t.Errorf("host got %v, want a 6", got)
	}
	if memberOf(t, service, host, lobbyID, 1).DisplayName != "Renamed Guest" {
		t.Error("the lobby does not show the new name")
	}
	if got := ids(settle(t, watcher.socket)); len(got) < 1 || got[len(got)-1] != MsgLobbyListUpdate {
		t.Errorf("watcher got %v, want a 7 after the member list", got)
	}
}

func TestAccountWithoutAWebSocketCannotCreateOrJoin(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 1)
	lobbyID := mustCreate(t, service, players[0], 4)
	stranger := service.newAccount()

	if _, err := service.hub.CreateLobby(stranger, createRequest(4)); !errors.Is(err, ErrNotConnected) {
		t.Errorf("CreateLobby error = %v, want ErrNotConnected", err)
	}
	if err := service.hub.JoinLobby(stranger, lobbyID, lobbies.JoinRequest{}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("JoinLobby error = %v, want ErrNotConnected", err)
	}
}

func TestListShowsOnlyPublicLobbiesThatHaveNotStarted(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	publicLobby := mustCreate(t, service, players[0], 4)
	friendsLobby := mustCreate(t, service, players[1], 4)
	field, joinability := lobbies.FieldJoinability, 1
	service.hub.ChangeLobby(players[1].id, friendsLobby, lobbies.FieldRequest{Field: &field, Joinability: &joinability})

	listed := service.hub.ListLobbies()
	if len(listed) != 1 || listed[0].LobbyID != publicLobby {
		t.Errorf("listed = %+v, want only lobby %d", listed, publicLobby)
	}
}

func TestConcurrentLobbyCallsKeepTheStateConsistent(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 8)
	lobbyID := mustCreate(t, service, players[0], 8)
	var group sync.WaitGroup
	for _, member := range players[1:] {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				_ = service.hub.JoinLobby(member.id, lobbyID, lobbies.JoinRequest{})
				service.hub.LeaveLobby(member.id, lobbyID)
			}
			_ = service.hub.JoinLobby(member.id, lobbyID, lobbies.JoinRequest{})
		}()
	}
	group.Wait()

	detail, found := service.hub.LobbyDetail(players[0].id, lobbyID)
	if !found {
		t.Fatal("the lobby is gone")
	}
	humans := 0
	for _, member := range detail.Members {
		if member.SlotState == lobbies.SlotHuman {
			humans++
		}
	}
	if humans != 8 {
		t.Errorf("%d humans in the lobby, want 8", humans)
	}
}
