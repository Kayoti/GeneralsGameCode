// This file holds the WebSocket upgrade (SERVICE.md section 6.1). The
// connection itself is served by the realtime package.

package api

import "net/http"

// webSocketPath is the address of the WebSocket (SERVICE.md section 2).
const webSocketPath = "/ws"

// serveWebSocket upgrades a request to a WebSocket for the account of its
// session token. A missing, unknown or expired token gets 401 with an empty
// object, before any upgrade.
func (routes *server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	userID, ok := routes.accountForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, emptyObject)
		return
	}
	routes.hub.ServeWS(w, r, userID)
}
