// Tests of the lobby calls over HTTP (SERVICE.md sections 5.1 and 7, CLIENT_HTTP.md 4.12 to 4.18 and 5).
// Accounts log in through the service's own calls and connect a WebSocket,
// as the game client does, because a lobby member is tied to its WebSocket place.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/coder/websocket"
)

// msgPongID is the msg_id of PONG (CLIENT_REALTIME.md 1.7).
const msgPongID = 15

// The key sets of CLIENT_HTTP.md 5.1 and 5.2.
var (
	listLobbyKeys = []string{
		"LobbyID", "Owner", "Name", "MapName", "MapPath", "IsMapOfficial", "NumCurrentPlayers", "MaxPlayers",
		"IsVanillaTeamsOnly", "StartingCash", "IsLimitSuperweapons", "IsTrackingStats", "IsPassworded",
		"AllowObservers", "MaximumCameraHeight", "ExeCRC", "IniCRC", "MatchID", "LobbyType", "Region", "Members",
	}
	listMemberKeys   = []string{"UserID", "DisplayName", "IsReady", "SlotIndex", "SlotState", "Region"}
	detailLobbyKeys  = append(slices.Clone(listLobbyKeys), "RNGSeed")
	detailMemberKeys = []string{
		"UserID", "DisplayName", "IsReady", "Port", "Side", "Color", "Team", "StartingPosition", "HasMap",
		"SlotState", "SlotIndex", "Region", "MiddlewareUserID",
	}
)

// createBody is the body of PUT Lobbies as the client sends it (CLIENT_HTTP.md 4.13).
const createBody = `{"name":"My game","map_name":"Alpine Assault","map_path":"alpine assault","map_official":true,` +
	`"max_players":4,"preferred_port":0,"vanilla_teams":false,"track_stats":true,"starting_cash":10000,` +
	`"passworded":false,"password":"","allow_observers":true,"exe_crc":7,"ini_crc":9,"max_cam_height":310.0,"anticheat_id":0}`

// lobbyEnv is a service with real HTTP handling and WebSocket upgrades.
type lobbyEnv struct {
	t       *testing.T
	handler http.Handler
	server  *httptest.Server
}

// lobbyAccount is a logged-in account with its WebSocket.
type lobbyAccount struct {
	id     int64
	token  string
	socket *websocket.Conn
}

// newLobbyEnv starts the service on a new database.
func newLobbyEnv(t *testing.T) *lobbyEnv {
	t.Helper()
	handler := testHandler(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &lobbyEnv{t: t, handler: handler, server: server}
}

// login creates an account through LoginCode and CheckLogin and connects its WebSocket.
func (env *lobbyEnv) login() *lobbyAccount {
	env.t.Helper()
	account := env.loginWithoutSocket()
	socket, _, err := dialService(env.t, env.server, account.token, "false")
	if err != nil {
		env.t.Fatalf("WebSocket connect failed: %v", err)
	}
	account.socket = socket
	return account
}

// loginWithoutSocket creates an account and does not connect a WebSocket.
func (env *lobbyEnv) loginWithoutSocket() *lobbyAccount {
	env.t.Helper()
	codeAnswer := perform(env.t, env.handler, http.MethodGet, testPrefix+"LoginCode", "")
	code := decodeAnswer(env.t, codeAnswer.Body.String())["login_code"].(string)
	loginAnswer := perform(env.t, env.handler, http.MethodPost, testPrefix+"CheckLogin", `{"code":"`+code+`"}`)
	login := decodeAnswer(env.t, loginAnswer.Body.String())
	return &lobbyAccount{id: int64(login["user_id"].(float64)), token: login["session_token"].(string)}
}

// call sends a request as the account and returns the status and the decoded body.
func (env *lobbyEnv) call(account *lobbyAccount, method string, endpoint string, body string) (int, map[string]any) {
	env.t.Helper()
	token := ""
	if account != nil {
		token = account.token
	}
	recorder := performAs(env.t, env.handler, token, method, testPrefix+endpoint, body)
	return recorder.Code, decodeAnswer(env.t, recorder.Body.String())
}

// create creates a lobby with createBody and returns its id.
func (env *lobbyEnv) create(host *lobbyAccount) int64 {
	env.t.Helper()
	status, answer := env.call(host, http.MethodPut, "Lobbies", createBody)
	if status != http.StatusOK || answer["result"] != float64(1) {
		env.t.Fatalf("create answered %d %v", status, answer)
	}
	return int64(answer["lobby_id"].(float64))
}

// join joins the lobby and returns the status.
func (env *lobbyEnv) join(joiner *lobbyAccount, lobbyID int64, body string) int {
	env.t.Helper()
	status, _ := env.call(joiner, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID, 10), body)
	return status
}

