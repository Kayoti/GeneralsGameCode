// Command lobbyprobe checks the login calls of a running lobby service the way
// the game client calls them (SERVICE.md section 13). It runs VersionCheck,
// LoginCode, CheckLogin, RefreshToken and LoginWithToken in that order, then
// connects the WebSocket for two accounts, joins room 1, exchanges a chat line
// and changes a name. It prints one line per step, and exits with status 1 at
// the first step that fails. It never prints a token.
//
// Usage: lobbyprobe --url <base address> --env <environment>
// For example: lobbyprobe --url http://127.0.0.1:9000 --env dev
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// requestTimeout limits one call to the service.
const requestTimeout = 10 * time.Second

// clientID is the client_id the game client sends at CheckLogin (CLIENT_HTTP.md 4.3).
const clientID = "gen_online_60hz"

// resultSucceeded and resultNoUpdate are values of the result fields (CLIENT_HTTP.md 4.1 and 4.3).
const (
	resultSucceeded = 1
	resultNoUpdate  = 0
)

// maxAnswerBytes limits how much of a reply the probe reads.
const maxAnswerBytes = 1 << 20

// probe holds the state that the steps pass to each other.
type probe struct {
	client   *http.Client
	baseURL  string
	contract string
	out      io.Writer
	code     string
	session  string
	refresh  string
	userID   float64
}

func main() {
	url := flag.String("url", "http://127.0.0.1:9000", "base address of the lobby service")
	env := flag.String("env", "dev", "environment name in the contract path")
	flag.Parse()
	if err := run(*url, *env, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "lobbyprobe:", err)
		os.Exit(1)
	}
}

// run performs every step against the service at baseURL and writes one line per step to out.
func run(baseURL string, env string, out io.Writer) error {
	p := &probe{
		client:   &http.Client{Timeout: requestTimeout},
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		contract: fmt.Sprintf("%s/env/%s/contract/1", strings.TrimSuffix(baseURL, "/"), env),
		out:      out,
	}
	steps := []func() error{
		p.versionCheck,
		p.loginCode,
		p.checkLogin,
		p.refreshToken,
		p.loginWithToken,
		p.webSocketRoom,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// versionCheck sends the four request keys of CLIENT_HTTP.md 4.1 and expects no update.
func (p *probe) versionCheck() error {
	answer, err := p.call("VersionCheck", http.MethodPost, "", `{"execrc":0,"ver":1,"netver":1,"servicesver":1}`)
	if err != nil {
		return p.fail("VersionCheck", err)
	}
	if err := requireKeys(answer, "result", "patcher_name", "patcher_path", "patcher_size"); err != nil {
		return p.fail("VersionCheck", err)
	}
	if answer["result"] != float64(resultNoUpdate) {
		return p.fail("VersionCheck", fmt.Errorf("result is %v, want %d (no update)", answer["result"], resultNoUpdate))
	}
	return p.report("VersionCheck: ok, no update")
}

// loginCode asks for a login code and keeps it for CheckLogin.
func (p *probe) loginCode() error {
	answer, err := p.call("LoginCode", http.MethodGet, "", "")
	if err != nil {
		return p.fail("LoginCode", err)
	}
	if err := requireKeys(answer, "success", "login_code"); err != nil {
		return p.fail("LoginCode", err)
	}
	code, _ := answer["login_code"].(string)
	if answer["success"] != true || code == "" {
		return p.fail("LoginCode", errors.New("no login code was given"))
	}
	p.code = code
	return p.report("LoginCode: ok")
}

// checkLogin uses the login code, which creates the account, and keeps its tokens.
func (p *probe) checkLogin() error {
	body, err := json.Marshal(map[string]string{"code": p.code, "client_id": clientID})
	if err != nil {
		return p.fail("CheckLogin", err)
	}
	answer, err := p.call("CheckLogin", http.MethodPost, "", string(body))
	if err != nil {
		return p.fail("CheckLogin", err)
	}
	if err := requireLoginAnswer(answer); err != nil {
		return p.fail("CheckLogin", err)
	}
	p.session, _ = answer["session_token"].(string)
	p.refresh, _ = answer["refresh_token"].(string)
	p.userID, _ = answer["user_id"].(float64)
	return p.report(fmt.Sprintf("CheckLogin: ok, user id %.0f", p.userID))
}

// refreshToken exchanges the refresh token for a new session token.
func (p *probe) refreshToken() error {
	answer, err := p.call("RefreshToken", http.MethodPost, p.refresh, "")
	if err != nil {
		return p.fail("RefreshToken", err)
	}
	if err := requireKeys(answer, "result", "session_token"); err != nil {
		return p.fail("RefreshToken", err)
	}
	session, _ := answer["session_token"].(string)
	if answer["result"] != float64(resultSucceeded) || session == "" || session == p.session {
		return p.fail("RefreshToken", errors.New("no new session token was given"))
	}
	p.session = session
	return p.report("RefreshToken: ok")
}

// loginWithToken logs in with the stored refresh token and expects the same account.
func (p *probe) loginWithToken() error {
	answer, err := p.call("LoginWithToken", http.MethodPost, p.refresh, "")
	if err != nil {
		return p.fail("LoginWithToken", err)
	}
	if err := requireLoginAnswer(answer); err != nil {
		return p.fail("LoginWithToken", err)
	}
	refresh, _ := answer["refresh_token"].(string)
	userID, _ := answer["user_id"].(float64)
	if refresh != p.refresh || userID != p.userID {
		return p.fail("LoginWithToken", errors.New("the account or the refresh token differs from the first login"))
	}
	return p.report(fmt.Sprintf("LoginWithToken: ok, user id %.0f", userID))
}

// call sends one request to endpoint of the contract. The token, when not
// empty, is sent as Authorization: Bearer. It returns an error for any status
// other than 200, and for a body that is not a JSON object. The error text
// never includes the body.
func (p *probe) call(endpoint string, method string, token string, body string) (map[string]any, error) {
	request, err := http.NewRequest(method, p.contract+"/"+endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return nil, fmt.Errorf("read answer: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", response.StatusCode)
	}
	var answer map[string]any
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&answer); err != nil || answer == nil {
		return nil, errors.New("answer is not a JSON object")
	}
	return answer, nil
}

// requireLoginAnswer checks the keys of AuthResponse and that the login succeeded.
func requireLoginAnswer(answer map[string]any) error {
	if err := requireKeys(answer, "result", "session_token", "refresh_token", "user_id", "display_name", "ws_uri"); err != nil {
		return err
	}
	session, _ := answer["session_token"].(string)
	refresh, _ := answer["refresh_token"].(string)
	userID, _ := answer["user_id"].(float64)
	if answer["result"] != float64(resultSucceeded) || session == "" || refresh == "" || userID == -1 {
		return errors.New("login did not succeed")
	}
	return nil
}

// requireKeys checks that the answer has every key listed.
func requireKeys(answer map[string]any, keys ...string) error {
	for _, key := range keys {
		if _, found := answer[key]; !found {
			return fmt.Errorf("answer has no key %q", key)
		}
	}
	return nil
}

// report writes one line for a step that passed.
func (p *probe) report(line string) error {
	_, err := fmt.Fprintln(p.out, line)
	return err
}

// fail returns the error of a step that did not pass, naming the step.
func (p *probe) fail(step string, err error) error {
	return fmt.Errorf("%s: failed: %w", step, err)
}
