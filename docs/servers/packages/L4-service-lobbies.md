# L4 Lobby service: lobbies

State: todo
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

- [ ] `PUT Lobbies`, `GET Lobbies`, `GET Lobby/{id}`, `PUT Lobby/{id}`, `POST Lobby/{id}`, `DELETE Lobby/{id}` and `POST Lobby/Outcome` behave as SERVICE.md sections 5.1 and 7 say.
- [ ] Every lobby and member object carries every key in CLIENT_HTTP.md 5.1 and 5.2, with the right JSON type. A lobby always has exactly 8 member entries. A test checks the key set of the list answer and of the detail answer against the lists in that reference.
- [ ] Join failures use the status codes in SERVICE.md 7.2.
- [ ] Every `field` value in SERVICE.md 7.3 except 19 is handled, with the "who" rule, range checks and the ready-state rules. The host is always ready. Unknown fields and field 7 have no effect.
- [ ] The countdown lock (message 23) follows SERVICE.md 6.3: host only, blocks joins and member changes, ends after 25 seconds, at match start, or on a new 26.
- [ ] A held place keeps its pending 6, 7 and 13 and receives them on reconnect (SERVICE.md 6.2).
- [ ] Messages 5, 10/11, 24, 25 and 23 behave as SERVICE.md 6.3 says.
- [ ] After every lobby change members get message 6, and after every change to the open list, watchers get message 7, through the notify helper from L3.
- [ ] Leaving, kicking, logout and an expired hold all follow SERVICE.md 7.4, including the change of `Owner` and deletion of empty lobbies.
- [ ] Message 13 from the host follows SERVICE.md 7.5 steps 1 to 3. Every human member, including the host, receives it.
- [ ] A player can be in one lobby at a time.
- [ ] Lobby state is in memory only, guarded for concurrent use. No database access on the message path.
- [ ] Tests cover create, list, join (each failure), each field change, ready, chat, password change, kick, host leaving, last member leaving, start with and without everyone ready.
- [ ] `lobbyprobe` now also runs: account A creates a lobby, account B sees it in the list and joins, both read the detail, B readies, A sends the start message, both receive it.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Player-to-player connection messages, TURN credentials and the connectivity check (L5). Until L5, `turn_username` and `turn_token` are empty strings and message 26 is ignored.
- Server lobbies (L6). Until L6 every lobby has `LobbyType` 0 and an empty `Region`.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
