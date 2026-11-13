# A6 Relay: removing players

State: done
Branch: servers-A6

## Goal

Remove a player from a running match in a way every remaining client applies on the same frame, and make sure one stuck player cannot hold a match forever.

## Depends on

A5

## Read first

- PROTOCOL.md section 4.7 in full, and sections 4.5, 4.9 and 6.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [x] Every cause in the PROTOCOL.md 4.7 table drops the player with the listed reason.
- [x] Loading rule: no `LOADCOMPLETE` within the configured time after `START`.
- [x] Stall rule: implemented exactly as the table states, and never fires before the first `FRAMEINFO` of the session has been forwarded.
- [x] On a drop the relay stops forwarding that player's records, closes the connection, then sends `DROP` with `last_frame = next_frame - 1`, then `ROSTER` with the recalculated `leader_slot`.
- [x] Clean leave: after a forwarded `PLAYERLEAVE` and every `FRAMEINFO` through its leave frame, `LEAVE` or a disconnect produces `ROSTER` only.
- [x] A disconnect before that point produces a normal `DROP`.
- [x] The ENet timeout that decides "connection lost", the loading time and the stall time are configuration values.
- [ ] Reviewed by someone other than the author.

## Out of scope

Dropping a player for being slower than the server clock (S1, S3).

## Handoff notes

- Finished:
  - `Servers/Relay/src/drop.cpp` (new): `NoteAccepted` (records LOADCOMPLETE, the first FRAMEINFO of the session, each slot's last `next_frame` increase and the forwarded PLAYERLEAVE frame), `LeaveCompleted`, `DropIfMatchActive`, and the Tick checks (`CollectTimeouts`, `IsStalled`).
  - `sessions.cpp/.h`: `OnLeave` and `OnClosed(connection, connection_lost)` now call `DropConnection` with `LEFT` (or `TIMEOUT` when ENet reported the connection lost) in a running match unless the leave is complete; otherwise ROSTER only as before. `Tick` also drops a player that missed the loading time (`TIMEOUT`) or stalled (`TIMEOUT`). `START` records its time. `DropSettings` holds the loading time, stall time and ENet timeout.
  - `match_data.cpp`: one call to `NoteAccepted` after a record is accepted. `DropConnection` is unchanged, so `last_frame = next_frame - 1` and the order DROP, ROSTER, close are as A5 left them.
  - `host.cpp/.h`, `main.cpp`: ENet peer timeout set from the configuration (`enet_peer_timeout`), a disconnect event is passed to the session manager as "lost" or "closed".
  - `config.cpp/.h`, `protocol.h`: optional `relay.conf` keys `loading_time_s` (default 120), `stall_time_s` (default 20) and `connection_timeout_s` (default 30), each 1 to 65535.
  - `tests/relay_tests.cpp`: 13 new tests (loading, stall, leave, disconnect reasons, clean leave, config keys). One A4 test (`TestLeaderAndLeaving`) changed: a LEAVE in a running match without a leave frame is now DROP then ROSTER, where it used to be ROSTER only.
- Left to do:
  - Review by someone other than the author.
  - Live run: the relay was not run against a real client. The ENet timeout and the lost-versus-closed decision in `host.cpp` are covered by no unit test, because they need a real ENet peer.
- Surprises and open questions:
  - PROTOCOL.md 4.7 lists "ENet reports the connection lost" (`TIMEOUT`) and "connection lost before the leave frame" (`LEFT`) as separate rows, which overlap. I read it as: an explicit `LEAVE` or a normal disconnect is `LEFT`; a connection ENet timed out is `TIMEOUT`.
  - ENet 1.3.18 gives the same disconnect event, with data 0, for a timeout and for a normal disconnect. The host tells them apart by silence: it keeps the peer's last receive time from each service pass and calls the connection lost when the peer was silent for at least half the configured timeout. A normal disconnect follows a packet by well under a second. This is my choice, not in PROTOCOL.md. A cleaner signal would be a disconnect code from the client; that needs a PROTOCOL.md change.
  - The ENet timeout default is 30 seconds (ENet's own default maximum). PROTOCOL.md gives no value. The configured value sets both ENet's minimum and maximum timeout.
  - PROTOCOL.md 7.2 says every section 6 limit can be set in `relay.conf` "by name" but gives no names. The three key names above are mine. `Servers/Relay/relay.conf` was not changed (the keys are optional). PROTOCOL.md 7.2 needs the names added on `servers`.
  - A player whose leave is complete but whose `LEAVE` arrives more than the stall time after its last FRAMEINFO can be dropped for a stall (`TIMEOUT`) first. The table has no exception for it; clients ignore a DROP for a slot they already removed.
  - A second `PLAYERLEAVE` from a slot moves its leave frame up, never down, so a clean leave needs the later frame.
  - Before `START` a leave or disconnect still gives ROSTER only (A4 behaviour); the 30-second rule handles the empty slot.
- Verification:
  - Built with `cmake -S Servers/Relay -B C:\cnc-build\A6 -G Ninja -DCMAKE_BUILD_TYPE=Release -DRELAY_BUILD_TESTS=ON` in the VS x86 environment, `/W4 /WX`, exit 0. `ctest`: 1 of 1 passed (`relay_tests`, prints "all relay tests passed").
  - `clang-format -i` on the changed files, then `clang-tidy` 22.1.3 with the compile database on `drop.cpp`, `sessions.cpp`, `host.cpp`, `main.cpp`, `config.cpp`, `match_data.cpp`, `relay_tests.cpp`: 0 warnings.
  - Not run: the relay against a client, the game builds (this package changes only `Servers/Relay/`).
- Review findings and fixes:
