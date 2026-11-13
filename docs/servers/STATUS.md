# Status

Updated when packages merge into `servers`. The live state of a package is in its own file under [packages/](packages/).

## Phases

| Phase | Goal | Gate |
|---|---|---|
| 0. Foundations | Protocol, build switch, region config, package files | Documents complete. Game builds with the switch on and off, and Online and Network behave as before. |
| 1. Core pieces | Relay, client netcode, client lobby changes, in three parallel tracks | Three checks, listed under Phase 1 |
| 2. Integration | Staging room start leads into a relay match | Two players go from the Servers button to a finished match |
| 3. Server clock | Relay sets the frame timing and drops slow players | Three-player match with one instance under injected latency: the other two do not freeze, and no desync is reported |
| 4. Deployment | EU relay and configuration | Match on the hosted relay. A flagged address and an over-limit ping are both refused. |

## File ownership

| Track | Owns |
|---|---|
| Main (phases 0 and 2) | `docs/servers/`, `cmake/`, `Servers/Client/` (client configuration files, such as the region file; first written in 0.3) |
| A. Relay server | `Servers/Relay/` (created in 0.2 with a placeholder build file that the game build does not use) |
| C. Client netcode | `Core/GameEngine/Source/GameNetwork/` and `Core/GameEngine/Include/GameNetwork/`, top level only, except `LANAPI*`, `NAT.*` and `FirewallHelper.*` |
| D. Client lobby | `GameNetwork/GameSpy/` in `Core/`; `*/GameClient/GUI/` and `GameNetwork/GUIUtil.cpp` in `GeneralsMD/` and `Generals/` |

`LANAPI*`, `NAT.*` and `FirewallHelper.*` are not edited by anyone.

Track D edits existing online lobby files. Every change there is behind the build switch and must leave Online behaviour identical.

## Phase 0

| Package | Title | Depends on | State |
|---|---|---|---|
| [0.1](packages/0.1-protocol.md) | Protocol specification | none | done |
| [0.2](packages/0.2-build-switch.md) | Build switch and folder layout | none | done |
| [0.3](packages/0.3-region-config.md) | Region configuration format | 0.1 | done |
| 0.4 | Write the package files for Phase 1 | 0.1 | done |

## Phase 1

D1 needs only the two headers that C1 defines, so tracks C and D can start together once C1 has published them.

| Package | Title | Depends on | State |
|---|---|---|---|
| [A1](packages/A1-relay-host.md) | Relay: ENet host, configuration, pass signing and checking | 0.1, 0.2, 0.3 | done |
| [A2](packages/A2-relay-gate.md) | Relay: entry check with ping measurement and ping limit | A1 | todo |
| [A3](packages/A3-relay-vpn.md) | Relay: VPN check on the entry check | A2 | todo |
| [A4](packages/A4-relay-sessions.md) | Relay: sessions (create, join, roster, start) | A1 | todo |
| [A5](packages/A5-relay-records.md) | Relay: record validation and forwarding | A4 | todo |
| [A6](packages/A6-relay-drop.md) | Relay: timeouts and dropping players | A5 | todo |
| [C1](packages/C1-client-link.md) | Client: ENet relay link, entry check client and server mode in connection setup | 0.1, 0.2, 0.3 | todo |
| [C2](packages/C2-client-records.md) | Client: commands sent and received as records, player drop from the server | C1 | todo |
| [C3](packages/C3-client-flags.md) | Client: launch flags to create or join a relay match directly | C1 | todo |
| [D1](packages/D1-servers-button.md) | Lobby: Servers button, entry check on entry, refusal messages | 0.2, 0.3, C1 | todo |
| [D2](packages/D2-tag-filter.md) | Lobby: tag on created lobbies, list filter in both modes | D1 | todo |
| [D3](packages/D3-gate-triggers.md) | Lobby: entry check on join, Accept and exit; ladder and Quick Match hidden | D1 | todo |

Phase 1 gate:

| Tracks | Check |
|---|---|
| A + C | Two instances finish a match through a relay on the same PC, started with the launch flags from C3 |
| A | The relay test client is refused from a flagged address and from an over-limit ping, and receives a pass otherwise |
| D | From Servers, a lobby created by one instance appears in the other's list, and does not appear from Online |

## Phase 2

| Package | Title | Depends on | State |
|---|---|---|---|
| [2.1](packages/2.1-match-start.md) | Staging room start creates the relay session and hands each player its join token | A, C, D | todo |
| 2.2 | Match end returns players to the lobby | 2.1 | todo |

## Phase 3

| Package | Title | Depends on | State |
|---|---|---|---|
| S1 | Relay: frame clock | Phase 2 | todo |
| S2 | Client: follow the server clock | Phase 2 | todo |
| S3 | Server-driven disconnects replace the vote screen in server mode | S1, S2 | todo |

## Phase 4

| Package | Title | Depends on | State |
|---|---|---|---|
| 4.1 | Relay build and run instructions, VPN data updates | Phase 3 | todo |
| 4.2 | EU relay rollout | 4.1 | todo |

## Log

| Date | Change |
|---|---|
| 2026-11-12 | Documentation set created. No code changes yet. |
| 2026-11-12 | `servers` is based on upstream commit `8aaa4f731`. Every function named in CODEMAP.md was checked against that commit. |
| 2026-11-12 | Launch region set to EU. Player-facing text set to English. Ping limit added. |
| 2026-11-12 | Scope reduced: the existing online lobby is kept, the separate lobby server is dropped, and the relay is the only new program. PROTOCOL.md rewritten to match. |
| 2026-11-12 | Technical choices D30 to D33 confirmed by the project owner. Phase 1 package files written. |
| 2026-11-12 | PROTOCOL.md reviewed against the engine code by a second reviewer. 17 findings, all addressed in the document. Details are in the handoff notes of package 0.1. |
| 2026-11-13 | Build switch added (0.2): `RTS_BUILD_OPTION_SERVERS`, off by default, gates the `RTS_SERVERS` define. `Servers/Relay/` and `Servers/Client/` folders registered. Game configures and builds with the switch on and off. |
| 2026-11-13 | Region example files added (0.3): `Servers/Client/regions.txt` and `Servers/Relay/relay.conf`, matching the PROTOCOL section 7 formats, EU region with a commented second region. Phase 0 documents complete. |
| 2026-11-13 | A1 and C1 blocked: PROTOCOL.md section 4.3 is inconsistent on the pass size. The field table totals 100 bytes, but the text says 96 and section 4.2 repeats "96-byte pass". The pass byte layout must be fixed on `servers` before the relay pass module (A1) and the client entry check and link (C1) can be written. Details in each package's handoff notes. |
| 2026-11-13 | Pass size resolved to 100 bytes (section 4.3), matching the field table. The three "96-byte pass" references in 4.2 and the "Total 96 bytes" line are corrected. CREATE body is 104 bytes, JOIN is 119. A1 and C1 reopened. |
| 2026-11-13 | Relay host merged (A1): standalone ENet relay in `Servers/Relay/` that reads its config, signs and checks the 100-byte pass, generates region keys, and disconnects connections with no valid first message. ENet and libsodium pinned. Incoming packets bounded to the message limit and to channel 0. Builds and unit tests pass. |
