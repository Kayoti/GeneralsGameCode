// Tests of server lobbies over HTTP (SERVICE.md section 10, PROTOCOL.md 5.2):
// the X-Servers-Pass header on create and join, empty TURN credentials, and
// the server session of field 19. Passes are signed with a key pair made here.

package api

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/pass"
)

const (
	passRegion = 1
	passTag    = "eu"
	otherTag   = "us"
	// The fingerprints of createBody.
	bodyExeCRC = 7
	bodyIniCRC = 9
)

// serverEnv is a service that checks passes of the region eu (number 1), and
// also knows the region us (number 2) with its own key.
type serverEnv struct {
	*lobbyEnv
	euKey ed25519.PrivateKey
	usKey ed25519.PrivateKey
	log   *syncBuffer
}

// syncBuffer is a log buffer that connection goroutines and the test may both use.
type syncBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.data.Write(data)
}

func (buffer *syncBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.data.String()
}

// newServerEnv starts a service with a TURN secret and two regions.
func newServerEnv(t *testing.T) *serverEnv {
	t.Helper()
	euPublic, euKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	usPublic, usKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	verifier := pass.NewVerifier()
	verifier.Add(passRegion, passTag, euPublic)
	verifier.Add(2, otherTag, usPublic)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	options := Options{WSURI: testWSURI, TurnSecret: testTurnSecret, Passes: verifier}
	handler := buildHandler(options, accounts.New(openTestDB(t), 0), logger)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &serverEnv{lobbyEnv: &lobbyEnv{t: t, handler: handler, server: server}, euKey: euKey, usKey: usKey, log: logs}
}

// passFor signs a pass for the region with the given key, valid for ten minutes now.
func passFor(region uint16, key ed25519.PrivateKey, exeCRC uint32, iniCRC uint32) string {
	now := uint32(time.Now().Unix())
	return pass.Sign(pass.Fields{
		Version: pass.Version, Region: region, ExeCRC: exeCRC, IniCRC: iniCRC, IssuedAt: now, ExpiresAt: now + 600,
	}, key)
}

// euPass is a pass of the region eu for the fingerprints of createBody.
func (env *serverEnv) euPass() string {
	return passFor(passRegion, env.euKey, bodyExeCRC, bodyIniCRC)
}

// callWithPass sends a request with the X-Servers-Pass header, when it is not empty.
func (env *serverEnv) callWithPass(account *lobbyAccount, method string, endpoint string, body string, header string) (int, map[string]any) {
	env.t.Helper()
	request := httptest.NewRequest(method, testPrefix+endpoint, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+account.token)
	if header != "" {
		request.Header.Set(pass.Header, header)
	}
	recorder := httptest.NewRecorder()
	env.handler.ServeHTTP(recorder, request)
	return recorder.Code, decodeAnswer(env.t, recorder.Body.String())
}

// serverBody is createBody with server_mode true.
func serverBody() string {
	return strings.Replace(createBody, `"max_players":4,`, `"max_players":4,"server_mode":true,`, 1)
}

// createServer creates a server lobby with a good pass and returns its id.
func (env *serverEnv) createServer(host *lobbyAccount) int64 {
	env.t.Helper()
	status, answer := env.callWithPass(host, http.MethodPut, "Lobbies", serverBody(), env.euPass())
	if status != http.StatusOK || answer["result"] != float64(1) {
		env.t.Fatalf("create answered %d %v", status, answer)
	}
	return int64(answer["lobby_id"].(float64))
}

func TestCreateWithAGoodPassMakesAServerLobbyOfThePassRegionWithEmptyTurnCredentials(t *testing.T) {
	env := newServerEnv(t)
	host := env.login()

	status, answer := env.callWithPass(host, http.MethodPut, "Lobbies", serverBody(), env.euPass())

	if status != http.StatusOK || answer["result"] != float64(1) {
		t.Fatalf("create answered %d %v", status, answer)
	}
	if answer["turn_username"] != "" || answer["turn_token"] != "" {
		t.Errorf("answer %v carries TURN credentials for a server lobby", answer)
	}
	_, lobby := env.detail(host, int64(answer["lobby_id"].(float64)))
	if lobby["LobbyType"] != float64(2) || lobby["Region"] != passTag {
		t.Errorf("lobby has LobbyType %v and Region %v, want 2 and %q", lobby["LobbyType"], lobby["Region"], passTag)
	}
}

