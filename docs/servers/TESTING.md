# Testing

> The build sections below match this base (Generals Online client). The launch-flag sections were written for the earlier base and are restored by package C3; the flags do not exist in the client until then. The flag names and their meaning stay the same.

## A Servers match without the lobby

This page explains how to start a relay match from launch flags, with several game instances on one PC. It is for the phase gates and for debugging. It needs a build with the Servers switch on and the game data installed.

## Build the client

The client is a 32-bit Windows build. Run everything in one `cmd` session from the repository root, in the x86 developer environment. Visual Studio 2026 Build Tools provide the compiler and a bundled vcpkg; `VCPKG_ROOT` points at that copy. CMake and Ninja must be on `PATH`. The first configure compiles the vcpkg libraries (about 45 minutes) and needs internet access. Later runs reuse them.

```
"C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\VC\Auxiliary\Build\vcvarsall.bat" x86
set "VCPKG_ROOT=C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\VC\vcpkg"
```

Switch off (the plain client; this is the baseline):

```
cmake --preset win32-vcpkg
cmake --build build/win32-vcpkg --config Release --target z_generals
```

Switch on (Servers mode, with several instances allowed):

```
cmake --preset win32-vcpkg-servers
cmake --build build/win32-vcpkg-servers --config Release --target z_generals
cmake --build build/win32-vcpkg-servers --config Debug --target z_generals
```

- The preset `win32-vcpkg-servers` inherits `win32-vcpkg` and sets `RTS_BUILD_OPTION_SERVERS=ON` (defines `RTS_SERVERS`), `RTS_DEBUG_MULTI_INSTANCE=ON` (defines `RTS_MULTI_INSTANCE`, which lets several game instances run side by side; without it a second instance exits at start) and `RTS_BUILD_GENERALS=OFF`.
- With `RTS_BUILD_OPTION_SERVERS` off the Servers code and the flags below do not exist.
- The Debug configuration of the Servers preset is the one used by the phase checks that need the game.
- Build folders are under `build/` and are ignored by git.

## Build the relay

The relay is a standalone x64 CMake project under `Servers/Relay`. It does not use the game build. It fetches ENet, libsodium and libmaxminddb on the first configure, so it needs internet access. Run in a `cmd` session from the repository root:

```
"C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\VC\Auxiliary\Build\vcvarsall.bat" x64
cmake -S Servers/Relay -B build/relay -G Ninja -DCMAKE_BUILD_TYPE=Release -DRELAY_BUILD_TESTS=ON
cmake --build build/relay
ctest --test-dir build/relay --output-on-failure
```

`Servers/Relay/README.md` gives the same configure and build commands for `<build dir>`. `relay_tests` can also be run directly: `build/relay/relay_tests.exe` prints `all relay tests passed`.

## Build and run the lobby service

The lobby service is a Go module under `Servers/Lobby`. It needs Go 1.26 or newer and internet access on the first build, to download its outside modules (the SQLite driver, pure Go, and the modules it needs). Run in a shell from the repository root:

```
cd Servers/Lobby
gofmt -l .
go vet ./...
go test ./...
go build -o <build dir>/lobby.exe ./cmd/lobby
```

`gofmt -l .` prints nothing when the formatting is clean. Put `<build dir>` outside the repository: `.gitignore` does not ignore `.exe` files.

Start the service with the example configuration, `Servers/Lobby/lobby.conf`. The database path in the file is relative to the working directory, so run the service from a folder outside the repository, or change `database`:

```
<build dir>/lobby.exe --config <repository>/Servers/Lobby/lobby.conf
```

The example listens on `127.0.0.1:9000`. Set the TURN secret in the environment rather than in the file: `set LOBBY_TURN_SECRET=<secret>` in `cmd`. A missing or invalid value stops the service at start and the message names the key. Check that it is up:

```
curl http://127.0.0.1:9000/healthz
```

It answers `ok`. Calls are at `/env/{prod,test,dev}/contract/1/{Endpoint}`. Each request is logged with its method, path, status, duration and the caller's address with the last part hidden.

### Check the login calls with lobbyprobe

`Servers/Lobby/tools/lobbyprobe` makes the login calls the way the game client does: version check, login code, check login, refresh, and login with token. It prints one line per step and exits with status 1 at the first step that fails. It never prints a token. With the service above running:

```
cd Servers/Lobby
go build -o <build dir>/lobbyprobe.exe ./tools/lobbyprobe
<build dir>/lobbyprobe.exe --url http://127.0.0.1:9000 --env dev
```

A passing run prints five lines, `VersionCheck: ok, no update` to `LoginWithToken: ok, user id <id>`, and exits with 0. Each run creates a new account, so user ids go up by one per run. Stop the service and run it again to see the failure message and status 1.

## Flags (restored by package C3)

All flags are read at engine start. The flag names are not case sensitive.

| Flag | Value | Meaning |
|---|---|---|
| `-serversHost` | none | This instance creates the match (sends `CREATE`). |
| `-serversJoin` | none | This instance joins a match (sends `JOIN`). |
| `-serversRelay` | host name or IPv4 address | The relay to use. Replaces the region file. |
| `-serversPort` | 1 to 65535 | The relay's UDP port. |
| `-serversSlot` | 0 to 7 | This instance's slot. It must be below `-serversPlayers`. |
| `-serversPlayers` | 2 to 8 | The number of human players in the match. Slots 0 to N-1 are used. |
| `-serversMap` | map name | The map to play. Every instance must have it. See below. |
| `-serversSession` | up to 16 hex digits | Joiner only. The session id the host logged. |
| `-serversToken` | up to 16 hex digits | Joiner only. The join token the host logged for this slot. |

