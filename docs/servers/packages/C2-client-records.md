# C2 Client: match commands through the relay

State: done
Branch: servers-go-C2

## Goal

Re-apply the server mode of the connection manager on this base: in a Servers match every command travels as a relay record, the engine's own packet layer and the player-to-player mesh are not used, and the relay decides who is dropped.

## Depends on

C1

## Read first

- DECISIONS.md D8, D30, D50
- PROTOCOL.md sections 4.6, 4.7 and 4.9, and V2, V3, V7, V8, V9 in section 10
- archive/C2-client-records.md, all of it including "Handoff notes" (what was built on the earlier base and why)
- reference/CLIENT_PORTING.md section 5, rows `ConnectionManager.h`, `ConnectionManager.cpp`, `Network.cpp`, `DisconnectManager.cpp`
- reference/CLIENT_REALTIME.md sections 3.1, 3.2, 4.3 (how a match is launched), 5 (what Generals Online changed in the shared netcode) and 8 "Seam A" constraints (what assumes the mesh exists)
- The earlier change itself: `git diff upstream/main...servers -- Core/GameEngine/Include/GameNetwork/ConnectionManager.h Core/GameEngine/Source/GameNetwork/ConnectionManager.cpp Core/GameEngine/Source/GameNetwork/Network.cpp Core/GameEngine/Source/GameNetwork/DisconnectManager.cpp`
- On this branch, in full: `ConnectionManager::initTransport`, `parseUserList`, `update`, `updateRunAhead`, `getMaximumLatency`, `sendLocalCommand`, `Network::parseUserList`, `Network::update`, `FrameMetrics::init`, `NGMPGame::launchGame`

## May edit

- `Core/GameEngine/Include/GameNetwork/ConnectionManager.h`
- `Core/GameEngine/Source/GameNetwork/ConnectionManager.cpp`, `Network.cpp`, `DisconnectManager.cpp`, `FrameMetrics.cpp`
- `Core/GameEngine/Include/GameNetwork/NetworkInterface.h` (only to declare the server-mode calls)

## Must not edit

- `ServerLink.*`, `ServerEntryCheck.*`, `ServerRegions.*`
- Anything under `GameNetwork/GeneralsOnline/` (the launch hook is package 2.1 and C3)
- Any file under `GameLogic/`

## Done when

- [x] Every hunk of the earlier change is re-applied by hand to the same functions, behind `RTS_SERVERS`. The new record-path block (`updateServerMode`, `sendServerModeCommand`, `queueServerRecord`, `flushServerSend`, `processServerDataMessage`, `processServerRecord`, `handleServerDrop`) is carried over unchanged unless a difference on this base forces a change; each forced change is listed in the handoff notes. Evidence: `ConnectionManager.h/.cpp` and `DisconnectManager.cpp` carry the earlier hunks; the record block is carried byte-for-byte except the comment dates (15/11/2026). Base-forced additions (new to this base) are listed under "Surprises".
- [x] In server mode `ConnectionManager::initTransport` creates no `NextGenTransport` and no `UDPTransport`, and nothing dereferences a missing transport. Evidence: `initTransport` returns early when `m_serverMode`; the only `m_transport` dereferences are in `update`/`doRelay`/`doKeepAlive` (all behind the `updateServerMode` divert) and in null-guarded getters/`flushConnections`.
- [x] In server mode the run-ahead is not forced to 0 because the mesh has no connections (`updateRunAhead`), `getMaximumLatency` does not read the mesh, and `FrameMetrics::init` does not divide by the mesh's connection count. Each of these three guards is behind `RTS_SERVERS` and tests `isServerMode`, not the mesh. Evidence: `updateRunAhead` `allowInstantRunAhead`; `getMaximumLatency` early `return maxLatency`; `FrameMetrics::init` `serverMode` skips the mesh-latency average.
- [x] In server mode the latency used for run-ahead comes from the relay link's own measurement, as on the earlier base. Evidence: `getMaximumLatency` returns `maxLatency` (max of `m_latencyAverages`), exactly as the plain base; the relay RTT reaches `m_latencyAverages` via `ConnectionManager::SeedLatencyData`, which 2.1 calls at launch (see Surprises).
- [x] The disconnect screen never appears in server mode. A `DROP` from the relay removes the player at the stated frame on every client. Evidence: `DisconnectManager::updateDisconnectStatus` and `ConnectionManager::voteForPlayerDisconnect` return early in server mode; `handleServerDrop` injects the destroy at `last_frame + 1` (PROTOCOL 4.7).
- [x] The engine's acknowledgement path (`ackCommand`, pending and relayed command lists, resends, keep-alives, frame resend requests) is not entered in server mode. Evidence: `update` diverts to `updateServerMode`, which never calls `doRelay`/`doKeepAlive`; the server send paths add to no pending/relayed list.
- [x] With the switch off, and with the switch on but server mode not set, the compiled behaviour of Online and Network matches is unchanged. The reviewer checks this by reading every changed line outside the new block. Evidence: every added path is behind `#ifdef RTS_SERVERS` and gated on `m_serverMode`, except four behaviour-preserving restructurings (`FrameMetrics::init` `serverMode` folds to constant FALSE off; `updateRunAhead` `allowInstantRunAhead` folds to TRUE off; `parseUserList`'s block runs unconditionally off; `getPlayerName`'s early `return` is equivalent to the old `else if`). Switch-off Release built clean.
- [x] Nothing that affects the simulation result changed. Evidence: records carry the engine's own `copyBytesForNetPacket` form with no recompression; the only state write is `handleServerDrop`'s destroy at the shared `last_frame + 1`; no `GameLogic/` file touched.
- [x] The first frame number and the frame window assumed by the relay (PROTOCOL.md 4.6) still hold at this base's 60 frames per second, by reading `Network::init` and `Network::update`. If they do not, the package stops as `blocked` with the numbers. Evidence: `Network::init` -> `m_runAhead = min(max(30,6),64) = 30`; `Network::parseUserList` -> `zeroFrames(1,29)`; relay `next_frame` starts 30, window `+128`; `Network::update` runs logic at `GENERALS_ONLINE_HIGH_FPS_LIMIT = 60` in game.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. Evidence: builds switch on (Release and Debug) and switch off (Release), all exit 0, no new warnings on the changed files (the C4018/C4305 at `ConnectionManager.cpp` 1948/2003/2018 and `FrameMetrics.cpp` 142/143 are pre-existing base code, present in the switch-off build too). Lint: `scripts/run-clang-tidy.py` not run (optional per the coordinator; the MSVC build reports no new warnings on the changed files).
- [ ] Reviewed by someone other than the author.

