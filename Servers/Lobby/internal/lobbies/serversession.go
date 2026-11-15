// This file holds the server session of a server lobby (SERVICE.md section
// 10): the session id and the join token of each member, which the host posts
// as field 19 and the service hands to each member in the match start. The
// session is never part of any list, detail or log.

package lobbies

// minServerHumans is how many humans a server lobby needs to start (SERVICE.md section 7.5).
const minServerHumans = 2

// FieldServerSession is field 19 of POST Lobby/{id} (CLIENT_HTTP.md 5.4).
const FieldServerSession = 19

// sessionDigits is the length of a session id and of a join token, in hexadecimal digits.
const sessionDigits = 16

// serverSession is what the host posted: one token per slot, and an empty
// string for a slot that has none (the host's own, and every slot without a human).
type serverSession struct {
	id     string
	tokens [SlotCount]string
}

// isSessionHex reports whether text is exactly 16 lowercase hexadecimal digits.
func isSessionHex(text string) bool {
	if len(text) != sessionDigits {
		return false
	}
	for index := 0; index < len(text); index++ {
		character := text[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// isZeroToken reports whether a token is sixteen zeros, which is refused.
func isZeroToken(token string) bool {
	return token == "0000000000000000"
}

// SetServerSession stores the server session of field 19 and returns true, or
// returns false and changes nothing. The user must be the host of a server
// lobby that has not started. The session id and each token are 16 lowercase
// hexadecimal digits, and a token is not all zeros. Each slot is 0 to 7, is
// not the host's, holds a human, and appears once, and there is exactly one
// entry for every human except the host.
func (lobby *Lobby) SetServerSession(userID int64, request FieldRequest) bool {
	if !lobby.isServer || lobby.hasStarted || userID != lobby.owner || !lobby.IsMember(userID) {
		return false
	}
	if request.ServerSession == nil || !isSessionHex(*request.ServerSession) {
		return false
	}
	hostSlot, _ := lobby.slotOf(lobby.owner)
	session := serverSession{id: *request.ServerSession}
	for _, entry := range request.Tokens {
		if entry.Slot == nil || entry.Token == nil {
			return false
		}
		index, token := *entry.Slot, *entry.Token
		if index < 0 || index >= SlotCount || index == hostSlot || !lobby.slots[index].isHuman() || session.tokens[index] != "" {
			return false
		}
		if !isSessionHex(token) || isZeroToken(token) {
			return false
		}
		session.tokens[index] = token
	}
	if len(request.Tokens) != len(lobby.Humans())-1 {
		return false
	}
	lobby.session = &session
	return true
}

// StartInfo returns what the match start tells a member of a server lobby
// that has a stored session: the session id, the member's slot and the
// member's token, which is empty for the host. ok is false when there is no
// session or the user is not a member.
func (lobby *Lobby) StartInfo(userID int64) (session string, slotIndex int, token string, ok bool) {
	if lobby.session == nil {
		return "", 0, "", false
	}
	slotIndex, found := lobby.slotOf(userID)
	if !found {
		return "", 0, "", false
	}
	return lobby.session.id, slotIndex, lobby.session.tokens[slotIndex], true
}

// ClearServerSession forgets the stored session, once the match has started.
func (lobby *Lobby) ClearServerSession() {
	lobby.session = nil
}