// detail returns the lobby object of GET Lobby/{id}, or nil with the status when it is not 200.
func (env *lobbyEnv) detail(viewer *lobbyAccount, lobbyID int64) (int, map[string]any) {
	env.t.Helper()
	status, answer := env.call(viewer, http.MethodGet, "Lobby/"+strconv.FormatInt(lobbyID, 10), "")
	if status != http.StatusOK {
		return status, nil
	}
	return status, answer["lobby"].(map[string]any)
}

// post sends a field change.
func (env *lobbyEnv) post(account *lobbyAccount, lobbyID int64, body string) (int, map[string]any) {
	env.t.Helper()
	return env.call(account, http.MethodPost, "Lobby/"+strconv.FormatInt(lobbyID, 10), body)
}

// wsSend sends a message as a binary frame, then a ping, and reads until the
// pong, so the service has handled the message when it returns. The messages
// read on the way are dropped.
func wsSend(t *testing.T, socket *websocket.Conn, message string) {
	t.Helper()
	exchangeWrite(t, socket, message)
	exchangeWrite(t, socket, `{"msg_id":14}`)
	awaitMessage(t, socket, msgPongID)
}

// exchangeWrite sends a message as a binary frame and reads nothing.
func exchangeWrite(t *testing.T, socket *websocket.Conn, message string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wsWait)
	defer cancel()
	if err := socket.Write(ctx, websocket.MessageBinary, []byte(message)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

// awaitMessage reads until a message with the msg_id arrives and returns it.
func awaitMessage(t *testing.T, socket *websocket.Conn, msgID int) map[string]any {
	t.Helper()
	for {
		message := readMessage(t, socket)
		if message["msg_id"] == float64(msgID) {
			return message
		}
	}
}

// requireExactKeys fails unless the object has exactly the keys.
func requireExactKeys(t *testing.T, what string, object map[string]any, keys []string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	want := slices.Clone(keys)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s has keys %v, want %v", what, got, want)
	}
}

func TestLobbyCallsNeedALogin(t *testing.T) {
	env := newLobbyEnv(t)
	for _, call := range []struct{ method, endpoint string }{
		{http.MethodGet, "Lobbies"}, {http.MethodPut, "Lobbies"}, {http.MethodGet, "Lobby/1"}, {http.MethodPut, "Lobby/1"},
		{http.MethodPost, "Lobby/1"}, {http.MethodDelete, "Lobby/1"}, {http.MethodPost, "Lobby/Outcome"},
	} {
		if status, _ := env.call(nil, call.method, call.endpoint, "{}"); status != http.StatusUnauthorized {
			t.Errorf("%s %s without a token answered %d, want 401", call.method, call.endpoint, status)
		}
	}
}

func TestEmptyListAnswersWithEveryKeyAsArrays(t *testing.T) {
	env := newLobbyEnv(t)
	player := env.login()

	status, answer := env.call(player, http.MethodGet, "Lobbies", "")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	for _, key := range []string{"lobbies", "latencies", "playerlatencies"} {
		if list, ok := answer[key].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %v, want an empty array", key, answer[key])
		}
	}
}

