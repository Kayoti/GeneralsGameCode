// This file holds the connectivity check of an Online lobby (SERVICE.md
// section 8.3): the host asks with message 26, every human is asked with 27
// whether it is connected to every other human, and the host gets 28 with the
// result. Each check is tied to its lobby and ends in exactly one of: a 28 to
// the host, being replaced by a new 26, or the host leaving.

package realtime

import (
	"encoding/json"
	"slices"
	"time"

	"servers-lobby/internal/lobbies"
)

// maxMeshAttempts is how many times the check asks the members in total (SERVICE.md section 8.3).
const maxMeshAttempts = 3

// firstMeshCheckID is the first mesh_check_id handed out.
const firstMeshCheckID int64 = 1

// Reasons of message 28 (CLIENT_REALTIME.md 1.7 row 28, SERVICE.md section 8.3).
const (
	meshReasonTimeout            = "timeout"
	meshReasonMissingConnections = "missing_connections"
	meshReasonMemberLeft         = "member_left"
)

// meshCheck is the check of one lobby. Its fields are guarded by Hub.mu.
type meshCheck struct {
	lobbyID int64
	hostID  int64
	// attempt counts from 1. id is the mesh_check_id of the current attempt.
	attempt int
	id      int64
	// members are the humans asked in the current attempt, and replies holds
	// the answers received from them so far.
	members []int64
	replies map[int64]meshCheckReply
	// isCollecting is true while replies are accepted, and false during the
	// pause before the next attempt.
	isCollecting bool
	timer        *time.Timer
	// step changes whenever the timer is replaced, so that a timer that has
	// fired and waits for the lock can tell it is out of date.
	step uint64
}

// startMeshCheck starts the check for a lobby on the host's message 26 and
// replaces a check that was running. A lobby of one human passes at once, and
// so does a server lobby, which has no connections to check (SERVICE.md section 10).
// The caller holds hub.mu.
func (hub *Hub) startMeshCheck(lobby *lobbies.Lobby) {
	hub.cancelMeshCheck(lobby.ID())
	humans := lobby.Humans()
	if lobby.IsServer() || len(humans) < 2 {
		hub.sendMeshPassed(lobby.Owner())
		return
	}
	check := &meshCheck{lobbyID: lobby.ID(), hostID: lobby.Owner()}
	hub.meshChecks[check.lobbyID] = check
	hub.beginMeshAttempt(check, humans)
}

// beginMeshAttempt asks every human of the lobby for its connections with a
// new mesh_check_id and waits for the replies. The caller holds hub.mu.
func (hub *Hub) beginMeshAttempt(check *meshCheck, humans []int64) {
	check.attempt++
	check.id = hub.nextMeshCheckID
	hub.nextMeshCheckID++
	check.members = humans
	check.replies = make(map[int64]meshCheckReply, len(humans))
	check.isCollecting = true
	for _, userID := range humans {
		if member, found := hub.players[userID]; found {
			hub.queueMessage(member, msgMeshCheckResponse, meshCheckRequest{
				MsgID:       msgMeshCheckResponse,
				MeshCheckID: check.id,
				Attempt:     check.attempt,
			})
		}
	}
	hub.setMeshTimer(check, hub.settings.MeshReplyWait, func() { hub.finishMeshAttempt(check) })
}

// setMeshTimer replaces the check's timer with one that runs action after
// delay, unless the check has been replaced or has moved on by then.
// The caller holds hub.mu.
func (hub *Hub) setMeshTimer(check *meshCheck, delay time.Duration, action func()) {
	if check.timer != nil {
		check.timer.Stop()
	}
	check.step++
	step := check.step
	check.timer = time.AfterFunc(delay, func() {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		if hub.meshChecks[check.lobbyID] != check || check.step != step {
			return
		}
		action()
	})
}

// cancelMeshCheck stops the lobby's check without any message. The caller holds hub.mu.
func (hub *Hub) cancelMeshCheck(lobbyID int64) {
	check, found := hub.meshChecks[lobbyID]
	if !found {
		return
	}
	if check.timer != nil {
		check.timer.Stop()
	}
	check.step++
	delete(hub.meshChecks, lobbyID)
}

