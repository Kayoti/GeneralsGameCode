// This file holds the lobby calls (SERVICE.md sections 5.1 and 7): list,
// create, detail, join, change, leave and the outcome. The rules are in the
// lobbies package and the Hub; this file reads requests and writes answers.

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"servers-lobby/internal/lobbies"
	"servers-lobby/internal/pass"
	"servers-lobby/internal/realtime"
)

// Values of CreateLobbyResponse.result (CLIENT_HTTP.md 4.13).
const (
	createResultFailed    = 0
	createResultSucceeded = 1
)

// lobbyIDNone is lobby_id in a failed create answer.
const lobbyIDNone = 0

// lobbySegment and outcomeSegment name the paths Lobby/{id} and Lobby/Outcome.
const (
	lobbySegment   = "Lobby"
	outcomeSegment = "Outcome"
)

// Values of the result of field 19 (SERVICE.md section 10).
const (
	serverSessionRefused = 0
	serverSessionStored  = 1
)

// listLatencyMs is the one latency given per lobby in GET Lobbies (SERVICE.md section 5.1).
const listLatencyMs = 0

// createLobbyAnswer is the answer of PUT Lobbies (CLIENT_HTTP.md 4.13).
type createLobbyAnswer struct {
	Result       int    `json:"result"`
	LobbyID      int64  `json:"lobby_id"`
	TurnUsername string `json:"turn_username"`
	TurnToken    string `json:"turn_token"`
}

// joinLobbyAnswer is the 200 answer of PUT Lobby/{id} (CLIENT_HTTP.md 4.15).
type joinLobbyAnswer struct {
	Success      bool   `json:"success"`
	TurnUsername string `json:"turn_username"`
	TurnToken    string `json:"turn_token"`
}

// lobbyListAnswer is the answer of GET Lobbies (CLIENT_HTTP.md 5.1). The two
// latency lists are arrays also when empty.
type lobbyListAnswer struct {
	Lobbies         []lobbies.ListLobby `json:"lobbies"`
	Latencies       []int               `json:"latencies"`
	PlayerLatencies []struct{}          `json:"playerlatencies"`
}

// lobbyDetailAnswer is the answer of GET Lobby/{id} (CLIENT_HTTP.md 5.2).
type lobbyDetailAnswer struct {
	Lobby lobbies.DetailLobby `json:"lobby"`
}

// serverSessionAnswer is the answer of field 19 (SERVICE.md section 10).
type serverSessionAnswer struct {
	Result int `json:"result"`
}

// outcomeAnswer is the answer of POST Lobby/Outcome (CLIENT_HTTP.md 4.18).
// Nothing is stored, so both addresses are empty and the client uploads nothing.
type outcomeAnswer struct {
	ScreenshotURL string `json:"screenshot_url"`
	ReplayURL     string `json:"replay_url"`
}

// lobbyCall is a handler of a lobby call that knows the caller's account.
type lobbyCall func(w http.ResponseWriter, r *http.Request, userID int64)

// lobbyCallFor returns the handler of a lobby call, when the method and the
// path name one. Each needs a session token.
func (routes *server) lobbyCallFor(method string, segments []string) (http.HandlerFunc, bool) {
	switch {
	case segmentsMatch([]string{"Lobbies"}, segments):
		return routes.lobbiesCallFor(method)
	case method == http.MethodPost && segmentsMatch([]string{lobbySegment, outcomeSegment}, segments):
		return routes.whenLoggedInAs(routes.outcomeCall), true
	case segmentsMatch([]string{lobbySegment, placeholderSegment}, segments):
		return routes.lobbyByIDCallFor(method, segments[1])
	}
	return nil, false
}

// lobbiesCallFor returns the handler of GET or PUT Lobbies.
func (routes *server) lobbiesCallFor(method string) (http.HandlerFunc, bool) {
	switch method {
	case http.MethodGet:
		return routes.whenLoggedInAs(routes.listLobbiesCall), true
	case http.MethodPut:
		return routes.whenLoggedInAs(routes.createLobbyCall), true
	}
	return nil, false
}

