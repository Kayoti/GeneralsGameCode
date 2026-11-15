// This file holds the calls that log a player in and out (SERVICE.md section 4).
// The rules are in the accounts package; this file only reads requests and writes answers.

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"servers-lobby/internal/accounts"
)

// Values of AuthResponse.result (CLIENT_HTTP.md 4.3).
const (
	authResultSucceeded = 1
	authResultFailed    = 2
)

// userIDNone is user_id in a failed answer. It is the client's own default (CLIENT_HTTP.md 4.3).
const userIDNone = -1

// loginCodeAnswer is the answer of GET LoginCode (CLIENT_HTTP.md 4.2).
type loginCodeAnswer struct {
	Success   bool   `json:"success"`
	LoginCode string `json:"login_code"`
}

// authAnswer is the answer of CheckLogin and LoginWithToken (CLIENT_HTTP.md 4.3 and 4.4).
type authAnswer struct {
	Result       int    `json:"result"`
	SessionToken string `json:"session_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id"`
	DisplayName  string `json:"display_name"`
	WSURI        string `json:"ws_uri"`
}

// refreshAnswer is the answer of RefreshToken (CLIENT_HTTP.md 4.5).
type refreshAnswer struct {
	Result       int    `json:"result"`
	SessionToken string `json:"session_token"`
}

// checkLoginRequest holds the one field of CheckLogin that the service uses.
// The hardware fields the client sends are not read and never stored
// (SERVICE.md section 4, decision D46).
type checkLoginRequest struct {
	Code string `json:"code"`
}

// loginCallFor returns the handler of a call that logs in or out, when the
// method and the path name one of them.
func (routes *server) loginCallFor(method string, segments []string) (http.HandlerFunc, bool) {
	switch {
	case method == http.MethodGet && segmentsMatch([]string{"LoginCode"}, segments):
		return routes.loginCode, true
	case method == http.MethodPost && segmentsMatch([]string{"CheckLogin"}, segments):
		return routes.checkLogin, true
	case method == http.MethodPost && segmentsMatch([]string{"LoginWithToken"}, segments):
		return routes.loginWithToken, true
	case method == http.MethodPost && segmentsMatch([]string{"RefreshToken"}, segments):
		return routes.refreshToken, true
	case method == http.MethodDelete && segmentsMatch([]string{"User", placeholderSegment}, segments):
		userIDText := segments[1]
		return func(w http.ResponseWriter, r *http.Request) {
			routes.deleteUser(w, r, userIDText)
		}, true
	}
	return nil, false
}

// loginCode issues a login code for the caller's address. Past the limit of
// unused codes it answers success false, as SERVICE.md section 4 says.
func (routes *server) loginCode(w http.ResponseWriter, r *http.Request) {
	code, err := routes.accounts.IssueLoginCode(clientAddress(r, routes.trustForwardedFor))
	if errors.Is(err, accounts.ErrTooManyLoginCodes) {
		writeAnswer(w, http.StatusOK, loginCodeAnswer{Success: false, LoginCode: ""})
		return
	}
	if err != nil {
		routes.writeInternalError(w, "issue login code", err)
		return
	}
	writeAnswer(w, http.StatusOK, loginCodeAnswer{Success: true, LoginCode: code})
}

// checkLogin uses a login code. An unknown, used or expired code, and an
// address over the account creation limit, both answer a failure with every key.
func (routes *server) checkLogin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnswer(w, http.StatusBadRequest, failedAuthAnswer())
		return
	}
	var request checkLoginRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeAnswer(w, http.StatusBadRequest, failedAuthAnswer())
		return
	}
	login, err := routes.accounts.CheckLoginCode(request.Code, clientAddress(r, routes.trustForwardedFor))
	switch {
	case errors.Is(err, accounts.ErrUnknownLoginCode), errors.Is(err, accounts.ErrAccountLimit):
		writeAnswer(w, http.StatusOK, failedAuthAnswer())
	case err != nil:
		routes.logger.Error("check login failed", "error", err)
		writeAnswer(w, http.StatusInternalServerError, failedAuthAnswer())
	default:
		writeAnswer(w, http.StatusOK, routes.loginAnswer(login))
	}
}

// loginWithToken logs in the account of the refresh token in the Bearer
// header. An unknown token answers 401, which sends the client back to the code flow.
func (routes *server) loginWithToken(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	login, err := routes.accounts.LoginWithRefreshToken(token)
	if errors.Is(err, accounts.ErrUnknownRefreshToken) {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	if err != nil {
		routes.writeInternalError(w, "login with token", err)
		return
	}
	writeAnswer(w, http.StatusOK, routes.loginAnswer(login))
}

// refreshToken gives a new session token to the account of the refresh token
// in the Bearer header.
func (routes *server) refreshToken(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	sessionToken, err := routes.accounts.RefreshSession(token)
	if errors.Is(err, accounts.ErrUnknownRefreshToken) {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	if err != nil {
		routes.writeInternalError(w, "refresh token", err)
		return
	}
	writeAnswer(w, http.StatusOK, refreshAnswer{Result: authResultSucceeded, SessionToken: sessionToken})
}

// deleteUser removes the refresh token of the caller's own account, so that
// the next start creates a new account (SERVICE.md section 4). The caller is
// the session token in the Bearer header. A user id that is not the caller's
// is refused with 403; the documents name no status for that case.
func (routes *server) deleteUser(w http.ResponseWriter, r *http.Request, userIDText string) {
	callerID, ok := routes.accountForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	userID, err := strconv.ParseInt(userIDText, 10, 64)
	if err != nil || userID != callerID {
		writeJSON(w, http.StatusForbidden, emptyObject)
		return
	}
	if err := routes.accounts.DeleteRefreshToken(callerID); err != nil {
		routes.writeInternalError(w, "delete user", err)
		return
	}
	writeJSON(w, http.StatusOK, emptyObject)
}

// loginAnswer builds the successful answer of CheckLogin and LoginWithToken.
func (routes *server) loginAnswer(login accounts.Login) authAnswer {
	return authAnswer{
		Result:       authResultSucceeded,
		SessionToken: login.SessionToken,
		RefreshToken: login.RefreshToken,
		UserID:       login.UserID,
		DisplayName:  login.DisplayName,
		WSURI:        routes.wsURI,
	}
}

// failedAuthAnswer is the failure answer of CheckLogin: every key, with the client's defaults.
func failedAuthAnswer() authAnswer {
	return authAnswer{Result: authResultFailed, UserID: userIDNone}
}

// writeInternalError logs a failure of the service, without any token, and
// answers 500 with an empty object.
func (routes *server) writeInternalError(w http.ResponseWriter, call string, err error) {
	routes.logger.Error("call failed", "call", call, "error", err)
	writeJSON(w, http.StatusInternalServerError, emptyObject)
}

// writeAnswer writes value as the JSON body of an answer with the given status.
func writeAnswer(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, emptyObject)
		return
	}
	writeJSON(w, status, string(body))
}
