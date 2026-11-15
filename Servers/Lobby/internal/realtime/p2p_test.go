// Tests of the player-to-player support of Online lobbies: connection setup
// (17, 12, 19, 18) and the connectivity check (26, 27, 28), SERVICE.md
// sections 8.2 and 8.3.

package realtime

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// meshReplyWait and meshRetryDelay are the check's waits in these tests.
const (
	meshReplyWait  = 300 * time.Millisecond
	meshRetryDelay = 50 * time.Millisecond
)

// meshSettings are lobbySettings with a short connectivity check.
func meshSettings() Settings {
	settings := lobbySettings()
	settings.MeshReplyWait = meshReplyWait
	settings.MeshRetryDelay = meshRetryDelay
	return settings
}

// lobbyOfPlayers creates a lobby hosted by the first player, joins the others
// in order and reads every message that arrived meanwhile.
func lobbyOfPlayers(t *testing.T, service *testService, players []*testPlayer) int64 {
	t.Helper()
	lobbyID := mustCreate(t, service, players[0], 8)
	for _, joiner := range players[1:] {
		mustJoin(t, service, joiner, lobbyID)
	}
	for _, member := range players {
		settle(t, member.socket)
	}
	return lobbyID
}

// requireStartSignalling checks a message 17 for every key and value.
func requireStartSignalling(t *testing.T, message map[string]any, lobbyID int64, peerID int64) {
	t.Helper()
	requireKeys(t, message, "msg_id", "lobby_id", "user_id", "preferred_port", "middleware_id")
	if message["lobby_id"] != float64(lobbyID) || message["user_id"] != float64(peerID) ||
		message["preferred_port"] != float64(0) || message["middleware_id"] != "" {
		t.Errorf("message 17 is %v, want lobby %d, user %d, port 0 and an empty middleware id", message, lobbyID, peerID)
	}
}

func TestJoinSendsSixThenSeventeenToBothPlayersOfAPair(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := mustCreate(t, service, host, 4)
	settle(t, host.socket)
	settle(t, guest.socket)

	mustJoin(t, service, guest, lobbyID)

	hostMessages := requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireStartSignalling(t, hostMessages[1], lobbyID, guest.id)
	guestMessages := requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireStartSignalling(t, guestMessages[1], lobbyID, host.id)
}

func TestThirdPlayerIsPairedWithBothOthersAndNobodyIsPairedTwice(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	host, guest, third := players[0], players[1], players[2]
	lobbyID := lobbyOfPlayers(t, service, players[:2])
	settle(t, third.socket)

	mustJoin(t, service, third, lobbyID)

	thirdMessages := requireIDs(t, third.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling, msgStartSignalling)
	requireStartSignalling(t, thirdMessages[1], lobbyID, host.id)
	requireStartSignalling(t, thirdMessages[2], lobbyID, guest.id)
	hostMessages := requireIDs(t, host.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireStartSignalling(t, hostMessages[1], lobbyID, third.id)
	guestMessages := requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate, msgStartSignalling)
	requireStartSignalling(t, guestMessages[1], lobbyID, third.id)
}

func TestCreatingALobbySendsNoSeventeen(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 1)
	mustCreate(t, service, players[0], 4)
	requireIDs(t, players[0].socket)
}

func TestSignalIsForwardedBetweenMembersWithTheSendersIdAndTheSamePayload(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	lobbyOfPlayers(t, service, players)
	sender, target, other := players[0], players[2], players[1]

	send(t, sender.socket, map[string]any{"msg_id": 12, "target_user_id": target.id, "payload": []int{0, 1, 127, 255}})

	message := receiveID(t, target.socket, msgSignal)
	requireKeys(t, message, "msg_id", "target_user_id", "payload")
	payload, isArray := message["payload"].([]any)
	if message["target_user_id"] != float64(sender.id) || !isArray || len(payload) != 4 ||
		payload[0] != float64(0) || payload[1] != float64(1) || payload[2] != float64(127) || payload[3] != float64(255) {
		t.Errorf("forwarded message is %v, want the sender's id and the payload [0 1 127 255]", message)
	}
	requireIDs(t, other.socket)
	requireIDs(t, sender.socket)
}

