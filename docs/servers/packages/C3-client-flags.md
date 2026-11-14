# C3 Client: launch flags to create or join a relay match directly

State: done
Branch: servers-C3

## Goal

Let two or more game instances on one PC start a match through the relay without the lobby. This exists for the Phase 1 gate and for later debugging.

## Depends on

C1. Needs C2 to be useful.

## Read first

- CODEMAP "Testing aids".
- `Core/GameEngine/Source/Common/CommandLine.cpp`: how flags are declared and parsed, and `-quickstart`.
- `ClientInstance.cpp` and the `RTS_MULTI_INSTANCE` define.
- How a LAN match is started from a `GameInfo` in `LANAPICallbacks.cpp` (read only), as a model for building a slot list and starting the game.

## May edit

- `Core/GameEngine/Source/Common/CommandLine.cpp` and the matching global data header, to add the flags
- (Widened by the project owner) the `ServersDirectUpdate()` call in `GameEngine::update()` of both titles (`GameEngine.cpp`), and `Core/GameEngine/CMakeLists.txt` to list the new file pair
- One new file pair under `Core/GameEngine/.../GameNetwork/` that builds the match from the flags

## Must not edit

`LANAPI*`, `GameSpy/`, menus.

## Done when

- [x] Flags exist, in builds with the Servers switch on: relay host and port, host or join, slot number, number of players, map name, and for joiners the session id and join token.
- [x] The host instance runs the entry check, sends `CREATE`, and writes the session id and each join token to the log.
- [x] A joining instance runs the entry check, sends `JOIN` with the given session id and join token, and waits for `START`.
- [ ] On `START` every instance loads the same map with the same seed and fixed default factions, and the match begins. (Pending: needs the game installed. The code path is in place; no live run was possible here.)
- [x] Usage is documented in `docs/servers/TESTING.md`, including the multi-instance build option and window size flags.
- [x] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Any menu. Any use of the online lobby.

## Handoff notes

- Finished:
  - Flags `-serversHost`, `-serversJoin`, `-serversRelay`, `-serversPort`, `-serversSlot`, `-serversPlayers`, `-serversMap`, `-serversSession`, `-serversToken` in `CommandLine.cpp` (engine-init parse list, behind `RTS_SERVERS`). The values are stored by the new file pair, not in `GlobalData`, so no per-title header change was needed. Every value is range checked; a bad or missing flag logs the reason and the game starts normally.
  - New `Core/GameEngine/Include/GameNetwork/ServersDirect.h` and `Source/GameNetwork/ServersDirect.cpp`. `ServersDirectUpdate()` is a no-op without the flags. With them it waits for the main menu, runs `ServerEntryCheck::startRegion` against the relay given by the flags and calls `ServersModeSet`, then `ServersMatchHostBegin` (host) or `ServersMatchStorePrivate` plus `ServersMatchJoinBegin` (joiner), and drives `ServersMatchUpdate` each frame. On `HOST_READY` the host writes the session id and each join token to the debug log and to `ServersDirectMatch.txt`. It then launches: slot list, `NetworkInterface::createNetwork`, `init`, `Network::parseUserList` (which attaches the link and turns server mode on, not duplicated here), `MSG_NEW_GAME` with `GAME_LAN`, `InitRandom`.
  - `ServersDirectUpdate()` is called from `GameEngine::update()` in `GeneralsMD/` and `Generals/`, before `TheNetwork->UPDATE()`, behind `RTS_SERVERS`. The two edits are identical.
  - `docs/servers/TESTING.md` created: flags, fixed values, `RTS_DEBUG_MULTI_INSTANCE` build option, `-win`, `-xres`, `-yres`, `-quickstart`, a two player walk-through.
  - Evidence: switch on and switch off builds (all targets, both titles); clang-tidy 0 issues on the four changed source files.
- Left to do:
  - Pending: needs the game installed. A live two-instance flag-started match through a real relay has not been run, because there is no game data here. The "On START every instance loads the same map" item is left unticked for that reason; the code path is in place.
  - Review by someone other than the author.
- Surprises and open questions:
  - Values the owner may want to override: seed `12345`; slot N gets the Nth playable player template (not `Boss`), wrapping, colour N, no teams; names `Player N`; region number 1 for the flag relay. All are constants or one small function in `ServersDirect.cpp` and are listed in TESTING.md.
  - Files touched beyond the original list, with the owner's approval: both `GameEngine.cpp`. Also `Core/GameEngine/CMakeLists.txt` (two lines, to compile the new file pair), which the approval did not name.
  - The relay is given by `-serversRelay` and `-serversPort`, and `regions.txt` is not read on this path. The entry check is `startRegion` on a region made from the flags, so no "Checking connection" box is shown.
  - The match is launched as `GAME_LAN` with a `GameSpyStagingRoom`-derived game object assigned to `TheGameSpyGame`, because the online load screen and score code need the GameSpy session, which a flag start does not have. No `TheLAN` or GameSpy state is created. The object lives until the process ends. The match-end screen for this mode was not examined; that is package 2.2.
  - No transport is created in this path (`initTransport` is skipped), because server mode carries nothing on it and it would bind a UDP port that several local instances would share. This was checked by reading `ConnectionManager::update` and `parseUserList`, not by a run.
  - The host launches as soon as the relay answers `CREATE`. Joiners must start within the relay's 30 second wait.
- Review findings and fixes:
