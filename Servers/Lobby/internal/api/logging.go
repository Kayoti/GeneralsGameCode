// This file writes the request log and works out the caller's address for it (SERVICE.md section 3).

package api

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// forwardedForHeader is the header that the reverse proxy in front of the
// service fills in (SERVICE.md section 11).
const forwardedForHeader = "X-Forwarded-For"

// ipv6KeptPrefixBits is how many leading bits of an IPv6 address are logged.
const ipv6KeptPrefixBits = 64

// unknownAddress is logged when the caller's address is not an IP address.
const unknownAddress = "-"

// statusRecorder keeps the status code of an answer for the request log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status == 0 {
		recorder.status = status
	}
	recorder.ResponseWriter.WriteHeader(status)
}

// Unwrap lets the WebSocket upgrade reach the connection under the recorder.
func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
	return recorder.ResponseWriter.Write(data)
}

// withRequestLog logs one line per request: method, path, status, duration
// and the caller's address with its last part hidden. No header value is
// logged (SERVICE.md section 3).
func withRequestLog(trustForwardedFor bool, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"durationMs", time.Since(start).Milliseconds(),
			"address", hideLastPart(clientAddress(r, trustForwardedFor)),
		)
	})
}

// clientAddress returns the caller's address. With trustForwardedFor on, it is
// the last entry of X-Forwarded-For, the entry added by the proxy in front of
// the service. Earlier entries come from the client and are ignored.
func clientAddress(r *http.Request, trustForwardedFor bool) string {
	if trustForwardedFor {
		if address := lastForwardedEntry(r.Header.Values(forwardedForHeader)); address != "" {
			return address
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}

// lastForwardedEntry returns the last entry of the last X-Forwarded-For line.
// A proxy may add its entry on a line of its own, so every line is considered
// and not only the first. It returns "" when there is no entry.
func lastForwardedEntry(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	entries := strings.Split(lines[len(lines)-1], ",")
	return strings.TrimSpace(entries[len(entries)-1])
}

// hideLastPart removes the last part of an IP address for the log. An IPv4
// address keeps its first three octets; an IPv6 address keeps its first 64
// bits. Anything that is not an IP address is logged as unknownAddress.
func hideLastPart(address string) string {
	ip := net.ParseIP(address)
	if ip == nil {
		return unknownAddress
	}
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("%d.%d.%d.x", ip4[0], ip4[1], ip4[2])
	}
	prefix := ip.Mask(net.CIDRMask(ipv6KeptPrefixBits, net.IPv6len*8))
	return prefix.String() + "x"
}
