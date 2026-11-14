# Decisions

Settled by the project owner unless marked "proposed". Do not reopen a settled decision inside a work package. Raise it in the handoff notes instead.

Decisions D1 to D34 were made when the project was based on the plain community source. On 2026-11-14 the base changed to the Generals Online client. Decisions that this changed are marked "replaced" and point to the new one.

## Base and scope

| # | Decision |
|---|---|
| D35 | The game client is a fork of the Generals Online client (`GeneralsOnlineDevelopmentTeam/GameClient`, commit `6ff404436`). All work is on the `servers-go` branch. The earlier `servers` branch is kept only as a source to port from. |
| D36 | The project runs its own online service. The client never contacts a host of the Generals Online team: not for the API, WebSocket, STUN, TURN, crash reports, login page, updates or links. |
| D37 | Zero Hour only. The `Generals/` title gets no online work. |
| D38 | Three programs are ours: the game client, the lobby service and the relay. `coturn` (free, third party) is run unchanged for the player-to-player fallback. |
| D39 | The lobby service is written in Go with SQLite. It implements the part of the Generals Online client contract listed in [SERVICE.md](SERVICE.md), so the client's existing lobby code keeps working. |
| D40 | The client offers two ways to play online, in one lobby service: **Online** (player to player, as Generals Online does it) and **Servers** (the match runs through the relay). |

## Architecture

| # | Decision |
|---|---|
| D1 | Keep the lockstep simulation. In Servers mode a dedicated relay replaces the player packet router. |
| D2 | The relay controls the frame clock and never waits for a slow player. Built in its own phase, after a plain relay match works. |
| D3 | No server-side simulation in this scope. |
| D4 | No reconnect after a drop. No spectators joining mid-match. |
| D5 | Free and open-source libraries only. Nothing paid. |
| D6 | Replaced by D40 and D41. |
| D7 | Replaced by D38 and D39. |
| D8 | In Phases 1 and 2 the lowest connected slot calculates run-ahead. The relay names that slot. Phase 3 moves this to the relay. |
| D41 | The lobby screens, lobby list, chat and staging room keep the Generals Online flow and code. Servers mode changes only how players are admitted and how the match is carried. |

## Product

| # | Decision |
|---|---|
| D9 | Servers mode is a new button in the Multiplayer menu. It opens the same lobby screens as Online. Online and Network (LAN) keep working. |
| D10 | All Servers code in the client sits behind the build switch `RTS_BUILD_OPTION_SERVERS` (define `RTS_SERVERS`), so a build without it has no Servers button and no relay code. One rule is compiled into every build: server lobbies are hidden from the Online list. |
| D11 | From Servers, the lobby list shows only server lobbies of the player's region. From Online, server lobbies are hidden. |
| D12 | A player creates server lobbies only in the assigned region. |
| D13 | Region is the one with the lowest server-measured ping. The player cannot choose. A player on a border gets the lowest ping, with no special case. |
| D14 | The region list is a configuration file, so regions can be added later without code changes. |
| D15 | The launch region is EU only. |
| D16 | All Servers mode text shown to players is in English. |
| D42 | A server lobby is marked with `LobbyType` 2 and its `Region`, both stored by the lobby service. This replaces D33. |
| D43 | The lobby service refuses to create or join a server lobby without a valid pass for that lobby's region. The region lock is enforced by the service, not only by the client's list filter. |

## Accounts and privacy

| # | Decision |
|---|---|
| D44 | No passwords and no web page at launch. The first login creates an account automatically; the client keeps a refresh token on the PC and uses it for later logins. Losing that token means a new account. |
| D45 | A new account gets the name `Player` plus a number. The player changes it with the existing `/name` chat command. |
| D46 | The client does not send hardware identifiers, the list of loaded modules or screenshots to the service. The service never asks for them. |
| D47 | No anti-cheat plugin at launch. The plugin loader stays in the code but nothing is shipped or loaded. |
| D48 | The client's self-update is disabled. The service always answers "no update". New builds are distributed by hand. |
| D49 | Friends, statistics, Quick Match and moderation are not built at launch. The service answers those calls with the empty responses listed in SERVICE.md, and the Quick Match entry is hidden. |

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
| D26 | No separate testing phase. Each phase ends with one short check. Testing is kept to what catches bugs in a short match. |
| D27 | Phase checks run on one PC, using several game instances side by side. |
| D28 | Every package is reviewed by someone other than its author before it merges. |
| D29 | `origin` is the project's fork. `go` is the Generals Online client. `upstream` is the plain community source. Nothing is ever pushed to `go` or `upstream`. |

## Technical choices

| # | Decision | Reason |
|---|---|---|
| D30 | In a Servers match, ENet carries the match and the engine's own packet, acknowledgement and resend layer is not used. Each game command keeps the engine's existing encoding. | Engine commands carry no length on the wire, so a relay cannot split engine packets without the engine's parser. Length-prefixed records keep the relay free of game code. Proven in a live two-instance match on the earlier base. |
| D31 | The relay is written in C++ with ENet, libsodium and libmaxminddb. It is carried over unchanged from the earlier base. | Built, tested and proven. |
| D32 | Players cannot send maps to each other during a server match. Everyone must already have the map. | Removes a known security risk and the largest message type. |
| D33 | Replaced by D42. | |
| D34 | Each player enters a match with a one-use join token. The host receives the tokens from the relay and hands them to the lobby service, which gives each player only its own token in the start message. | Stops one lobby member taking another member's slot. The lobby service replaces the private lobby message used on the earlier base. |
| D50 | In a Servers lobby the client opens no player-to-player connections. The service sends no connection instructions and no TURN credentials for it. | The match never uses them, and it keeps player addresses private. |

## Later, not in this scope

| # | Idea | Why later |
|---|---|---|
| L2 | Encrypt match traffic | Not needed to fix lag and disconnects. |
| L3 | Headless referee for verified results | Same. |
| L4 | Accounts with e-mail or password, account recovery | D44 is enough to launch. |
| L5 | Friends, statistics, Quick Match, moderation tools | D49. |
| L6 | Anti-cheat | D47. |
| L7 | Offer the relay mode to the Generals Online team for their client | Optional. Our service does not depend on it. |

## Open

None. O1 (permission from a third-party lobby operator) no longer applies, because the lobby service is ours.
