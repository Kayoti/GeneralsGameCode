// Tests of the contract calls, routing and the body limit, using httptest.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/store"
)

const testPrefix = "/env/prod/contract/1/"

// testWSURI is the ws_uri of the test service.
const testWSURI = "ws://127.0.0.1:9000/ws"

// openTestDB opens a new database file for one test.
func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "lobby.db"))
	if err != nil {
		t.Fatalf("store.Open returned error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testHandler returns the service handler on a new database, with no account
// creation limit. Its log goes to a buffer that the tests do not read.
func testHandler(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewHandler(false, testWSURI, accounts.New(openTestDB(t), 0), logger)
}

func perform(t *testing.T, handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	return performAs(t, handler, "", method, path, body)
}

// performAs sends a request with token in its Authorization: Bearer header.
// An empty token sends no Authorization header.
func performAs(t *testing.T, handler http.Handler, token string, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// sessionFor returns a handler and a session token of a new account, got
// through the login calls themselves (LoginCode, then CheckLogin).
func sessionFor(t *testing.T) (http.Handler, string) {
	t.Helper()
	handler := testHandler(t)
	codeAnswer := perform(t, handler, http.MethodGet, testPrefix+"LoginCode", "")
	var code struct {
		LoginCode string `json:"login_code"`
	}
	if err := json.Unmarshal(codeAnswer.Body.Bytes(), &code); err != nil || code.LoginCode == "" {
		t.Fatalf("LoginCode answer %q gave no code: %v", codeAnswer.Body.String(), err)
	}
	loginAnswer := perform(t, handler, http.MethodPost, testPrefix+"CheckLogin", `{"code":"`+code.LoginCode+`"}`)
	var login struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(loginAnswer.Body.Bytes(), &login); err != nil || login.SessionToken == "" {
		t.Fatalf("CheckLogin answer %q gave no session: %v", loginAnswer.Body.String(), err)
	}
	return handler, login.SessionToken
}

// assertJSONKeys checks that body is a JSON object with every key listed.
func assertJSONKeys(t *testing.T, body string, keys []string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &object); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Errorf("body %q has no key %q", body, key)
		}
	}
}

func TestFixedAnswers(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
		wantKeys   []string
	}{
		{"VersionCheck", http.MethodPost, testPrefix + "VersionCheck", http.StatusOK,
			`{"result":0,"patcher_name":"","patcher_path":"","patcher_size":0}`,
			[]string{"result", "patcher_name", "patcher_path", "patcher_size"}},
		{"PlayerStats by id", http.MethodGet, testPrefix + "PlayerStats/1000", http.StatusNotFound, `{}`, nil},
		{"PlayerStats update", http.MethodPut, testPrefix + "PlayerStats", http.StatusNotFound, `{}`, nil},
		{"PlayerStats batch", http.MethodPost, testPrefix + "PlayerStats/Batch", http.StatusOK,
			`{"stats":[]}`, []string{"stats"}},
		{"GlobalStats", http.MethodGet, testPrefix + "GlobalStats", http.StatusOK, `{}`, nil},
		{"Social friends", http.MethodGet, testPrefix + "Social/Friends", http.StatusOK,
			`{"friends":[],"pending_requests":[]}`, []string{"friends", "pending_requests"}},
		{"Social blocked", http.MethodGet, testPrefix + "Social/Blocked", http.StatusOK,
			`{"blocked":[]}`, []string{"blocked"}},
		{"Matchmaking playlists", http.MethodGet, testPrefix + "Matchmaking/Playlists", http.StatusOK,
			`{"playlists":[]}`, []string{"playlists"}},
		{"Matchmaking start", http.MethodPut, testPrefix + "Matchmaking", http.StatusOK, `{}`, nil},
		{"Matchmaking cancel", http.MethodDelete, testPrefix + "Matchmaking", http.StatusOK, `{}`, nil},
		{"Matchmaking widen", http.MethodPost, testPrefix + "Matchmaking/Widen", http.StatusOK, `{}`, nil},
		{"ProvideMWToken", http.MethodPost, testPrefix + "ProvideMWToken", http.StatusOK, `{}`, nil},
	}
	handler, token := sessionFor(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAs(t, handler, token, tc.method, tc.path, "")
			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if recorder.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tc.wantBody)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if tc.wantKeys != nil {
				assertJSONKeys(t, recorder.Body.String(), tc.wantKeys)
			}
		})
	}
}

