// Tests of the WebSocket upgrade and of MOTD, Rooms and ServiceConfig (SERVICE.md sections 5.1, 6 and 9).

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wsWait is the longest a test waits for the service.
const wsWait = 2 * time.Second

// sessionOn logs a new account in through the handler's own calls and returns its session token.
func sessionOn(t *testing.T, handler http.Handler) string {
	t.Helper()
	codeAnswer := perform(t, handler, http.MethodGet, testPrefix+"LoginCode", "")
	code := decodeAnswer(t, codeAnswer.Body.String())["login_code"].(string)
	loginAnswer := perform(t, handler, http.MethodPost, testPrefix+"CheckLogin", `{"code":"`+code+`"}`)
	return decodeAnswer(t, loginAnswer.Body.String())["session_token"].(string)
}

// dialService opens a WebSocket to the test server with the given token and
// reconnect header. An empty token sends no Authorization header.
func dialService(t *testing.T, server *httptest.Server, token string, isReconnect string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wsWait)
	defer cancel()
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	header.Set("is-reconnect", isReconnect)
	socket, response, err := websocket.Dial(ctx, server.URL+"/ws", &websocket.DialOptions{HTTPHeader: header})
	if socket != nil {
		t.Cleanup(func() { socket.CloseNow() })
	}
	return socket, response, err
}

// exchange sends one message as a binary frame and returns the next message received.
func exchange(t *testing.T, socket *websocket.Conn, message string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wsWait)
	defer cancel()
	if err := socket.Write(ctx, websocket.MessageBinary, []byte(message)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	return readMessage(t, socket)
}

// readMessage returns the next message received, which must be a text frame.
func readMessage(t *testing.T, socket *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wsWait)
	defer cancel()
	frameType, data, err := socket.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if frameType != websocket.MessageText {
		t.Fatalf("frame type = %v, want text", frameType)
	}
	var message map[string]any
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("message %q is not a JSON object: %v", data, err)
	}
	return message
}

func TestWebSocketWithoutValidTokenGets401(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	for name, token := range map[string]string{"no token": "", "unknown token": strings.Repeat("a", 64), "malformed token": "abc"} {
		t.Run(name, func(t *testing.T) {
			_, response, err := dialService(t, server, token, "false")
			if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("dial = %v, %v; want HTTP 401", response, err)
			}
		})
	}
}

func TestWebSocketWithValidTokenAnswersPing(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	socket, _, err := dialService(t, server, sessionOn(t, handler), "false")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	if answer := exchange(t, socket, `{"msg_id":14}`); answer["msg_id"] != float64(15) {
		t.Errorf("answer to 14 = %v, want msg_id 15", answer)
	}
}

func TestReconnectWithoutHeldPlaceGets205(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	_, response, err := dialService(t, server, sessionOn(t, handler), "true")

	if err == nil || response == nil || response.StatusCode != http.StatusResetContent {
		t.Fatalf("dial = %v, %v; want HTTP 205", response, err)
	}
}

func TestTwoPlayersChatThroughTheRoomAndRenameThroughTheService(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	first, _, err := dialService(t, server, sessionOn(t, handler), "false")
	if err != nil {
		t.Fatalf("first dial failed: %v", err)
	}
	second, _, err := dialService(t, server, sessionOn(t, handler), "false")
	if err != nil {
		t.Fatalf("second dial failed: %v", err)
	}

	exchange(t, first, `{"msg_id":3,"room":1,"request_id":1}`)
	exchange(t, second, `{"msg_id":3,"room":1,"request_id":1}`)
	readMessage(t, first)
	heard := exchange(t, first, `{"msg_id":1,"message":"hello","action":false}`)
	got := readMessage(t, second)

	if heard["message"] != "Player1000: hello" || got["message"] != "Player1000: hello" {
		t.Errorf("chat lines = %v and %v, want Player1000: hello", heard["message"], got["message"])
	}
	renamed := exchange(t, second, `{"msg_id":9,"name":"Second Player"}`)
	if renamed["msg_id"] != float64(4) {
		t.Errorf("answer to a name change = %v, want a member list", renamed)
	}
}

