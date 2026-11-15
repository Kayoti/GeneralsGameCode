// This file holds the session tokens of SERVICE.md section 4. They are kept in memory only.

package accounts

import (
	"fmt"
	"time"
)

// sessionLifetime is how long a session token is valid (SERVICE.md section 4).
const sessionLifetime = 15 * time.Minute

// session is an issued session token and the account it belongs to.
type session struct {
	userID    int64
	expiresAt time.Time
}

// newSessionToken returns a random session token in hexadecimal.
func newSessionToken() (string, error) {
	token, err := randomHex(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("make session token: %w", err)
	}
	return token, nil
}

// storeSession keeps a session token for userID until sessionLifetime after
// now, and removes the sessions that have already expired. The caller holds the lock.
func (service *Service) storeSession(sessionToken string, userID int64, now time.Time) {
	for token, entry := range service.sessions {
		if !now.Before(entry.expiresAt) {
			delete(service.sessions, token)
		}
	}
	service.sessions[sessionToken] = session{userID: userID, expiresAt: now.Add(sessionLifetime)}
}
