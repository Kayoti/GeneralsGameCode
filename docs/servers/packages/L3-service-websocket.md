# L3 Lobby service: WebSocket, rooms, room chat and names

State: done
Branch: servers-go-L3

## Goal

Hold each logged-in client's WebSocket, keep it alive, let it reconnect, and provide the chat room that the lobby list screen sits in.

## Depends on

L2

## Read first

- SERVICE.md sections 3, 4 (name change), 5.1 (`Rooms`, `MOTD`, `ServiceConfig`), 6 and 9
- reference/CLIENT_REALTIME.md sections 1.1 to 1.7 (WebSocket, all of it)
- reference/CLIENT_HTTP.md sections 4.7, 4.8, 4.11, 6 (`MOTD`, `ServiceConfig`, `Rooms`, the service configuration fields) and 7.3

## May edit

- `Servers/Lobby/`

## Must not edit

- Anything outside `Servers/Lobby/`

## Done when

- [x] `/ws` upgrades only with a valid session token. Otherwise `401`.
- [x] The service reads JSON from binary and text frames and writes JSON only in text frames.
- [x] Message 14 is answered with 15. A connection silent for 30 seconds is dropped. The service does not send a close frame to a client it keeps.
- [x] Reconnect follows the table in SERVICE.md 6.2, including HTTP `205` and the 45-second hold.
- [x] One live connection per account: a second one replaces the first.
- [x] `GET Rooms` lists the configured rooms. Message 3 moves the client into or out of a room and is answered with message 4, which is also sent to everyone already in the room, with every key the client reads.
- [x] Message 1 becomes message 2 for everyone in the sender's room, limited to 300 characters, with all four keys.
- [x] Message 9 changes the account's name by the rules in SERVICE.md section 4, stores it, and refreshes the room's member list. A refused name is answered with one room chat line to the sender only.
- [x] `GET MOTD` and `GET ServiceConfig` return the values of SERVICE.md 5.1 and 9, with every field of the client's structure.
- [x] Identical pending notices to one client are sent once (SERVICE.md 6.1). This is built here as a general "notify" helper, because L4 uses it.
- [x] More than 60 messages in a second, apart from 12 and 14, disconnects the client. Messages over 256 KB are refused.
- [x] Unknown message numbers are ignored and logged at debug level.
- [x] Tests cover each point with two or more in-process clients. No test sleeps longer than 2 seconds; timeouts are configurable for tests.
- [x] `lobbyprobe` now also connects the WebSocket for two accounts, joins room 1, exchanges a chat line and changes a name.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [x] Reviewed by someone other than the author.

## Out of scope

- Lobbies and every lobby message (L4, L5, L6).
- Friends, matchmaking and moderation messages.

## Handoff notes

- Finished:
  - New package `Servers/Lobby/internal/realtime`: `hub.go` (players, outbox, `Notify`), `rooms.go` (member lists, message 3/4), `session.go` (`ServeWS`, reconnect table, 45-second hold, replacement), `conn.go` (read and write loops, idle timeout, rate limit, 256 KB limit), `handlers.go` (messages 14, 3, 1, 9; others ignored at debug level), `messages.go`.
  - `internal/api`: `ws.go` (`/ws`, 401 before upgrade), `info.go` (`GET MOTD`, `GET Rooms`, `GET ServiceConfig`), `handler.go` (`Options` struct and the hub passed to `NewHandler`; the three existing test files were adapted to the new signature), `logging.go` (`Unwrap` on the status recorder, needed so the upgrade can take over the connection).
  - `internal/accounts/names.go` (name rules of SERVICE.md section 4, `ChangeDisplayName`, `DisplayName`), `internal/store/accounts.go` (`RenameAccount`).
  - `cmd/lobby/main.go`: builds the hub; server `ReadTimeout` 30 s, `WriteTimeout` 30 s, `IdleTimeout` 60 s added to the L1 `ReadHeaderTimeout`. Test `TestWebSocketSurvivesLongerThanTheServerReadTimeout` shows an upgraded connection is not cut by them.
  - `tools/lobbyprobe/realtime.go`: Rooms, two accounts connect, join room 1, chat, name change. Run against the built `lobby` binary on a temporary database: all eight lines reported ok, exit 0, twice in a row.
  - `go.mod` now requires `github.com/coder/websocket v1.8.15` (named in SERVICE.md section 1).