func TestFixedAnswersAcceptEveryEnvironment(t *testing.T) {
	for _, env := range []string{"prod", "test", "dev"} {
		path := "/env/" + env + "/contract/1/VersionCheck"
		recorder := perform(t, testHandler(t), http.MethodPost, path, "{}")
		if recorder.Code != http.StatusOK {
			t.Errorf("env %s: status = %d, want 200", env, recorder.Code)
		}
	}
}

func TestOtherSocialCallsAnswerEmptyObject(t *testing.T) {
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPut, testPrefix + "Social/Friends/Requests/1001"},
		{http.MethodPost, testPrefix + "Social/Friends/Requests/1001"},
		{http.MethodDelete, testPrefix + "Social/Friends/Requests/1001"},
		{http.MethodDelete, testPrefix + "Social/Friends/1001"},
		{http.MethodPut, testPrefix + "Social/Blocked/1001"},
		{http.MethodDelete, testPrefix + "Social/Blocked/1001"},
		{http.MethodPost, testPrefix + "Social/Friends"},
	}
	handler, token := sessionFor(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			recorder := performAs(t, handler, token, tc.method, tc.path, "")
			if recorder.Code != http.StatusOK || recorder.Body.String() != `{}` {
				t.Errorf("got %d %q, want 200 {}", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestUnservedPathsAnswer404(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"unknown endpoint", http.MethodGet, testPrefix + "Nope"},
		{"endpoint not in this milestone", http.MethodGet, testPrefix + "Lobbies"},
		{"wrong method on a fixed call", http.MethodGet, testPrefix + "VersionCheck"},
		{"wrong method on PlayerStats batch", http.MethodGet, testPrefix + "PlayerStats/Batch"},
		{"unknown environment", http.MethodPost, "/env/staging/contract/1/VersionCheck"},
		{"wrong contract version", http.MethodPost, "/env/prod/contract/2/VersionCheck"},
		{"no environment", http.MethodPost, "/contract/1/VersionCheck"},
		{"empty endpoint", http.MethodGet, "/env/prod/contract/1/"},
		{"empty segment inside endpoint", http.MethodGet, "/env/prod/contract/1/Social//Friends"},
		{"social with no call", http.MethodGet, testPrefix + "Social"},
		{"root", http.MethodGet, "/"},
		{"health check with wrong method", http.MethodPost, "/healthz"},
	}
	handler, token := sessionFor(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAs(t, handler, token, tc.method, tc.path, "")
			if recorder.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", recorder.Code)
			}
			if recorder.Body.String() != `{}` {
				t.Errorf("body = %q, want {}", recorder.Body.String())
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	recorder := perform(t, testHandler(t), http.MethodGet, "/healthz", "")
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
	if recorder.Body.String() != "ok" {
		t.Errorf("body = %q, want ok", recorder.Body.String())
	}
}

func TestBodyOfExactlyTheLimitIsAccepted(t *testing.T) {
	body := strings.Repeat("a", maxRequestBodyBytes)
	recorder := perform(t, testHandler(t), http.MethodPost, testPrefix+"VersionCheck", body)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
}

func TestBodyOverTheLimitIsRefused(t *testing.T) {
	body := strings.Repeat("a", maxRequestBodyBytes+1)
	recorder := perform(t, testHandler(t), http.MethodPost, testPrefix+"VersionCheck", body)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", recorder.Code)
	}
	if recorder.Body.String() != `{}` {
		t.Errorf("body = %q, want {}", recorder.Body.String())
	}
}

func TestWithBodyLimitPutsBodyBackForNextHandler(t *testing.T) {
	const payload = `{"field":1}`
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading body in next handler: %v", err)
		}
		seen = string(data)
	})
	request := httptest.NewRequest(http.MethodPost, "/anything", strings.NewReader(payload))
	withBodyLimit(next).ServeHTTP(httptest.NewRecorder(), request)
	if seen != payload {
		t.Errorf("next handler read %q, want %q", seen, payload)
	}
}
