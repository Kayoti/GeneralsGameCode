# A5 Relay: record validation and forwarding

State: done
Branch: servers-A5

## Goal

Carry the match: accept `DATA` messages from each player, check every record against the rules for its command type, and forward it to the right players.

## Depends on

A4

## Read first

- PROTOCOL.md section 4.6 in full, and sections 6 and 10.
- STANDARDS.md "Security" and "Performance".

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [x] `DATA` is accepted only after `START`. A record before `START` hands the player to the drop path.
- [x] Each record passes checks 1 to 5 of PROTOCOL.md 4.6 or is dropped and counted. The header must match the tag list for its type exactly. The scan never reads past the record.
- [x] The per-type table is implemented as written: the extra checks, and who each type is forwarded to. For `GAMECOMMAND`, `FRAMEINFO` and `PLAYERLEAVE` the record's own relay mask is ignored.
- [x] `next_frame` per slot starts at 30 and advances only on a valid `FRAMEINFO`. The frame window is enforced.
- [x] `RUNAHEAD` is accepted only from the current `leader_slot` and only with values inside the stated ranges.
- [x] `TIMEOUTSTART` is accepted only from the host slot.
- [x] A valid record's bytes are forwarded unchanged.
- [x] Command count: the relay counts per slot and frame as PROTOCOL.md describes, and logs a mismatch without acting on it. A configuration switch turns a mismatch into a violation, for use once V7 is settled.
- [x] The data rate limits per player are enforced.
- [x] The forwarding path allocates no memory per record.
- [x] 32 failed records from one player hands that player to the drop path with reason `PROTOCOL`.
- [x] The relay keeps counters per session: records in, forwarded, and dropped by reason. A debug setting logs each violation with type, frame and `next_frame`, so V7 and V8 can be checked against real traffic.
- [x] Reviewed by someone other than the author, with a dedicated pass on the header scan and on every bound.

## Out of scope

Rewriting frames or closing frames (S1).

## Handoff notes

- Finished:
  - `Servers/Relay/src/records.h`, `records.cpp`: `ScanRecord` (checks 1 to 3 of PROTOCOL.md 4.6: length 7 to 4096, tag list per type each tag once and ending with `D`, known type; every read is bounded by the record length) and `MatchRecords` (check 4 and the per-type table: frame window, `next_frame` starting at 30 and advancing only on a valid `FRAMEINFO`, leader, host, `leaving_slot`, run-ahead ranges, forward masks, command count per slot and frame, data rate windows, failed record count, per-session counters).
  - `Servers/Relay/src/match_data.cpp`: `SessionManager::OnData` and helpers. Splits a `DATA` message into records, forwards valid records unchanged into one preallocated buffer per destination slot (allocated once in the constructor, one `enet_packet_create` per send is the only allocation), flushes before a drop, and drops a player with `DROP`, then `ROSTER`, then close (`last_frame = next_frame - 1`). Logs per-session counters when the session ends.
  - `sessions.h/.cpp`, `host.h/.cpp`, `main.cpp`, `protocol.h`, `CMakeLists.txt`: wiring. `protocol.h` gains the record limits and the command type values. `RelayHost` takes a `RecordSettings`.
  - Relay options (command line, like `--no-address-check`): `--strict-command-count` turns a `command_count` mismatch into a refused record (the switch for after V7), `--log-violations` logs every refused record with type, frame and `next_frame`.
  - `tests/relay_tests.cpp`: tests for the scanner (each type, length limits, every header prefix, wrong order, repeated, missing and `Z` tags, unknown types, reads never past the record), forwarding for each type and each relay mask, unchanged bytes, frame window and `next_frame`, run-ahead rules and leader change, command count (logged and strict), 32 failures, drop before `START`, framing errors, sender slot from the connection, and both data rate limits.
- Left to do:
  - Review by someone other than the author, with a dedicated pass on the header scan and every bound.
  - A6 builds the other causes in the PROTOCOL.md 4.7 table on top of `SessionManager::DropConnection`. A5 needed a drop to exist, so it contains the "what the relay does" steps (stop forwarding, `DROP`, `ROSTER`, close). A6 still owns timeouts, stall, loading, clean leave and `LEFT`.
  - A live run with a real client is not done: the test tools do not send `DATA`. Checked by unit tests only.