func TestSignalIsDroppedUnlessBothAreDifferentMembersOfOneLobby(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 5)
	member, mate, outsider, otherLobbyMember, idle := players[0], players[1], players[2], players[3], players[4]
	lobbyOfPlayers(t, service, players[:2])
	mustCreate(t, service, otherLobbyMember, 4)
	for _, each := range players {
		settle(t, each.socket)
	}
	valid := []int{1, 2, 3}

	for _, message := range []map[string]any{
		{"msg_id": 12, "target_user_id": outsider.id, "payload": valid},
		{"msg_id": 12, "target_user_id": otherLobbyMember.id, "payload": valid},
		{"msg_id": 12, "target_user_id": member.id, "payload": valid},
		{"msg_id": 12, "target_user_id": 987654321, "payload": valid},
		{"msg_id": 12, "payload": valid},
		{"msg_id": 12, "target_user_id": mate.id},
		{"msg_id": 12, "target_user_id": mate.id, "payload": "not an array"},
		{"msg_id": 12, "target_user_id": mate.id, "payload": nil},
		{"msg_id": 12, "target_user_id": "x", "payload": valid},
	} {
		send(t, member.socket, message)
	}
	send(t, outsider.socket, map[string]any{"msg_id": 12, "target_user_id": member.id, "payload": valid})
	send(t, otherLobbyMember.socket, map[string]any{"msg_id": 12, "target_user_id": member.id, "payload": valid})
	send(t, idle.socket, map[string]any{"msg_id": 12, "target_user_id": mate.id, "payload": valid})
	for _, each := range players {
		requireIDs(t, each.socket)
	}
}

func TestSignalIsNotCountedAgainstTheMessageRate(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	lobbyOfPlayers(t, service, players)

	for range maxMessagesPerSecond + 10 {
		send(t, players[0].socket, map[string]any{"msg_id": 12, "target_user_id": players[1].id, "payload": []int{1}})
	}
	barrier(t, players[0].socket)
}

func TestRequestForSignallingSendsSeventeenToBothPlayersOfThePair(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	lobbyID := lobbyOfPlayers(t, service, players)
	asker, wanted, third := players[2], players[0], players[1]

	send(t, asker.socket, map[string]any{"msg_id": 19, "target_user_id": wanted.id})

	askerMessages := requireIDs(t, asker.socket, msgStartSignalling)
	requireStartSignalling(t, askerMessages[0], lobbyID, wanted.id)
	wantedMessages := requireIDs(t, wanted.socket, msgStartSignalling)
	requireStartSignalling(t, wantedMessages[0], lobbyID, asker.id)
	requireIDs(t, third.socket)
}

func TestRequestForSignallingIsIgnoredForAPlayerOutsideTheLobby(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	lobbyOfPlayers(t, service, players[:2])
	settle(t, players[2].socket)

	send(t, players[0].socket, map[string]any{"msg_id": 19, "target_user_id": players[2].id})
	send(t, players[0].socket, map[string]any{"msg_id": 19, "target_user_id": players[0].id})
	send(t, players[0].socket, map[string]any{"msg_id": 19})
	send(t, players[2].socket, map[string]any{"msg_id": 19, "target_user_id": players[0].id})
	for _, each := range players {
		requireIDs(t, each.socket)
	}
}

func TestLeaveSendsEighteenForTheLeaverToTheRemainingMembersOnly(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	lobbyID := lobbyOfPlayers(t, service, players)
	leaver := players[1]

	service.hub.LeaveLobby(leaver.id, lobbyID)

	for _, remaining := range []*testPlayer{players[0], players[2]} {
		messages := requireIDs(t, remaining.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer)
		requireKeys(t, messages[1], "msg_id", "lobby_id", "user_id")
		if messages[1]["lobby_id"] != float64(lobbyID) || messages[1]["user_id"] != float64(leaver.id) {
			t.Errorf("message 18 is %v, want lobby %d and user %d", messages[1], lobbyID, leaver.id)
		}
	}
	requireIDs(t, leaver.socket, MsgLobbyListUpdate)
}

// asked reads the message 27 every listed player receives and returns the
// mesh_check_id and attempt of the first one, after checking they agree.
func asked(t *testing.T, members []*testPlayer, wantAttempt int) float64 {
	t.Helper()
	var checkID float64
	for index, member := range members {
		message := receiveID(t, member.socket, msgMeshCheckResponse)
		requireKeys(t, message, "msg_id", "mesh_check_id", "attempt")
		if message["attempt"] != float64(wantAttempt) {
			t.Fatalf("attempt is %v, want %d", message["attempt"], wantAttempt)
		}
		if index == 0 {
			checkID = message["mesh_check_id"].(float64)
		} else if message["mesh_check_id"] != checkID {
			t.Fatalf("members got different mesh_check_id values: %v and %v", checkID, message["mesh_check_id"])
		}
	}
	return checkID
}

// answer sends the reply of a member to a connectivity check.
func answer(t *testing.T, member *testPlayer, checkID float64, attempt int, connected []int64, connecting []int64) {
	t.Helper()
	if connected == nil {
		connected = []int64{}
	}
	if connecting == nil {
		connecting = []int64{}
	}
	send(t, member.socket, map[string]any{
		"msg_id": msgMeshCheckResponse, "mesh_check_id": checkID, "attempt": attempt,
		"connectivity_map": connected, "connecting_map": connecting,
	})
}

