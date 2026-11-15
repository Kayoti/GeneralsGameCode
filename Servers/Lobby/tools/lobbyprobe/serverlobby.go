// This file holds the server lobby step of the probe (SERVICE.md sections 10
// and 13). The probe stands in for the relay and the game client: it signs
// its own passes, creates and joins a server lobby with them, posts the server
// session as the host does after the relay's CREATED, starts the match and
// reads its own session, slot and token from message 13.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"servers-lobby/internal/pass"
)

// Values the server lobby step checks (SERVICE.md section 10, CLIENT_HTTP.md 5.2).
const (
	lobbyTypeServer = 2
	passLifetime    = 10 * time.Minute
	// readyPolls and readyPause bound how long the step waits for the guest's ready state to show.
	readyPolls = 50
	readyPause = 100 * time.Millisecond
	// The build fingerprints of createLobbyBody.
	probeExeCRC = 1
	probeIniCRC = 2
	// sessionBytes is the size of a session id and of a join token; each is written as 16 hexadecimal digits.
	sessionBytes = 8
)

// serverCreateBody is createLobbyBody as a server lobby.
var serverCreateBody = strings.Replace(createLobbyBody, `"max_players":4,`, `"max_players":4,"server_mode":true,`, 1)

// newPass signs a pass for the probe's region with the probe's own key, with
// the fingerprints of createLobbyBody.
func (p *probe) newPass() string {
	now := uint32(time.Now().Unix())
	return pass.Sign(pass.Fields{
		Version:   pass.Version,
		Region:    p.server.region,
		ExeCRC:    probeExeCRC,
		IniCRC:    probeIniCRC,
		IssuedAt:  now,
		ExpiresAt: now + uint32(passLifetime/time.Second),
	}, p.server.key)
}

