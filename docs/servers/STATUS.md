# Status

Updated when packages merge into `servers-go`. The live state of a package is in its own file under [packages/](packages/).

The status of the earlier work on the plain community source is in [archive/STATUS-vanilla.md](archive/STATUS-vanilla.md).

## Phases

| Phase | Goal | Phase check |
|---|---|---|
| 0. Foundations | Plan and contracts on the new base, build baseline, relay carried over | Documents complete. The client builds with the switch on and off. The relay builds and its tests pass. |
| 1. Core pieces | Lobby service, client on its own service, relay match netcode, Servers menus, in parallel tracks | Four checks, listed under Phase 1 |
| 2. Integration | A server lobby starts a relay match | Two players go from the Servers button to a finished match |
| 3. Server clock | Relay sets the frame timing and drops slow players | Three-player match with one instance under injected latency: the other two do not freeze, and no desync is reported |
| 4. Deployment | EU relay, lobby service, `coturn`, TLS | A match on the hosted programs. A flagged address and an over-limit ping are both refused. |

## File ownership

| Track | Owns |
|---|---|
| Main (phases 0 and 2) | `docs/servers/`, `cmake/`, `CMakePresets.json`, `Servers/Client/` |
| R. Relay | `Servers/Relay/` |
| L. Lobby service | `Servers/Lobby/` |
| B. Client base | The address, reporting and login-flow code listed in package B1 |
| C. Client netcode | `Core/GameEngine/Source/GameNetwork/` and `Core/GameEngine/Include/GameNetwork/`, top level only; `ServersDirect.*`; `NGMPGame.*`; the launch-flag and per-frame call sites listed in C3 |
| D. Client menus | `GeneralsMD/.../GUICallbacks/Menus/`, `Core/.../GameSpy/LobbyUtils.cpp`, `ServersMode.*`, and the create and join code in `OnlineServices_LobbyInterface.*` |
| 2.1 | The files in its own list, after tracks C and D are merged |