// requireMeshFailed checks a message 28 with mesh_complete false and returns
// its missing pairs as "source>target" strings.
func requireMeshFailed(t *testing.T, message map[string]any, reason string) []string {
	t.Helper()
	requireKeys(t, message, "msg_id", "mesh_complete", "reason", "missing_connections")
	if message["mesh_complete"] != false || message["reason"] != reason {
		t.Fatalf("message 28 is %v, want mesh_complete false and reason %q", message, reason)
	}
	var pairs []string
	for _, entry := range message["missing_connections"].([]any) {
		pair := entry.(map[string]any)
		requireKeys(t, pair, "source_user_id", "target_user_id")
		pairs = append(pairs, formatPair(int64(pair["source_user_id"].(float64)), int64(pair["target_user_id"].(float64))))
	}
	return pairs
}

// formatPair writes a missing pair as the tests compare it.
func formatPair(source int64, target int64) string {
	return fmt.Sprintf("%d>%d", source, target)
}

// requireMeshPassed checks a message 28 with mesh_complete true.
func requireMeshPassed(t *testing.T, message map[string]any) {
	t.Helper()
	requireKeys(t, message, "msg_id", "mesh_complete")
	if message["mesh_complete"] != true {
		t.Fatalf("message 28 is %v, want mesh_complete true", message)
	}
}

func TestCheckOfTwoPlayersPassesWhenBothListTheOther(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players, 1)
	answer(t, host, checkID, 1, []int64{guest.id}, nil)
	requireIDs(t, host.socket)
	answer(t, guest, checkID, 1, []int64{host.id}, nil)

	requireMeshPassed(t, receiveID(t, host.socket, msgMeshCheckComplete))
	requireIDs(t, guest.socket)
}

func TestCheckOfThreePlayersPassesOnlyWhenEveryoneListsEveryone(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	a, b, c := players[0], players[1], players[2]
	lobbyOfPlayers(t, service, players)

	send(t, a.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players, 1)
	answer(t, a, checkID, 1, []int64{b.id, c.id}, nil)
	answer(t, b, checkID, 1, []int64{a.id, c.id}, nil)
	answer(t, c, checkID, 1, []int64{b.id, a.id}, nil)

	requireMeshPassed(t, receiveID(t, a.socket, msgMeshCheckComplete))
}

func TestCheckWithAMissingPairFailsAndNamesThePair(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	a, b, c := players[0], players[1], players[2]
	lobbyOfPlayers(t, service, players)

	send(t, a.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players, 1)
	answer(t, a, checkID, 1, []int64{b.id, c.id}, nil)
	answer(t, b, checkID, 1, []int64{a.id}, nil)
	answer(t, c, checkID, 1, []int64{a.id, b.id}, nil)

	pairs := requireMeshFailed(t, receiveID(t, a.socket, msgMeshCheckComplete), "missing_connections")
	if len(pairs) != 1 || pairs[0] != formatPair(b.id, c.id) {
		t.Errorf("missing pairs %v, want only %s", pairs, formatPair(b.id, c.id))
	}
	requireIDs(t, a.socket)
}

func TestCheckWaitsAgainWhileAPeerIsStillConnectingAndThenPasses(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	firstID := asked(t, players, 1)
	answer(t, host, firstID, 1, nil, []int64{guest.id})
	answer(t, guest, firstID, 1, []int64{host.id}, nil)

	secondID := asked(t, players, 2)
	if secondID == firstID {
		t.Error("the second attempt reuses the mesh_check_id")
	}
	answer(t, host, secondID, 2, []int64{guest.id}, nil)
	answer(t, guest, secondID, 2, []int64{host.id}, nil)
	requireMeshPassed(t, receiveID(t, host.socket, msgMeshCheckComplete))
}

func TestCheckStopsAfterThreeAttempts(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	for attempt := 1; attempt <= 3; attempt++ {
		checkID := asked(t, players, attempt)
		answer(t, host, checkID, attempt, nil, []int64{guest.id})
		answer(t, guest, checkID, attempt, []int64{host.id}, nil)
	}

	pairs := requireMeshFailed(t, receiveID(t, host.socket, msgMeshCheckComplete), "missing_connections")
	if len(pairs) != 1 || pairs[0] != formatPair(host.id, guest.id) {
		t.Errorf("missing pairs %v, want only %s", pairs, formatPair(host.id, guest.id))
	}
	expectSilence(t, guest.socket)
}

