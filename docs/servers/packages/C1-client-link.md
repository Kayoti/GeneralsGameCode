# C1 Client: ENet relay link, entry check client and server mode in connection setup

State: blocked
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

- Finished: nothing yet. Work did not start because the pass layout is not settled (see below).
- Left to do: the whole package, once the pass size is resolved.
- Surprises and open questions:
  - Blocking: PROTOCOL.md section 4.3 is internally inconsistent about the pass size. The field table totals 100 bytes, but the section says "Total 96 bytes" and section 4.2 describes PASS, CREATE and JOIN as carrying a "96-byte pass". The client must read `issued_at`/`expires_at` from the pass and size the CREATE, JOIN and PASS bodies exactly, so the size cannot be guessed. Resolve on the `servers` branch (see A1's note for the byte breakdown and the likely cause). The same decision unblocks A1.
  - Design proposed for when work resumes: a small static library `core_serverlink` with no precompiled header, so ENet's `winsock2.h` does not clash with the engine's `windows.h`/`winsock.h`; ENet headers included only inside the new `.cpp` files. The server-mode flag on `ConnectionManager` is reachable from callers only through a setter on `NetworkInterface.h`; that file is not in C1's edit list, so C2 or 2.1 adds the setter.
- Review findings and fixes:
