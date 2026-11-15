// This file holds the lobby step of the probe: one account creates a lobby,
// the other finds it in the list and joins it, both read it, the second
// readies and the first starts the match (SERVICE.md sections 5.1, 7 and 13).

package main

import (
	"errors"
	"fmt"
	"net/http"
)

// Message ids of CLIENT_REALTIME.md section 1.7 that the lobby step uses.
const (
	msgMarkReady          = 5
	msgLobbyCurrentUpdate = 6
	msgLobbyListUpdate    = 7
	msgStartGame          = 13
	msgMeshCheckBegin     = 26
	msgMeshCheckReply     = 27
	msgMeshCheckComplete  = 28
)

// slotCount is the number of member entries of a lobby (SERVICE.md section 7.1).
const slotCount = 8

// humanSlotState is the SlotState of a slot that holds a player (CLIENT_HTTP.md 5.2).
const humanSlotState = 5

// createLobbyBody is a PUT Lobbies body with the keys of CLIENT_HTTP.md 4.13.
const createLobbyBody = `{"name":"Probe game","map_name":"Alpine Assault","map_path":"alpine assault","map_official":true,` +
	`"max_players":4,"preferred_port":0,"vanilla_teams":false,"track_stats":false,"starting_cash":10000,` +
	`"passworded":false,"password":"","allow_observers":true,"exe_crc":1,"ini_crc":2,"max_cam_height":310.0,"anticheat_id":0}`

// joinLobbyBody is a PUT Lobby/{id} body (CLIENT_HTTP.md 4.15).
const joinLobbyBody = `{"preferred_port":0,"anticheat_id":0,"has_map":true}`

// lobbyToMatchStart runs the lobby step with the two accounts of the WebSocket step.
func (p *probe) lobbyToMatchStart() error {
	if len(p.members) != 2 {
		return p.fail("lobby", errors.New("the WebSocket step did not connect two accounts"))
	}
	host, guest := p.members[0], p.members[1]
	lobbyID, err := p.createLobby(host)
	if err != nil {
		return p.fail("create lobby", err)
	}
	if err := p.joinFromList(guest, lobbyID); err != nil {
		return p.fail("join lobby", err)
	}
	if err := p.readDetails(host, guest, lobbyID); err != nil {
		return p.fail("lobby detail", err)
	}
	if err := p.readyAndStart(host, guest, lobbyID); err != nil {
		return p.fail("match start", err)
	}
	return p.report("Lobby: created, listed, joined, ready, match start received by both")
}

// createLobby has the account create a lobby and returns its id.
func (p *probe) createLobby(host *account) (int64, error) {
	answer, err := p.call("Lobbies", http.MethodPut, host.session, createLobbyBody)
	if err != nil {
		return 0, err
	}
	if err := requireKeys(answer, "result", "lobby_id", "turn_username", "turn_token"); err != nil {
		return 0, err
	}
	lobbyID, _ := answer["lobby_id"].(float64)
	if answer["result"] != float64(resultSucceeded) || lobbyID < 1 {
		return 0, errors.New("the service did not create the lobby")
	}
	return int64(lobbyID), p.report(fmt.Sprintf("Lobbies (create): ok, lobby %.0f", lobbyID))
}

// joinFromList waits for the list notice, finds the lobby in GET Lobbies and joins it.
func (p *probe) joinFromList(guest *account, lobbyID int64) error {
	if _, err := guest.await(msgLobbyListUpdate, nil); err != nil {
		return err
	}
	answer, err := p.call("Lobbies", http.MethodGet, guest.session, "")
	if err != nil {
		return err
	}
	if err := requireKeys(answer, "lobbies", "latencies"); err != nil {
		return err
	}
	if !listHasLobby(answer, lobbyID) {
		return fmt.Errorf("lobby %d is not in the list", lobbyID)
	}
	if err := p.report("Lobbies (list): ok, the lobby is listed"); err != nil {
		return err
	}
	joined, err := p.call(lobbyPath(lobbyID), http.MethodPut, guest.session, joinLobbyBody)
	if err != nil {
		return err
	}
	if joined["success"] != true {
		return errors.New("the join was not a success")
	}
	return p.report("Lobby (join): ok")
}

// lobbyPath returns the endpoint of Lobby/{id}.
func lobbyPath(lobbyID int64) string {
	return fmt.Sprintf("Lobby/%d", lobbyID)
}

// listHasLobby reports whether a GET Lobbies answer lists the lobby id.
func listHasLobby(answer map[string]any, lobbyID int64) bool {
	listed, _ := answer["lobbies"].([]any)
	for _, entry := range listed {
		lobby, _ := entry.(map[string]any)
		if lobby["LobbyID"] == float64(lobbyID) {
			return true
		}
	}
	return false
}

// readDetails has both accounts read the lobby after the notice 6 and checks
// that it has eight member entries and two humans.
func (p *probe) readDetails(host *account, guest *account, lobbyID int64) error {
	for _, member := range []*account{host, guest} {
		if _, err := member.await(msgLobbyCurrentUpdate, nil); err != nil {
			return err
		}
		answer, err := p.call(lobbyPath(lobbyID), http.MethodGet, member.session, "")
		if err != nil {
			return err
		}
		if err := requireDetailMembers(answer); err != nil {
			return fmt.Errorf("account %.0f: %w", member.userID, err)
		}
	}
	return p.report("Lobby (detail): ok, eight member entries, two humans")
}

// requireDetailMembers checks that a GET Lobby/{id} answer has eight member
// entries, two of them at the slot state of a human.
func requireDetailMembers(answer map[string]any) error {
	lobby, _ := answer["lobby"].(map[string]any)
	if err := requireKeys(lobby, "LobbyID", "Owner", "RNGSeed", "MatchID", "Members"); err != nil {
		return err
	}
	members, _ := lobby["Members"].([]any)
	if len(members) != slotCount {
		return fmt.Errorf("the lobby has %d member entries, want %d", len(members), slotCount)
	}
	humans := 0
	for _, entry := range members {
		member, _ := entry.(map[string]any)
		if err := requireKeys(member, "UserID", "DisplayName", "IsReady", "SlotIndex", "SlotState", "Region", "HasMap"); err != nil {
			return err
		}
		if member["SlotState"] == float64(humanSlotState) {
			humans++
		}
	}
	if humans != 2 {
		return fmt.Errorf("the lobby has %d humans, want 2", humans)
	}
	return nil
}

// readyAndStart has the guest ready up, checks that the host sees it, runs the
// connectivity check, has the host start the match and waits for message 13
// at both accounts.
func (p *probe) readyAndStart(host *account, guest *account, lobbyID int64) error {
	if err := guest.send(map[string]any{"msg_id": msgMarkReady, "ready": true}); err != nil {
		return err
	}
	if err := p.requireGuestReady(host, lobbyID); err != nil {
		return err
	}
	if err := p.connectivityCheck(host, guest); err != nil {
		return err
	}
	if err := host.send(map[string]any{"msg_id": msgStartGame}); err != nil {
		return err
	}
	for _, member := range []*account{host, guest} {
		start, err := member.await(msgStartGame, nil)
		if err != nil {
			return err
		}
		if err := requireKeys(start, "screenshot_url"); err != nil {
			return err
		}
	}
	return nil
}

// connectivityCheck has the host begin the check (message 26), has both
// accounts answer message 27 as clients that are connected to each other, and
// waits for message 28 at the host. Only mesh_complete true lets the host start.
func (p *probe) connectivityCheck(host *account, guest *account) error {
	if err := host.send(map[string]any{"msg_id": msgMeshCheckBegin}); err != nil {
		return err
	}
	for _, member := range []*account{host, guest} {
		peer := guest
		if member == guest {
			peer = host
		}
		request, err := member.await(msgMeshCheckReply, nil)
		if err != nil {
			return err
		}
		if err := requireKeys(request, "mesh_check_id", "attempt"); err != nil {
			return err
		}
		reply := map[string]any{
			"msg_id":           msgMeshCheckReply,
			"mesh_check_id":    request["mesh_check_id"],
			"attempt":          request["attempt"],
			"connectivity_map": []float64{peer.userID},
			"connecting_map":   []float64{},
		}
		if err := member.send(reply); err != nil {
			return err
		}
	}
	complete, err := host.await(msgMeshCheckComplete, nil)
	if err != nil {
		return err
	}
	if complete["mesh_complete"] != true {
		return fmt.Errorf("the connectivity check did not pass: %v", complete)
	}
	return p.report("Connectivity check: ok, both accounts connected, message 28 reports mesh_complete")
}

// requireGuestReady waits for a notice at the host, reads the lobby as the
// host, and requires that the second slot is ready. The host may hold older
// notices, so it reads again until the guest shows as ready or the wait ends.
func (p *probe) requireGuestReady(host *account, lobbyID int64) error {
	for {
		if _, err := host.await(msgLobbyCurrentUpdate, nil); err != nil {
			return fmt.Errorf("the second account did not show as ready: %w", err)
		}
		answer, err := p.call(lobbyPath(lobbyID), http.MethodGet, host.session, "")
		if err != nil {
			return err
		}
		lobby, _ := answer["lobby"].(map[string]any)
		members, _ := lobby["Members"].([]any)
		if len(members) > 1 {
			if second, _ := members[1].(map[string]any); second["IsReady"] == true {
				return nil
			}
		}
	}
}