## Out of scope

- Starting a match in server mode (C3 from launch flags, 2.1 from the lobby).
- The server clock (Phase 3).

## Handoff notes

- Finished:
  - `ConnectionManager.h/.cpp`: the server-mode match-record path behind `#ifdef RTS_SERVERS`.
    - Forward decl `class ServerLink;`; public `setServerMode`/`isServerMode`/`attachServerLink`; private record-path methods; members `m_serverMode`, `m_serverStarted`, `m_serverLink` (non-owning), `m_serverSendBuffer[16383]`, `m_serverSendLength`.
    - `update` diverts to `updateServerMode` (polls the link, turns events into state/records, runs the disconnect screen, flushes, deletes quit-frame data). No transport, no `doRelay`, no `doKeepAlive`.
    - `sendLocalCommand`/`sendLocalCommandDirect` divert to `sendServerModeCommand` (local frame-data add for synchronized commands + `queueServerRecord` for carried commands, little-endian `u16` length prefix, flush at the 16383-byte limit, nothing before `START`, over-4096 dropped).
    - Receive: `processServerDataMessage` (length-checked record split) + `processServerRecord` (`ConstructNetCommandMsgFromRawData`, slot/carried/per-type checks, `FRAMEINFO` special-cased to `processFrameInfo`, synchronized -> sender frame data, others -> the existing `process*` handlers).
    - `handleServerDrop` implements PROTOCOL 4.7.
    - `parseUserList` skips `Connection`/`Transport` in server mode (keeps the frame data); `isPlayerConnected`, `getPlayerName` and `areAllQueuesEmpty` have server-mode branches driven by frame data / `TheGameInfo`; `updateRunAhead` routes `RUNAHEADMETRICS` through the record path; `voteForPlayerDisconnect` is a no-op in server mode.
  - Base-specific guards new to this (Generals Online) base, each behind `RTS_SERVERS` and testing `isServerMode` (the earlier plain base had none of these):
    - `initTransport` returns early in server mode, so no `NextGenTransport`/`UDPTransport` is created (on this base the transport is created here, not in `parseUserList`).
    - `updateRunAhead` does not force run-ahead to 0 on an empty mesh in server mode (`allowInstantRunAhead`).
    - `getMaximumLatency` returns `maxLatency` in server mode and never reads the mesh.
    - `FrameMetrics::init` does not read the mesh latency (which would divide by the 0 mesh connections) in server mode; it starts from the default and reaches server mode through `TheNetwork->GetConnectionManager()->isServerMode()`.
  - `DisconnectManager.cpp`: `updateDisconnectStatus` returns early in server mode (no vote, no removal).
  - `NetworkInterface.h` and `Network.cpp` were NOT edited. They were in "May edit" but the guards reach `isServerMode` through `ConnectionManager` without a `Network`-level accessor, and turning server mode on is 2.1's wiring (reachable via `TheNetwork->GetConnectionManager()->setServerMode(...)` / `attachServerLink(...)`). This matches C1's note that the `ConnectionManager`/`Network` flag is C2's.
