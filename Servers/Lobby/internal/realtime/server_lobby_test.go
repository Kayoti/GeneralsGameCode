// Tests of server lobbies on the WebSocket side: no player-to-player setup,
// the connectivity check answered at once, ready only with the map, and the
// server session handed out in the match start (SERVICE.md sections 7.5 and 10).

package realtime

import (
	"errors"
	"slices"
	"testing"

	"servers-lobby/internal/lobbies"
)

const (
	serverRegionTag = "eu"
	serverExeCRC    = 7
	serverIniCRC    = 9
	sessionID       = "00112233445566ff"
	tokenOne        = "1111111111111111"
	tokenTwo        = "2222222222222222"
)

// serverRequest returns a create request for a server lobby of the region eu.
func serverRequest(maxPlayers int) lobbies.CreateRequest {
	request := createRequest(maxPlayers)
	request.ServerMode = true
	request.RegionTag = serverRegionTag
	request.ExeCRC = serverExeCRC
	request.IniCRC = serverIniCRC
	return request
}

// admitWhen returns a pass check that accepts the region eu with the lobby's fingerprints
// when ok is true, and records what it was asked.
func admitWhen(ok bool, asked *[]string) func(string, uint32, uint32) bool {
	return func(regionTag string, exeCRC uint32, iniCRC uint32) bool {
		*asked = append(*asked, regionTag)
		return ok && regionTag == serverRegionTag && exeCRC == serverExeCRC && iniCRC == serverIniCRC
	}
}

// admitAll accepts every pass of the region eu.
func admitAll(regionTag string, exeCRC uint32, iniCRC uint32) bool {
	return regionTag == serverRegionTag && exeCRC == serverExeCRC && iniCRC == serverIniCRC
}

// mustCreateServer creates a server lobby hosted by the player.
func mustCreateServer(t *testing.T, service *testService, host *testPlayer, maxPlayers int) int64 {
	t.Helper()
	lobbyID, err := service.hub.CreateLobby(host.id, serverRequest(maxPlayers))
	if err != nil {
		t.Fatalf("CreateLobby returned error: %v", err)
	}
	return lobbyID
}

// mustJoinServer puts the player in the server lobby with a pass that is accepted.
func mustJoinServer(t *testing.T, service *testService, joiner *testPlayer, lobbyID int64, hasMap bool) {
	t.Helper()
	isServer, err := service.hub.JoinLobby(joiner.id, lobbyID, lobbies.JoinRequest{HasMap: hasMap, Admit: admitAll})
	if err != nil || !isServer {
		t.Fatalf("JoinLobby returned %v, %v; want true, nil", isServer, err)
	}
}

// serverLobbyOfPlayers creates a server lobby hosted by the first player, joins
// the others in order, and reads every message that arrived meanwhile.
func serverLobbyOfPlayers(t *testing.T, service *testService, players []*testPlayer) int64 {
	t.Helper()
	lobbyID := mustCreateServer(t, service, players[0], 8)
	for _, joiner := range players[1:] {
		mustJoinServer(t, service, joiner, lobbyID, true)
	}
	for _, member := range players {
		settle(t, member.socket)
	}
	return lobbyID
}

// sessionRequest returns the field 19 request with the tokens given by slot.
func sessionRequest(session string, tokens map[int]string) lobbies.FieldRequest {
	field := lobbies.FieldServerSession
	request := lobbies.FieldRequest{Field: &field, ServerSession: &session}
	for slotIndex, token := range tokens {
		request.Tokens = append(request.Tokens, lobbies.SessionToken{Slot: &slotIndex, Token: &token})
	}
	return request
}

// readyAll sends ready from every player but the host and reads what arrived.
func readyAll(t *testing.T, players []*testPlayer) {
	t.Helper()
	for _, member := range players[1:] {
		send(t, member.socket, map[string]any{"msg_id": 5, "ready": true})
		settle(t, member.socket)
	}
	for _, member := range players {
		settle(t, member.socket)
	}
}

func TestServerLobbyIsTypeTwoWithItsRegionInTheListAndTheDetail(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	lobbyID := mustCreateServer(t, service, players[0], 4)

	listed := service.hub.ListLobbies()
	detail, _ := service.hub.LobbyDetail(players[0].id, lobbyID)

	if len(listed) != 1 || listed[0].LobbyType != 2 || listed[0].Region != serverRegionTag {
		t.Errorf("list entry is %+v, want LobbyType 2 and Region %q", listed, serverRegionTag)
	}
	if detail.LobbyType != 2 || detail.Region != serverRegionTag {
		t.Errorf("detail has LobbyType %d and Region %q, want 2 and %q", detail.LobbyType, detail.Region, serverRegionTag)
	}
}

