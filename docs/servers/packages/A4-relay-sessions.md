# A4 Relay: sessions

State: done
Branch: servers-A4

## Goal

Implement match sessions: the host creates one and receives a join token per player, other players join with their token, everyone is told who is connected, and the relay announces the start.

## Depends on

A1

## Read first

- PROTOCOL.md sections 4.2, 4.3, 4.5, 4.9 and 6.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [x] `CREATE` is accepted only with a valid pass, `host_slot` inside `slot_mask`, and at least one other bit set. Otherwise `REFUSE` `BAD_REQUEST` or the pass code that applies.
- [x] An accepted `CREATE` makes a session with a random 64-bit id and one random 64-bit join token per other expected slot, binds the connection to `host_slot`, and replies `CREATED` then `WELCOME`.
- [x] `JOIN` is accepted only when every condition in PROTOCOL.md 4.5 step 5 holds. Each failure maps to its own `REFUSE` code, including `ALREADY_STARTED` and `BAD_TOKEN`. A token is used up by a successful join.
- [x] A `pass_id` already in use by a live match connection gets `REFUSE` `PASS_IN_USE`.
- [x] `ROSTER` is sent to all connected players on every change.
- [x] `START` is sent once, when all expected slots are connected, or 30 seconds after `CREATED`. In the second case a `DROP` with `NEVER_JOINED` and `last_frame` 29 is sent for each missing slot first. With fewer than 2 connected players everyone gets `REFUSE` `NOT_ENOUGH_PLAYERS`.
- [x] `leader_slot` is always the lowest connected slot.
- [x] A session with no connected player is removed and its memory released.
- [x] The limits in PROTOCOL.md 6 for `CREATE`, `JOIN`, failed joins per session, concurrent sessions and connections without a first message are enforced.
- [x] Random values come from the operating system's secure source.
- [x] The test client from A2 gains modes to create and join a session.
- [x] Reviewed by someone other than the author.

## Out of scope

Forwarding records (A5). Removing players during a match (A6).

## Handoff notes

- Finished:
  - `Servers/Relay/src/sessions.h`, `sessions.cpp`: `SessionManager`. It has no ENet dependency and writes through a `SessionOutput` interface, so the unit tests drive it directly. It implements CREATE (pass, `host_slot` in `slot_mask`, at least one other bit, `SERVER_BUSY` above `max_sessions`), `CREATED` then `WELCOME`, JOIN (the 4.5 step 5 checks, each with its own `REFUSE` code), `ROSTER` on every change, `START` when all expected slots are connected, the 30 second timer (`DROP` `NEVER_JOINED` with `last_frame` 29 per missing slot, then `START`, or `REFUSE` `NOT_ENOUGH_PLAYERS` when fewer than 2 are connected), `leader_slot` as the lowest connected slot, and removal of a session when its last player goes. The `pass_id` registry (`PASS_IN_USE`) is a set of ids held by bound connections. A connection is bound to its slot when the pass is accepted, and the manager keeps the connection id to slot map.
  - `Servers/Relay/src/rate_limit.h`, `rate_limit.cpp`: per-address sliding window used for `CREATE` (6/min) and `JOIN` (12/min).
  - `Servers/Relay/src/wire.h`: little-endian helpers. `pass.cpp` now uses them instead of its own copies.
  - `Servers/Relay/src/host.*`, `main.cpp`: the host passes CREATE and JOIN to the manager, sends and closes on the manager's behalf (`enet_peer_disconnect_later` after a `REFUSE`), forwards `LEAVE` from a bound connection, calls `Tick` every service loop, and enforces the first-message limits (4 per address, 256 in total; extra connections are dropped at connect). `RelayHost` now takes `SessionSettings`.
  - `Servers/Relay/src/protocol.h`: added message body sizes for the messages the relay sends, the session limits from section 6, and renamed `MAX_PEERS` to `MAX_UNCHECKED_CONNECTIONS`.
  - `Servers/Relay/tools/session_client.cpp` (CMake target `session_client`): `create`, `join` and `idle` modes. It signs its own pass from the region key file, so it works against any relay started with that key.
  - `Servers/Relay/tests/relay_tests.cpp`: tests for validation, every JOIN refusal, token use, pass id reuse, roster and start, the timeout both ways, leader, session release, session and address limits, failed joins per session, and random values.
- Left to do:
  - Review by someone other than the author.
  - Records after the first message are still dropped with a log line (A5). Only `LEAVE` is handled. Attribution of records to the bound slot is available through `SessionManager` bindings, but no code reads it yet.
  - After `START`, a `LEAVE` or lost connection sends `ROSTER` only. The `DROP` and clean-leave rules of PROTOCOL.md 4.7 belong to A6.
  - The entry check (`CHECK_HELLO`) is A2. The host still only logs it. The two packages both edit `host.cpp` and `main.cpp`, so the second to merge may need a rebase.
