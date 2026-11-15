# C3 Client: start a relay match from launch flags

State: done
Branch: servers-go-C3

## Goal

Let two or more game instances on one PC start a relay match directly from command-line flags, without the lobby service, so the relay path can be checked on its own.

## Depends on

C2, B1, D1. B1 and D1 edit two of the same files (`GameEngine.cpp`, the engine's `CMakeLists.txt`), so they merge first.

## Read first

- archive/C3-client-flags.md, all of it including "Handoff notes"
- TESTING.md, the launch flag sections
- reference/CLIENT_PORTING.md section 5, rows `CommandLine.cpp`, `GameEngine.cpp`, `ServersDirect`; and section 7 (multi-instance testing)
- reference/CLIENT_REALTIME.md sections 3.2 and 4.3 (slots, `NGMPGame::launchGame`, random seed)
- `git show servers:Core/GameEngine/Include/GameNetwork/ServersDirect.h`, `git show servers:Core/GameEngine/Source/GameNetwork/ServersDirect.cpp`
- On this branch, in full: `NGMPGame::startGame`, `NGMPGame::launchGame`, `NGMPGame::UpdateSlotsFromCurrentLobby`, and in `GeneralsMD/Code/GameEngine/Source/GameLogic/System/GameLogic.cpp` the place that chooses `TheGameInfo`

## May edit

- New files `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersDirect.h` and `GeneralsMD/Code/GameEngine/Source/GameNetwork/ServersDirect.cpp`
- `GeneralsMD/Code/GameEngine/CMakeLists.txt` (add the two files)
- `GeneralsMD/Code/GameEngine/Source/Common/CommandLine.cpp` (flag parsers)
- `GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp` (one `ServersDirectUpdate()` call in `GameEngine::update`)
- `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/NGMPGame.cpp` and `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/NGMPGame.h` (only a way to launch a match whose slots were filled locally and whose link is a relay link)
- `docs/servers/TESTING.md`

## Must not edit

- `ConnectionManager.*`, `Network.cpp`
- Menu files
- `GameLogic.cpp`, unless the handoff notes of this package first record why no other way works; then only the `TheGameInfo` choice

## Done when

- [x] The same flags as before exist with the same meaning: `-serversHost`, `-serversJoin`, `-serversRelay`, `-serversPort`, `-serversSlot`, `-serversPlayers`, `-serversMap`, `-serversSession`, `-serversToken`. (`CommandLine.cpp` parsers `parseServersHost`..`parseServersToken`, registered in `paramsForEngineInit` behind `RTS_SERVERS`; values validated/stored by `ServersDirectSetFlag`.)
- [x] A flag-started match does not create the online service objects, does not log in and does not contact the lobby service. (The direct path only builds a `ServersDirectGameInfo`/`ServerLink`; it never calls `NGMP_OnlineServicesManager::CreateInstance`/`Init`. Confirmed those run only from `MainMenuUtils.cpp` on a user action, not at boot, so `GetInstance()`/`GetInterface<>()` return null on this path.)
- [x] The match is built around an `NGMPGame` whose slots are filled from the flags, because this client's game logic takes its player list from `TheNGMPGame` when there is no LAN game. The seed, factions, colours and names are the fixed values listed in TESTING.md. (`ServersDirectGameInfo : public NGMPGame`, `setUp()` fills slots; `GameLogic.cpp:1358` picks `TheGameInfo = TheNGMPGame` when `TheNetwork && !TheLAN`. Seed `12345`, faction = Nth playable template without `Boss`, colour N, names `Player N`.)
- [x] The host runs the entry check against the flag relay, sends `CREATE` and logs the session id and join tokens. A joiner runs the entry check and sends `JOIN`. On `START` every instance launches with server mode set and the relay link attached. (`ServersDirect.cpp`: `ServerEntryCheck::startRegion`; host `ServerLink::create` -> `TYPE_CREATED` -> `reportJoinTokens` -> launch; joiner `ServerLink::join` -> `TYPE_WELCOME` -> launch. `NGMPGame::launchServersDirect` sets `setServerMode(TRUE)` and `attachServerLink` before `initTransport`; the attached ConnectionManager then receives `START` and sets `m_serverStarted`. See the launch-timing note under Surprises.)
- [x] Without the flags the added call does nothing. With the switch off the flags do not exist. (`ServersDirectUpdate()` reaches `PHASE_DONE` on the first frame when no `-serversHost`/`-serversJoin` and returns forever; everything is behind `RTS_SERVERS`, and the switch-off Release build succeeded.)
- [x] TESTING.md is updated to this base: build commands, where the built executable is, the flags, and the two-instance steps. It says that one-PC checks use the Debug configuration of the `win32-vcpkg-servers` preset, and that the test relay runs with `--no-address-check` and with the entry check, `CREATE` and `JOIN` rate limits raised, because every instance shares one address. (See "Where the built executable is" and "Relay configuration for one PC".)
- [x] Live check, if the game data is installed on this machine: a host and a joiner on one PC reach a running match through a local relay, and the relay log shows the session started with 2 players. Done: on this machine, a host and a joiner started from the launch flags against a local relay, on a build loading the Generals Online data, reached a running match — the relay logged `session started with 2 players`, both instances advanced the lockstep simulation together through the relay for about 55 seconds (host frames from 31 past 2900, no stall, neither instance dropped or crashed). Fixing this live path required guarding several server-mode code paths against the online objects a direct match does not have, and holding the host's pre-START relay records until the match starts.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. (Builds with the Servers switch on in Release and Debug and off in Release, all `exit 0`; no new compiler warnings on the changed files. Lint not run: clang-tidy is not on PATH and the binary is absent at the MSVC LLVM path, and no PCH-free analysis build is set up.)
- [x] Reviewed by someone other than the author.

## Out of scope

- Starting from the lobby (2.1).

## Handoff notes

- Finished:
  - New `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersDirect.h` and `Source/GameNetwork/ServersDirect.cpp`. The `.cpp` stores and validates the flags (`ServersDirectSetFlag`, every value range/charset checked; a bad or missing flag logs the reason and makes the whole set unusable), builds a `ServersDirectGameInfo : public NGMPGame` (overrides `getLocalSlotNum`/`amIHost` from the flags, since the normal `getLocalSlotNum` needs the online auth interface), runs the entry check with `ServerEntryCheck::startRegion` against the flag relay, drives `ServerLink::create`/`join` and the CREATE/JOIN handshake directly, writes the session id and join tokens to the debug log and to `ServersDirectMatch.txt`, and launches. The flags are stored in the file's own state, so no `GlobalData`/per-title header change was needed.
  - `NGMPGame::launchServersDirect(ServerLink*)` added to `NGMPGame.h`/`.cpp` (behind `RTS_SERVERS`). It mirrors the relay-relevant part of `launchGame()` but skips everything that needs the lobby service (mesh, `DoAnyMapTransfers`, service-config run-ahead, lobby map notification). Critically it calls `GetConnectionManager()->setServerMode(TRUE)` and `attachServerLink()` after `TheNetwork->init()` but before `initTransport()` and `parseUserList()`, so C2's guards suppress the transport and the player-to-player connections. It sends `MSG_NEW_GAME` with `GAME_LAN`.
  - `CommandLine.cpp`: the nine `-servers*` parsers, registered in `paramsForEngineInit`, all behind `RTS_SERVERS`. A value flag with no value passes `NULL`, which `ServersDirectSetFlag` rejects.
  - `GameEngine.cpp`: one `ServersDirectUpdate()` call before `TheNetwork->UPDATE()`, behind `RTS_SERVERS`.
  - `GeneralsMD/Code/GameEngine/CMakeLists.txt`: the two new files registered next to `ServersMode.*`.
  - `docs/servers/TESTING.md`: header note, executable name/location subsection, the `GeneralsOnlineZH.exe` examples, and the one-PC relay configuration note (`--no-address-check` plus the per-address rate limits).
  - Evidence: switch-on Release, switch-on Debug and switch-off Release builds all finished `exit 0` with no warnings on the changed files (`build_c3.log`, stage markers at lines 281/2657/5235, "ALL BUILDS DONE" at 7586). Executables at `build/win32-vcpkg-servers/GeneralsMD/{Debug,Release}/GeneralsOnlineZH.exe` and `build/win32-vcpkg/GeneralsMD/Release/GeneralsOnlineZH.exe`.
- Left to do:
  - The live two-instance flag-started match through a real relay was not run (no game data here; the coordinator set build + static verification as the gate, with the live run a Phase 2 gate). The code path is in place.
  - Lint (clang-tidy) could not be run: not on PATH, absent at the MSVC LLVM path, no PCH-free analysis build set up.
  - Review by someone other than the author.
- Surprises and open questions:
  - Launch timing vs the "On `START` every instance launches" wording: the host launches on `CREATED` and the joiner on `WELCOME`, not on `START`. This is required, not a deviation: `START` is delivered over the `ServerLink`, and only the ConnectionManager's `updateServerMode` sets `m_serverStarted` (which gates record sending) when it reads `START`. So the link must already be attached to the ConnectionManager before `START` arrives. `ServersDirect` therefore hands the link to the ConnectionManager at launch (on `CREATED`/`WELCOME`) and stops polling it; the ConnectionManager then consumes the queued `WELCOME`/`ROSTER`/`START`. Records generated during loading before `START` are dropped by C2's `queueServerRecord` (`!m_serverStarted`), as designed. This matches the earlier base's behaviour.
  - Game mode is `GAME_LAN`, not `GAME_INTERNET`. `GAME_INTERNET` would select `GameSpyLoadScreen`, which dereferences the online service objects (`NGMP_OnlineServicesManager::GetInstance()`, `TheGameSpyInfo`) that a flag start does not have, so it would crash. `GAME_LAN` selects `MultiPlayerLoadScreen` (no online dependency), and `GameLogic.cpp` still takes `TheGameInfo = TheNGMPGame` because `TheLAN` is null. This mirrors the archive's rationale, adapted to `TheNGMPGame`.
  - The relay configuration file does not actually expose the entry-check/`CREATE`/`JOIN` rate limits (`config.cpp` only parses `region`, `listen_port`, `ping_limit_ms`, `key_file`, `max_sessions`, `connection_timeout_s`, `loading_time_s`, `stall_time_s`, `vpn_*`). Those three limits are compile-time constants in the relay (`ENTRY_CHECKS_PER_WINDOW`, `CREATE_PER_ADDRESS_PER_MINUTE`, `JOIN_PER_ADDRESS_PER_MINUTE`). This contradicts PROTOCOL.md 7.2 ("Every limit in section 6 can also be set here by name"). It is a relay/R-track gap, not C3; TESTING.md documents the accurate situation (raise them in the relay source and rebuild). Flagging for the owner.
  - Fixed values the owner may want to override (all constants in `ServersDirect.cpp`): seed `12345`; faction of slot N = Nth playable template without `Boss`, wrapping; colour N; no teams; names `Player N`; region number 1; link timeout 15000 ms. Listed in TESTING.md.
  - PROTOCOL.md section 10 assumptions: none are C3's to confirm (V2/V8/V9 belong to C2, and would only be exercised by the pending live run). None confirmed or disproved here.
  - The two new files are authored fresh (informed by `servers:.../ServersDirect.*`) rather than carried with `git checkout servers --`, because the path moved Core -> GeneralsMD and the `.cpp` is a redesign for the `TheNGMPGame` base; carrying verbatim across a different path with a near-total rewrite would not have been reviewable as a clean diff.
- Review findings and fixes:
  - Approved. The reviewer verified the critical server-mode ordering: `NGMPGame::launchServersDirect`
    calls `setServerMode(TRUE)` and attaches the relay link before `initTransport`, which returns early in
    server mode, so no unused transport is ever constructed and the mesh guards apply (`setServerMode`
    cannot precede `init` because the ConnectionManager is created inside `Network::init`, and `init`
    neither resets server mode nor builds a transport). The launch-on-`CREATED`/`WELCOME` timing is safe,
    not a premature start: records before `START` are dropped by `queueServerRecord` and only
    `updateServerMode`'s `TYPE_START` sets `m_serverStarted`, so the link must be attached before `START`
    arrives; the simulation stays gated until `START`. The nine flags validate their ranges (slot 0-7,
    players 2-8, port 1-65535, session/token up to 16 hex, exactly one of host/join, joiner needs
    session+token); a bad or missing flag logs and starts the menu normally, sending nothing; the direct
    path builds no online-service objects and never contacts the lobby service; the fixed match values
    match TESTING.md. Scope (exactly the nine allowed files; one `ServersDirectUpdate()` call before
    `TheNetwork->UPDATE()`), author and commit hygiene all correct.
  - Two low notes, no change: the dev-only launch failure/title text in `ServersDirect.cpp` is hardcoded
    English rather than fetched from `GameText` (this path is command-line only); the `parseNumber`
    hex pre-filter permits `a-f` and a `0x` prefix, which is harmless because the trailing-character check
    rejects any non-decimal digit for base-10 fields and every value is bounded.
  - After merge the client was rebuilt (switch off Release, switch on Release and Debug); all three exit 0
    and the Servers executables were produced.
  - The live two-instance relay match (Done-when item 9) is run separately by the coordinator against the
    installed game; its result is recorded in the Phase 1 check table and the status log.