// randomHex returns sessionBytes random bytes as 16 lowercase hexadecimal digits.
func randomHex() (string, error) {
	data := make([]byte, sessionBytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

// serverLobbyToMatchStart runs the server lobby step with the two accounts of the WebSocket step.
func (p *probe) serverLobbyToMatchStart() error {
	if len(p.members) != 2 {
		return p.fail("server lobby", errors.New("the WebSocket step did not connect two accounts"))
	}
	host, guest := p.members[0], p.members[1]
	if err := p.refusedWithoutPass(host); err != nil {
		return p.fail("server lobby create without a pass", err)
	}
	lobbyID, err := p.createServerLobby(host)
	if err != nil {
		return p.fail("server lobby create", err)
	}
	if err := p.joinServerLobby(guest, lobbyID); err != nil {
		return p.fail("server lobby join", err)
	}
	if err := p.serverConnectivityCheck(host); err != nil {
		return p.fail("server lobby connectivity check", err)
	}
	if err := p.readyForServerStart(guest, host, lobbyID); err != nil {
		return p.fail("server lobby ready", err)
	}
	if err := p.sessionAndStart(host, guest, lobbyID); err != nil {
		return p.fail("server lobby match start", err)
	}
	return p.report("Server lobby: pass checked, joined, session stored, match start carried each account's own token")
}

// refusedWithoutPass requires that creating a server lobby with no pass and
// with a pass that is not a pass both fail with result 0.
func (p *probe) refusedWithoutPass(host *account) error {
	for _, header := range []string{"", "not a pass"} {
		status, answer, err := p.request("Lobbies", http.MethodPut, host.session, serverCreateBody, header)
		if err != nil {
			return err
		}
		if status != http.StatusOK || answer["result"] != float64(0) {
			return fmt.Errorf("status %d, want 200 with result 0", status)
		}
		if err := requireKeys(answer, "result", "lobby_id", "turn_username", "turn_token"); err != nil {
			return err
		}
	}
	return p.report("Lobbies (create server lobby without a valid pass): refused with result 0")
}

// createServerLobby creates a server lobby with a signed pass, checks that
// the answer has no TURN credentials, and returns the lobby id.
func (p *probe) createServerLobby(host *account) (int64, error) {
	status, answer, err := p.request("Lobbies", http.MethodPut, host.session, serverCreateBody, p.newPass())
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("status %d", status)
	}
	if err := requireKeys(answer, "result", "lobby_id", "turn_username", "turn_token"); err != nil {
		return 0, err
	}
	lobbyID, _ := answer["lobby_id"].(float64)
	if answer["result"] != float64(resultSucceeded) || lobbyID < 1 {
		return 0, errors.New("the service did not create the lobby")
	}
	if answer["turn_username"] != "" || answer["turn_token"] != "" {
		return 0, errors.New("the answer carries TURN credentials")
	}
	if err := p.requireServerListing(host, int64(lobbyID)); err != nil {
		return 0, err
	}
	return int64(lobbyID), p.report(fmt.Sprintf("Lobbies (create server lobby): ok, lobby %.0f, no TURN credentials", lobbyID))
}

// requireServerListing requires that GET Lobbies shows the lobby with
// LobbyType 2 and the region tag.
func (p *probe) requireServerListing(viewer *account, lobbyID int64) error {
	answer, err := p.call("Lobbies", http.MethodGet, viewer.session, "")
	if err != nil {
		return err
	}
	listed, _ := answer["lobbies"].([]any)
	for _, entry := range listed {
		lobby, _ := entry.(map[string]any)
		if lobby["LobbyID"] != float64(lobbyID) {
			continue
		}
		if lobby["LobbyType"] != float64(lobbyTypeServer) || lobby["Region"] != p.server.regionTag {
			return fmt.Errorf("the lobby has LobbyType %v and Region %v, want %d and %q",
				lobby["LobbyType"], lobby["Region"], lobbyTypeServer, p.server.regionTag)
		}
		return nil
	}
	return fmt.Errorf("lobby %d is not in the list", lobbyID)
}

// joinServerLobby requires 403 without a pass and success with one, and that
// the answer has no TURN credentials.
func (p *probe) joinServerLobby(guest *account, lobbyID int64) error {
	status, _, err := p.request(lobbyPath(lobbyID), http.MethodPut, guest.session, joinLobbyBody, "")
	if err != nil {
		return err
	}
	if status != http.StatusForbidden {
		return fmt.Errorf("a join without a pass answered %d, want 403", status)
	}
	status, answer, err := p.request(lobbyPath(lobbyID), http.MethodPut, guest.session, joinLobbyBody, p.newPass())
	if err != nil {
		return err
	}
	if status != http.StatusOK || answer["success"] != true {
		return fmt.Errorf("a join with a pass answered %d", status)
	}
	if answer["turn_username"] != "" || answer["turn_token"] != "" {
		return errors.New("the join answer carries TURN credentials")
	}
	return p.report("Lobby (join server lobby): 403 without a pass, ok with one, no TURN credentials")
}

// serverConnectivityCheck has the host send message 26 and requires message
// 28 with mesh_complete true, with no reply from any account.
func (p *probe) serverConnectivityCheck(host *account) error {
	if err := host.send(map[string]any{"msg_id": msgMeshCheckBegin}); err != nil {
		return err
	}
	complete, err := host.await(msgMeshCheckComplete, nil)
	if err != nil {
		return err
	}
	if complete["mesh_complete"] != true {
		return fmt.Errorf("the check did not pass: %v", complete)
	}
	return p.report("Connectivity check (server lobby): message 28 at once, mesh_complete")
}

// readyForServerStart has the guest ready up and waits until the lobby shows it.
func (p *probe) readyForServerStart(guest *account, host *account, lobbyID int64) error {
	if err := guest.send(map[string]any{"msg_id": msgMarkReady, "ready": true}); err != nil {
		return err
	}
	for range readyPolls {
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
		time.Sleep(readyPause)
	}
	return errors.New("the guest did not show as ready")
}

// sessionAndStart posts the server session as the host does, starts the match
// and requires that each account's message 13 carries its own session, slot
// and token.
func (p *probe) sessionAndStart(host *account, guest *account, lobbyID int64) error {
	session, err := randomHex()
	if err != nil {
		return err
	}
	token, err := randomHex()
	if err != nil {
		return err
	}
	if err := p.requireRefusedSession(host, lobbyID, session); err != nil {
		return err
	}
	body := fmt.Sprintf(`{"field":19,"server_session":%q,"tokens":[{"slot":1,"token":%q}]}`, session, token)
	answer, err := p.call(lobbyPath(lobbyID), http.MethodPost, host.session, body)
	if err != nil {
		return err
	}
	if err := requireKeys(answer, "result"); err != nil {
		return err
	}
	if answer["result"] != float64(resultSucceeded) {
		return errors.New("the service refused the server session")
	}
	if err := p.report("Lobby (field 19): server session stored"); err != nil {
		return err
	}
	if err := host.send(map[string]any{"msg_id": msgStartGame}); err != nil {
		return err
	}
	wants := []struct {
		member *account
		slot   float64
		token  string
	}{{host, 0, ""}, {guest, 1, token}}
	for _, want := range wants {
		start, err := want.member.await(msgStartGame, func(message map[string]any) bool {
			_, hasSession := message["server_session"]
			return hasSession
		})
		if err != nil {
			return err
		}
		if err := requireKeys(start, "screenshot_url", "server_session", "server_slot", "server_token"); err != nil {
			return err
		}
		if start["server_session"] != session || start["server_slot"] != want.slot || start["server_token"] != want.token {
			return fmt.Errorf("account %.0f got the wrong session, slot or token in message 13", want.member.userID)
		}
	}
	return nil
}

// requireRefusedSession requires that a session with a token of sixteen zeros
// is refused with result 0.
func (p *probe) requireRefusedSession(host *account, lobbyID int64, session string) error {
	body := fmt.Sprintf(`{"field":19,"server_session":%q,"tokens":[{"slot":1,"token":"0000000000000000"}]}`, session)
	answer, err := p.call(lobbyPath(lobbyID), http.MethodPost, host.session, body)
	if err != nil {
		return err
	}
	if answer["result"] != float64(0) {
		return errors.New("a token of zeros was not refused")
	}
	return nil
}