func TestCreateAnswersAndTheListAndDetailCarryEveryKeyWithEightMembers(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest := env.login(), env.login()

	status, created := env.call(host, http.MethodPut, "Lobbies", createBody)
	if status != http.StatusOK {
		t.Fatalf("create status = %d", status)
	}
	requireExactKeys(t, "create answer", created, []string{"result", "lobby_id", "turn_username", "turn_token"})
	if created["result"] != float64(1) || created["lobby_id"] != float64(1) || created["turn_username"] != "" || created["turn_token"] != "" {
		t.Errorf("create answer = %v", created)
	}

	_, listed := env.call(guest, http.MethodGet, "Lobbies", "")
	lobbyList := listed["lobbies"].([]any)
	if len(lobbyList) != 1 {
		t.Fatalf("%d lobbies listed, want 1", len(lobbyList))
	}
	entry := lobbyList[0].(map[string]any)
	requireExactKeys(t, "list lobby", entry, listLobbyKeys)
	members := entry["Members"].([]any)
	if len(members) != 8 {
		t.Errorf("list lobby has %d members, want 8", len(members))
	}
	for _, member := range members {
		requireExactKeys(t, "list member", member.(map[string]any), listMemberKeys)
	}
	if latencies := listed["latencies"].([]any); len(latencies) != 1 || latencies[0] != float64(0) {
		t.Errorf("latencies = %v, want [0]", latencies)
	}
	if entry["Name"] != "My game" || entry["Owner"] != float64(host.id) || entry["NumCurrentPlayers"] != float64(1) ||
		entry["MaxPlayers"] != float64(4) || entry["ExeCRC"] != float64(7) || entry["MaximumCameraHeight"] != float64(310) ||
		entry["LobbyType"] != float64(0) || entry["Region"] != "" {
		t.Errorf("list lobby = %v", entry)
	}

	if status := env.join(guest, 1, `{"preferred_port":0,"anticheat_id":0,"has_map":true}`); status != http.StatusOK {
		t.Fatalf("join status = %d", status)
	}
	status, detail := env.detail(guest, 1)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d", status)
	}
	requireExactKeys(t, "detail lobby", detail, detailLobbyKeys)
	detailMembers := detail["Members"].([]any)
	if len(detailMembers) != 8 {
		t.Fatalf("detail has %d members, want 8", len(detailMembers))
	}
	for _, member := range detailMembers {
		requireExactKeys(t, "detail member", member.(map[string]any), detailMemberKeys)
	}
	second := detailMembers[1].(map[string]any)
	if second["UserID"] != float64(guest.id) || second["SlotState"] != float64(5) || second["HasMap"] != true || second["DisplayName"] == "" {
		t.Errorf("guest member = %v", second)
	}
	if _, isNumber := detail["RNGSeed"].(float64); !isNumber {
		t.Errorf("RNGSeed = %v, want a number", detail["RNGSeed"])
	}
}

func TestDetailIsForMembersOnly(t *testing.T) {
	env := newLobbyEnv(t)
	host, outsider := env.login(), env.login()
	lobbyID := env.create(host)

	if status, _ := env.detail(outsider, lobbyID); status != http.StatusNotFound {
		t.Errorf("detail for a non-member = %d, want 404", status)
	}
	if status, _ := env.detail(host, 999); status != http.StatusNotFound {
		t.Errorf("detail of an unknown lobby = %d, want 404", status)
	}
	if status, _ := env.call(host, http.MethodGet, "Lobby/abc", ""); status != http.StatusNotFound {
		t.Errorf("detail of a non-numeric id = %d, want 404", status)
	}
}