// handleMeshCheckReply handles message 27 from a member. A reply that is not
// for the current attempt of the sender's lobby, that comes from someone who
// was not asked, or that repeats an earlier reply, is ignored.
func (hub *Hub) handleMeshCheckReply(live *connection, data []byte) {
	var reply meshCheckReply
	if err := json.Unmarshal(data, &reply); err != nil || reply.MeshCheckID == nil || reply.Attempt == nil {
		hub.logger.Debug("ignored a connectivity reply with no valid id or attempt", "userId", live.player.userID)
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sender := live.player
	lobby := hub.lobbyOf(sender)
	if lobby == nil {
		return
	}
	check, found := hub.meshChecks[lobby.ID()]
	if !found || !check.isCollecting || check.id != *reply.MeshCheckID || check.attempt != *reply.Attempt {
		hub.logger.Debug("ignored a connectivity reply that is not for the running attempt", "userId", sender.userID)
		return
	}
	if _, replied := check.replies[sender.userID]; replied || !slices.Contains(check.members, sender.userID) {
		return
	}
	check.replies[sender.userID] = reply
	if len(check.replies) == len(check.members) {
		hub.finishMeshAttempt(check)
	}
}

// finishMeshAttempt judges the replies of the current attempt: the check
// passes, or is tried again after the retry delay, or fails. The caller holds hub.mu.
func (hub *Hub) finishMeshAttempt(check *meshCheck) {
	check.isCollecting = false
	missing := missingConnections(check)
	if len(missing) == 0 {
		hub.sendMeshPassed(check.hostID)
		hub.cancelMeshCheck(check.lobbyID)
		return
	}
	if check.attempt < maxMeshAttempts && anyStillConnecting(check) {
		hub.setMeshTimer(check, hub.settings.MeshRetryDelay, func() { hub.retryMeshCheck(check) })
		return
	}
	reason := meshReasonMissingConnections
	if len(check.replies) < len(check.members) {
		reason = meshReasonTimeout
	}
	hub.sendMeshFailed(check.hostID, reason, missing)
	hub.cancelMeshCheck(check.lobbyID)
}

// retryMeshCheck starts the next attempt with the humans the lobby has now.
// The caller holds hub.mu.
func (hub *Hub) retryMeshCheck(check *meshCheck) {
	lobby, found := hub.lobbies[check.lobbyID]
	if !found {
		hub.cancelMeshCheck(check.lobbyID)
		return
	}
	hub.beginMeshAttempt(check, lobby.Humans())
}

// missingConnections returns every pair of the attempt's members in which the
// first does not list the second as connected. A member that did not reply lists nobody.
func missingConnections(check *meshCheck) []missingConnection {
	missing := []missingConnection{}
	for _, source := range check.members {
		connected := check.replies[source].ConnectivityMap
		for _, target := range check.members {
			if target != source && !slices.Contains(connected, target) {
				missing = append(missing, missingConnection{SourceUserID: source, TargetUserID: target})
			}
		}
	}
	return missing
}

// anyStillConnecting reports whether some member lists a peer in connecting_map.
func anyStillConnecting(check *meshCheck) bool {
	for _, reply := range check.replies {
		if len(reply.ConnectingMap) > 0 {
			return true
		}
	}
	return false
}

// meshMemberLeft ends the lobby's check when a member who was asked leaves:
// the host gets 28 with reason member_left. When the host is the one who
// leaves nobody is left to tell. The caller holds hub.mu.
func (hub *Hub) meshMemberLeft(lobbyID int64, leaverID int64) {
	check, found := hub.meshChecks[lobbyID]
	if !found || !slices.Contains(check.members, leaverID) {
		return
	}
	if leaverID != check.hostID {
		hub.sendMeshFailed(check.hostID, meshReasonMemberLeft, []missingConnection{})
	}
	hub.cancelMeshCheck(lobbyID)
}

// sendMeshPassed sends message 28 with mesh_complete true to the host.
// The caller holds hub.mu.
func (hub *Hub) sendMeshPassed(hostID int64) {
	if host, found := hub.players[hostID]; found {
		hub.queueMessage(host, msgMeshCheckComplete, meshPassedMessage{MsgID: msgMeshCheckComplete, MeshComplete: true})
	}
}

// sendMeshFailed sends message 28 with mesh_complete false, the reason and
// the missing pairs to the host. The caller holds hub.mu.
func (hub *Hub) sendMeshFailed(hostID int64, reason string, missing []missingConnection) {
	if host, found := hub.players[hostID]; found {
		hub.queueMessage(host, msgMeshCheckComplete, meshFailedMessage{
			MsgID:              msgMeshCheckComplete,
			MeshComplete:       false,
			Reason:             reason,
			MissingConnections: missing,
		})
	}
}
