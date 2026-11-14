# C3 Client: start a relay match from launch flags

State: todo
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

- [ ] The same flags as before exist with the same meaning: `-serversHost`, `-serversJoin`, `-serversRelay`, `-serversPort`, `-serversSlot`, `-serversPlayers`, `-serversMap`, `-serversSession`, `-serversToken`.
- [ ] A flag-started match does not create the online service objects, does not log in and does not contact the lobby service.
- [ ] The match is built around an `NGMPGame` whose slots are filled from the flags, because this client's game logic takes its player list from `TheNGMPGame` when there is no LAN game. The seed, factions, colours and names are the fixed values listed in TESTING.md.
- [ ] The host runs the entry check against the flag relay, sends `CREATE` and logs the session id and join tokens. A joiner runs the entry check and sends `JOIN`. On `START` every instance launches with server mode set and the relay link attached.
- [ ] Without the flags the added call does nothing. With the switch off the flags do not exist.
- [ ] TESTING.md is updated to this base: build commands, where the built executable is, the flags, and the two-instance steps. It says that one-PC checks use the Debug configuration of the `win32-vcpkg-servers` preset, and that the test relay runs with `--no-address-check` and with the entry check, `CREATE` and `JOIN` rate limits raised in its configuration file, because every instance shares one address.
- [ ] Live check, if the game data is installed on this machine: a host and a joiner on one PC reach a running match through a local relay, and the relay log shows the session started with 2 players. If the game data is not installed, this item stays open with the note "pending: needs the game installed" and the package can still be `done`.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Starting from the lobby (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