Give exactly one of `-serversHost` and `-serversJoin`. `-serversRelay`, `-serversPort`, `-serversSlot`, `-serversPlayers` and `-serversMap` are needed by both. A joiner also needs `-serversSession` and `-serversToken`. A host must not be given them. If any flag has a bad value or one is missing, the game logs why (debug builds) and starts normally with the menu. Nothing is sent to the relay.

`-serversMap` is the map's name in the map cache, for example `Maps\Tournament Desert\Tournament Desert.map`. A shorter form is also tried: `Tournament Desert\Tournament Desert.map`, or just `Tournament Desert`. Every instance must be given the same map.

## Fixed match values (restored by package C3)

The match settings are the same for every instance, so all instances load the same game. These values are chosen for testing and can be changed in `Core/GameEngine/Source/GameNetwork/ServersDirect.cpp`.

- Random seed: `12345`.
- Faction of slot N: the Nth playable player template in the order of the game's player template list (without the `Boss` side), wrapping around when there are more slots than templates.
- Colour of slot N: colour number N.
- Team: none (every player for themselves). Start positions are assigned from the seed by the game, as for a random start position in the lobby.
- Player names: `Player 1` to `Player N`.

## Window flags (restored by package C3)

Use the existing flags to put the instances side by side:

- `-win` runs in a window.
- `-xres <width>` and `-yres <height>` set the window size, for example `-xres 800 -yres 600`.
- `-quickstart` skips the intro, the shell map and the window animation, so the main menu comes up fast. The direct start waits for the main menu.

## Running a two player match (restored by package C3)

1. Start the relay (see the relay's own notes) so that it listens on a known address and port. The examples use `127.0.0.1` and `7777`.
2. Start the host from the game folder:

```
generalszh.exe -win -xres 800 -yres 600 -quickstart -serversHost -serversRelay 127.0.0.1 -serversPort 7777 -serversSlot 0 -serversPlayers 2 -serversMap "Tournament Desert"
```

3. The host runs the entry check, sends `CREATE`, and on `CREATED` writes the session id and each other slot's join token. They are in the debug log (`Servers direct: slot 1 joins with ...`) and, in any build, in `ServersDirectMatch.txt` in the working directory. One line per joiner, for example:

```
-serversJoin -serversSlot 1 -serversSession 00000000000000a1 -serversToken 0123456789abcdef
```

4. Start each joiner with that line and the common flags:

```
generalszh.exe -win -xres 800 -yres 600 -quickstart -serversRelay 127.0.0.1 -serversPort 7777 -serversPlayers 2 -serversMap "Tournament Desert" -serversJoin -serversSlot 1 -serversSession 00000000000000a1 -serversToken 0123456789abcdef
```

5. Each instance runs the entry check and sends `JOIN`, then waits for `START`. On `START` every instance loads the map with the fixed seed and factions and the match begins.

The host launches as soon as the relay has answered `CREATE`. The relay waits 30 seconds after `CREATED` for the joiners (PROTOCOL.md 4.5), so start them promptly. A joiner that is late is dropped as `NEVER_JOINED`.

A join token works once. To run again, start a new host and use its new tokens.

## What is logged (restored by package C3)

The lines start with `Servers direct:` and tell which step ran: the entry check result, `CREATE` or `JOIN` sent, the tokens, and the launch (map, seed, local slot). A failure shows a message box and the reason in the log.

## Notes (restored by package C3)

- The match runs through the relay only. The online lobby, NAT negotiation and the engine's player-to-player connections are not used.
- This path is a test aid. Ending the match returns to the main menu, and the score screen is not the one of a Servers match (package 2.2).

## Findings from the first live run (2026-11-14)

Verified on Windows with a build of the earlier base (`RTS_BUILD_OPTION_SERVERS=ON`, `RTS_DEBUG_MULTI_INSTANCE=ON`). A host and a joiner started from the flags above completed the full path on one PC: entry check pass, `CREATE`/`JOIN`, `session started with 2 players`, both instances loaded the map and ran the lockstep simulation with `FRAMEINFO` flowing through the relay. Points that are easy to trip over:

- **Relay on one PC:** run it with `--no-address-check`, or the second instance from `127.0.0.1` is refused by the per-address limit. The config must list every `vpn_*` key even when `vpn_check = off` (only the data *files* are skipped, not the keys). Generate the region key once with `relay --generate-key <file>` before first run.
- **`-serversMap` needs the official map cache.** The resolver only matches a map held under an official-style key (`maps\<name>\<name>.map`); it lowercases and looks up that exact key (`MapCache::findMap`). An install whose `MapCache.ini` holds only user maps (each keyed by its absolute path) has no such entry, so the host logs `map <name> is not in the map cache` and aborts before `CREATE` completes. This is common on a GeneralsOnline install, whose local cache carries only custom maps. Build the standard cache (`generalszh.exe -buildmapcache`, or feed the official map list to `mapcachebuilder`) so the chosen map is present under that key.
- **The GeneralsOnline community data patch is not loadable into a vanilla build.** Passing `-mod <...>\GeneralsOnlineGameData\500_900_CommunityPatch_CoreINI.big` loads the archive (`BIGFileSystem::loadMods`) but the vanilla engine then crashes at startup on the patched data, because the patch targets GeneralsOnline's own modified engine. Run the live test on the clean build (both instances share the same executable, so their CRCs match each other, which is all the relay needs). Running Servers mode against GeneralsOnline's data and clients requires building this feature into the GeneralsOnline engine, not loading its patch into the vanilla build.