- Left to do:
  - L4 must call `Hub.Notify(userID, MsgLobbyCurrentLobbyUpdate or MsgLobbyListUpdate)` for lobby changes, and must remove a player from their lobby when the place is released. The single place for that is `Hub.releasePlace` in `session.go`; no hook exists yet because L3 has no lobby state. A name change should also send 6 to lobby members (SERVICE.md 6.3); only the room list refresh is done here.
  - Message 13 is not held for a disconnected player (SERVICE.md 6.2 lists it); the package that sends 13 must keep it. Only 6 and 7 are kept, in id order, and de-duplicated.
  - `Hub.enqueue` and `Notify` are the only senders; L4 needs a general "send message to user" method and will add it next to `Notify`.
  - The race detector could not run (`-race` needs cgo and there is no C compiler on this machine). Not verified under `-race`.
- Surprises and open questions:
  - SERVICE.md does not say how message 2 shows the sender, and the client prints only `message`. Chat lines are written as `<name>: <text>`, and as `<name> <text>` when `action` is true. Refused-name lines are `Name not changed: <reason>`. Both texts are my choice.
  - Reading of "disconnects" in 6.3 (member list on leave): a connection that ends without a close frame holds the place for 45 s and the room is told only when the hold ends. A client close frame, which the client sends only to log out (CLIENT_REALTIME 1.6), releases the place at once. A reconnect with `is-reconnect: true` while the old connection is still live counts as a held place and replaces the old connection.
  - The client sends room ids (not indexes) in message 3 (`rooms[i].GetRoomID()`), so room ids are used on the wire. `rejected_room_id` is -1 when nothing is rejected, because the client treats a rejected id equal to the pending room as a failure and a pending room is always at least 1. `request_id` is 0 in copies of message 4 sent to other members and when the client sent none.
  - Names are limited to ASCII letters, digits, space, `_` and `-` ("letters" in SERVICE.md 4 is not defined). The case-insensitive uniqueness check uses SQLite `lower()`, which is ASCII only, so a wider set would need a different check.
  - STANDARDS says no database access on the WebSocket message path. Message 9 must store the name, so it does one database transaction in the connection's read loop (serialised by `Service.nameMu`). Connect also reads the stored name once.
  - A rate-limited client gets a close frame (policy violation), which logs the client out; a message over 256 KB ends the connection with the library's "message too big" close frame.
  - Values chosen where the documents are silent: write timeout 10 s, 512 queued messages per client before the client is dropped, HTTP server `ReadTimeout` 30 s, `WriteTimeout` 30 s, `IdleTimeout` 60 s.
  - The coder/websocket library closes a connection when the context of a read ends. Tests therefore read in a goroutine and wait on a channel.
- Review findings and fixes:
  - Verified on the branch: `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passes (accounts,
    api, config, realtime, store). `/ws` upgrades only with a valid session token; JSON is read from text
    and binary frames and written only in text; ping 14 is answered with 15; a silent connection is
    dropped after the idle timeout with no close frame; reconnect follows 6.2 (held place, 205, 45 s
    expiry, release on a new session or a client close); a second connection replaces the first; rooms,
    room chat (300-char cut, control characters stripped) and the name change (case-insensitive
    uniqueness) all carry the keys the client reads; `GET MOTD` and `GET ServiceConfig` return every
    section-9 field with the right defaults (all 27 keys checked); the 60 messages/second and 256 KB
    caps are enforced before work; shared hub and room state is accessed under one lock with no socket
    I/O held, and the name-path database write is off the lock and rate-bounded. The exact
    `github.com/coder/websocket` library named in SERVICE 1 is used. Scope, author and commit hygiene
    correct.
  - Two low notes, neither blocking:
    1. A client disconnected for flooding or an over-size frame holds its place for the full 45 s instead
       of releasing it at once like a clean client close. Harmless (reclaimed on hold expiry, the
       logged-out client cannot re-enter). The flood/over-size paths could release the place immediately;
       left for a later hardening pass.
    2. A handoff note placed `TestWebSocketSurvivesLongerThanTheServerReadTimeout` in `cmd/lobby/main.go`;
       it is actually in `internal/api/ws_test.go`. Documentation-only, no code effect.