func TestRefusedCreateAnswersResultZeroWithEveryKey(t *testing.T) {
	env := newLobbyEnv(t)
	player, noSocket := env.login(), env.loginWithoutSocket()

	for name, call := range map[string]struct {
		account *lobbyAccount
		body    string
	}{
		"empty name":   {player, `{"name":"  ","max_players":4}`},
		"missing max":  {player, `{"name":"x"}`},
		"max too high": {player, `{"name":"x","max_players":9}`},
		"wrong type":   {player, `{"name":5,"max_players":4}`},
		"not json":     {player, `nonsense`},
		"no websocket": {noSocket, createBody},
	} {
		status, answer := env.call(call.account, http.MethodPut, "Lobbies", call.body)
		if status != http.StatusOK {
			t.Errorf("%s: status = %d", name, status)
		}
		requireExactKeys(t, name, answer, []string{"result", "lobby_id", "turn_username", "turn_token"})
		if answer["result"] != float64(0) {
			t.Errorf("%s: result = %v, want 0", name, answer["result"])
		}
	}
	if _, listed := env.call(player, http.MethodGet, "Lobbies", ""); len(listed["lobbies"].([]any)) != 0 {
		t.Error("a refused create made a lobby")
	}
}

func TestJoinFailuresUseTheStatusCodesOfTheSpecification(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest, late := env.login(), env.login(), env.login()
	lobbyID := env.create(host)

	if status := env.join(guest, 999, `{}`); status != http.StatusNotFound {
		t.Errorf("unknown lobby = %d, want 404", status)
	}
	if status := env.join(guest, lobbyID, `not json`); status != http.StatusBadRequest {
		t.Errorf("bad body = %d, want 400", status)
	}
	wsSend(t, host.socket, `{"msg_id":25,"new_password":"secret"}`)
	if status := env.join(guest, lobbyID, `{"password":"wrong"}`); status != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", status)
	}
	if status := env.join(guest, lobbyID, `{}`); status != http.StatusUnauthorized {
		t.Errorf("no password = %d, want 401", status)
	}
	if status := env.join(guest, lobbyID, `{"password":"secret"}`); status != http.StatusOK {
		t.Fatalf("right password = %d, want 200", status)
	}

	wsSend(t, host.socket, `{"msg_id":23}`)
	if status := env.join(late, lobbyID, `{"password":"secret"}`); status != http.StatusNotAcceptable {
		t.Errorf("locked lobby = %d, want 406", status)
	}
}

func TestFullLobbyAnswers406AndAStartedLobbyAnswers404(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest, late := env.login(), env.login(), env.login()
	lobbyID := env.create(host)
	_, _ = env.post(host, lobbyID, `{"field":0,"map":"m","map_path":"m","map_official":true,"max_players":2}`)
	if status := env.join(guest, lobbyID, `{}`); status != http.StatusOK {
		t.Fatalf("join = %d", status)
	}

	if status := env.join(late, lobbyID, `{}`); status != http.StatusNotAcceptable {
		t.Errorf("full lobby = %d, want 406", status)
	}
	wsSend(t, guest.socket, `{"msg_id":5,"ready":true}`)
	wsSend(t, host.socket, `{"msg_id":13}`)
	if status := env.join(late, lobbyID, `{}`); status != http.StatusNotFound {
		t.Errorf("started lobby = %d, want 404", status)
	}
}

func TestJoinAnswersSuccessWithEveryKey(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest := env.login(), env.login()
	lobbyID := env.create(host)

	status, answer := env.call(guest, http.MethodPut, "Lobby/"+strconv.FormatInt(lobbyID, 10), `{"has_map":true}`)

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	requireExactKeys(t, "join answer", answer, []string{"success", "turn_username", "turn_token"})
	if answer["success"] != true || answer["turn_username"] != "" || answer["turn_token"] != "" {
		t.Errorf("join answer = %v", answer)
	}
}

