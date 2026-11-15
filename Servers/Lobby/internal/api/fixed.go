// This file holds the calls answered without state (SERVICE.md section 5.2).

package api

import (
	"net/http"
	"strings"
)

// fixedAnswer is a call that is answered without any state. The bodies are the
// minimal responses of CLIENT_HTTP.md section 7.1 (SERVICE.md section 5.2).
// needsLogin is false only for VersionCheck, which the client sends before
// any login (CLIENT_HTTP.md 2.1).
type fixedAnswer struct {
	method     string
	endpoint   string
	status     int
	body       string
	needsLogin bool
}

// placeholderSegment is the path segment that matches any one non-empty segment.
const placeholderSegment = "{id}"

// socialSegment is the first segment of the social calls (SERVICE.md section 5.2).
const socialSegment = "Social"

// fixedAnswers lists the calls of SERVICE.md section 5.2 and VersionCheck of
// section 4. Each body holds every key the client reads for that call.
var fixedAnswers = []fixedAnswer{
	{http.MethodPost, "VersionCheck", http.StatusOK, `{"result":0,"patcher_name":"","patcher_path":"","patcher_size":0}`, false},
	{http.MethodGet, "PlayerStats/{id}", http.StatusNotFound, `{}`, true},
	// Any status other than 2xx only makes the client log the answer. A 2xx
	// would make it fetch PlayerStats/{id} again.
	{http.MethodPut, "PlayerStats", http.StatusNotFound, `{}`, true},
	{http.MethodPost, "PlayerStats/Batch", http.StatusOK, `{"stats":[]}`, true},
	{http.MethodGet, "GlobalStats", http.StatusOK, `{}`, true},
	{http.MethodGet, "Social/Friends", http.StatusOK, `{"friends":[],"pending_requests":[]}`, true},
	{http.MethodGet, "Social/Blocked", http.StatusOK, `{"blocked":[]}`, true},
	{http.MethodGet, "Matchmaking/Playlists", http.StatusOK, `{"playlists":[]}`, true},
	{http.MethodPut, "Matchmaking", http.StatusOK, `{}`, true},
	{http.MethodDelete, "Matchmaking", http.StatusOK, `{}`, true},
	{http.MethodPost, "Matchmaking/Widen", http.StatusOK, `{}`, true},
	{http.MethodPost, "ProvideMWToken", http.StatusOK, `{}`, true},
}

// fixedAnswerFor returns the fixed answer for a method and endpoint segments.
// Every other social call gets an empty object, because the client ignores
// its answer (SERVICE.md section 5.2 and CLIENT_HTTP.md 7.1).
func fixedAnswerFor(method string, segments []string) (fixedAnswer, bool) {
	for _, answer := range fixedAnswers {
		if answer.method == method && segmentsMatch(strings.Split(answer.endpoint, "/"), segments) {
			return answer, true
		}
	}
	if len(segments) >= 2 && segments[0] == socialSegment {
		return fixedAnswer{status: http.StatusOK, body: emptyObject, needsLogin: true}, true
	}
	return fixedAnswer{}, false
}

// segmentsMatch reports whether the path segments fit a pattern. A pattern
// segment that is placeholderSegment fits any one segment.
func segmentsMatch(pattern []string, segments []string) bool {
	if len(pattern) != len(segments) {
		return false
	}
	for index, patternSegment := range pattern {
		if patternSegment != placeholderSegment && patternSegment != segments[index] {
			return false
		}
	}
	return true
}
