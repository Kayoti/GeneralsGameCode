// This file holds the TURN credentials that the answers to creating and
// joining an Online lobby carry (SERVICE.md section 8.1). They are the
// time-limited credentials of coturn's use-auth-secret option.

package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"time"
)

// turnCredentialLifetime is how long a TURN username is valid (SERVICE.md section 8.1).
const turnCredentialLifetime = 6 * time.Hour

// turnCredentials returns turn_username and turn_token for the account at
// now, or two empty strings when the secret is empty, so only direct
// connections work. The username is "<expiry in Unix seconds>:<user id>" and
// the token is the Base64 of HMAC-SHA1 over the username keyed with secret.
func turnCredentials(secret string, userID int64, now time.Time) (username string, token string) {
	if secret == "" {
		return "", ""
	}
	username = strconv.FormatInt(now.Add(turnCredentialLifetime).Unix(), 10) + ":" + strconv.FormatInt(userID, 10)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// turnFor returns the TURN credentials for the answer to creating or joining
// a lobby: empty for a server lobby, which has no player-to-player
// connections (SERVICE.md section 10), and as turnCredentials says for any other.
func (routes *server) turnFor(isServerLobby bool, userID int64) (username string, token string) {
	if isServerLobby {
		return "", ""
	}
	return turnCredentials(routes.turnSecret, userID, time.Now())
}
