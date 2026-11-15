// Tests of the WebSocket side: rooms, chat, names, reconnecting, notices and
// the limits on a client. Each test runs real clients against an httptest server.

package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/store"
)

// testWait is the longest a test waits for a message that should arrive.
const testWait = 2 * time.Second

// quietWait is how long a test waits to be sure that no message arrives.
const quietWait = 150 * time.Millisecond

// lockedBuffer is a log destination that several goroutines may write to.
type lockedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.data.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.data.String()
}

// testService is a hub behind an httptest server. The server maps the query
// value user to the account of the request, so no login is needed.
type testService struct {
	t        *testing.T
	hub      *Hub
	accounts *accounts.Service
	server   *httptest.Server
	log      *lockedBuffer
}

// shortSettings are the test settings: the documents' rooms and short timeouts.
func shortSettings() Settings {
	settings := DefaultSettings([]string{"Lobby", "Second"})
	settings.IdleTimeout = 400 * time.Millisecond
	settings.ReconnectHold = 400 * time.Millisecond
	settings.WriteTimeout = time.Second
	return settings
}

// newTestService starts a hub with the given settings on a new database.
func newTestService(t *testing.T, settings Settings) *testService {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "lobby.db"))
	if err != nil {
		t.Fatalf("store.Open returned error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	log := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service := accounts.New(db, 0)
	hub := NewHub(settings, service, logger)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.URL.Query().Get("user"), 10, 64)
		if err != nil {
			http.Error(w, "bad user", http.StatusBadRequest)
			return
		}
		hub.ServeWS(w, r, userID)
	}))
	t.Cleanup(server.Close)
	return &testService{t: t, hub: hub, accounts: service, server: server, log: log}
}

// newAccount creates an account and returns its user id.
func (service *testService) newAccount() int64 {
	service.t.Helper()
	code, err := service.accounts.IssueLoginCode("127.0.0.1")
	if err != nil {
		service.t.Fatalf("IssueLoginCode returned error: %v", err)
	}
	login, err := service.accounts.CheckLoginCode(code, "127.0.0.1")
	if err != nil {
		service.t.Fatalf("CheckLoginCode returned error: %v", err)
	}
	return login.UserID
}

// client is a test client. A goroutine reads its socket for good, because
// the library closes a connection whose read is cancelled, so a test cannot
// use a read timeout to wait for silence.
type client struct {
	*websocket.Conn
	incoming chan map[string]any
	ended    chan struct{}
	status   websocket.StatusCode
}

// readIncoming reads every message of the socket into c.incoming until the
// connection ends, then records the close status and closes c.ended. A frame
// that is not a text frame holding a JSON object is passed on as a message
// with msg_id -1, which no test expects.
func (c *client) readIncoming() {
	defer close(c.ended)
	for {
		frameType, data, err := c.Read(context.Background())
		if err != nil {
			c.status = websocket.CloseStatus(err)
			return
		}
		var message map[string]any
		if frameType != websocket.MessageText || json.Unmarshal(data, &message) != nil {
			message = map[string]any{"msg_id": -1.0, "frame": string(data)}
		}
		c.incoming <- message
	}
}

// waitUntilHeld waits until the hub has noticed that the player's connection
// ended, so the place is held and notices are kept for it.
func (service *testService) waitUntilHeld(userID int64) {
	service.t.Helper()
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		service.hub.mu.Lock()
		held, found := service.hub.players[userID]
		disconnected := found && held.conn == nil
		service.hub.mu.Unlock()
		if disconnected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	service.t.Fatalf("the hub did not notice that user %d disconnected", userID)
}

// dial opens a WebSocket for the account. The response is returned also when
// the upgrade is refused.
func (service *testService) dial(userID int64, isReconnect bool) (*client, *http.Response, error) {
	service.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	header := http.Header{}
	header.Set("is-reconnect", strconv.FormatBool(isReconnect))
	socket, response, err := websocket.Dial(ctx, service.server.URL+"/?user="+strconv.FormatInt(userID, 10), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, response, err
	}
	service.t.Cleanup(func() { socket.CloseNow() })
	opened := &client{Conn: socket, incoming: make(chan map[string]any, 1024), ended: make(chan struct{})}
	go opened.readIncoming()
	return opened, response, nil
}