func TestServerLobbyNeedsARegion(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 1)
	request := serverRequest(4)
	request.RegionTag = ""

	_, err := service.hub.CreateLobby(players[0].id, request)

	var invalid *lobbies.InvalidError
	if !errors.As(err, &invalid) {
		t.Errorf("CreateLobby error = %v, want an *InvalidError", err)
	}
}

func TestJoiningAServerLobbyAsksThePassCheckWithTheLobbysRegionAndNeedsItToSucceed(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, refused, admitted := players[0], players[1], players[2]
	lobbyID := mustCreateServer(t, service, host, 4)
	var asked []string

	_, withoutCheck := service.hub.JoinLobby(refused.id, lobbyID, lobbies.JoinRequest{})
	_, rejected := service.hub.JoinLobby(refused.id, lobbyID, lobbies.JoinRequest{Admit: admitWhen(false, &asked)})
	_, accepted := service.hub.JoinLobby(admitted.id, lobbyID, lobbies.JoinRequest{Admit: admitWhen(true, &asked)})

	if !errors.Is(withoutCheck, lobbies.ErrPassRefused) || !errors.Is(rejected, lobbies.ErrPassRefused) {
		t.Errorf("joins without a pass gave %v and %v, want ErrPassRefused", withoutCheck, rejected)
	}
	if accepted != nil {
		t.Errorf("join with a pass returned error %v", accepted)
	}
	if len(asked) != 2 || asked[0] != serverRegionTag {
		t.Errorf("the pass check was asked %v, want two questions about region eu", asked)
	}
	if service.hub.lobbies[lobbyID].IsMember(refused.id) {
		t.Error("a player without a pass is a member")
	}
}

func TestOnlineLobbyNeverAsksForAPassAndIsNotAServerLobby(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	lobbyID := mustCreate(t, service, players[0], 4)
	var asked []string

	isServer, err := service.hub.JoinLobby(players[1].id, lobbyID, lobbies.JoinRequest{Admit: admitWhen(false, &asked)})

	if err != nil || isServer || len(asked) != 0 {
		t.Errorf("join returned %v, %v and asked %v; want false, nil and no question", isServer, err, asked)
	}
	detail, _ := service.hub.LobbyDetail(players[0].id, lobbyID)
	if detail.LobbyType != 0 || detail.Region != "" {
		t.Errorf("Online lobby has LobbyType %d and Region %q", detail.LobbyType, detail.Region)
	}
}

func TestServerLobbySendsNoSignallingAndNoDisconnect(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	host, guest, third := players[0], players[1], players[2]
	lobbyID := mustCreateServer(t, service, host, 4)
	settle(t, host.socket)
	settle(t, guest.socket)
	settle(t, third.socket)

	mustJoinServer(t, service, guest, lobbyID, true)
	mustJoinServer(t, service, third, lobbyID, true)

	requireNoPeerMessages(t, host.socket)
	requireNoPeerMessages(t, guest.socket)
	requireNoPeerMessages(t, third.socket)

	service.hub.LeaveLobby(third.id, lobbyID)

	requireNoPeerMessages(t, host.socket)
	requireNoPeerMessages(t, guest.socket)
}

// requireNoPeerMessages reads what arrived for the socket and fails if it
// holds a message 17 or 18, or no message 6.
func requireNoPeerMessages(t *testing.T, socket *client) {
	t.Helper()
	got := ids(settle(t, socket))
	if slices.Contains(got, msgStartSignalling) || slices.Contains(got, msgDisconnectPlayer) {
		t.Errorf("messages %v hold a player-to-player instruction", got)
	}
	if !slices.Contains(got, MsgLobbyCurrentLobbyUpdate) {
		t.Errorf("messages %v hold no message 6", got)
	}
}

func TestSignalsAndSignallingRequestsFromServerLobbyMembersAreDropped(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	serverLobbyOfPlayers(t, service, players)

	send(t, guest.socket, map[string]any{"msg_id": 12, "target_user_id": host.id, "payload": []int{1, 2}})
	send(t, guest.socket, map[string]any{"msg_id": 19, "target_user_id": host.id})
	send(t, host.socket, map[string]any{"msg_id": 19, "target_user_id": guest.id})

	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
}

func TestHostsConnectivityCheckInAServerLobbyIsAnsweredAtOnceWithoutAsking(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	host, guest, third := players[0], players[1], players[2]
	serverLobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})

	complete := receiveID(t, host.socket, msgMeshCheckComplete)
	requireKeys(t, complete, "msg_id", "mesh_complete")
	if complete["mesh_complete"] != true {
		t.Errorf("message 28 is %v, want mesh_complete true", complete)
	}
	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
	requireIDs(t, third.socket)
}

func TestMemberWithoutTheMapCannotBecomeReadyInAServerLobby(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, withMap, withoutMap := players[0], players[1], players[2]
	lobbyID := mustCreateServer(t, service, host, 4)
	mustJoinServer(t, service, withMap, lobbyID, true)
	mustJoinServer(t, service, withoutMap, lobbyID, false)
	for _, member := range players {
		settle(t, member.socket)
	}

	send(t, withMap.socket, map[string]any{"msg_id": 5, "ready": true})
	send(t, withoutMap.socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, withoutMap.socket)

	if !memberOf(t, service, host, lobbyID, 1).IsReady {
		t.Error("a member with the map could not become ready")
	}
	if memberOf(t, service, host, lobbyID, 2).IsReady {
		t.Error("a member without the map became ready")
	}

	field := lobbies.FieldMyHasMap
	hasMap := true
	service.hub.ChangeLobby(withoutMap.id, lobbyID, lobbies.FieldRequest{Field: &field, HasMap: &hasMap})
	send(t, withoutMap.socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, withoutMap.socket)

	if !memberOf(t, service, host, lobbyID, 2).IsReady {
		t.Error("a member who then got the map could not become ready")
	}
}

func TestOnlineLobbyLetsAMemberWithoutTheMapBecomeReady(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	lobbyID := mustCreate(t, service, players[0], 4)
	if _, err := service.hub.JoinLobby(players[1].id, lobbyID, lobbies.JoinRequest{HasMap: false}); err != nil {
		t.Fatalf("JoinLobby returned error: %v", err)
	}
	settle(t, players[0].socket)
	settle(t, players[1].socket)

	send(t, players[1].socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, players[1].socket)

	if !memberOf(t, service, players[0], lobbyID, 1).IsReady {
		t.Error("an Online member without the map could not become ready")
	}
}

func TestStartIsIgnoredInAServerLobbyWithoutAStoredSession(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := serverLobbyOfPlayers(t, service, players)
	readyAll(t, players)

	send(t, host.socket, map[string]any{"msg_id": 13})

	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
	if detail, _ := service.hub.LobbyDetail(host.id, lobbyID); detail.MatchID != 0 {
		t.Error("the match started without a server session")
	}
}

func TestStartGivesEachMemberItsOwnSessionSlotAndToken(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, third := players[0], players[1], players[2]
	lobbyID := serverLobbyOfPlayers(t, service, players)
	readyAll(t, players)
	if !service.hub.SetServerSession(host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: tokenTwo})) {
		t.Fatal("the server session was refused")
	}

	send(t, host.socket, map[string]any{"msg_id": 13})

	want := []struct {
		member *testPlayer
		slot   float64
		token  string
	}{{host, 0, ""}, {guest, 1, tokenOne}, {third, 2, tokenTwo}}
	for _, test := range want {
		messages := requireIDs(t, test.member.socket, MsgLobbyCurrentLobbyUpdate, msgStartGame)
		message := messages[1]
		requireKeys(t, message, "msg_id", "screenshot_url", "server_session", "server_slot", "server_token")
		if message["screenshot_url"] != "" || message["server_session"] != sessionID ||
			message["server_slot"] != test.slot || message["server_token"] != test.token {
			t.Errorf("message 13 for slot %v is %v, want session %q, slot %v and token %q",
				test.slot, message, sessionID, test.slot, test.token)
		}
	}
}

func TestOnlineLobbyStartCarriesNoServerKeys(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	lobbyID := lobbyOfPlayers(t, service, players)
	send(t, players[1].socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, players[1].socket)
	settle(t, players[0].socket)

	send(t, players[0].socket, map[string]any{"msg_id": 13})

	messages := requireIDs(t, players[1].socket, MsgLobbyCurrentLobbyUpdate, msgStartGame)
	if len(messages[1]) != 2 {
		t.Errorf("message 13 of an Online lobby is %v, want only msg_id and screenshot_url", messages[1])
	}
	if lobby := service.hub.lobbies[lobbyID]; lobby == nil || !lobby.HasStarted() {
		t.Error("the Online lobby did not start")
	}
}

func TestJoinAfterTheSessionWasStoredClearsItAndStartIsIgnored(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, late := players[0], players[1], players[2]
	lobbyID := serverLobbyOfPlayers(t, service, players[:2])
	settle(t, late.socket)
	readyAll(t, players[:2])
	if !service.hub.SetServerSession(host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne})) {
		t.Fatal("the server session was refused")
	}

	mustJoinServer(t, service, late, lobbyID, true)
	settle(t, host.socket)
	settle(t, guest.socket)
	settle(t, late.socket)
	send(t, late.socket, map[string]any{"msg_id": 5, "ready": true})
	settle(t, late.socket)
	settle(t, host.socket)
	settle(t, guest.socket)
	send(t, host.socket, map[string]any{"msg_id": 13})

	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
	if service.hub.SetServerSession(host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne})) {
		t.Error("a session that does not cover the new member was stored")
	}
}