func TestChangesAnswer200WithAnEmptyObjectAndAreVisibleInTheDetail(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest := env.login(), env.login()
	lobbyID := env.create(host)
	env.join(guest, lobbyID, `{"has_map":true}`)

	for _, body := range []string{
		`{"field":2,"color":3}`,
		`{"field":5,"startingcash":25000}`,
		`{"field":999}`,
		`{"field":2,"color":"red"}`,
		`nonsense`,
	} {
		account := guest
		if body == `{"field":5,"startingcash":25000}` {
			account = host
		}
		status, answer := env.post(account, lobbyID, body)
		if status != http.StatusOK || len(answer) != 0 {
			t.Errorf("POST %s answered %d %v, want 200 {}", body, status, answer)
		}
	}
	_, detail := env.detail(host, lobbyID)
	if detail["StartingCash"] != float64(25000) {
		t.Errorf("StartingCash = %v, want 25000", detail["StartingCash"])
	}
	if guestMember := detail["Members"].([]any)[1].(map[string]any); guestMember["Color"] != float64(3) {
		t.Errorf("guest colour = %v, want 3", guestMember["Color"])
	}
}

func TestFieldNineteenAnswersAResult(t *testing.T) {
	env := newLobbyEnv(t)
	host := env.login()
	lobbyID := env.create(host)

	status, answer := env.post(host, lobbyID, `{"field":19,"server_session":"0123456789abcdef","tokens":[]}`)

	if status != http.StatusOK || answer["result"] != float64(0) {
		t.Errorf("field 19 answered %d %v, want 200 with result 0", status, answer)
	}
}

func TestLeaveAnswers200AlwaysAndTheLobbyIsDeletedWhenEmpty(t *testing.T) {
	env := newLobbyEnv(t)
	host, stranger := env.login(), env.login()
	lobbyID := env.create(host)

	if status, _ := env.call(stranger, http.MethodDelete, "Lobby/"+strconv.FormatInt(lobbyID, 10), ""); status != http.StatusOK {
		t.Errorf("leave by a stranger = %d, want 200", status)
	}
	if _, found := env.detail(host, lobbyID); found == nil {
		t.Error("a stranger's leave removed the host")
	}
	if status, _ := env.call(host, http.MethodDelete, "Lobby/"+strconv.FormatInt(lobbyID, 10), ""); status != http.StatusOK {
		t.Errorf("leave by the host = %d, want 200", status)
	}
	if status, _ := env.call(host, http.MethodDelete, "Lobby/"+strconv.FormatInt(lobbyID, 10), ""); status != http.StatusOK {
		t.Errorf("second leave = %d, want 200", status)
	}
	if status, _ := env.call(host, http.MethodDelete, "Lobby/abc", ""); status != http.StatusOK {
		t.Errorf("leave of a non-numeric id = %d, want 200", status)
	}
	if _, listed := env.call(stranger, http.MethodGet, "Lobbies", ""); len(listed["lobbies"].([]any)) != 0 {
		t.Error("the empty lobby is still listed")
	}
}

func TestOutcomeAnswersEmptyAddresses(t *testing.T) {
	env := newLobbyEnv(t)
	player := env.login()

	status, answer := env.call(player, http.MethodPost, "Lobby/Outcome", `{"won":true,"match_id":1}`)

	if status != http.StatusOK || answer["screenshot_url"] != "" || answer["replay_url"] != "" || len(answer) != 2 {
		t.Errorf("outcome answered %d %v", status, answer)
	}
}

func TestLobbyFromCreateToMatchStartOverHTTPAndWebSocket(t *testing.T) {
	env := newLobbyEnv(t)
	host, guest := env.login(), env.login()
	lobbyID := env.create(host)
	env.join(guest, lobbyID, `{"has_map":true}`)
	awaitMessage(t, guest.socket, 6)

	wsSend(t, guest.socket, `{"msg_id":5,"ready":true}`)
	awaitMessage(t, host.socket, 6)
	exchangeWrite(t, host.socket, `{"msg_id":13}`)

	for _, account := range []*lobbyAccount{host, guest} {
		start := awaitMessage(t, account.socket, 13)
		if start["screenshot_url"] != "" {
			t.Errorf("screenshot_url = %v, want an empty string", start["screenshot_url"])
		}
	}
	_, detail := env.detail(guest, lobbyID)
	if detail["MatchID"] != float64(1) {
		t.Errorf("MatchID = %v, want 1", detail["MatchID"])
	}
}
