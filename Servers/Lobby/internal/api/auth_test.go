// Tests of the calls that log a player in and out (SERVICE.md section 4).

package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"servers-lobby/internal/accounts"
)

// hexToken matches a session or refresh token: 32 random bytes in hexadecimal.
var hexToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

// testRequest describes one request of a test. Empty fields are not sent.
type testRequest struct {
	method       string
	path         string
	body         string
	token        string
	remoteAddr   string
	forwardedFor string
}

// sendRequest sends a testRequest through handler.
func sendRequest(t *testing.T, handler http.Handler, testReq testRequest) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(testReq.method, testReq.path, bytes.NewBufferString(testReq.body))
	if testReq.token != "" {
		request.Header.Set("Authorization", "Bearer "+testReq.token)
	}
	if testReq.remoteAddr != "" {
		request.RemoteAddr = testReq.remoteAddr
	}
	if testReq.forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", testReq.forwardedFor)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// newLoginHandler returns a handler whose account creation limit and
// trust_forwarded_for are given. The log is discarded.
func newLoginHandler(t *testing.T, trustForwardedFor bool, limit int) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewHandler(trustForwardedFor, testWSURI, accounts.New(openTestDB(t), limit), logger)
}

// decodeAnswer decodes a JSON object body into a map of its keys.
func decodeAnswer(t *testing.T, body string) map[string]any {
	t.Helper()
	var answer map[string]any
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	return answer
}

// getLoginCode asks for a login code from remoteAddr and returns it.
func getLoginCode(t *testing.T, handler http.Handler, remoteAddr string) string {
	t.Helper()
	recorder := sendRequest(t, handler, testRequest{method: http.MethodGet, path: testPrefix + "LoginCode", remoteAddr: remoteAddr})
	answer := decodeAnswer(t, recorder.Body.String())
	code, ok := answer["login_code"].(string)
	if recorder.Code != http.StatusOK || answer["success"] != true || !ok || code == "" {
		t.Fatalf("LoginCode answered %d %q, want success with a code", recorder.Code, recorder.Body.String())
	}
	return code
}

// checkLoginBody builds a CheckLogin body with a code and the hardware fields
// the client sends. The service must ignore the hardware fields.
func checkLoginBody(code string) string {
	return `{"code":"` + code + `","client_id":"gen_online_60hz","machine_guid":"guid-value",` +
		`"mac_addr":"AA:BB:CC:DD:EE:FFZZ","vol_serial":"1234ABCDZZ","exe_crc":"abc","ini_crc":42}`
}

// checkLogin performs CheckLogin with a code from remoteAddr and returns the recorder.
func checkLogin(t *testing.T, handler http.Handler, code string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	return sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "CheckLogin", body: checkLoginBody(code), remoteAddr: remoteAddr})
}