func TestCreateOfAServerLobbyTakesTheRegionOfThePassNotOfTheRequest(t *testing.T) {
	env := newServerEnv(t)
	host := env.login()
	header := passFor(2, env.usKey, bodyExeCRC, bodyIniCRC)

	_, answer := env.callWithPass(host, http.MethodPut, "Lobbies", serverBody(), header)

	_, lobby := env.detail(host, int64(answer["lobby_id"].(float64)))
	if lobby["Region"] != otherTag {
		t.Errorf("Region = %v, want %q", lobby["Region"], otherTag)
	}
}

func TestCreateOfAServerLobbyWithoutAValidPassFailsWithAllFourKeys(t *testing.T) {
	env := newServerEnv(t)
	_, stranger, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"not a pass", "abc"},
		{"signed by another key", passFor(passRegion, stranger, bodyExeCRC, bodyIniCRC)},
		{"other exe fingerprint", passFor(passRegion, env.euKey, bodyExeCRC+1, bodyIniCRC)},
		{"other ini fingerprint", passFor(passRegion, env.euKey, bodyExeCRC, bodyIniCRC+1)},
		{"unknown region", passFor(9, env.euKey, bodyExeCRC, bodyIniCRC)},
		{"expired", pass.Sign(pass.Fields{Version: 1, Region: passRegion, ExeCRC: bodyExeCRC, IniCRC: bodyIniCRC, ExpiresAt: 100}, env.euKey)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			host := env.login()

			status, answer := env.callWithPass(host, http.MethodPut, "Lobbies", serverBody(), test.header)

			if status != http.StatusOK || answer["result"] != float64(0) {
				t.Fatalf("create answered %d %v, want 200 and result 0", status, answer)
			}
			requireExactKeys(t, "answer", answer, []string{"result", "lobby_id", "turn_username", "turn_token"})
			_, listed := env.call(host, http.MethodGet, "Lobbies", "")
			if lobbies, _ := listed["lobbies"].([]any); len(lobbies) != 0 {
				t.Errorf("a lobby exists after the refused create: %v", lobbies)
			}
		})
	}
}

func TestLobbyCreatedWithoutServerModeIgnoresThePassHeader(t *testing.T) {
	env := newServerEnv(t)
	host, guest := env.login(), env.login()

	status, answer := env.callWithPass(host, http.MethodPut, "Lobbies", createBody, "not a pass")
	if status != http.StatusOK || answer["result"] != float64(1) {
		t.Fatalf("create answered %d %v", status, answer)
	}
	requireValidCredentials(t, answer, host.id)
	lobbyID := int64(answer["lobby_id"].(float64))
	_, lobby := env.detail(host, lobbyID)
	if lobby["LobbyType"] != float64(0) || lobby["Region"] != "" {
		t.Errorf("lobby has LobbyType %v and Region %v, want 0 and an empty region", lobby["LobbyType"], lobby["Region"])
	}

	status, joined := env.callWithPass(guest, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID, 10), `{"has_map":true}`, "not a pass")
	if status != http.StatusOK || joined["success"] != true {
		t.Fatalf("join answered %d %v", status, joined)
	}
	requireValidCredentials(t, joined, guest.id)

	status, plain := env.call(host, http.MethodPost, "Lobby/"+strconv.FormatInt(lobbyID, 10), `{"field":19,"server_session":"00112233445566ff","tokens":[]}`)
	if status != http.StatusOK || plain["result"] != float64(0) {
		t.Errorf("field 19 on an Online lobby answered %d %v, want 200 and result 0", status, plain)
	}
}

func TestJoinOfAServerLobbyNeedsAPassOfItsRegionAndFingerprints(t *testing.T) {
	env := newServerEnv(t)
	host := env.login()
	lobbyID := env.createServer(host)
	path := "Lobby/" + strconv.FormatInt(lobbyID, 10)
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no pass", "", http.StatusForbidden},
		{"not a pass", "zz", http.StatusForbidden},
		{"pass of another region", passFor(2, env.usKey, bodyExeCRC, bodyIniCRC), http.StatusForbidden},
		{"other fingerprints", passFor(passRegion, env.euKey, 1, 2), http.StatusForbidden},
		{"expired", pass.Sign(pass.Fields{Version: 1, Region: passRegion, ExeCRC: bodyExeCRC, IniCRC: bodyIniCRC, ExpiresAt: 100}, env.euKey), http.StatusForbidden},
		{"good pass", env.euPass(), http.StatusOK},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			joiner := env.login()

			status, answer := env.callWithPass(joiner, http.MethodPut, path, `{"has_map":true}`, test.header)

			if status != test.want {
				t.Fatalf("join answered %d %v, want %d", status, answer, test.want)
			}
			if status == http.StatusOK && (answer["turn_username"] != "" || answer["turn_token"] != "") {
				t.Errorf("join answer %v carries TURN credentials for a server lobby", answer)
			}
		})
	}
}

