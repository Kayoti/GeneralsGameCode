# L3 Lobby service: WebSocket, rooms, room chat and names

State: todo
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

- [ ] `/ws` upgrades only with a valid session token. Otherwise `401`.
- [ ] The service reads JSON from binary and text frames and writes JSON only in text frames.
- [ ] Message 14 is answered with 15. A connection silent for 30 seconds is dropped. The service does not send a close frame to a client it keeps.
- [ ] Reconnect follows the table in SERVICE.md 6.2, including HTTP `205` and the 45-second hold.
- [ ] One live connection per account: a second one replaces the first.
- [ ] `GET Rooms` lists the configured rooms. Message 3 moves the client into or out of a room and is answered with message 4, which is also sent to everyone already in the room, with every key the client reads.
- [ ] Message 1 becomes message 2 for everyone in the sender's room, limited to 300 characters, with all four keys.
- [ ] Message 9 changes the account's name by the rules in SERVICE.md section 4, stores it, and refreshes the room's member list. A refused name is answered with one room chat line to the sender only.
- [ ] `GET MOTD` and `GET ServiceConfig` return the values of SERVICE.md 5.1 and 9, with every field of the client's structure.
- [ ] Identical pending notices to one client are sent once (SERVICE.md 6.1). This is built here as a general "notify" helper, because L4 uses it.
- [ ] More than 60 messages in a second, apart from 12 and 14, disconnects the client. Messages over 256 KB are refused.
- [ ] Unknown message numbers are ignored and logged at debug level.
- [ ] Tests cover each point with two or more in-process clients. No test sleeps longer than 2 seconds; timeouts are configurable for tests.
- [ ] `lobbyprobe` now also connects the WebSocket for two accounts, joins room 1, exchanges a chat line and changes a name.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Lobbies and every lobby message (L4, L5, L6).
- Friends, matchmaking and moderation messages.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
