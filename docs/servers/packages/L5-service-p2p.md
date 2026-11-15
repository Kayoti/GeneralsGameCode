# L5 Lobby service: player-to-player connection support

State: done
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

- [x] Create and join answers for an Online lobby carry TURN credentials as SERVICE.md 8.1 says. With an empty `turn_secret` both are empty strings. A test checks one credential against a value computed independently in the test.
- [x] `turn_secret` can come from the configuration file or from `LOBBY_TURN_SECRET`, and is never logged.
- [x] After a join, message 6 goes out first and then message 17 to both players of every new pair, with all four keys (SERVICE.md 8.2).
- [x] Message 12 is forwarded only between two members of the same lobby, with `target_user_id` set to the sender and `payload` untouched. Anything else is dropped.
- [x] Message 19 sends 17 again to both players of that pair, if both are in the same lobby.
- [x] When a member leaves, the others get message 18 for that player.
- [x] Message 26 from the host runs the check in SERVICE.md 8.3 and ends with message 28 to the host, with `mesh_complete` always present.
- [x] A new 26 cancels a check in progress. A member leaving during a check ends it with `member_left`.
- [x] Forwarding message 12 does not parse or copy the payload more than once.
- [x] Tests cover two and three players, a missing pair, a late reply, a member leaving mid-check, and forwarding to a non-member.
- [x] `lobbyprobe` now also answers message 27 as a connected client and waits for 28 before the host starts.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [x] Reviewed by someone other than the author.

## Out of scope

- Running `coturn` (Phase 4, and the Phase 1 check uses direct connections on one PC).
- Server lobbies (L6).

## Handoff notes

- Finished:
  - `internal/api/turn.go` (`turnCredentials`), `Options.TurnSecret` in `api/handler.go`, used by the create and join answers in `api/lobbies.go`; `cmd/lobby/main.go` passes `cfg.TurnSecret`. `config` already read `turn_secret` from the file and from `LOBBY_TURN_SECRET` (L1) and nothing logs it. Tests in `api/turn_test.go`: a fixed value computed with Python's hmac, a recomputation from the answer's own username, and empty strings without a secret.
  - `realtime/p2p.go`: 17 to both players of every new pair after 6 (`JoinLobby`), 12 forwarding, 19, and 18 to the remaining humans on every leave (`leaveLobby`, so kick, logout and an expired hold are covered).
  - `realtime/meshcheck.go`: 26 / 27 / 28 as in SERVICE 8.3, one check per lobby in `Hub.meshChecks`, timers guarded by a step counter; `Settings.MeshReplyWait` and `MeshRetryDelay` (5 s and 2 s by default) so tests can shorten them.
  - One read of each message: `readLoop` parses the envelope once (`msg_id`, `target_user_id`, raw `payload`) and passes it to the rate check and to `handleMessage`; message 12 is then written once into the target's outbox from the received bytes. `handleMessage` and `withinRate` signatures changed for this.
  - `tools/lobbyprobe/lobby.go`: sends 26, answers 27 for both accounts, waits for 28 with `mesh_complete` true before the host starts.
  - Tests: `realtime/p2p_test.go` (two and three players, missing pair, late reply, member leaving mid-check, host leaving, new 26, non-member forwarding and the other drops, 19, 18). Four existing expectations in `realtime/lobbies_test.go` gained 17 or 18, as the package requires.
- Left to do:
  - Review by someone other than the author.
  - L6: nothing sends 17 or 18 for a server lobby yet because no server lobby exists; when `LobbyType` 2 is added, `JoinLobby`, `leaveLobby` and `handleConnectivityCheck` must skip 17, 18 and the 8.3 check for it (SERVICE 6.3, section 10), and `turnCredentials` should not be given to it if the lobby has none.
  - `-race` not run: no cgo toolchain on this machine. All new state is under `Hub.mu`.
- Surprises and open questions:
  - `TestLobbyChatReachesMembersWithEveryKeyAndAnnouncementRules` and `TestStartIsIgnoredUntilEveryHumanIsReady` (L4 tests) fail now and then, also on the unchanged `servers-go` code (checked with `go test -count=15` after stashing this work): they settle one socket while a message sent on another may not be handled yet. Not changed here.
  - SERVICE 8.3 does not say which pairs are listed or what `member_left` lists. Chosen: every ordered pair (source lacks target in its `connectivity_map`; a member with no reply lists nobody); `member_left` sends an empty list. Reason is `timeout` when a member did not reply, else `missing_connections`.
  - The retry (step 4) runs when some replied member lists a peer in `connecting_map`; the next attempt asks the humans the lobby has then and uses a new `mesh_check_id`.
  - If the host or a member who was asked leaves, the check ends at once: `member_left` goes to the host, or nothing when the host left. The 28 goes out before the 6 and 18 of that leave.
  - A 26 is ignored for a lobby that has started; it still ends the countdown lock as in L4.
  - A signal (12) for a target whose outbox is full, or whose connection is down, is dropped; the existing limit would otherwise disconnect the target for a flood sent by another member. 12 is not counted against the 60 messages a second (L3 rule), so this keeps a member from flooding another out.
  - Verified by running: `go test ./...`, and `lobbyprobe` against a started `lobby.exe` with `LOBBY_TURN_SECRET` set (all steps passed, including the new connectivity step; the secret did not appear in the log). Not verified: the real game client.
- Review findings and fixes:
  - Approved with no defects. `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passes. The TURN
    credential scheme was checked against SERVICE 8.1 and recomputed independently: username is
    `<expiry+6h>:<user id>` and the token is Base64(HMAC-SHA1(username, turn_secret)); the credentials are
    empty when the secret is empty and the secret is never logged. After a join, 6 then 17 go to both
    players of every new pair with all four keys; message 12 is forwarded only between two different
    members of one lobby, with `target_user_id` set to the sender and the payload copied once; message 19
    re-sends 17; a leave sends 18; message 26 runs the 8.3 connectivity check ending in 28 with
    `mesh_complete` always present, a new 26 cancels one in progress, and a member leaving ends it. Every
    message id and JSON key matches CLIENT_REALTIME 1.7. All shared hub and mesh-check state is under one
    mutex with no socket write held under the lock, and the replaced-timer double-fire is guarded by map
    identity and a step counter. The 60 messages/second cap correctly exempts 12 and 14 per SERVICE 6.3,
    and a flood of 12s is dropped silently rather than amplified. Scope, author and commit hygiene correct.
  - Pre-existing flaky tests (from L4, NOT introduced here): `TestLobbyChatReachesMembersWithEveryKeyAnd
    AnnouncementRules` and `TestStartIsIgnoredUntilEveryHumanIsReady` in `internal/realtime` fail
    intermittently on a socket-settle race; they pass on a re-run and also fail on unchanged `servers-go`.
    Worth a later test-robustness pass; not an L5 defect.