// connect opens a new-session WebSocket for the account and fails the test if it cannot.
func (service *testService) connect(userID int64) *client {
	service.t.Helper()
	opened, _, err := service.dial(userID, false)
	if err != nil {
		service.t.Fatalf("dial for user %d failed: %v", userID, err)
	}
	return opened
}

// send writes a JSON message as a binary frame, as the game client does.
func send(t *testing.T, socket *client, message any) {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}
	sendFrame(t, socket, websocket.MessageBinary, data)
}

// sendFrame writes raw data as a frame of the given type.
func sendFrame(t *testing.T, socket *client, frameType websocket.MessageType, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	if err := socket.Write(ctx, frameType, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

// receive returns the next message the client got, which must be a text frame holding a JSON object.
func receive(t *testing.T, socket *client) map[string]any {
	t.Helper()
	select {
	case message := <-socket.incoming:
		return message
	case <-socket.ended:
		t.Fatalf("connection ended while waiting for a message (close status %d)", socket.status)
	case <-time.After(testWait):
		t.Fatalf("no message within %v", testWait)
	}
	return nil
}

// receiveID reads the next message and requires its msg_id.
func receiveID(t *testing.T, socket *client, msgID int) map[string]any {
	t.Helper()
	message := receive(t, socket)
	if message["msg_id"] != float64(msgID) {
		t.Fatalf("got message %v, want msg_id %d", message, msgID)
	}
	return message
}

// expectSilence requires that nothing arrives for quietWait and that the connection stays open.
func expectSilence(t *testing.T, socket *client) {
	t.Helper()
	select {
	case message := <-socket.incoming:
		t.Fatalf("got message %v, want silence", message)
	case <-socket.ended:
		t.Fatalf("connection ended while waiting for silence (close status %d)", socket.status)
	case <-time.After(quietWait):
	}
}

// expectEnded requires that the connection ends within testWait and returns
// the close status the client saw, or -1 when it ended without a close frame.
func expectEnded(t *testing.T, socket *client) websocket.StatusCode {
	t.Helper()
	timeout := time.After(testWait)
	for {
		select {
		case <-socket.incoming:
		case <-socket.ended:
			return socket.status
		case <-timeout:
			t.Fatalf("connection was still open after %v", testWait)
		}
	}
}

// barrier returns when the service has handled every message the socket sent
// before it: it sends a ping and waits for the pong.
func barrier(t *testing.T, socket *client) {
	t.Helper()
	send(t, socket, map[string]any{"msg_id": 14})
	receiveID(t, socket, 15)
}

// joinRoom puts the socket's player in room 1 and reads the answer.
func joinRoom(t *testing.T, socket *client) map[string]any {
	t.Helper()
	send(t, socket, map[string]any{"msg_id": 3, "room": 1, "request_id": 1})
	return receiveID(t, socket, 4)
}

// memberNames returns the Name of each entry of a message 4, in order.
func memberNames(t *testing.T, message map[string]any) []string {
	t.Helper()
	entries, ok := message["members"].([]any)
	if !ok {
		t.Fatalf("members is not an array in %v", message)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.(map[string]any)["Name"].(string))
	}
	return names
}

// requireKeys fails the test unless the message has every key.
func requireKeys(t *testing.T, message map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, found := message[key]; !found {
			t.Errorf("message %v has no key %q", message, key)
		}
	}
}

func TestPingIsAnsweredWithPongInATextFrame(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	send(t, socket, map[string]any{"msg_id": 14})
	receiveID(t, socket, 15)
}

func TestJSONIsReadFromTextAndBinaryFrames(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	sendFrame(t, socket, websocket.MessageText, []byte(`{"msg_id":14}`))
	receiveID(t, socket, 15)
	sendFrame(t, socket, websocket.MessageBinary, []byte(`{"msg_id":14}`))
	receiveID(t, socket, 15)
}

func TestSilentConnectionIsDroppedWithoutCloseFrame(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	if status := expectEnded(t, socket); status != -1 {
		t.Errorf("close status = %d, want -1 (no close frame)", status)
	}
}

func TestConnectionThatKeepsPingingIsKept(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	for range 8 {
		time.Sleep(100 * time.Millisecond)
		barrier(t, socket)
	}
}