func TestCheckWithoutAReplyTimesOutAndALateReplyChangesNothing(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players, 1)
	answer(t, host, checkID, 1, []int64{guest.id}, nil)

	pairs := requireMeshFailed(t, receiveID(t, host.socket, msgMeshCheckComplete), "timeout")
	if len(pairs) != 1 || pairs[0] != formatPair(guest.id, host.id) {
		t.Errorf("missing pairs %v, want only %s", pairs, formatPair(guest.id, host.id))
	}
	answer(t, guest, checkID, 1, []int64{host.id}, nil)
	requireIDs(t, host.socket)
}

func TestRepliesThatDoNotMatchTheRunningAttemptAreIgnored(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	a, b, outsider := players[0], players[1], players[2]
	lobbyOfPlayers(t, service, players[:2])
	settle(t, outsider.socket)

	send(t, a.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players[:2], 1)
	answer(t, a, checkID+1, 1, []int64{b.id}, nil)
	answer(t, a, checkID, 2, []int64{b.id}, nil)
	answer(t, outsider, checkID, 1, []int64{a.id, b.id}, nil)
	send(t, a.socket, map[string]any{"msg_id": 27, "connectivity_map": []int64{b.id}})
	answer(t, a, checkID, 1, []int64{b.id}, nil)
	answer(t, a, checkID, 1, nil, nil)
	answer(t, b, checkID, 1, []int64{a.id}, nil)

	requireMeshPassed(t, receiveID(t, a.socket, msgMeshCheckComplete))
}

func TestMemberLeavingDuringTheCheckEndsItWithMemberLeft(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 3)
	a, b, c := players[0], players[1], players[2]
	lobbyID := lobbyOfPlayers(t, service, players)

	send(t, a.socket, map[string]any{"msg_id": 26})
	checkID := asked(t, players, 1)
	answer(t, a, checkID, 1, []int64{b.id, c.id}, nil)
	service.hub.LeaveLobby(c.id, lobbyID)

	messages := settle(t, a.socket)
	if !slices.Equal(ids(messages), []int{msgMeshCheckComplete, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer}) {
		t.Fatalf("host got %v, want 28, 6 and 18", ids(messages))
	}
	requireMeshFailed(t, messages[0], "member_left")
	answer(t, b, checkID, 1, []int64{a.id, c.id}, nil)
	requireIDs(t, a.socket)
}

func TestHostLeavingDuringTheCheckEndsItWithoutAMessage(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyID := lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	asked(t, players, 1)
	service.hub.LeaveLobby(host.id, lobbyID)

	requireIDs(t, host.socket, MsgLobbyListUpdate)
	requireIDs(t, guest.socket, MsgLobbyCurrentLobbyUpdate, msgDisconnectPlayer)
	expectSilence(t, host.socket)
}

func TestANew26CancelsTheCheckInProgress(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)

	send(t, host.socket, map[string]any{"msg_id": 26})
	firstID := asked(t, players, 1)
	send(t, host.socket, map[string]any{"msg_id": 26})
	secondID := asked(t, players, 1)
	if secondID == firstID {
		t.Fatal("the new check has the old mesh_check_id")
	}
	answer(t, host, firstID, 1, []int64{guest.id}, nil)
	answer(t, guest, firstID, 1, []int64{host.id}, nil)
	requireIDs(t, host.socket)
	answer(t, host, secondID, 1, []int64{guest.id}, nil)
	answer(t, guest, secondID, 1, []int64{host.id}, nil)

	requireMeshPassed(t, receiveID(t, host.socket, msgMeshCheckComplete))
	expectSilence(t, host.socket)
}

func TestCheckOfOneHumanPassesAtOnce(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 1)
	mustCreate(t, service, players[0], 4)
	settle(t, players[0].socket)

	send(t, players[0].socket, map[string]any{"msg_id": 26})

	requireMeshPassed(t, receiveID(t, players[0].socket, msgMeshCheckComplete))
}

func TestCheckStartedByANonHostIsIgnored(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	lobbyOfPlayers(t, service, players)

	send(t, players[1].socket, map[string]any{"msg_id": 26})

	requireIDs(t, players[0].socket)
	requireIDs(t, players[1].socket)
}

func TestCheckIsNotStartedForALobbyThatHasStarted(t *testing.T) {
	service := newTestService(t, meshSettings())
	players := newPlayers(t, service, 2)
	host, guest := players[0], players[1]
	lobbyOfPlayers(t, service, players)
	sendSync(t, guest.socket, map[string]any{"msg_id": 5, "ready": true})
	sendSync(t, host.socket, map[string]any{"msg_id": 13})
	settle(t, guest.socket)

	send(t, host.socket, map[string]any{"msg_id": 26})

	requireIDs(t, host.socket)
	requireIDs(t, guest.socket)
}