Order that keeps two packages out of the same file: B1 before D1 (menu files), B1 and D1 before C3 (`GameEngine.cpp`, the engine's `CMakeLists.txt`), C3 before 2.1 (`NGMPGame.*`). `Generals/` is not edited by anyone.

## Phase 0

| Package | Title | Depends on | State |
|---|---|---|---|
| 0.1 | Plan, protocol, service specification and code map on the Generals Online base | none | done |
| [0.2](packages/0.2-build-baseline.md) | Build baseline, build switch and relay carried over | none | done |

## Phase 1

After 0.2, tracks L, B and C can start together.

| Package | Title | Depends on | State |
|---|---|---|---|
| [L1](packages/L1-service-skeleton.md) | Lobby service: program skeleton, configuration and fixed answers | 0.2 | done |
| [L2](packages/L2-service-login.md) | Lobby service: accounts, login and tokens | L1 | done |
| [L3](packages/L3-service-websocket.md) | Lobby service: WebSocket, rooms, room chat and names | L2 | done |
| [L4](packages/L4-service-lobbies.md) | Lobby service: lobbies | L3 | todo |
| [L5](packages/L5-service-p2p.md) | Lobby service: player-to-player connection support | L4 | todo |
| [L6](packages/L6-service-server-lobbies.md) | Lobby service: server lobbies | L5 | todo |
| [B1](packages/B1-own-service.md) | Client talks only to our own service | 0.2 | done |
| [C1](packages/C1-client-link.md) | Client: relay link, entry check and region file, carried over | 0.2 | done |
| [C2](packages/C2-client-records.md) | Client: match commands through the relay | C1 | todo |
| [C3](packages/C3-client-flags.md) | Client: start a relay match from launch flags | C2, B1, D1 | todo |
| [D1](packages/D1-servers-button.md) | Menus: Servers button and entry check on entry | B1, C1 | done |
| [D2](packages/D2-server-lobbies.md) | Menus: create and join server lobbies, list filter | D1 | todo |
| [D3](packages/D3-entry-check-triggers.md) | Menus: entry check on join, Accept and exit | D2 | todo |

Phase 1 check:

The checks that need the game use the Debug configuration of the `win32-vcpkg-servers` preset, a local lobby service and a local relay (TESTING.md).

| Tracks | Check | Needs the game |
|---|---|---|
| L | `lobbyprobe` runs the Online path and the server lobby path end to end against a local lobby service | no |
| R | `relay_tests` pass. The relay test client is refused from a flagged address and from an over-limit ping, and receives a pass otherwise. | no |
| R + C | Two instances finish a match through a local relay, started with the launch flags from C3 | yes |
| L + B | Two instances log in to a local lobby service, one creates an Online lobby, the other joins, and an Online match starts player to player | yes |
| L + D | From Servers, a server lobby created by one instance appears in the other's list, and does not appear from Online | yes |

## Phase 2

| Package | Title | Depends on | State |
|---|---|---|---|
| [2.1](packages/2.1-match-start.md) | Match start from a server lobby | C2, C3, D3, L6 | todo |
| 2.2 | Match end returns players to the lobby list | 2.1 | todo, package file not written |

## Phase 3

| Package | Title | Depends on | State |
|---|---|---|---|
| S1 | Relay: frame clock | Phase 2 | todo, package file not written |
| S2 | Client: follow the server clock | Phase 2 | todo, package file not written |
| S3 | Server-driven disconnects replace the vote screen in server mode | S1, S2 | todo, package file not written |

## Phase 4

| Package | Title | Depends on | State |
|---|---|---|---|
| 4.1 | Run instructions for relay, lobby service and `coturn`; VPN data updates; TLS | Phase 3 | todo, package file not written |
| 4.2 | EU rollout | 4.1 | todo, package file not written |

## Log

| Date | Change |
|---|---|
| 2026-11-14 | The base changed from the plain community source to the Generals Online client (commit `6ff404436`), on the new branch `servers-go`. Reason: a live test showed that the Generals Online data does not run on the plain build, and players use the Generals Online client. |
| 2026-11-14 | The project runs its own lobby service (decisions D35 to D50). The client's side of the service contract was read from the client code and recorded under `reference/`. `SERVICE.md` written. `PROTOCOL.md` section 5 rewritten for the own service; the relay link (section 4) is unchanged. |
| 2026-11-14 | The earlier packages and status moved to `archive/`. New packages written for phases 0 to 2. The relay and the client's relay pieces are carried over from the `servers` branch by packages 0.2, C1, C2 and C3. |
| 2026-11-15 | L1 done: the lobby service skeleton, configuration reader, SQLite store and the fixed answers of SERVICE 5.2 plus VersionCheck and the health check. Go sources are pinned to LF on checkout. |
| 2026-11-15 | C1 done: the relay link, entry check and region file reader carried over from `servers` and built as `core_serverlink` behind the Servers switch. Nothing calls it yet (C2 and D1 do). |
| 2026-11-15 | L2 done: accounts, login codes, sessions and refresh tokens (SERVICE 4), the login requirement on the fixed calls, the per-address creation limit, and the `lobbyprobe` tool for the login path. |
| 2026-11-15 | B1 done: the client's service addresses, STUN and TURN lists and display name are centralised in `ServiceHosts.h`; crash reporting, the Discord presence, the login browser step, the hardware identifiers, the anti-cheat probe and the update path are removed or made inert; player text uses the display name. |
| 2026-11-15 | D1 done: a Servers button on the main menu runs the entry check across every region, shows the refusal texts on a flagged or over-limit result, and on a pass carries the region tag and the 200-hex pass forward for creating and joining server lobbies. |
| 2026-11-15 | L3 done: the WebSocket endpoint with session-token upgrade, rooms and room chat, the name change rules, reconnect holds, the `MOTD` and `ServiceConfig` calls, and the message rate and size limits. |
| 2026-11-15 | Package 0.2 merged. The Generals Online client builds with the `win32-vcpkg` preset. The `RTS_BUILD_OPTION_SERVERS` switch and the `win32-vcpkg-servers` preset are added. The relay and the client region files are carried over from the earlier work; the relay builds and `relay_tests` pass. |
