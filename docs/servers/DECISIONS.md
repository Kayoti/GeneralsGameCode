# Decisions

Settled by the project owner unless marked "proposed". Do not reopen a settled decision inside a work package. Raise it in the handoff notes instead.

## Architecture

| # | Decision |
|---|---|
| D1 | Keep the lockstep simulation. Add a dedicated relay server that replaces the player packet router. |
| D2 | The relay server controls the frame clock and never waits for a slow player. This is built in its own phase, after a plain relay match works. |
| D3 | No server-side simulation in this scope. A headless referee for verified results is a possible later addition. |
| D4 | No reconnect after a drop. No spectators joining mid-match. |
| D5 | Free and open-source libraries only. Nothing paid. |
| D6 | The Servers layer only decides how players connect and carries the match. The lobby, lobby list, chat and staging room keep their existing flow and code. |
| D7 | The relay is the only new program. There is no separate lobby server, no database and no new account system. |
| D8 | In Phases 1 and 2 the lowest connected slot calculates run-ahead, as the packet router does today. The relay names that slot. Phase 3 moves this to the relay. |

## Product

| # | Decision |
|---|---|
| D9 | Servers mode is a new button in the Multiplayer menu. It opens the existing online lobby screens. Online and Network keep working as they do today. |
| D10 | All Servers code sits behind a build switch, so a build without it behaves exactly like upstream. |
| D11 | From Servers, the lobby list shows only lobbies tagged as server games for the player's region. From Online, tagged lobbies are hidden. |
| D12 | A player creates lobbies only in the assigned region. |
| D13 | Region is the one with the lowest server-measured ping. The player cannot choose. A player on a border gets the lowest ping, with no special case. |
| D14 | The region list is a configuration file, so regions can be added later without code changes. |
| D15 | The launch region is EU only. |
| D16 | All Servers mode text shown to players is in English. |

## Ping limit

| # | Decision |
|---|---|
| D17 | A player whose ping to the region is above the limit is refused with a message and returns to the Multiplayer menu. |
| D18 | The limit is a value in the relay's region configuration. It starts at 150 ms. |
| D19 | The ping is measured by the server, not reported by the client. |

## VPN check

| # | Decision |
|---|---|
| D20 | The check is done by the relay. The game client holds no VPN data. |
| D21 | It applies only inside Servers mode. It never runs for single player, Skirmish, Online or Network. |
| D22 | It runs on player actions: clicking Servers, attempting to join a lobby, pressing Accept in a lobby, and exiting a lobby. The client also repeats it silently before a match if its pass is about to expire. |
| D23 | A flagged player sees "VPN detected. Disable your VPN to play on Servers." and returns to the Multiplayer menu. |
| D24 | Undetected VPN types and occasional false positives are accepted. |

## Process

| # | Decision |
|---|---|
| D25 | Work is split into small packages. Packages that touch different folders run in parallel. |
| D26 | No separate testing phase. Each phase ends with one short gate. Testing is kept to what catches bugs in a short match. |
| D27 | Gates run on one PC, using several game instances side by side. |
| D28 | Every package is reviewed by someone other than its author before it merges. |
| D29 | This is a fork. `origin` is the fork, `upstream` is the original project, and `main` tracks upstream. All work is on `servers`. |

## Technical choices

Confirmed by the project owner on 2026-11-12.

| # | Decision | Reason |
|---|---|---|
| D30 | In server mode, ENet replaces the engine's own packet, acknowledgement and resend layer. Each game command keeps the engine's existing encoding. | Engine commands carry no length on the wire, so a relay cannot split engine packets without the engine's parser. Length-prefixed records keep the relay small and free of game code. |
| D31 | The relay is written in C++ with ENet, libsodium and libmaxminddb. | All free. One program, one language. |
| D32 | Players cannot send maps to each other during a server match. Everyone must already have the map. | Removes a known security risk and the largest message type. |
| D34 | Each player enters a match with a one-use join token that the host receives from the relay and passes on privately. | Stops one lobby member taking another member's slot. |
| D33 | A server lobby is tagged through the lobby's existing ladder address fields. | The game already reads them for every lobby, and existing code already stops unknown builds joining. |

## Later, not in this scope

| # | Idea | Why later |
|---|---|---|
| L1 | Host the lobby on the project's own server | No effect on match speed. Gives control and independence from the third-party lobby service. |
| L2 | Encrypt match traffic | Not needed to fix lag and disconnects. |
| L3 | Headless referee for verified results | Same. |

## Open

| # | Question | Needed by |
|---|---|---|
| O1 | Ask the lobby service operator whether tagged lobbies and an own relay are acceptable (PROTOCOL.md V6) | Before public release |