- Surprises and open questions:
  - PROTOCOL.md 4.6 does not give the numeric `T` values. They are the engine's `NetCommandType` values from `NetworkDefs.h` (FRAMEINFO 3, GAMECOMMAND 4, PLAYERLEAVE 5, RUNAHEADMETRICS 6, RUNAHEAD 7, CHAT 11, PROGRESS 14, LOADCOMPLETE 15, TIMEOUTSTART 16). I checked the tag order of each type against the packet structs in `NetPacketStructs.h`; they match the table. `copyBytesForNetPacket` is only declared in this tree (C2 adds the writers), so the values are not yet confirmed by a round trip (V2). Worth adding the numbers to PROTOCOL.md on `servers`.
  - The "configuration switch" and "debug setting" are command-line options, not keys in `relay.conf`, because `config.cpp` accepts exactly the section 7.2 keys and PROTOCOL.md cannot be edited by this package. If they should be config keys, PROTOCOL.md 7.2 needs them first.
  - Decisions where PROTOCOL.md is silent: a `DATA` message with no record, with a length prefix that does not fit, or over 16384 bytes counts as one failed record and the rest of the message is ignored (records before it were handled). A record outside 7 to 4096 bytes is skipped and counted, and the message continues. `RUNAHEADMETRICS` sent by the leader itself has no recipient (never echoed to the sender). `forwarded` counts valid records, whatever the number of recipients. Data from a connection that holds no slot is ignored, as before.
  - Rate limits use a fixed one-second window per slot, counting every byte of the `DATA` message (type byte and length prefixes included) and every record parsed, valid or not. A player can send up to twice the limit across a window boundary.
  - Command count mismatches are logged for the first 20 per session, then only counted. With the strict option the `FRAMEINFO` is refused and `next_frame` does not move.
  - Record header size: a `GAMECOMMAND` header is 15 bytes, so its data is at most 4081 bytes inside the 4096 limit.
  - V7 and V8 (PROTOCOL.md section 10) are not settled by this package. They need real traffic from C2 using `--log-violations` and the mismatch log.
- Verification:
  - Built with `cmake -S Servers/Relay -B C:\cnc-build\A5 -G Ninja -DCMAKE_BUILD_TYPE=Release -DRELAY_BUILD_TESTS=ON` in the VS x86 environment, `/W4 /WX`, exit 0. `ctest`: 1 of 1 passed (`relay_tests`).
  - `clang-format --dry-run --Werror` over `src`, `tests`, `tools`: clean. `clang-tidy` 22.1.3 (VS build tools) on `records.cpp`, `match_data.cpp`, `sessions.cpp`, `host.cpp`, `main.cpp`, `relay_tests.cpp`: 0 warnings after fixing three.
  - Not run: the relay against a client, the game builds (this package changes only `Servers/Relay/`).
- Review findings and fixes: reviewed by a second developer with a dedicated pass on the header scan and every bound. Verdict: approved, no changes required. All findings are low or informational.
  - In strict command-count mode a FRAMEINFO count mismatch is tallied in both the mismatch counter and the dropped-by-reason counter, so the end-of-session log counts the one event twice. Accounting only, no behavioural effect; left as is.
  - The strict command-count switch and the violation log are command-line flags rather than relay.conf keys, because the config parser accepts only the PROTOCOL 7.2 keys and this package cannot change PROTOCOL. Reasonable; satisfies the intent.
  - Rate windows are fixed one-second windows, so a burst can reach about twice the limit across a boundary. PROTOCOL is silent; acceptable.
  - Confirmed correct: the header scan exact-matches the tag set and order per type and never reads past the record on any crafted header; checks 1 to 5 hold; the sender slot comes from the connection binding, never from the record; the forwarding table matches 4.6 and the first three types ignore the record's own mask; frame tracking starts at 30 with a 129-wide window and advances only on a valid FRAMEINFO; 32 failed records drop the player with reason PROTOCOL and last_frame = next_frame - 1; forwarding does no per-record allocation and forwards bytes unchanged; the command-type numbers are hardcoded constants that match the engine's `NetworkDefs.h`, with no engine header pulled into the relay; the minimal drop path takes a reason and does not clash with A6. Relay builds `/W4 /WX` clean and its tests pass.