func TestReconnectWithHeldPlaceKeepsRoom(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	firstSocket.CloseNow()
	reconnected, response, err := service.dial(first, true)
	if err != nil {
		t.Fatalf("reconnect failed with status %v: %v", response, err)
	}
	send(t, reconnected, map[string]any{"msg_id": 1, "message": "back"})
	line := receiveID(t, reconnected, 2)
	if !strings.HasSuffix(line["message"].(string), ": back") {
		t.Errorf("chat line = %v, want one ending in \": back\"", line["message"])
	}
	receiveID(t, secondSocket, 2)
}

func TestReconnectWithoutHeldPlaceGets205(t *testing.T) {
	service := newTestService(t, shortSettings())

	_, response, err := service.dial(service.newAccount(), true)

	if err == nil || response == nil || response.StatusCode != http.StatusResetContent {
		t.Fatalf("reconnect = %v, %v; want HTTP 205", response, err)
	}
}

func TestPlaceIsReleasedWhenTheHoldRunsOut(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	firstSocket.CloseNow()
	expectSilence(t, secondSocket)
	update := receiveID(t, secondSocket, 4)

	if names := memberNames(t, update); len(names) != 1 {
		t.Errorf("members after the hold = %v, want only the second player", names)
	}
	_, response, err := service.dial(first, true)
	if err == nil || response == nil || response.StatusCode != http.StatusResetContent {
		t.Errorf("reconnect after the hold = %v, %v; want HTTP 205", response, err)
	}
}

func TestNewSessionReleasesAHeldPlace(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)
	firstSocket.CloseNow()

	fresh := service.connect(first)
	update := receiveID(t, secondSocket, 4)

	if names := memberNames(t, update); len(names) != 1 {
		t.Errorf("members after a new session = %v, want only the second player", names)
	}
	send(t, fresh, map[string]any{"msg_id": 1, "message": "nobody hears this"})
	barrier(t, fresh)
	expectSilence(t, secondSocket)
}

func TestClientCloseReleasesThePlaceAtOnce(t *testing.T) {
	settings := shortSettings()
	settings.ReconnectHold = time.Minute
	service := newTestService(t, settings)
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	firstSocket.Close(websocket.StatusNormalClosure, "")
	update := receiveID(t, secondSocket, 4)

	if names := memberNames(t, update); len(names) != 1 {
		t.Errorf("members after a close = %v, want only the second player", names)
	}
}

func TestSecondConnectionReplacesTheFirst(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	userID := service.newAccount()
	first := service.connect(userID)
	joinRoom(t, first)

	second, _, err := service.dial(userID, true)
	if err != nil {
		t.Fatalf("second connection failed: %v", err)
	}

	if status := expectEnded(t, first); status == -1 {
		t.Errorf("replaced connection ended without a close frame")
	}
	send(t, second, map[string]any{"msg_id": 1, "message": "still here"})
	receiveID(t, second, 2)
}

func TestRoomChangeAnswersWithEveryKeyAndInformsTheRoom(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)

	answer := joinRoom(t, firstSocket)
	requireKeys(t, answer, "members", "selected_room_id", "effective_room_id", "rejected_room_id", "room_selection_error", "request_id")
	if answer["request_id"] != float64(1) || answer["selected_room_id"] != float64(1) || answer["effective_room_id"] != float64(1) {
		t.Errorf("answer = %v, want request 1, selected 1, effective 1", answer)
	}
	entry := answer["members"].([]any)[0].(map[string]any)
	requireKeys(t, entry, "UserID", "Name", "IsAdmin")
	joinRoom(t, secondSocket)
	update := receiveID(t, firstSocket, 4)

	requireKeys(t, update, "members", "selected_room_id", "effective_room_id", "rejected_room_id", "room_selection_error", "request_id")
	if names := memberNames(t, update); len(names) != 2 {
		t.Errorf("members seen by the first player = %v, want two", names)
	}
}

func TestLeavingARoomAnswersAndInformsTheRoom(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	send(t, secondSocket, map[string]any{"msg_id": 3, "room": -1, "request_id": 2})
	answer := receiveID(t, secondSocket, 4)
	update := receiveID(t, firstSocket, 4)

	if answer["effective_room_id"] != float64(-1) || len(answer["members"].([]any)) != 0 {
		t.Errorf("answer to leaving = %v, want effective -1 and no members", answer)
	}
	if names := memberNames(t, update); len(names) != 1 {
		t.Errorf("members after the other left = %v, want one", names)
	}
}