- Surprises and open questions:
  - Package item "The test client from A2 gains modes to create and join": A2 has no commits and no client on `servers`, so there was nothing to extend. `tools/session_client.cpp` is a separate tool. If A2's client lands, the two can be merged or kept side by side.
  - The pass in the test client is signed locally with the key file, because the entry check does not exist yet. It also needs the relay started with `--no-address-check` when the client's source address is not the one written in the pass (`--ip`).
  - PROTOCOL.md does not say which `REFUSE` code applies once a session has refused 32 failed joins. `RATE_LIMITED` is used. A `JOIN` that arrives for a started session still gets `ALREADY_STARTED` first.
  - PROTOCOL.md does not say whether the joining player also gets a `ROSTER`. Everyone connected, including the joiner, gets `ROSTER` after `WELCOME`, which is the literal reading of "sent to all connected players on every change".
  - At the 30 second timeout every expected slot that is not connected gets `DROP` `NEVER_JOINED`, including a slot whose player joined and left before `START`. PROTOCOL.md names `NEVER_JOINED` for "expected slot never connected" only.
  - A token that was used is zeroed, so a second `JOIN` for a slot whose first player has left gets `BAD_TOKEN`. A slot that is currently connected gets `SLOT_TAKEN`. A wrong build number does not use the token up.
  - `MAX_PEERS` (256) was also the whole ENet peer table, which would cap all connections including in-match ones at 256. The peer table is now 256 plus 8 per allowed session, and `Start` fails with a message if `max_sessions` would need more than ENet's 4095 peers (more than 479 sessions).
  - Failed-join counting covers refusals after the session is found and the pass accepted (`BAD_REQUEST` slot, `NOT_EXPECTED`, `SLOT_TAKEN`, `BAD_TOKEN`, `BUILD_MISMATCH`). Pass failures and `NO_SESSION` do not count, since they are not tied to the session.
  - PROTOCOL.md section 10 assumptions V1 to V8 are not touched by this package.
- Verification:
  - Relay built with `cmake -S Servers/Relay -B <build dir> -G Ninja -DCMAKE_BUILD_TYPE=Release -DRELAY_BUILD_TESTS=ON` in the VS x86 environment, warnings as errors on relay code, build exit 0. `ctest` passed (`relay_tests`).
  - Live run on one PC (relay with `--no-address-check`, port 17777): create plus two joins gave `WELCOME`, `ROSTER`, `ROSTER`, `START` in order on all three; 4 expected with 2 connected gave `ROSTER`, `DROP` slot 1, `DROP` slot 3 (`NEVER_JOINED`, `last_frame` 29), `START` after 30 s; host alone gave `REFUSE` 17 after 30 s; a second `CREATE` with the same pass id gave `REFUSE` 9; the `idle` mode with 6 connections kept 4 and the relay logged the refusals.
  - `clang-format --dry-run --Werror` and `clang-tidy` 22.1.3 with the root `.clang-tidy` on `sessions.cpp`, `rate_limit.cpp`, `host.cpp`, `pass.cpp`, `main.cpp`, `tools/session_client.cpp`, `tests/relay_tests.cpp`: 0 warnings.
  - Not run: more than 1 address in a live test (all clients were 127.0.0.1), the 256 total limit, and the address check switched on.
- Review findings and fixes: reviewed by a second developer after the branch was rebased onto A2. Verdict: approved, no changes required. All findings are low and spec-compliant.
  - At the 30-second timeout, a slot that joined and then left before START gets `DROP NEVER_JOINED`, like a slot that never connected. No `FRAMEINFO` was forwarded before START, so `last_frame` 29 is correct either way; PROTOCOL is silent on this case. No change.
  - The per-address CREATE/JOIN rate limit is consulted before the length/proto/pass checks, so a malformed CREATE/JOIN still spends the budget. This is a reasonable anti-abuse reading of the per-address limit; no change.
  - After 32 failed joins a session answers `RATE_LIMITED` to further joins, as PROTOCOL 6 requires the session to refuse further joins; the code is a fair choice and is throttled by the per-address JOIN limit.
  - Rate-limiter reconciliation checked: A2's and A4's two limiter classes were unified into one `AddressRateLimiter(max_events, window_ms)`. The entry check still uses 6 per 60 s with the identical window boundary, A2's entry-check test is preserved and passes, and both A2 and A4 test suites run and pass in the one `relay_tests` binary. No A2 regression.
  - Confirmed correct: CREATE/JOIN validation and single-use tokens; `PASS_IN_USE`; ROSTER on change; START and the 30-second timeout with `NEVER_JOINED`/`NOT_ENOUGH_PLAYERS`; `leader_slot`; session cleanup frees pass ids; all slot/mask/index values bounded before use; the connection is bound to its slot at the pass check and later messages are attributed by connection, not by the claimed slot; session ids and tokens come from libsodium `randombytes_buf`; scope is `Servers/Relay/` only; the peer table is `256 + 8 per session` capped at ENet's limit.
