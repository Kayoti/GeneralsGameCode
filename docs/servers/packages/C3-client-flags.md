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
- [x] Reviewed by someone other than the author. Approved; three low notes, none needing a change.

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
  - APPROVED, no changes required. All three findings are low and were judged fine as they stand:
    - On a failure (bad relay answer, refused entry, failed handshake) the path shows a modal "failed" box, which would block a headless/`-quickstart` instance until dismissed. Accepted: this is a debug aid run by hand, and the message is the point.
    - The host slot is whatever `-serversSlot` gives, not forced to 0. This is protocol-consistent (the leader is the lowest connected slot, independent of which slot hosts), so it is correct; the TESTING.md example uses slot 0 by convention.
    - This package file is edited though it is not in the "May edit" list. That is the normal package workflow (State and handoff notes) and is expected.
  - Confirmed correct by the reviewer: the new `ServersDirectUpdate()` call is a true no-op on a normal launch (`s_params` is zero-init and only written by the `-servers*` parsers; with no flags it reaches the done phase on the first frame and returns forever) and the whole feature is behind `RTS_SERVERS`, so the switch-off build is byte-identical and Online/LAN are untouched. The two `GameEngine.cpp` edits are identical. Flag validation is complete (port 1-65535, slot 0-7, players 2-8, slot < players, session/token bounded hex rejecting 0, host charset/length bounded, `strlcpy` throughout); nothing indexes an array by an unchecked value. The JOIN payload reuses 2.1's `ServersMatchStorePrivate`/`BuildPrivate` and matches PROTOCOL 5.4 exactly, with no re-encoding. The `GAME_LAN` + `TheGameSpyGame` launch choice gives the multiplayer load screen and avoids the GameSpy-session code paths; skipping the transport is safe because `ConnectionManager::update` returns early in server mode. The `CMakeLists.txt` change is exactly the two lines that register the new file. Commits are by Kayoti with no trailers.
  - Needs the game installed (not run here): the live on-START match — two instances loading the same map (seed 12345) with the fixed factions and finishing. This is the Phase 1 A+C gate; the code path was verified by reading but not run.
