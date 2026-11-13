# Overview

## The problem

In the existing online mode, two things spoil matches:

1. The game becomes slow and laggy for everyone.
2. The game freezes on a screen listing players with countdown timers, and players vote on who to drop.

Both come from one rule in the engine, described below.

## How multiplayer works today

**Deterministic lockstep.** Every client runs the full simulation. Only player commands travel over the network. Unit positions and health are never sent.

**Run-ahead.** A command issued now is scheduled a few logic frames in the future, so it has time to reach every player. This delay is the "run-ahead". It is recalculated during the match from the worst measured latency and the lowest frame rate among all players.

**The wait rule.** A logic frame runs only when the commands of every player for that frame have arrived (`ConnectionManager::allCommandsReady`). If one player is late, every player waits.

- Slow and laggy: the run-ahead and the logic frame rate are set by the worst player (`ConnectionManager::updateRunAhead`).
- Freeze and vote: one player's data stops arriving, so no one can advance. `DisconnectManager` shows the disconnect screen, runs timeouts and counts votes.

**Topology.** Each client opens a UDP connection to every other player. Game commands are sent to one player called the packet router, who forwards them to the rest. The packet router is the first slot at the start, with a fallback order if that player leaves. A fixed set of command types skips the router and goes directly between players (`CommandRequiresDirectSend`).

**Transport.** One UDP socket per client. The engine has its own reliability: acknowledgements, resends, ordering, and splitting of large commands. Packets carry a CRC and a magic number and are obfuscated with XOR. There is no authentication.

**Desync detection.** Clients periodically exchange a CRC of the game state and flag a mismatch.

**Lobby.** The Online mode uses the GameSpy SDK for login, chat, rooms and statistics. Before a match, `NAT` and `FirewallHelper` negotiate a UDP path between every pair of players. The Network mode uses UDP broadcast on the local network (`LANAPI`).

**Code location.** Networking is in `Core/GameEngine/.../GameNetwork` and is shared by both titles. Menu code is per title, under `Generals/` and `GeneralsMD/`.

## What Servers mode builds

| Part | What it is |
|---|---|
| Relay server | A standalone program, and the only new one. It takes the packet router role permanently. Every client connects only to it during a match. It runs no game logic. It also checks each player for a VPN and for ping before letting them in. |
| Server clock | The relay decides when each logic frame closes and never waits for a slow player. A late command moves to the next frame. A player who falls behind catches up alone or is dropped by the server. |
| Client netcode | A server mode in the existing networking code: one connection to the relay, no NAT negotiation, all commands through the relay, disconnects decided by the server. |
| Client lobby changes | A Servers button in the Multiplayer menu that opens the existing online lobby. Server lobbies carry a tag, and the lobby list is filtered by it. |

## How a Servers match goes

1. The player clicks Servers. The client runs the entry check: the relay checks for a VPN, measures ping, and gives a pass.
2. The player reaches the existing online lobby and sees only lobbies tagged as server games for their region.
3. Creating, joining, chatting and pressing Accept work as they do in Online.
4. The host presses Start. Where an Online match would negotiate player to player, every client connects to the relay instead.
5. The match runs through the relay.

## Where things live

| Thing | Where |
|---|---|
| Lobby list and lobby chat | The third-party online lobby service, as today |
| A lobby's settings (map, slots, who accepted) | The host player's game, as today |
| The match | The relay |

## What does not change

- The simulation and the lockstep model.
- The lobby, lobby list, chat and staging room flow.
- The Online and Network (LAN) modes.
- Skirmish, campaign and replays.

## What this fixes

| Cause | After Servers mode |
|---|---|
| Players cannot reach each other through their routers | Fixed. Clients only make an outbound connection to the server. |
| The packet router player has a weak connection or leaves | Fixed. The server is the router. |
| One player has a bad connection | From Phase 3: only that player is affected. Before Phase 3: the others still slow down, but no vote is needed and the server removes a stuck player. |
| One player has a slow PC | From Phase 3: only that player falls behind. |
| The match needs players to reach each other directly | Fixed. During a match clients only talk to the server. The existing lobby still shares player addresses in the staging room. |

## What it does not do

- It does not prevent map hacks. Every client still holds the full game state.
- It does not add reconnect or mid-match spectators.
- It does not make the lobby independent. The lobby still depends on the third-party service.

## Glossary

| Term | Meaning |
|---|---|
| Logic frame | One step of the simulation |
| Run-ahead | Number of logic frames between issuing a command and executing it |
| Packet router | The participant that forwards game commands to all players |
| Slot | A player position in a match, 0 to 7 |
| Desync | Two clients whose simulations no longer match |
| Gate | The short check at the end of a phase |
| Entry check | The relay's VPN and ping check that gives a player a pass |
| Pass | Signed proof from the relay that a player passed the entry check |
| Join token | One-use secret the host gives each player privately to enter a match |
