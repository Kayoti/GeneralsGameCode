// Package api serves the lobby service's HTTP calls (SERVICE.md sections 2, 3 and 5).
//
// Routing is done here and not by http.ServeMux, because ServeMux answers a
// known path with the wrong method with 405. SERVICE.md asks for 404 with an
// empty object for every path that is not served.
package api

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"servers-lobby/internal/accounts"
	"servers-lobby/internal/pass"
	"servers-lobby/internal/realtime"
)

// maxRequestBodyBytes is the largest request body the service reads (SERVICE.md section 3).
const maxRequestBodyBytes = 64 * 1024

// The contract path is /env/{env}/contract/1/{Endpoint} (SERVICE.md section 2).
// These are its fixed parts; contractPrefixSegments counts the empty first
// segment and the four before the endpoint.
const (
	contractEnvSegment     = "env"
	contractSegment        = "contract"
	contractVersionSegment = "1"
	contractPrefixSegments = 5
)

// healthPath and healthBody are the health check of SERVICE.md section 2.
const (
	healthPath = "/healthz"
	healthBody = "ok"
)

// emptyObject is the body of every answer that carries no data.
const emptyObject = `{}`

// environments are the values of {env} the service accepts. It has no effect (SERVICE.md section 2).
var environments = map[string]bool{
	"prod": true,
	"test": true,
	"dev":  true,
}

// Options holds the configuration values the HTTP layer needs (SERVICE.md section 11).
type Options struct {
	// TrustForwardedFor says whether a caller's address is taken from X-Forwarded-For.
	TrustForwardedFor bool
	// WSURI is returned as ws_uri at login (section 2).
	WSURI string
	// MOTD is the text of GET MOTD.
	MOTD string
	// TurnSecret is the key of the TURN credentials (SERVICE.md section 8.1).
	// It is never logged. Empty means the answers carry empty credentials.
	TurnSecret string
	// Passes checks the X-Servers-Pass header of a server lobby (SERVICE.md
	// section 10). Nil refuses every pass.
	Passes *pass.Verifier
}

// server answers the contract calls. It holds what the calls need.
type server struct {
	accounts          *accounts.Service
	hub               *realtime.Hub
	trustForwardedFor bool
	wsURI             string
	motd              string
	turnSecret        string
	passes            *pass.Verifier
	logger            *slog.Logger
}

// NewHandler returns the handler of every HTTP call and of the WebSocket
// upgrade at /ws. Every request is logged to logger.
func NewHandler(options Options, accountService *accounts.Service, hub *realtime.Hub, logger *slog.Logger) http.Handler {
	routes := &server{
		accounts:          accountService,
		hub:               hub,
		trustForwardedFor: options.TrustForwardedFor,
		wsURI:             options.WSURI,
		motd:              options.MOTD,
		turnSecret:        options.TurnSecret,
		passes:            options.Passes,
		logger:            logger,
	}
	return withRequestLog(options.TrustForwardedFor, logger, withBodyLimit(http.HandlerFunc(routes.serve)))
}

// serve answers the health check, the WebSocket upgrade, the login calls, the
// calls that return configuration, the fixed calls, and 404 for anything else.
func (routes *server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == healthPath {
		writeText(w, http.StatusOK, healthBody)
		return
	}
	if r.URL.Path == webSocketPath {
		routes.serveWebSocket(w, r)
		return
	}
	segments, ok := contractSegments(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, emptyObject)
		return
	}
	if handle, found := routes.loginCallFor(r.Method, segments); found {
		handle(w, r)
		return
	}
	if handle, found := routes.infoCallFor(r.Method, segments); found {
		handle(w, r)
		return
	}
	if handle, found := routes.lobbyCallFor(r.Method, segments); found {
		handle(w, r)
		return
	}
	answer, ok := fixedAnswerFor(r.Method, segments)
	if !ok {
		writeJSON(w, http.StatusNotFound, emptyObject)
		return
	}
	if answer.needsLogin && !routes.isLoggedIn(r) {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	writeJSON(w, answer.status, answer.body)
}

// bearerPrefix starts the Authorization header of a call that carries a token (SERVICE.md section 3).
const bearerPrefix = "Bearer "

// bearerToken returns the token of the request's Authorization: Bearer header.
// The bool is false when there is no such header or it has no token.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}
	token := strings.TrimPrefix(header, bearerPrefix)
	return token, token != ""
}

// accountForRequest returns the account that the request's session token
// belongs to. The bool is false when the request has no valid session token,
// which SERVICE.md section 3 answers with 401.
func (routes *server) accountForRequest(r *http.Request) (int64, bool) {
	token, ok := bearerToken(r)
	if !ok {
		return 0, false
	}
	return routes.accounts.UserForSession(token)
}

// isLoggedIn reports whether the request carries a valid session token.
func (routes *server) isLoggedIn(r *http.Request) bool {
	_, ok := routes.accountForRequest(r)
	return ok
}

// contractSegments returns the endpoint segments of a path of the form
// /env/{env}/contract/1/{Endpoint}. It returns false for any other path,
// for an unknown environment, and for an empty segment.
func contractSegments(path string) ([]string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) <= contractPrefixSegments {
		return nil, false
	}
	if parts[0] != "" || parts[1] != contractEnvSegment || !environments[parts[2]] {
		return nil, false
	}
	if parts[3] != contractSegment || parts[4] != contractVersionSegment {
		return nil, false
	}
	segments := parts[contractPrefixSegments:]
	for _, segment := range segments {
		if segment == "" {
			return nil, false
		}
	}
	return segments, true
}

// withBodyLimit reads the request body up to maxRequestBodyBytes and answers
// 413 when it is longer. The body is put back in the request, so the handlers
// can read it again.
func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeJSON(w, http.StatusRequestEntityTooLarge, emptyObject)
				return
			}
			writeJSON(w, http.StatusBadRequest, emptyObject)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// writeJSON writes a JSON answer with the given status and body.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeBody(w, body)
}

// writeText writes a plain text answer with the given status and body.
func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	writeBody(w, body)
}

// writeBody writes the body of an answer. A write error means the client has
// gone away, so there is no one left to tell.
func writeBody(w http.ResponseWriter, body string) {
	_, _ = io.WriteString(w, body)
}
