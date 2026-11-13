# C1 Client: ENet relay link, entry check client and server mode in connection setup

State: todo
Branch: servers-C1

## Goal

Give the game client a connection to the relay: one class that runs the entry check and one that holds a match connection. Add a server mode flag to the networking setup. No game commands flow yet.

## Depends on

0.1, 0.2, 0.3

## Read first

- PROTOCOL.md sections 3, 4.1 to 4.5, 4.9, 5.1 and 7.1.
- CODEMAP "Match networking": `Network.cpp`, `ConnectionManager.cpp` (`parseUserList`, `init`, `reset`), `Transport.cpp`, `udp.h`.
- `cmake/gamespy.cmake`, as the model for fetching a dependency.

## May edit

- New: `Core/GameEngine/Include/GameNetwork/ServerLink.h`, `Core/GameEngine/Source/GameNetwork/ServerLink.cpp`
- New: `Core/GameEngine/Include/GameNetwork/ServerEntryCheck.h`, `Core/GameEngine/Source/GameNetwork/ServerEntryCheck.cpp`
- New: `Core/GameEngine/Include/GameNetwork/ServerRegions.h`, `Core/GameEngine/Source/GameNetwork/ServerRegions.cpp`
- New: `cmake/enet.cmake`, and the one line that includes it
- The `CMakeLists.txt` that lists GameNetwork sources, to add the new files
- `ConnectionManager.h/.cpp` and `Network.cpp`, only to add and store the server mode flag

## Must not edit

`Transport.*`, `udp.*`, `Connection.*`, `NAT.*`, `FirewallHelper.*`, `LANAPI*`, `GameSpy/`.

## Done when

- [ ] ENet is fetched at a pinned version, only when the Servers switch is on. This settles PROTOCOL.md V1: ENet compiles with the game's toolchain and does not clash with the engine's use of `winsock.h`. ENet headers are included only inside the new `.cpp` files.
- [ ] `ServerRegions` reads the client region file and exposes the list.
- [ ] `ServerEntryCheck`: start against all regions, update once per tick without blocking, answer each `PING` with `PONG` at once, then report either the best pass and its region, or the refusal code with its two values. It follows PROTOCOL.md 4.4 and 5.1, and keeps the time each pass was received so its remaining life is judged by the local clock.
- [ ] `ServerLink`: connect with `CREATE` (returning the session id and join tokens) or `JOIN` (with a join token), update once per tick without blocking, and report `WELCOME`, `ROSTER`, `START`, `DROP`, `REFUSE` and connection loss as events. It can send and receive `DATA` as raw byte buffers. It never blocks the game thread.
- [ ] Both classes are usable with no knowledge of ENet by their callers. Their headers are the interface that D1 and C2 build against.
- [ ] `ConnectionManager` has a server mode flag, off by default, set before `parseUserList`. With the flag off, nothing changes.
- [ ] All new code is behind the Servers build switch. The game builds with the switch on and off.
- [ ] Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Sending game commands (C2). Menus (D1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
