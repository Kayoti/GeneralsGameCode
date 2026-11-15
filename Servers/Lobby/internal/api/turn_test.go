// Tests of the TURN credentials in the answers to creating and joining a
// lobby (SERVICE.md section 8.1).

package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"servers-lobby/internal/accounts"
)

// testTurnSecret is the turn_secret of the tests that need credentials.
const testTurnSecret = "test-secret"

func TestTurnCredentialsMatchAValueComputedOutsideTheCode(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	username, token := turnCredentials(testTurnSecret, 42, now)

	// The token was computed with Python's hmac and hashlib over this username.
	if username != "1700021600:42" || token != "Hec+w4c8xPdctZ+ub0a2IEdyR1Q=" {
		t.Errorf("credentials = %q, %q; want 1700021600:42 and Hec+w4c8xPdctZ+ub0a2IEdyR1Q=", username, token)
	}
}

func TestTurnCredentialsAreEmptyWithoutASecret(t *testing.T) {
	username, token := turnCredentials("", 42, time.Now())

	if username != "" || token != "" {
		t.Errorf("credentials = %q, %q; want two empty strings", username, token)
	}
}

// newTurnEnv starts the service with a turn_secret.
func newTurnEnv(t *testing.T, secret string) *lobbyEnv {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler := buildHandler(Options{WSURI: testWSURI, TurnSecret: secret}, accounts.New(openTestDB(t), 0), logger)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &lobbyEnv{t: t, handler: handler, server: server}
}

// requireValidCredentials checks that username and token are those of the
// account, expire about six hours ahead, and that the token is the HMAC-SHA1
// of the username under testTurnSecret, computed here again.
func requireValidCredentials(t *testing.T, answer map[string]any, userID int64) {
	t.Helper()
	username, _ := answer["turn_username"].(string)
	token, _ := answer["turn_token"].(string)
	expiryText, userText, found := strings.Cut(username, ":")
	expiry, err := strconv.ParseInt(expiryText, 10, 64)
	if !found || err != nil || userText != strconv.FormatInt(userID, 10) {
		t.Fatalf("turn_username = %q, want <expiry>:%d", username, userID)
	}
	if ahead := time.Until(time.Unix(expiry, 0)); ahead < 5*time.Hour+55*time.Minute || ahead > 6*time.Hour+time.Minute {
		t.Errorf("expiry is %v ahead, want about 6 hours", ahead)
	}
	mac := hmac.New(sha1.New, []byte(testTurnSecret))
	mac.Write([]byte(username))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); token != want {
		t.Errorf("turn_token = %q, want %q", token, want)
	}
}

func TestCreateAndJoinCarryTurnCredentialsWhenThereIsASecret(t *testing.T) {
	env := newTurnEnv(t, testTurnSecret)
	host, guest := env.login(), env.login()

	status, created := env.call(host, http.MethodPut, "Lobbies", createBody)
	if status != http.StatusOK || created["result"] != float64(1) {
		t.Fatalf("create answered %d %v", status, created)
	}
	requireValidCredentials(t, created, host.id)

	lobbyID := int64(created["lobby_id"].(float64))
	status, joined := env.call(guest, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID, 10), `{"has_map":true}`)
	if status != http.StatusOK || joined["success"] != true {
		t.Fatalf("join answered %d %v", status, joined)
	}
	requireValidCredentials(t, joined, guest.id)
}

func TestCreateAndJoinCarryEmptyTurnCredentialsWithoutASecret(t *testing.T) {
	env := newTurnEnv(t, "")
	host, guest := env.login(), env.login()

	_, created := env.call(host, http.MethodPut, "Lobbies", createBody)
	lobbyID := int64(created["lobby_id"].(float64))
	_, joined := env.call(guest, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID, 10), `{"has_map":true}`)

	for _, answer := range []map[string]any{created, joined} {
		if answer["turn_username"] != "" || answer["turn_token"] != "" {
			t.Errorf("answer = %v, want empty TURN credentials", answer)
		}
	}
}
