# Overview

## The problem

In online matches two things spoil the game:

1. The game becomes slow and laggy for everyone.
2. The game freezes on a screen listing players with countdown timers, and players vote on who to drop.

Both come from one rule in the engine, described below. The Generals Online client improved how players connect, but kept that rule.

## What the project is

A fork of the Generals Online client (Zero Hour) with its own online service and a new way to carry a match.

| Program | What it is |
|---|---|
| Game client | The Generals Online client, pointed at our service, with a Servers mode added |
| Lobby service | Our own replacement for the Generals Online service: login, lobby list, chat, staging room. New program, written in Go. |
| Relay | Carries Servers matches and runs the VPN and ping check. Already built and tested on the earlier base. |
| `coturn` | Free third-party program, run unchanged, that relays player-to-player traffic when a direct link fails in Online mode |

## How multiplayer works in the client today

**Deterministic lockstep.** Every client runs the full simulation. Only player commands travel over the network.

**Run-ahead.** A command issued now is scheduled a few logic frames in the future, so it has time to reach every player. It is recalculated during the match from the worst latency and frame rate among all players (`ConnectionManager::updateRunAhead`).

**The wait rule.** A logic frame runs only when the commands of every player for that frame have arrived (`ConnectionManager::allCommandsReady`). If one player is late, every player waits.

- Slow and laggy: the run-ahead and the logic frame rate are set by the worst player.
- Freeze and vote: one player's data stops arriving, so no one can advance. `DisconnectManager` shows the disconnect screen, runs timeouts and counts votes. This code is unchanged in Generals Online.

**Topology.** Each client connects to every other player. Game commands go to one player called the packet router, who forwards them. This is unchanged in Generals Online.

**Connections.** Generals Online replaced the engine's UDP socket with Valve's GameNetworkingSockets (`NetworkMesh`, `NextGenTransport`): a direct link when possible, a TURN relay otherwise. The engine's own acknowledgement and resend layer still runs on top. Logic runs at 60 frames per second.

**Lobby.** The old GameSpy code is compiled but never reached. The `WOL*` menu files now call the `NGMP_*` interfaces, which talk to a web service over HTTP and a WebSocket. Lobbies live in that service. A match starts in `NGMPGame::launchGame`.

## What the project builds

| Part | What it is |
|---|---|
| Own service | The client talks only to our lobby service. Every address of the Generals Online team is removed from the client. |
| Online mode | Player-to-player matches as in Generals Online, through our service. No new game code. |
| Servers mode | A new button. Server lobbies of the player's region only. The match runs through the relay, with no player-to-player links. |
| Entry check | Before Servers mode and on lobby actions, the relay checks the player for a VPN and measures ping, and gives a pass. |
| Server clock | Phase 3. The relay decides when each logic frame closes and never waits for a slow player. |

## How a Servers match goes

1. The player clicks Servers. The client runs the entry check: the relay checks for a VPN, measures ping, and gives a pass.
2. The client logs in to the lobby service and shows the lobby list, filtered to server lobbies of the player's region.
3. Creating and joining a server lobby send the pass; the service refuses without a valid one. Chat, slots and Accept work as in Online.
4. The host presses Start. The host's client opens a session on the relay and gives the join tokens to the lobby service.
5. The service sends the start message, with each player's own token. Every client connects to the relay.
6. The match runs through the relay.

## Where things live

| Thing | Where |
|---|---|
| Accounts, lobby list, lobby settings, chat | Lobby service |
| Online match | Between the players, with `coturn` as fallback |
| Servers match | Relay |

## What does not change

- The simulation and the lockstep model.
- The lobby screens and staging room flow.
- Network (LAN) mode, Skirmish, campaign and replays.

## What Servers mode fixes

| Cause | After Servers mode |
|---|---|
| Players cannot reach each other through their routers | Fixed. Clients only make an outbound connection to the relay. |
| The packet router player has a weak connection or leaves | Fixed. The relay is the router. |
| One player has a bad connection | From Phase 3: only that player is affected. Before Phase 3: the others still slow down, but no vote is needed and the relay removes a stuck player. |
| One player has a slow PC | From Phase 3: only that player falls behind. |
| Players learn each other's addresses | Fixed in server lobbies. No player-to-player links are opened. |

## What it does not do

- It does not prevent map hacks. Every client still holds the full game state.
- It does not add reconnect or mid-match spectators.
- It has no anti-cheat, friends, statistics or Quick Match at launch.
- It does not let players of the official Generals Online client join. The two services are separate.

## Glossary

| Term | Meaning |
|---|---|
| Logic frame | One step of the simulation |
| Run-ahead | Number of logic frames between issuing a command and executing it |
| Packet router | The participant that forwards game commands to all players |
| Slot | A player position in a match, 0 to 7 |
| Desync | Two clients whose simulations no longer match |
| Phase check | The short check at the end of a phase |
| Entry check | The relay's VPN and ping check that gives a player a pass |
| Pass | Signed proof from the relay that a player passed the entry check |
| Join token | One-use secret that lets one player take one slot of one match |
| Online lobby | A lobby whose match runs player to player (`LobbyType` 0) |
| Server lobby | A lobby whose match runs through the relay (`LobbyType` 2) |
| Mesh | The set of player-to-player connections of a lobby (`NetworkMesh`) |