func TestWebSocketSurvivesLongerThanTheServerReadTimeout(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewUnstartedServer(handler)
	server.Config.ReadTimeout = 200 * time.Millisecond
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Config.IdleTimeout = 200 * time.Millisecond
	server.Start()
	defer server.Close()
	socket, _, err := dialService(t, server, sessionOn(t, handler), "false")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	for range 6 {
		time.Sleep(100 * time.Millisecond)
		if answer := exchange(t, socket, `{"msg_id":14}`); answer["msg_id"] != float64(15) {
			t.Fatalf("answer to 14 = %v, want msg_id 15", answer)
		}
	}
}

func TestMOTD(t *testing.T) {
	handler, token := sessionFor(t)

	recorder := performAs(t, handler, token, http.MethodGet, testPrefix+"MOTD", "")

	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"MOTD":"`+testMOTD+`"}` {
		t.Errorf("MOTD = %d %q, want 200 with the configured text", recorder.Code, recorder.Body.String())
	}
}

func TestRoomsListTheConfiguredRoomsWithEveryKey(t *testing.T) {
	handler, token := sessionFor(t)

	recorder := performAs(t, handler, token, http.MethodGet, testPrefix+"Rooms", "")

	want := `{"rooms":[{"id":1,"name":"Lobby","flags":0,"parent_id":-1}],"supports_room_selection_results":false,"supports_moderation_commands":false}`
	if recorder.Code != http.StatusOK || recorder.Body.String() != want {
		t.Errorf("Rooms = %d %q, want 200 %q", recorder.Code, recorder.Body.String(), want)
	}
}

func TestServiceConfigHasEveryFieldWithTheValuesOfSection9(t *testing.T) {
	handler, token := sessionFor(t)

	recorder := performAs(t, handler, token, http.MethodGet, testPrefix+"ServiceConfig", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("ServiceConfig status = %d, want 200", recorder.Code)
	}
	answer := decodeAnswer(t, recorder.Body.String())
	wantKeys := []string{
		"retry_signalling", "use_mapped_port", "min_run_ahead_frames", "ra_update_frequency_frames", "relay_all_traffic",
		"ra_slack_percent", "frame_grouping_frames", "enable_host_migration", "network_do_immediate_flush_per_frame",
		"network_send_flags", "network_latency_logic_model", "use_default_config", "ra_slack_override_percent_in_default",
		"do_probes", "do_replay_upload", "network_mesh_histogram_duration", "ibra_ra_tweaks", "ibra_minslack_default",
		"ibra_maxslack_default", "ibra_minslack_greaterthan300ms", "ibra_maxslack_greaterthan300ms",
		"ibra_minslack_greaterthan200ms", "ibra_maxslack_greaterthan200ms", "screenshot_width", "screenshot_height",
		"ice_implementation", "use_new_networking",
	}
	assertJSONKeys(t, recorder.Body.String(), wantKeys)
	if len(answer) != len(wantKeys) {
		t.Errorf("ServiceConfig has %d keys, want %d", len(answer), len(wantKeys))
	}
	wantValues := map[string]any{
		"relay_all_traffic": false, "retry_signalling": true, "use_new_networking": false,
		"do_probes": false, "do_replay_upload": false, "ice_implementation": float64(2), "network_send_flags": float64(-1),
	}
	for key, want := range wantValues {
		if answer[key] != want {
			t.Errorf("ServiceConfig %s = %v, want %v", key, answer[key], want)
		}
	}
}

func TestConfigurationCallsNeedALogin(t *testing.T) {
	handler := testHandler(t)

	for _, endpoint := range []string{"MOTD", "Rooms", "ServiceConfig"} {
		recorder := perform(t, handler, http.MethodGet, testPrefix+endpoint, "")
		if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != "{}" {
			t.Errorf("%s without a token = %d %q, want 401 {}", endpoint, recorder.Code, recorder.Body.String())
		}
	}
}