func TestRoomThatDoesNotExistIsRejected(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	send(t, socket, map[string]any{"msg_id": 3, "room": 99, "request_id": 5})
	answer := receiveID(t, socket, 4)

	if answer["rejected_room_id"] != float64(99) || answer["request_id"] != float64(5) || answer["room_selection_error"] == "" {
		t.Errorf("answer = %v, want room 99 rejected for request 5 with an error text", answer)
	}
}

func TestChatGoesToTheSendersRoomWithAllKeys(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second, third := service.newAccount(), service.newAccount(), service.newAccount()
	firstSocket, secondSocket, thirdSocket := service.connect(first), service.connect(second), service.connect(third)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)
	send(t, thirdSocket, map[string]any{"msg_id": 3, "room": 2, "request_id": 1})
	receiveID(t, thirdSocket, 4)

	send(t, firstSocket, map[string]any{"msg_id": 1, "message": "hello\u0007 there", "action": false})
	heard := receiveID(t, secondSocket, 2)
	echoed := receiveID(t, firstSocket, 2)

	requireKeys(t, heard, "message", "action", "admin", "name_change")
	if heard["message"] != "Player1000: hello there" || heard["action"] != false || heard["admin"] != false || heard["name_change"] != false {
		t.Errorf("chat line = %v, want Player1000: hello there with action, admin and name_change false", heard)
	}
	if echoed["message"] != heard["message"] {
		t.Errorf("the sender heard %v, the room heard %v", echoed["message"], heard["message"])
	}
	barrier(t, thirdSocket)
	expectSilence(t, thirdSocket)
}

func TestChatIsCutAt300Characters(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())
	joinRoom(t, socket)

	send(t, socket, map[string]any{"msg_id": 1, "message": strings.Repeat("é", 400)})
	line := receiveID(t, socket, 2)["message"].(string)

	if text := strings.TrimPrefix(line, "Player1000: "); len([]rune(text)) != 300 {
		t.Errorf("chat text has %d characters, want 300", len([]rune(text)))
	}
}

func TestChatFromAPlayerInNoRoomIsIgnored(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	send(t, socket, map[string]any{"msg_id": 1, "message": "anyone?"})
	barrier(t, socket)
	expectSilence(t, socket)
}

func TestNameChangeIsStoredAndRefreshesTheMemberList(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	send(t, secondSocket, map[string]any{"msg_id": 9, "name": "Brave Fox"})
	update := receiveID(t, firstSocket, 4)

	if names := memberNames(t, update); len(names) != 2 || names[1] != "Brave Fox" {
		t.Errorf("member names = %v, want the second to be Brave Fox", names)
	}
	stored, _, err := service.accounts.DisplayName(second)
	if err != nil || stored != "Brave Fox" {
		t.Errorf("stored name = %q, %v; want Brave Fox", stored, err)
	}
}

func TestRefusedNameIsAnsweredWithOneChatLineToTheSender(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	joinRoom(t, firstSocket)
	joinRoom(t, secondSocket)
	receiveID(t, firstSocket, 4)

	refused := []string{"ab", "name with $", " leading", "trailing ", "Player7", "player1000", strings.Repeat("x", 21)}
	for _, name := range refused {
		send(t, secondSocket, map[string]any{"msg_id": 9, "name": name})
		line := receiveID(t, secondSocket, 2)
		if !strings.HasPrefix(line["message"].(string), nameRefusalPrefix) {
			t.Errorf("name %q: chat line = %v, want one starting %q", name, line["message"], nameRefusalPrefix)
		}
	}
	send(t, secondSocket, map[string]any{"msg_id": 9, "name": "PLAYER1000"})
	receiveID(t, secondSocket, 2)
	barrier(t, secondSocket)
	expectSilence(t, firstSocket)
}

func TestNameOfAnotherAccountIsRefusedWithoutRegardToCase(t *testing.T) {
	service := newTestService(t, shortSettings())
	first, second := service.newAccount(), service.newAccount()
	firstSocket, secondSocket := service.connect(first), service.connect(second)
	send(t, firstSocket, map[string]any{"msg_id": 9, "name": "Taken Name"})
	barrier(t, firstSocket)

	send(t, secondSocket, map[string]any{"msg_id": 9, "name": "taken name"})
	line := receiveID(t, secondSocket, 2)

	if !strings.Contains(line["message"].(string), "already taken") {
		t.Errorf("chat line = %v, want a refusal that says the name is taken", line["message"])
	}
}