// lobbyByIDCallFor returns the handler of a call on Lobby/{id}. A segment that
// is not a lobby id names no lobby: GET and PUT answer 404, POST and DELETE
// answer 200 with no effect, as they do for a lobby that does not exist.
func (routes *server) lobbyByIDCallFor(method string, idText string) (http.HandlerFunc, bool) {
	lobbyID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || lobbyID < 1 {
		lobbyID = lobbyIDNone
	}
	var call lobbyCall
	switch method {
	case http.MethodGet:
		call = func(w http.ResponseWriter, _ *http.Request, userID int64) { routes.lobbyDetailCall(w, userID, lobbyID) }
	case http.MethodPut:
		call = func(w http.ResponseWriter, r *http.Request, userID int64) {
			routes.joinLobbyCall(w, r, userID, lobbyID)
		}
	case http.MethodPost:
		call = func(w http.ResponseWriter, r *http.Request, userID int64) {
			routes.changeLobbyCall(w, r, userID, lobbyID)
		}
	case http.MethodDelete:
		call = func(w http.ResponseWriter, _ *http.Request, userID int64) { routes.leaveLobbyCall(w, userID, lobbyID) }
	default:
		return nil, false
	}
	return routes.whenLoggedInAs(call), true
}

// whenLoggedInAs wraps a lobby call so that a request without a valid session
// token gets 401 with an empty object (SERVICE.md section 3).
func (routes *server) whenLoggedInAs(next lobbyCall) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := routes.accountForRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, emptyObject)
			return
		}
		next(w, r, userID)
	}
}

// readJSONBody decodes the request body into target. An empty body leaves
// target as it is. It returns an error for a body that is not valid JSON of
// the shape of target.
func readJSONBody(r *http.Request, target any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, target)
}

// listLobbiesCall answers GET Lobbies with the public lobbies that have not started.
func (routes *server) listLobbiesCall(w http.ResponseWriter, _ *http.Request, _ int64) {
	listed := routes.hub.ListLobbies()
	answer := lobbyListAnswer{
		Lobbies:         listed,
		Latencies:       make([]int, len(listed)),
		PlayerLatencies: []struct{}{},
	}
	for index := range answer.Latencies {
		answer.Latencies[index] = listLatencyMs
	}
	writeAnswer(w, http.StatusOK, answer)
}

// createLobbyCall answers PUT Lobbies. A request that is not valid, and an
// account with no WebSocket, get result 0 with every key present.
func (routes *server) createLobbyCall(w http.ResponseWriter, r *http.Request, userID int64) {
	failed := createLobbyAnswer{Result: createResultFailed, LobbyID: lobbyIDNone}
	var request lobbies.CreateRequest
	if err := readJSONBody(r, &request); err != nil {
		writeAnswer(w, http.StatusOK, failed)
		return
	}
	if request.ServerMode {
		tag, ok := routes.passes.Check(r.Header.Get(pass.Header), time.Now(), request.ExeCRC, request.IniCRC)
		if !ok {
			writeAnswer(w, http.StatusOK, failed)
			return
		}
		request.RegionTag = tag
	}
	lobbyID, err := routes.hub.CreateLobby(userID, request)
	if err != nil {
		var invalid *lobbies.InvalidError
		if !errors.As(err, &invalid) && !errors.Is(err, realtime.ErrNotConnected) {
			routes.logger.Error("create lobby", "userId", userID, "error", err)
		}
		writeAnswer(w, http.StatusOK, failed)
		return
	}
	username, token := routes.turnFor(request.ServerMode, userID)
	writeAnswer(w, http.StatusOK, createLobbyAnswer{
		Result: createResultSucceeded, LobbyID: lobbyID, TurnUsername: username, TurnToken: token,
	})
}