func TestLeaveAfterTheSessionWasStoredClearsIt(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest, third := players[0], players[1], players[2]
	lobbyID := serverLobbyOfPlayers(t, service, players)
	readyAll(t, players)
	if !service.hub.SetServerSession(host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: tokenTwo})) {
		t.Fatal("the server session was refused")
	}

	service.hub.LeaveLobby(third.id, lobbyID)
	settle(t, host.socket)
	settle(t, guest.socket)
	send(t, host.socket, map[string]any{"msg_id": 13})

	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
}

func TestServerSessionRules(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	host, guest := players[0], players[1]
	lobbyID := serverLobbyOfPlayers(t, service, players)
	onlineLobby := mustCreate(t, service, newPlayers(t, service, 1)[0], 4)
	onlineHost := service.hub.lobbies[onlineLobby].Owner()
	good := map[int]string{1: tokenOne, 2: tokenTwo}

	cases := []struct {
		name    string
		user    int64
		lobby   int64
		request lobbies.FieldRequest
		want    bool
	}{
		{"all rules met", host.id, lobbyID, sessionRequest(sessionID, good), true},
		{"sent by a member", guest.id, lobbyID, sessionRequest(sessionID, good), false},
		{"unknown lobby", host.id, 999, sessionRequest(sessionID, good), false},
		{"Online lobby", onlineHost, onlineLobby, sessionRequest(sessionID, nil), false},
		{"session too short", host.id, lobbyID, sessionRequest("00112233445566f", good), false},
		{"session too long", host.id, lobbyID, sessionRequest("00112233445566ff0", good), false},
		{"session upper case", host.id, lobbyID, sessionRequest("00112233445566FF", good), false},
		{"session not hexadecimal", host.id, lobbyID, sessionRequest("00112233445566fg", good), false},
		{"token of zeros", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: "0000000000000000"}), false},
		{"token upper case", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: "AAAAAAAAAAAAAAAA"}), false},
		{"token too short", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: "222"}), false},
		{"token for the host", host.id, lobbyID, sessionRequest(sessionID, map[int]string{0: tokenOne, 1: tokenOne, 2: tokenTwo}), false},
		{"token for an empty slot", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 3: tokenTwo}), false},
		{"slot out of range", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 8: tokenTwo}), false},
		{"negative slot", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, -1: tokenTwo}), false},
		{"a member without a token", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne}), false},
		{"one token too many", host.id, lobbyID, sessionRequest(sessionID, map[int]string{1: tokenOne, 2: tokenTwo, 3: tokenTwo}), false},
		{"no session", host.id, lobbyID, lobbies.FieldRequest{Field: ptr(lobbies.FieldServerSession), Tokens: sessionRequest("", good).Tokens}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := service.hub.SetServerSession(test.user, test.lobby, test.request); got != test.want {
				t.Errorf("SetServerSession = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSessionIsRefusedWhenTwoEntriesNameOneSlot(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 3)
	lobbyID := serverLobbyOfPlayers(t, service, players)
	request := sessionRequest(sessionID, map[int]string{1: tokenOne})
	slotOne := 1
	token := tokenTwo
	request.Tokens = append(request.Tokens, lobbies.SessionToken{Slot: &slotOne, Token: &token})

	if service.hub.SetServerSession(players[0].id, lobbyID, request) {
		t.Error("a session with two entries for one slot was stored")
	}
}

func TestSessionIsRefusedAfterTheMatchStarted(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 2)
	host := players[0]
	lobbyID := serverLobbyOfPlayers(t, service, players)
	readyAll(t, players)
	request := sessionRequest(sessionID, map[int]string{1: tokenOne})
	if !service.hub.SetServerSession(host.id, lobbyID, request) {
		t.Fatal("the server session was refused")
	}
	send(t, host.socket, map[string]any{"msg_id": 13})
	settle(t, host.socket)

	if service.hub.SetServerSession(host.id, lobbyID, request) {
		t.Error("a session was stored after the match started")
	}
}

// ptr returns a pointer to a copy of value.
func ptr[T any](value T) *T {
	return &value
}

func TestStartIsIgnoredInAServerLobbyOfOneHuman(t *testing.T) {
	service := newTestService(t, lobbySettings())
	players := newPlayers(t, service, 1)
	host := players[0]
	lobbyID := mustCreateServer(t, service, host, 4)
	settle(t, host.socket)
	if !service.hub.SetServerSession(host.id, lobbyID, sessionRequest(sessionID, nil)) {
		t.Fatal("the server session was refused")
	}

	send(t, host.socket, map[string]any{"msg_id": 13})

	requireIDs(t, host.socket)
	if detail, _ := service.hub.LobbyDetail(host.id, lobbyID); detail.MatchID != 0 {
		t.Error("a server lobby of one human started")
	}
}