func TestOwnDefaultNameIsAllowedAgain(t *testing.T) {
	service := newTestService(t, shortSettings())
	userID := service.newAccount()
	socket := service.connect(userID)
	joinRoom(t, socket)
	send(t, socket, map[string]any{"msg_id": 9, "name": "Other Name"})
	receiveID(t, socket, 4)

	send(t, socket, map[string]any{"msg_id": 9, "name": "Player1000"})
	update := receiveID(t, socket, 4)

	if names := memberNames(t, update); names[0] != "Player1000" {
		t.Errorf("member names = %v, want Player1000", names)
	}
}

func TestIdenticalNoticesWaitingForAPlayerAreSentOnce(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	userID := service.newAccount()
	socket := service.connect(userID)
	joinRoom(t, socket)
	socket.CloseNow()
	service.waitUntilHeld(userID)

	for _, msgID := range []int{MsgLobbyListUpdate, MsgLobbyCurrentLobbyUpdate, MsgLobbyListUpdate, MsgLobbyCurrentLobbyUpdate, MsgLobbyListUpdate} {
		service.hub.Notify(userID, msgID)
	}
	reconnected, _, err := service.dial(userID, true)
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	receiveID(t, reconnected, MsgLobbyCurrentLobbyUpdate)
	receiveID(t, reconnected, MsgLobbyListUpdate)
	expectSilence(t, reconnected)
}

func TestNoticeForAnUnknownPlayerIsIgnored(t *testing.T) {
	service := newTestService(t, shortSettings())

	service.hub.Notify(424242, MsgLobbyListUpdate)
}

func TestMoreThan60MessagesInASecondDisconnectsTheClient(t *testing.T) {
	service := newTestService(t, shortSettings())
	socket := service.connect(service.newAccount())

	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	for range 70 {
		// A write fails once the service has closed the connection, which is expected.
		_ = socket.Write(ctx, websocket.MessageBinary, []byte(`{"msg_id":8}`))
	}

	if status := expectEnded(t, socket); status != websocket.StatusPolicyViolation {
		t.Errorf("close status = %d, want %d", status, websocket.StatusPolicyViolation)
	}
}

func TestPingsAndSignalsDoNotCountAgainstTheLimit(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	socket := service.connect(service.newAccount())

	for range 100 {
		send(t, socket, map[string]any{"msg_id": 14})
		send(t, socket, map[string]any{"msg_id": 12, "target_user_id": 1, "payload": []int{1}})
	}
	for range 100 {
		receiveID(t, socket, 15)
	}
	barrier(t, socket)
}

func TestMessageOver256KBIsRefused(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	socket := service.connect(service.newAccount())

	big := `{"msg_id":1,"message":"` + strings.Repeat("a", 300*1024) + `"}`
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	_ = socket.Write(ctx, websocket.MessageBinary, []byte(big))

	if status := expectEnded(t, socket); status != websocket.StatusMessageTooBig {
		t.Errorf("close status = %d, want %d", status, websocket.StatusMessageTooBig)
	}
}

func TestUnknownAndMalformedMessagesAreIgnoredAndLogged(t *testing.T) {
	settings := shortSettings()
	settings.IdleTimeout = time.Minute
	service := newTestService(t, settings)
	socket := service.connect(service.newAccount())

	send(t, socket, map[string]any{"msg_id": 999})
	sendFrame(t, socket, websocket.MessageText, []byte(`not json`))
	send(t, socket, map[string]any{"msg_id": "three"})
	send(t, socket, map[string]any{"msg_id": 3, "room": "one"})
	send(t, socket, map[string]any{"msg_id": 1, "message": 7})
	send(t, socket, map[string]any{"msg_id": 9, "name": 7})
	barrier(t, socket)

	logText := service.log.String()
	if !strings.Contains(logText, "level=DEBUG") || !strings.Contains(logText, "ignored a message") || !strings.Contains(logText, "msgId=999") {
		t.Errorf("log does not show the ignored message at debug level:\n%s", logText)
	}
}