func TestLoginCodeAnswerHasEveryKey(t *testing.T) {
	handler := testHandler(t)
	recorder := sendRequest(t, handler, testRequest{method: http.MethodGet, path: testPrefix + "LoginCode"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONKeys(t, recorder.Body.String(), []string{"success", "login_code"})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer["success"] != true {
		t.Errorf("success = %v, want true", answer["success"])
	}
	if code, _ := answer["login_code"].(string); code == "" {
		t.Errorf("login_code is empty")
	}
}

func TestLoginCodeIsRefusedPastTwentyUnusedCodes(t *testing.T) {
	handler := testHandler(t)
	for issued := 0; issued < 20; issued++ {
		getLoginCode(t, handler, "203.0.113.20:1000")
	}
	recorder := sendRequest(t, handler, testRequest{method: http.MethodGet, path: testPrefix + "LoginCode", remoteAddr: "203.0.113.20:1000"})

	answer := decodeAnswer(t, recorder.Body.String())
	if answer["success"] != false || answer["login_code"] != "" {
		t.Errorf("21st code answered %q, want success false and an empty code", recorder.Body.String())
	}
}

func TestCheckLoginCreatesAccountAndAnswersEveryKey(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	recorder := checkLogin(t, handler, code, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer["result"] != float64(1) {
		t.Errorf("result = %v, want 1", answer["result"])
	}
	if answer["user_id"] != float64(1000) {
		t.Errorf("user_id = %v, want 1000", answer["user_id"])
	}
	if answer["display_name"] != "Player1000" {
		t.Errorf("display_name = %v, want Player1000", answer["display_name"])
	}
	if answer["ws_uri"] != testWSURI {
		t.Errorf("ws_uri = %v, want %s", answer["ws_uri"], testWSURI)
	}
	session, _ := answer["session_token"].(string)
	refresh, _ := answer["refresh_token"].(string)
	if !hexToken.MatchString(session) || !hexToken.MatchString(refresh) {
		t.Errorf("tokens are not 64 hexadecimal digits")
	}
}

func TestCheckLoginCodeWorksOnceAndThenAnswersFailure(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	if recorder := checkLogin(t, handler, code, ""); recorder.Code != http.StatusOK {
		t.Fatalf("first CheckLogin status = %d, want 200", recorder.Code)
	}
	recorder := checkLogin(t, handler, code, "")

	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer["result"] != float64(2) || answer["user_id"] != float64(-1) {
		t.Errorf("second use answered %q, want result 2 and user_id -1", recorder.Body.String())
	}
}

func TestCheckLoginRefusesUnknownCodeWithEveryKey(t *testing.T) {
	handler := testHandler(t)
	recorder := checkLogin(t, handler, "never-issued", "")

	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
	if answer := decodeAnswer(t, recorder.Body.String()); answer["result"] != float64(2) {
		t.Errorf("result = %v, want 2", answer["result"])
	}
}

func TestCheckLoginMalformedBodyAnswersFailureWithEveryKey(t *testing.T) {
	handler := testHandler(t)
	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "CheckLogin", body: `{"code":5}`})

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
}

func TestCheckLoginAtAccountLimitAnswersFailureWithEveryKey(t *testing.T) {
	handler := newLoginHandler(t, false, 1)
	const publicAddress = "203.0.113.9:4000"
	first := getLoginCode(t, handler, publicAddress)
	if recorder := checkLogin(t, handler, first, publicAddress); recorder.Code != http.StatusOK {
		t.Fatalf("first account status = %d, want 200", recorder.Code)
	}

	second := getLoginCode(t, handler, publicAddress)
	recorder := checkLogin(t, handler, second, publicAddress)
	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
	if answer := decodeAnswer(t, recorder.Body.String()); answer["result"] != float64(2) || answer["user_id"] != float64(-1) {
		t.Errorf("over the limit answered %q, want result 2 and user_id -1", recorder.Body.String())
	}
}

func TestLoopbackAddressIsNotLimited(t *testing.T) {
	handler := newLoginHandler(t, false, 1)
	const loopbackAddress = "127.0.0.1:4000"
	for attempt := 0; attempt < 2; attempt++ {
		code := getLoginCode(t, handler, loopbackAddress)
		if answer := decodeAnswer(t, checkLogin(t, handler, code, loopbackAddress).Body.String()); answer["result"] != float64(1) {
			t.Errorf("loopback account %d refused: %v", attempt+1, answer)
		}
	}
}

func TestLimitUsesLastForwardedEntryWhenTrusted(t *testing.T) {
	handler := newLoginHandler(t, true, 1)
	const proxyAddress = "127.0.0.1:4000"
	first := getLoginCode(t, handler, proxyAddress)
	if answer := decodeAnswer(t, sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "CheckLogin", body: checkLoginBody(first), remoteAddr: proxyAddress, forwardedFor: "203.0.113.4"}).Body.String()); answer["result"] != float64(1) {
		t.Fatalf("first account refused: %v", answer)
	}

	second := sendRequest(t, handler, testRequest{method: http.MethodGet, path: testPrefix + "LoginCode", remoteAddr: proxyAddress, forwardedFor: "198.51.100.1, 203.0.113.4"})
	code := decodeAnswer(t, second.Body.String())["login_code"]
	if code == nil || code == "" {
		t.Fatalf("LoginCode with forwarded entries gave no code: %q", second.Body.String())
	}
	answer := decodeAnswer(t, sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "CheckLogin", body: checkLoginBody(code.(string)), remoteAddr: proxyAddress, forwardedFor: "198.51.100.1, 203.0.113.4"}).Body.String())
	if answer["result"] != float64(2) {
		t.Errorf("second account from the same last forwarded entry answered %v, want result 2", answer)
	}
}

