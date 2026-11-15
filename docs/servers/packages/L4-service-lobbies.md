# L4 Lobby service: lobbies

State: done
Branch: servers-go-L4

## Goal

Let players create, list, join, change and leave lobbies, with chat and ready state, up to the start message.

## Depends on

L3

## Read first

- SERVICE.md sections 5.1, 6.3 and 7
- reference/CLIENT_HTTP.md sections 4.12 to 4.18 and 5 (lobby calls and the lobby JSON, all of it)
- reference/CLIENT_REALTIME.md section 1.7 rows 5, 6, 7, 10, 11, 13, 23, 24, 25, and section 4.1 (match start in a custom lobby)

## May edit

- `Servers/Lobby/`

## Must not edit

- Anything outside `Servers/Lobby/`

## Done when

- [x] `PUT Lobbies`, `GET Lobbies`, `GET Lobby/{id}`, `PUT Lobby/{id}`, `POST Lobby/{id}`, `DELETE Lobby/{id}` and `POST Lobby/Outcome` behave as SERVICE.md sections 5.1 and 7 say.
- [x] Every lobby and member object carries every key in CLIENT_HTTP.md 5.1 and 5.2, with the right JSON type. A lobby always has exactly 8 member entries. A test checks the key set of the list answer and of the detail answer against the lists in that reference.
- [x] Join failures use the status codes in SERVICE.md 7.2.
- [x] Every `field` value in SERVICE.md 7.3 except 19 is handled, with the "who" rule, range checks and the ready-state rules. The host is always ready. Unknown fields and field 7 have no effect.
- [x] The countdown lock (message 23) follows SERVICE.md 6.3: host only, blocks joins and member changes, ends after 25 seconds, at match start, or on a new 26.
- [x] A held place keeps its pending 6, 7 and 13 and receives them on reconnect (SERVICE.md 6.2).
- [x] Messages 5, 10/11, 24, 25 and 23 behave as SERVICE.md 6.3 says.
- [x] After every lobby change members get message 6, and after every change to the open list, watchers get message 7, through the notify helper from L3.
- [x] Leaving, kicking, logout and an expired hold all follow SERVICE.md 7.4, including the change of `Owner` and deletion of empty lobbies.
- [x] Message 13 from the host follows SERVICE.md 7.5 steps 1 to 3. Every human member, including the host, receives it.
- [x] A player can be in one lobby at a time.
- [x] Lobby state is in memory only, guarded for concurrent use. No database access on the message path.
- [x] Tests cover create, list, join (each failure), each field change, ready, chat, password change, kick, host leaving, last member leaving, start with and without everyone ready.
- [x] `lobbyprobe` now also runs: account A creates a lobby, account B sees it in the list and joins, both read the detail, B readies, A sends the start message, both receive it.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Player-to-player connection messages, TURN credentials and the connectivity check (L5). Until L5, `turn_username` and `turn_token` are empty strings and message 26 is ignored.
- Server lobbies (L6). Until L6 every lobby has `LobbyType` 0 and an empty `Region`.

## Handoff notes

- Finished:
  - New package `Servers/Lobby/internal/lobbies`: the lobby model (`lobby.go`), POST field changes (`changes.go`), request shapes (`requests.go`), the list and detail JSON shapes (`views.go`) and `CleanText` (`text.go`, moved from `realtime`).
  - `internal/realtime/lobbies.go`: Hub calls `CreateLobby`, `ListLobbies`, `LobbyDetail`, `JoinLobby`, `ChangeLobby`, `LeaveLobby`, the 6 and 7 notices (through `notifyLocked`, the locked twin of `Notify`), leave and kick. `lobby_handlers.go`: messages 5, 10, 13, 23, 24, 25 and 26. `handlers.go` routes them and a name change now also sends 6 and 7. `session.go`: `releasePlace` calls `leaveLobby`, so logout, a new session and an expired hold follow SERVICE 7.4. `hub.go`/`messages.go`: held messages (13 survives a disconnect like the notices), lobby maps and counters, `Settings.CountdownLock` (25 s), message ids.
  - `internal/api/lobbies.go` and a route in `handler.go`: `GET`/`PUT Lobbies`, `GET`/`PUT`/`POST`/`DELETE Lobby/{id}`, `POST Lobby/Outcome`.
  - `tools/lobbyprobe`: new `lobby.go` step (create, list, join, detail, ready, start); the earlier WebSocket step now keeps its two sockets open for it.
  - Tests: `lobbies/lobby_test.go`, `realtime/lobbies_test.go`, `api/lobbies_test.go` (key sets of list and detail checked exactly against CLIENT_HTTP 5.1 and 5.2). One existing case in `api/handler_test.go` changed: it used `GET Lobbies` as an example of an unserved path; it now uses `GET Lobby` with no id.
- Left to do:
  - Review by someone other than the author.
  - `-race` was not run: this machine has no cgo toolchain ("-race requires cgo"). All lobby state is behind `Hub.mu`; `TestConcurrentLobbyCallsKeepTheStateConsistent` exercises it from 7 goroutines.
  - L5 adds message 18 to the remaining members on a leave (SERVICE 7.4) and the 17/12/19 setup and 27/28 check; the hook for 18 is `Hub.leaveLobby`. L5 also fills `turn_username` and `turn_token`. L6 handles field 19 (the HTTP layer answers `{"result":0}` for it now), `LobbyType` 2 and `Region`.
- Surprises and open questions:
  - Lobby calls need the caller to have a WebSocket place on the hub (connected or held), because the place decides when a member is removed. Without one: create answers `result` 0, join answers 404. SERVICE does not say this; the real client always has its WebSocket first.
  - Message 26: the package says it is ignored until L5, but SERVICE 6.3 and the "Done when" say a new 26 from the host ends the countdown lock. 26 from the host now ends the lock and does nothing else; no 27/28 are sent.
  - SERVICE 7.4 says remaining members get 18; that is a P2P message (L5), so it is not sent here.
  - `max_players` (field 0): SERVICE says refuse a value below the number of occupied slots. The code refuses when any slot at or above the new value holds a human or an AI, which includes that case and also stops an occupant being cut off.
  - The documents give no ranges for side, colour, team, start position, map name, map path or password. Chosen: side -2..63 (-1..63 for AI), colour -1..7, team -1..7, start position -1..`max_players`-1 and not taken by another slot, colour not taken by another slot, map name 128 characters, map path 260, password 128 (refused, not cut). Constants in `changes.go` and `lobby.go`.
  - Message 25 with an empty or missing `new_password` is ignored (removal is message 24).
  - A kicked client gets a 6 so it finds itself missing (CLIENT_HTTP 5.2); the member who joined earliest becomes owner only while the lobby has not started.
  - On a successful create the host gets no 6 (nothing changed for other members); watchers get 7. On match start members get 6 then 13 (so the client can read `MatchID`), watchers get 7.
  - `RNGSeed` is chosen at creation only; SERVICE 7.1 says "again each time a match ends", but the service has no match-end event.
  - A lobby set to friends-only (field 18 value 1) is hidden from `GET Lobbies`, as "public" requires.
  - `NumCurrentPlayers` counts slots with a human or an AI. AI slots show `UserID` -1, empty `DisplayName` and `HasMap` false.
  - A join by an account already in the target lobby answers 200 without change; a join of another lobby leaves the old one only after the new join is accepted.
  - Verified by running: `go test ./...` and `lobbyprobe` against a started `lobby.exe` (all steps passed, including the new lobby step). Not verified: the real game client.
- Review findings and fixes:

