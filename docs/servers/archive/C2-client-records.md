# C2 Client: commands as records, player removal from the server

State: done
Branch: servers-C2

## Goal

In server mode, send every outgoing network command to the relay as a record and feed every incoming record into the existing command handling, so a match runs with no player-to-player connections. Apply a `DROP` from the relay.

## Depends on

C1

## Read first

- PROTOCOL.md sections 4.6 and 4.7 in full.
- CODEMAP "Match networking", all rows.
- In `ConnectionManager.cpp`: `parseUserList`, `update`, `doRelay`, `processNetCommand`, `sendLocalCommand`, `sendLocalCommandDirect`, `sendRemoteCommand`, `ackCommand`, `updateRunAhead`, `allCommandsReady`, `disconnectPlayer`, `processPlayerLeave`, `doKeepAlive`.
- In `Network.cpp`: `init`, `updateLogicFrame`, `processRunAheadCommand`, `processDestroyPlayerCommand`.
- `NetworkUtil.cpp`: `IsCommandSynchronized`, `CommandRequiresAck`, `CommandRequiresDirectSend`.
- `DisconnectManager.cpp`: `update`, `allCommandsReady`, `allowedToContinue`.
- Where the game waits for every player's load-complete before starting, in `GameLogic.cpp`.

## May edit

- `ConnectionManager.h/.cpp`, `Network.cpp`, `DisconnectManager.h/.cpp`
- `ServerLink.*` from C1, if its interface needs a small addition

## Must not edit

`Transport.*`, `Connection.*`, `NetPacket*`, `NetCommandMsg*`, `FrameData*`, `NAT.*`, `FirewallHelper.*`, `LANAPI*`, `GameSpy/`. The command encoding must not change.

## Done when

Every item applies only when the server mode flag is on. With it off, behaviour is identical to before.

- [x] `parseUserList` creates a `FrameDataManager` for each human slot as today, and creates no `Connection` objects and no `Transport`. `ConnectionManager::parseUserList` wraps the connection/transport creation in `if (!m_serverMode)`; the frame data is still created.
- [x] Outgoing: a carried command is written with `copyBytesForNetPacket` and sent as one record. Synchronized commands are also added to the local frame data, as today. Nothing is sent before `START`. Commands that are not carried are never sent. `sendServerModeCommand` + `queueServerRecord` (gated on `m_serverStarted`); `IsServerCarriedCommand` is the carried allowlist.
- [x] A command longer than 4096 bytes is not sent and not counted. `sendServerModeCommand` returns before the local frame-data add and the send when `getSizeForNetPacket() > 4096`.
- [x] Incoming: each record is turned into a command with `NetPacket::ConstructNetCommandMsgFromRawData` and handled by the code that handles a relayed command today, without acknowledgement and without relaying. A record that fails to parse is discarded. `processServerDataMessage` + `processServerRecord`.
- [x] Client checks from PROTOCOL.md 4.6 are applied to received records: `RUNAHEAD` only from the leader and inside range, `PLAYERLEAVE` only for the sender's own slot, `TIMEOUTSTART` only from the host slot. `DESTROYPLAYER` and every other not-carried type is discarded. In `processServerRecord`; host slot is 0 (the engine always seats the host there).
- [x] The acknowledgement path (`ackCommand`, pending and relayed command lists, resends, keep-alives, frame resend requests) is not entered. `updateServerMode` replaces `update`; it never calls `doRelay`/`doKeepAlive`, and the server send paths never add to the pending/relayed lists. The disconnect keep-alive is a not-carried command, so it is dropped before the wire.
- [x] The packet router slot follows `leader_slot` from `WELCOME` and `ROSTER`. Run-ahead is calculated by the leader as today, and others send their metrics to the leader through the relay. `m_packetRouterSlot = event.leaderSlot`; `updateRunAhead`'s non-leader branch now routes `RUNAHEADMETRICS` through `sendServerModeCommand`.
- [x] `DROP` is applied as PROTOCOL.md 4.7 lists: discard that slot's records above `last_frame`; keep the slot through `last_frame`; at `last_frame + 1` apply locally what the destroy-player command does; count the slot as loaded if the match is still loading; ignore a `DROP` for a slot already removed. `handleServerDrop`.
- [x] The disconnect screen never starts a vote and never removes a player in server mode. If frames stall it may show that the game is waiting. Removal comes only from `DROP` or from the player's own leave command. `ConnectionManager::voteForPlayerDisconnect` and `DisconnectManager::updateDisconnectStatus` both return early in server mode.
- [x] Loss of the relay connection ends the match for the local player with a message. `TYPE_LOST`/`TYPE_REFUSE` in `updateServerMode` show a message and call `TheGameLogic->clearGameData()`.
- [x] This settles PROTOCOL.md V2 and V3, and gives the traffic needed for V7 and V8. Anything that does not hold is recorded in the handoff notes with the command type and the difference. See handoff notes: V2/V3 hold by construction, confirmed by code inspection; full runtime confirmation is the Phase 2 gate.
- [x] Builds with the switch on and off. Lint reports no new warnings. Built both titles switch ON (`C:\cnc-build\C2-on`) and OFF (`C:\cnc-build\C2-off`); no new warnings from the changed files (the two C4018 in `updateRunAhead` are pre-existing, identical in the OFF build). Lint: `scripts/run-clang-tidy.py` on both changed files, no warnings.
- [x] Reviewed by someone other than the author, with a dedicated pass on anything that could change the simulation result.

## Out of scope

The server clock (S2). Starting a match from the staging room (2.1).

## Handoff notes

- Finished:
  - `ConnectionManager.h/.cpp`: the server-mode match-record path behind `#ifdef RTS_SERVERS`.
    - Members: `m_serverStarted`, `m_serverLink` (non-owning, set by `attachServerLink`), `m_serverSendBuffer[16383]`, `m_serverSendLength`.
    - `update` diverts to `updateServerMode` (polls the link, turns events into state/records, runs the disconnect screen, flushes, deletes quit-frame data). No transport, no `doRelay`, no `doKeepAlive`.
    - `sendLocalCommand`/`sendLocalCommandDirect` divert to `sendServerModeCommand` (local frame-data add for synchronized commands + `queueServerRecord` for carried commands). `queueServerRecord` writes `copyBytesForNetPacket` full form with a little-endian `u16` length prefix, flushes at the 16383-byte buffer limit, and never sends before `START`.
    - Receive: `processServerDataMessage` (length-checked record split) + `processServerRecord` (`ConstructNetCommandMsgFromRawData`, slot/carried/per-type checks, synchronized -> sender frame data, others -> the existing `process*` handlers).
    - `handleServerDrop` implements PROTOCOL 4.7.
    - `parseUserList` skips `Connection`/`Transport` in server mode; `isPlayerConnected`, `getPlayerName` and `areAllQueuesEmpty` have server-mode branches driven by frame data / `TheGameInfo` instead of connections; `updateRunAhead` routes `RUNAHEADMETRICS` through the record path; `voteForPlayerDisconnect` is a no-op in server mode.
  - `DisconnectManager.cpp`: `updateDisconnectStatus` returns early in server mode (no vote, no removal).
  - `NetworkInterface.h` NOT edited (not in May edit). C1 already added `setServerMode`/`isServerMode` to the concrete `Network` class, which is enough for C2; the public `NetworkInterface` setter is still left for 2.1 as C1 noted.
- Left to do (out of C2 scope):
  - Wiring who creates the `ServerLink`, performs CREATE/JOIN, calls `attachServerLink`, `setServerMode(TRUE)` (after `init`, before `parseUserList`) and `ServerLink::leave()` on match end -> package 2.1. With no link attached and the flag off, behaviour is upstream and the switch-off build is identical.
  - Runtime match verification (two instances through a relay) -> Phase 1/2 gate, needs A (relay) + C3 (launch flags) + 2.1.
- Surprises and open questions:
  - V2 (round-trip): the record is exactly what the engine's own `copyBytesForNetPacket` writes and what its own `ConstructNetCommandMsgFromRawData` reads; the relay forwards the bytes unchanged. So the round trip holds by construction for every carried type. Confirmed by code inspection; a live match confirms it end to end in Phase 2.
  - V3 (remove a player from a `DROP` on a fixed frame): the destroy is injected into the dropped slot's frame data at `last_frame + 1` and every client uses the same `last_frame`, so the `MSG_SELF_DESTRUCT` lands on the same frame on every client. The sim cannot pass `last_frame + 1` before the `DROP` is processed, because the dropped slot's frame data for `last_frame + 1` is empty and not quitting until the `DROP` marks it so, which blocks `allCommandsReady`. Argued from the code; a 3-player drop test in Phase 2/3 is the final check.
  - Host slot for the `TIMEOUTSTART` client check is taken as slot 0, because `GameInfo::amIHost()` defines the host as slot 0. If a future change lets the host occupy another slot, this check and 2.1's `CREATE host_slot` must agree.
  - Player names in server mode come from `TheGameInfo` slots (no `Connection`/`User` objects). Chat and the disconnect screen therefore read names from the staging-room slot list; this matches what `parseUserList` reads in Online mode.
  - `clearGameData()` is called directly from `updateServerMode` on relay loss, mirroring `LANAPICallbacks.cpp` which calls it the same way during an update. The function returns immediately afterwards and touches no members.
  - The over-size (>4096) rule drops the command entirely (no local execution, no send, no count). For a `GAMECOMMAND` this means the local action is lost, but identically on every client, so the lockstep stays consistent. Engine game commands are far below 4096 and `WRAPPER` fragmentation is not used in server mode (D30/D32), so this is not expected to fire in practice.
- Review findings and fixes: reviewed by a second developer with a dedicated pass on simulation integrity, over two rounds. Verdict: approved after one fix.
  - Fixed (was high, simulation integrity): a received FRAMEINFO was routed into the sender's frame data instead of being processed, because FRAMEINFO counts as a synchronized command. A remote slot's frame command count was therefore never set, so no frame was ever ready and the match stalled at about frame 30 on every client. The fix special-cases FRAMEINFO in `processServerRecord` before the synchronized branch: it calls `processFrameInfo` (setting the sender slot's frame command count) and returns without adding the record to frame data, exactly as the normal receive path does; the dead FRAMEINFO case was removed. GAMECOMMAND, PLAYERLEAVE and RUNAHEAD still go through the frame-data path. Re-reviewed: frames now advance. Commit `8b58980d0`.
  - Confirmed correct: with the switch off the build is byte-identical to upstream (every new path is behind the build switch and the server-mode flag); commands are carried in the engine's own full form and round-trip through the existing encode/decode with no repeat compression; the carried allowlist, the drop of commands over 4096 bytes (the sender does not execute or count them either, so clients stay in step), and the not-carried discard all hold; the client checks RUNAHEAD only from the leader and in range, PLAYERLEAVE only where the leaving slot equals the sender, and TIMEOUTSTART only from the host (slot 0, which is the host in this codebase); a relay DROP replaces the vote screen and injects the destroy at last_frame+1 per 4.7; received record lengths and indices are bounds-checked; scope is within the package's files; `NetworkInterface.h` is correctly left for 2.1; commits are by Kayoti with no trailers.
  - Still to confirm with the game installed: a real two-instance match through a relay (the Phase 1/2 gate), which needs C3 and 2.1 as well.
  - HIGH (simulation integrity), fixed: in `processServerRecord` a received `FRAMEINFO` fell into the
    `IsCommandSynchronized` branch (`IsCommandSynchronized` returns TRUE for `FRAMEINFO`), so it was added
    to the sender's frame data and `processFrameInfo` was never called (dead `case` in the else-switch).
    The sender's `m_frameCommandCount` stayed -1 while `m_commandCount` grew, so
    `FrameData::allCommandsReady` (ready when `m_frameCommandCount == m_commandCount`) never became ready
    and the match stalled around frame 30. Fix: special-case `FRAMEINFO` before the `IsCommandSynchronized`
    branch to call `processFrameInfo` and return without adding it to the frame data, mirroring the normal
    `processNetCommand` path (which processes the FRAMEINFO and clears the local relay bit so
    `sendRemoteCommand` does not add it); removed the now-dead `FRAMEINFO` case from the else-switch.
