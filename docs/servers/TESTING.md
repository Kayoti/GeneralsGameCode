# Testing a Servers match without the lobby

This page explains how to start a relay match from launch flags, with several game instances on one PC. It is for the phase gates and for debugging. It needs a build with the Servers switch on and the game data installed.

## Build

Configure with both options:

```
cmake --preset win32 -B <build dir> -DRTS_BUILD_OPTION_SERVERS=ON -DRTS_DEBUG_MULTI_INSTANCE=ON
```

- `RTS_BUILD_OPTION_SERVERS=ON` builds Servers mode and the flags below. With it off the flags do not exist and are ignored like any unknown argument.
- `RTS_DEBUG_MULTI_INSTANCE=ON` defines `RTS_MULTI_INSTANCE`, which lets several game instances run side by side (`ClientInstance.cpp`). Without it a second instance exits at start.

## Flags

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

## Fixed match values

The match settings are the same for every instance, so all instances load the same game. These values are chosen for testing and can be changed in `Core/GameEngine/Source/GameNetwork/ServersDirect.cpp`.

- Random seed: `12345`.
- Faction of slot N: the Nth playable player template in the order of the game's player template list (without the `Boss` side), wrapping around when there are more slots than templates.
- Colour of slot N: colour number N.
- Team: none (every player for themselves). Start positions are assigned from the seed by the game, as for a random start position in the lobby.
- Player names: `Player 1` to `Player N`.

## Window flags

Use the existing flags to put the instances side by side:

- `-win` runs in a window.
- `-xres <width>` and `-yres <height>` set the window size, for example `-xres 800 -yres 600`.
- `-quickstart` skips the intro, the shell map and the window animation, so the main menu comes up fast. The direct start waits for the main menu.

## Running a two player match

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

## What is logged

The lines start with `Servers direct:` and tell which step ran: the entry check result, `CREATE` or `JOIN` sent, the tokens, and the launch (map, seed, local slot). A failure shows a message box and the reason in the log.

## Notes

- The match runs through the relay only. The online lobby, NAT negotiation and the engine's player-to-player connections are not used.
- This path is a test aid. Ending the match returns to the main menu, and the score screen is not the one of a Servers match (package 2.2).