func TestJoinChecksTheLobbyBeforeThePass(t *testing.T) {
	env := newServerEnv(t)
	host := env.login()
	lobbyID := env.createServer(host)
	joiner := env.login()

	status, _ := env.callWithPass(joiner, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID+50, 10), `{}`, "")

	if status != http.StatusNotFound {
		t.Errorf("join of a lobby that does not exist answered %d, want 404", status)
	}
}

func TestFieldNineteenStoresARightSessionAndRefusesTheRest(t *testing.T) {
	env := newServerEnv(t)
	host, guest := env.login(), env.login()
	lobbyID := env.createServer(host)
	path := "Lobby/" + strconv.FormatInt(lobbyID, 10)
	if status, _ := env.callWithPass(guest, http.MethodPut, path, `{"has_map":true}`, env.euPass()); status != http.StatusOK {
		t.Fatalf("join answered %d", status)
	}
	good := `{"field":19,"server_session":"00112233445566ff","tokens":[{"slot":1,"token":"1111111111111111"}]}`
	cases := []struct {
		name    string
		account *lobbyAccount
		body    string
		want    float64
	}{
		{"by a member", guest, good, 0},
		{"zero token", host, strings.Replace(good, "1111111111111111", "0000000000000000", 1), 0},
		{"no tokens", host, `{"field":19,"server_session":"00112233445566ff","tokens":[]}`, 0},
		{"no session", host, `{"field":19,"tokens":[{"slot":1,"token":"1111111111111111"}]}`, 0},
		{"session as a number", host, `{"field":19,"server_session":5,"tokens":[{"slot":1,"token":"1111111111111111"}]}`, 0},
		{"slot as text", host, `{"field":19,"server_session":"00112233445566ff","tokens":[{"slot":"1","token":"1111111111111111"}]}`, 0},
		{"tokens not a list", host, `{"field":19,"server_session":"00112233445566ff","tokens":7}`, 0},
		{"not JSON", host, `{"field":19,`, -1},
		{"by the host", host, good, 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			status, answer := env.post(test.account, lobbyID, test.body)

			if status != http.StatusOK {
				t.Fatalf("answered %d, want 200", status)
			}
			if test.want < 0 {
				return
			}
			requireExactKeys(t, "answer", answer, []string{"result"})
			if answer["result"] != test.want {
				t.Errorf("answer %v, want result %v", answer, test.want)
			}
		})
	}
}

func TestServerSessionAppearsInNoAnswer(t *testing.T) {
	env := newServerEnv(t)
	host, guest := env.login(), env.login()
	lobbyID := env.createServer(host)
	path := "Lobby/" + strconv.FormatInt(lobbyID, 10)
	env.callWithPass(guest, http.MethodPut, path, `{"has_map":true}`, env.euPass())
	const session = "0123456789abcdef"
	const token = "fedcba9876543210"
	body := `{"field":19,"server_session":"` + session + `","tokens":[{"slot":1,"token":"` + token + `"}]}`

	_, stored := env.post(host, lobbyID, body)
	_, detail := env.call(host, http.MethodGet, path, "")
	_, guestDetail := env.call(guest, http.MethodGet, path, "")
	_, list := env.call(host, http.MethodGet, "Lobbies", "")

	if stored["result"] != float64(1) {
		t.Fatalf("the session was refused: %v", stored)
	}
	for _, answer := range []map[string]any{detail, guestDetail, list} {
		text, err := json.Marshal(answer)
		if err != nil {
			t.Fatalf("Marshal returned error: %v", err)
		}
		if strings.Contains(string(text), session) || strings.Contains(string(text), token) {
			t.Errorf("an answer holds the server session: %s", text)
		}
	}
	if logged := env.log.String(); strings.Contains(logged, session) || strings.Contains(logged, token) ||
		strings.Contains(logged, env.euPass()[:40]) {
		t.Errorf("the log holds the server session or a pass: %s", logged)
	}
}