// lobbyDetailCall answers GET Lobby/{id}; only members see a lobby.
func (routes *server) lobbyDetailCall(w http.ResponseWriter, userID int64, lobbyID int64) {
	detail, found := routes.hub.LobbyDetail(userID, lobbyID)
	if !found {
		writeJSON(w, http.StatusNotFound, emptyObject)
		return
	}
	writeAnswer(w, http.StatusOK, lobbyDetailAnswer{Lobby: detail})
}

// joinStatuses are the answers to a failed join (SERVICE.md section 7.2).
var joinStatuses = []struct {
	err    error
	status int
}{
	{lobbies.ErrLobbyNotFound, http.StatusNotFound},
	{lobbies.ErrLobbyLocked, http.StatusNotAcceptable},
	{lobbies.ErrWrongPassword, http.StatusUnauthorized},
	{lobbies.ErrPassRefused, http.StatusForbidden},
	{lobbies.ErrLobbyFull, http.StatusNotAcceptable},
	{realtime.ErrNotConnected, http.StatusNotFound},
}

// joinLobbyCall answers PUT Lobby/{id}. A failed join has the status of
// SERVICE.md section 7.2 and an empty object.
func (routes *server) joinLobbyCall(w http.ResponseWriter, r *http.Request, userID int64, lobbyID int64) {
	var request lobbies.JoinRequest
	if err := readJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, emptyObject)
		return
	}
	header := r.Header.Get(pass.Header)
	request.Admit = func(regionTag string, exeCRC uint32, iniCRC uint32) bool {
		tag, ok := routes.passes.Check(header, time.Now(), exeCRC, iniCRC)
		return ok && tag == regionTag
	}
	isServer, err := routes.hub.JoinLobby(userID, lobbyID, request)
	if err == nil {
		username, token := routes.turnFor(isServer, userID)
		writeAnswer(w, http.StatusOK, joinLobbyAnswer{Success: true, TurnUsername: username, TurnToken: token})
		return
	}
	for _, failure := range joinStatuses {
		if errors.Is(err, failure.err) {
			writeJSON(w, failure.status, emptyObject)
			return
		}
	}
	routes.writeInternalError(w, "join lobby", err)
}

// changeLobbyCall answers POST Lobby/{id}: 200 with an empty object, and a
// refused change has no effect. Field 19 answers with a result (SERVICE.md section 10).
func (routes *server) changeLobbyCall(w http.ResponseWriter, r *http.Request, userID int64, lobbyID int64) {
	var request lobbies.FieldRequest
	// A key of the wrong type makes the read fail, but the keys that could be
	// read are kept, so a field 19 with a bad key is still answered with result 0.
	err := readJSONBody(r, &request)
	isSession := request.Field != nil && *request.Field == lobbies.FieldServerSession
	if err != nil && !isSession {
		writeJSON(w, http.StatusOK, emptyObject)
		return
	}
	if isSession {
		result := serverSessionRefused
		if err == nil && routes.hub.SetServerSession(userID, lobbyID, request) {
			result = serverSessionStored
		}
		writeAnswer(w, http.StatusOK, serverSessionAnswer{Result: result})
		return
	}
	routes.hub.ChangeLobby(userID, lobbyID, request)
	writeJSON(w, http.StatusOK, emptyObject)
}

// leaveLobbyCall answers DELETE Lobby/{id}, always with 200.
func (routes *server) leaveLobbyCall(w http.ResponseWriter, userID int64, lobbyID int64) {
	routes.hub.LeaveLobby(userID, lobbyID)
	writeJSON(w, http.StatusOK, emptyObject)
}

// outcomeCall answers POST Lobby/Outcome. Nothing is stored.
func (routes *server) outcomeCall(w http.ResponseWriter, _ *http.Request, _ int64) {
	writeAnswer(w, http.StatusOK, outcomeAnswer{})
}
