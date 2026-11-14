# C2 Client: match commands through the relay

State: todo
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

- [ ] Every hunk of the earlier change is re-applied by hand to the same functions, behind `RTS_SERVERS`. The new record-path block (`updateServerMode`, `sendServerModeCommand`, `queueServerRecord`, `flushServerSend`, `processServerDataMessage`, `processServerRecord`, `handleServerDrop`) is carried over unchanged unless a difference on this base forces a change; each forced change is listed in the handoff notes.
- [ ] In server mode `ConnectionManager::initTransport` creates no `NextGenTransport` and no `UDPTransport`, and nothing dereferences a missing transport.
- [ ] In server mode the run-ahead is not forced to 0 because the mesh has no connections (`updateRunAhead`), `getMaximumLatency` does not read the mesh, and `FrameMetrics::init` does not divide by the mesh's connection count. Each of these three guards is behind `RTS_SERVERS` and tests `isServerMode`, not the mesh.
- [ ] In server mode the latency used for run-ahead comes from the relay link's own measurement, as on the earlier base.
- [ ] The disconnect screen never appears in server mode. A `DROP` from the relay removes the player at the stated frame on every client.
- [ ] The engine's acknowledgement path (`ackCommand`, pending and relayed command lists, resends, keep-alives, frame resend requests) is not entered in server mode.
- [ ] With the switch off, and with the switch on but server mode not set, the compiled behaviour of Online and Network matches is unchanged. The reviewer checks this by reading every changed line outside the new block.
- [ ] Nothing that affects the simulation result changed.
- [ ] The first frame number and the frame window assumed by the relay (PROTOCOL.md 4.6) still hold at this base's 60 frames per second, by reading `Network::init` and `Network::update`. If they do not, the package stops as `blocked` with the numbers.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Starting a match in server mode (C3 from launch flags, 2.1 from the lobby).
- The server clock (Phase 3).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
