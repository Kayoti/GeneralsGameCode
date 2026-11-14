# L5 Lobby service: player-to-player connection support

State: todo
Branch: servers-go-L5

## Goal

Give Online lobbies what the client needs to connect players to each other: TURN credentials, connection setup messages, and the connectivity check before start.

## Depends on

L4

## Read first

- SERVICE.md section 8 and section 6.3
- reference/CLIENT_REALTIME.md section 2 (player-to-player connection setup, all of it) and section 1.7 rows 12, 17, 18, 19, 26, 27, 28

## May edit

- `Servers/Lobby/`

## Must not edit

- Anything outside `Servers/Lobby/`

## Done when

- [ ] Create and join answers for an Online lobby carry TURN credentials as SERVICE.md 8.1 says. With an empty `turn_secret` both are empty strings. A test checks one credential against a value computed independently in the test.
- [ ] `turn_secret` can come from the configuration file or from `LOBBY_TURN_SECRET`, and is never logged.
- [ ] After a join, message 6 goes out first and then message 17 to both players of every new pair, with all four keys (SERVICE.md 8.2).
- [ ] Message 12 is forwarded only between two members of the same lobby, with `target_user_id` set to the sender and `payload` untouched. Anything else is dropped.
- [ ] Message 19 sends 17 again to both players of that pair, if both are in the same lobby.
- [ ] When a member leaves, the others get message 18 for that player.
- [ ] Message 26 from the host runs the check in SERVICE.md 8.3 and ends with message 28 to the host, with `mesh_complete` always present.
- [ ] A new 26 cancels a check in progress. A member leaving during a check ends it with `member_left`.
- [ ] Forwarding message 12 does not parse or copy the payload more than once.
- [ ] Tests cover two and three players, a missing pair, a late reply, a member leaving mid-check, and forwarding to a non-member.
- [ ] `lobbyprobe` now also answers message 27 as a connected client and waits for 28 before the host starts.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Running `coturn` (Phase 4, and the Phase 1 check uses direct connections on one PC).
- Server lobbies (L6).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
