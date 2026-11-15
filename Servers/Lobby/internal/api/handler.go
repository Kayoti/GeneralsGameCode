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

// NewHandler returns the handler of every HTTP call. trustForwardedFor says
// whether the address in the request log is taken from X-Forwarded-For
// (SERVICE.md section 11). Every request is logged to logger.
func NewHandler(trustForwardedFor bool, logger *slog.Logger) http.Handler {
	return withRequestLog(trustForwardedFor, logger, withBodyLimit(http.HandlerFunc(serve)))
}

// serve answers the health check, the contract calls, and 404 for anything else.
func serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == healthPath {
		writeText(w, http.StatusOK, healthBody)
		return
	}
	segments, ok := contractSegments(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, emptyObject)
		return
	}
	answer, ok := fixedAnswerFor(r.Method, segments)
	if !ok {
		writeJSON(w, http.StatusNotFound, emptyObject)
		return
	}
	writeJSON(w, answer.status, answer.body)
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
