# C3 Client: launch flags to create or join a relay match directly

State: blocked
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
- One new file pair under `Core/GameEngine/.../GameNetwork/` that builds the match from the flags

## Must not edit

`LANAPI*`, `GameSpy/`, menus.

## Done when

- [ ] Flags exist, in builds with the Servers switch on: relay host and port, host or join, slot number, number of players, map name, and for joiners the session id and join token.
- [ ] The host instance runs the entry check, sends `CREATE`, and writes the session id and each join token to the log.
- [ ] A joining instance runs the entry check, sends `JOIN` with the given session id and join token, and waits for `START`.
- [ ] On `START` every instance loads the same map with the same seed and fixed default factions, and the match begins.
- [ ] Usage is documented in `docs/servers/TESTING.md`, including the multi-instance build option and window size flags.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Any menu. Any use of the online lobby.

## Handoff notes

Re-attempt after 2.1 merged. Result: still blocked, by a single missing call site. The networking wiring that stopped the first attempt is now sufficient.

- Finished: nothing in code. Feasibility re-checked against the merged code. State set to `blocked`.
- What 2.1 resolved (reachable from the allowed files):
  - (a) Server-mode flag: `Network::parseUserList` (`Core/.../GameNetwork/Network.cpp`) calls `setServerMode(TRUE)` itself when `ServersModeIsOn()` and `ServersMatchLink()` is non-null. A flag path does not need to touch it.
  - (b) Handing over the link: the same function calls `attachServerLink(ServersMatchLink())`. `ServersMatchHostBegin` and `ServersMatchJoinBegin` own the link. The new file would build a `GameInfo` slot list from the flags and run the same public sequence as `LANAPI::OnGameStart` and `GameSpyStagingRoom::launchGame` (`NetworkInterface::createNetwork`, `init`, `setLocalAddress`, `initTransport`, `parseUserList`, then `MSG_NEW_GAME` and `InitRandom`). All of these are public.
  - (c, after launch) The per-frame match drive is `ConnectionManager::updateServerMode`, reached through `TheNetwork->UPDATE()` in `GameEngine::update`. No `GameClient.cpp` edit is needed for it.
- What is still missing (outside the May-edit list):
  - The handshake before launch has to be driven every frame. `ServersEntryStart`/`ServersEntryUpdate` (the entry check; `ServersMatchHostBegin` and `ServersMatchJoinBegin` return FALSE unless it passed and Servers mode is on) and then `ServersMatchUpdate` (CREATE/JOIN up to WELCOME) are called only from menus: `ServersEntryUpdate` from `MainMenu.cpp`, `ServersMatchUpdate` from `WOLGameSetupMenu.cpp` (`updateServersMatchStart`). Nothing a flag can reach calls them. `CommandLine.cpp` runs only at parse time. `SubsystemInterfaceList` has no per-frame update in `GameEngine::update` (each subsystem is updated by hand), so a subsystem registered from the new file would not run either.
  - The start itself needs a call site after engine init. `CommandLine.cpp` runs inside `GameEngine::init` before `TheGameClient`, `TheShell` and `TheGameText` exist.
  - Needed: one call, `ServersDirectUpdate();` (new function in the new C3 file pair, a no-op unless a Servers launch flag was given), in `GameEngine::update()` before `TheNetwork->UPDATE()`, in both `GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp` and `Generals/Code/GameEngine/Source/Common/GameEngine.cpp`, plus the include. It would start the entry check once the shell is up, then the CREATE or JOIN, then the launch. These two files are the only additions to the May-edit list that C3 needs. Nothing else (no `Network.cpp`, `NetworkInterface.h`, `GameLogic.cpp`, menu or staging edits).
  - Alternative the owner may prefer: 2.2 or a later package that touches `GameEngine.cpp` adds the same call.
- Planned design once unblocked (so the review is quick):
  - Flags, parsed in `parseCommandLineForEngineInit`, stored in `GlobalData` of each title (`GeneralsMD/` and `Generals/` headers, "matching global data header") and only with `RTS_SERVERS`: `-serversHost`, `-serversJoin`, `-serversRelay <host>`, `-serversPort <port>`, `-serversSlot <n>`, `-serversPlayers <n>`, `-serversMap <map name>`, `-serversSession <16 hex>`, `-serversToken <16 hex>`. The region comes from the existing `regions.txt` via the entry check; the relay host and port flags override the region's address for local runs only if the entry check API allows it (to confirm when the code is written).
  - Fixed seed: `12345`. Fixed default factions: every slot uses the random template (`PLAYERTEMPLATE_RANDOM` is not deterministic across instances, so slot N gets playable template N modulo the template count, in slot order). Colors are the slot number. These are placeholders; the owner may want specific values.
  - The host logs the session id and each join token (PROTOCOL 5.4 form), joiners take them from `-serversSession` and `-serversToken`.
  - `docs/servers/TESTING.md` is created then: usage, `RTS_MULTI_INSTANCE` build option, `-win -xres -yres` window flags.
- Left to do: whole package, after the owner approves the `GameEngine.cpp` call above (both titles).
- Not verified: nothing was built or run. No code was changed, so no build or lint was needed.
- Review findings and fixes:
