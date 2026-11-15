// Tests of the request log and the address shown in it.

package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"servers-lobby/internal/accounts"
)

// loggedRequest sends one request through the handler and returns the log text.
func loggedRequest(t *testing.T, trustForwardedFor bool, request *http.Request) string {
	t.Helper()
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	handler := buildHandler(Options{TrustForwardedFor: trustForwardedFor, WSURI: testWSURI}, accounts.New(openTestDB(t), 0), logger)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	return logBuffer.String()
}

func TestRequestLogShowsMethodPathStatusDurationAndMaskedAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, testPrefix+"VersionCheck", strings.NewReader("{}"))
	request.RemoteAddr = "203.0.113.45:5555"
	logText := loggedRequest(t, false, request)

	for _, want := range []string{
		"method=POST",
		"path=/env/prod/contract/1/VersionCheck",
		"status=200",
		"durationMs=",
		"address=203.0.113.x",
	} {
		if !strings.Contains(logText, want) {
			t.Errorf("log %q does not contain %q", logText, want)
		}
	}
	if strings.Contains(logText, "203.0.113.45") {
		t.Errorf("log %q contains the full address", logText)
	}
}

func TestRequestLogNeverShowsHeaderValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, testPrefix+"Social/Friends", nil)
	request.Header.Set("Authorization", "Bearer secret-session-value")
	request.Header.Set("User-Agent", "agent-string-value")
	logText := loggedRequest(t, false, request)

	for _, secret := range []string{"secret-session-value", "agent-string-value"} {
		if strings.Contains(logText, secret) {
			t.Errorf("log %q contains header value %q", logText, secret)
		}
	}
}

func TestRequestLogUsesLastForwardedEntryWhenTrusted(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, testPrefix+"GlobalStats", nil)
	request.RemoteAddr = "192.0.2.10:4444"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.2")
	logText := loggedRequest(t, true, request)

	if !strings.Contains(logText, "address=10.0.0.x") {
		t.Errorf("log %q does not use the last forwarded entry", logText)
	}
	if strings.Contains(logText, "198.51.100") {
		t.Errorf("log %q uses the client-supplied forwarded entry", logText)
	}
}

func TestRequestLogUsesLastForwardedLineWhenProxyAddsOwnLine(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, testPrefix+"GlobalStats", nil)
	request.RemoteAddr = "192.0.2.10:4444"
	request.Header.Add("X-Forwarded-For", "198.51.100.7")
	request.Header.Add("X-Forwarded-For", "10.0.0.2")
	logText := loggedRequest(t, true, request)

	if !strings.Contains(logText, "address=10.0.0.x") {
		t.Errorf("log %q does not use the entry on the last forwarded line", logText)
	}
}

func TestRequestLogIgnoresForwardedHeaderWhenNotTrusted(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, testPrefix+"GlobalStats", nil)
	request.RemoteAddr = "192.0.2.10:4444"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	logText := loggedRequest(t, false, request)

	if !strings.Contains(logText, "address=192.0.2.x") {
		t.Errorf("log %q does not use the connection address", logText)
	}
}

func TestRequestLogShowsRefusedBodyStatus(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, testPrefix+"VersionCheck",
		strings.NewReader(strings.Repeat("a", maxRequestBodyBytes+1)))
	logText := loggedRequest(t, false, request)

	if !strings.Contains(logText, "status=413") {
		t.Errorf("log %q does not show status 413", logText)
	}
}

func TestHideLastPart(t *testing.T) {
	cases := map[string]string{
		"203.0.113.45":         "203.0.113.x",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::x",
		"not-an-address":       unknownAddress,
		"":                     unknownAddress,
	}
	for address, want := range cases {
		if got := hideLastPart(address); got != want {
			t.Errorf("hideLastPart(%q) = %q, want %q", address, got, want)
		}
	}
}