- Left to do (out of C2 scope):
  - The wiring that creates the `ServerLink`, performs CREATE/JOIN, calls `attachServerLink`, `setServerMode(TRUE)` and `ServerLink::leave()` on match end -> package 2.1 (and C3 for launch flags). With no link attached and the flag off, behaviour is upstream and the switch-off build is unchanged.
  - Runtime match verification (two instances through a relay) -> Phase 1/2 gate, needs the relay + C3 (launch flags) + 2.1.
- Surprises and open questions:
  - ORDERING, important for 2.1/C3: on this base the transport is created in `ConnectionManager::initTransport` (launchGame step 5, per CLIENT_REALTIME 4.3), which runs BEFORE `parseUserList`. On the earlier base the transport was created inside `parseUserList`, so the earlier "set server mode after init, before parseUserList" was enough. Here, for `initTransport` to create no transport, 2.1/C3 must call `setServerMode(TRUE)` BEFORE `initTransport` (i.e. before launchGame step 5), not merely before `parseUserList`. The code is still safe if set later (update() diverts and never dereferences `m_transport`), but a transport would be constructed, which Done-when item 2 forbids.
  - `FrameMetrics::init` runs inside `ConnectionManager::init` (launchGame step 3), before any `setServerMode`. Its `isServerMode` guard therefore only takes effect if server mode is set before `init`. In practice this is not a crash on this base: Generals Online already divides the mesh-latency sum by `connections.size()` (0 for a solo/AI mesh) and tolerates the resulting value, and `launchGame` step 4 `SeedLatencyData` overwrites `m_averageLatency` straight after. The guard still gives server mode a clean default instead of a NaN when the flag is set early. Recorded so 2.1 can seed the relay RTT through `SeedLatencyData`.
  - Done-when item 4 (latency from the relay): `getMaximumLatency` in server mode returns `maxLatency`, the max of `m_latencyAverages`, exactly as the earlier plain base did (it never had the Generals Online latency-logic-model / mesh code). The real relay RTT reaches `m_latencyAverages` through `ConnectionManager::SeedLatencyData`, which 2.1 must call in `launchGame` with the relay's measurement in place of `mesh->getMaximumHistoricalLatency()`.
  - The `ServersModeIsOn()` block that the earlier `servers` branch added to `Network::parseUserList` is NOT part of C2: it is the match-start wiring (2.1) and it includes `GameNetwork/GameSpy/ServersMode.h`, a D-track file that does not exist on this branch yet. Not ported.
  - Host slot for the `TIMEOUTSTART` client check is slot 0 (`SERVER_HOST_SLOT`), because `GameInfo::amIHost()` defines the host as slot 0. If a future change lets the host occupy another slot, this check and 2.1's `CREATE host_slot` must agree.
  - PROTOCOL.md section 10 assumptions:
    - V2 (round-trip): holds by construction. A record is exactly what `NetCommandMsg::copyBytesForNetPacket` writes and what `NetPacket::ConstructNetCommandMsgFromRawData` reads; the relay forwards the bytes unchanged. Confirmed by code inspection on this base; a live match is the Phase 2 check.
    - V3 (remove a player on a fixed frame from `DROP`): holds by construction. `handleServerDrop` injects the destroy at `last_frame + 1` into the dropped slot's frame data, discards that slot's records above `last_frame`, and marks the slot quitting; every client uses the same `last_frame`, so the destroy lands on the same frame everywhere. Argued from the code; a 3-player drop test is the Phase 2/3 check.
    - V7 (`command_count`): the client emits `FRAMEINFO` with the count and carries `GAMECOMMAND`/`PLAYERLEAVE`/`RUNAHEAD` as records, which is the traffic the relay's `--strict-command-count` compares. Runtime confirmation needs the relay (Phase 2).
    - V8 (frame window at 60 fps): confirmed by reading. `Network::init` sets `m_runAhead = min(max(30, MIN_RUNAHEAD=6), MAX_FRAMES_AHEAD/2=64) = 30`; `Network::parseUserList` calls `zeroFrames(1, 29)`; the relay's `next_frame` starts at 30 and the window is `next_frame..next_frame+128`; `Network::update` runs the logic at `GENERALS_ONLINE_HIGH_FPS_LIMIT = 60` when in game. So Done-when item 9 and V8 hold at this base.
    - V9 (run-ahead/latency/frame metrics with no player-to-player connections): addressed by the three mesh guards above. Confirmed by code inspection; runtime is Phase 2.
- Review findings and fixes: not yet reviewed. The reviewer box stays unticked for a second developer, with a dedicated pass on anything that could change the simulation result and on the switch-off behaviour of the four non-`#ifdef` restructurings.