func TestLoginWithTokenReturnsSameAccountAndTheGivenRefreshToken(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	first := decodeAnswer(t, checkLogin(t, handler, code, "").Body.String())
	refresh := first["refresh_token"].(string)

	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "LoginWithToken", token: refresh})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer["user_id"] != first["user_id"] || answer["display_name"] != "Player1000" {
		t.Errorf("login by token answered %q, want the account of the first login", recorder.Body.String())
	}
	if answer["refresh_token"] != refresh {
		t.Errorf("refresh_token = %v, want the presented token", answer["refresh_token"])
	}
}

func TestLoginWithUnknownTokenAnswers401(t *testing.T) {
	handler := testHandler(t)
	unknown := "00000000000000000000000000000000000000000000000000000000000000ff"
	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "LoginWithToken", token: unknown})

	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != `{}` {
		t.Errorf("got %d %q, want 401 {}", recorder.Code, recorder.Body.String())
	}
}

func TestLoginWithoutTokenHeaderAnswers401(t *testing.T) {
	handler := testHandler(t)
	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "LoginWithToken"})

	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != `{}` {
		t.Errorf("got %d %q, want 401 {}", recorder.Code, recorder.Body.String())
	}
}

func TestRefreshTokenGivesNewSessionToken(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	first := decodeAnswer(t, checkLogin(t, handler, code, "").Body.String())
	session := first["session_token"].(string)
	refresh := first["refresh_token"].(string)

	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "RefreshToken", token: refresh})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONKeys(t, recorder.Body.String(), []string{"result", "session_token"})
	answer := decodeAnswer(t, recorder.Body.String())
	if answer["result"] != float64(1) {
		t.Errorf("result = %v, want 1", answer["result"])
	}
	if newSession, _ := answer["session_token"].(string); !hexToken.MatchString(newSession) || newSession == session {
		t.Errorf("session_token = %v, want a new token", answer["session_token"])
	}
}

func TestRefreshWithUnknownTokenAnswers401(t *testing.T) {
	handler := testHandler(t)
	recorder := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "RefreshToken", token: "not-a-token"})

	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != `{}` {
		t.Errorf("got %d %q, want 401 {}", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteUserRemovesRefreshTokenOfCaller(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	first := decodeAnswer(t, checkLogin(t, handler, code, "").Body.String())
	session := first["session_token"].(string)
	refresh := first["refresh_token"].(string)
	userPath := testPrefix + "User/1000"

	recorder := sendRequest(t, handler, testRequest{method: http.MethodDelete, path: userPath, token: session})
	if recorder.Code != http.StatusOK || recorder.Body.String() != `{}` {
		t.Fatalf("DELETE User got %d %q, want 200 {}", recorder.Code, recorder.Body.String())
	}
	after := sendRequest(t, handler, testRequest{method: http.MethodPost, path: testPrefix + "LoginWithToken", token: refresh})
	if after.Code != http.StatusUnauthorized {
		t.Errorf("login with the deleted token got %d, want 401", after.Code)
	}
}

func TestDeleteOtherUserIsRefused(t *testing.T) {
	handler := testHandler(t)
	code := getLoginCode(t, handler, "")
	session := decodeAnswer(t, checkLogin(t, handler, code, "").Body.String())["session_token"].(string)

	recorder := sendRequest(t, handler, testRequest{method: http.MethodDelete, path: testPrefix + "User/1001", token: session})
	if recorder.Code != http.StatusForbidden || recorder.Body.String() != `{}` {
		t.Errorf("got %d %q, want 403 {}", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteUserWithoutSessionAnswers401(t *testing.T) {
	handler := testHandler(t)
	recorder := sendRequest(t, handler, testRequest{method: http.MethodDelete, path: testPrefix + "User/1000"})

	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != `{}` {
		t.Errorf("got %d %q, want 401 {}", recorder.Code, recorder.Body.String())
	}
}

func TestFixedCallWithoutLoginAnswers401(t *testing.T) {
	handler := testHandler(t)
	cases := []testRequest{
		{method: http.MethodGet, path: testPrefix + "Social/Friends"},
		{method: http.MethodPut, path: testPrefix + "Matchmaking"},
		{method: http.MethodGet, path: testPrefix + "GlobalStats"},
	}
	for _, testReq := range cases {
		recorder := sendRequest(t, handler, testReq)
		if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != `{}` {
			t.Errorf("%s %s got %d %q, want 401 {}", testReq.method, testReq.path, recorder.Code, recorder.Body.String())
		}
	}
}
