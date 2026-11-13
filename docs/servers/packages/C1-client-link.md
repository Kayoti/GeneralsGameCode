# C1 Client: ENet relay link, entry check client and server mode in connection setup

State: partial
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

- [x] ENet is fetched at a pinned version, only when the Servers switch is on. This settles PROTOCOL.md V1: ENet compiles with the game's toolchain and does not clash with the engine's use of `winsock.h`. ENet headers are included only inside the new `.cpp` files.
- [x] `ServerRegions` reads the client region file and exposes the list.
- [x] `ServerEntryCheck`: start against all regions, update once per tick without blocking, answer each `PING` with `PONG` at once, then report either the best pass and its region, or the refusal code with its two values. It follows PROTOCOL.md 4.4 and 5.1, and keeps the time each pass was received so its remaining life is judged by the local clock.
- [x] `ServerLink`: connect with `CREATE` (returning the session id and join tokens) or `JOIN` (with a join token), update once per tick without blocking, and report `WELCOME`, `ROSTER`, `START`, `DROP`, `REFUSE` and connection loss as events. It can send and receive `DATA` as raw byte buffers. It never blocks the game thread.
- [x] Both classes are usable with no knowledge of ENet by their callers. Their headers are the interface that D1 and C2 build against.
- [x] `ConnectionManager` has a server mode flag, off by default, set before `parseUserList`. With the flag off, nothing changes.
- [x] All new code is behind the Servers build switch. The game builds with the switch on and off.
- [ ] Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Sending game commands (C2). Menus (D1).

## Handoff notes

- Finished:
  - `cmake/enet.cmake` (ENet v1.3.18, commit `2662c0de09e36f2a2030ccc2c528a3e4c9e8138a`, `EXCLUDE_FROM_ALL`), included by one line after `gamespy.cmake` in the top-level `CMakeLists.txt`. The file returns at once when `RTS_BUILD_OPTION_SERVERS` is off, so nothing is fetched with the switch off.
  - Static library `core_serverlink`, declared in `Core/GameEngine/CMakeLists.txt` inside `if(RTS_BUILD_OPTION_SERVERS)`. It has no precompiled header and no forced includes. It links ENet, `ws2_32` and `winmm` privately, and is linked into `corei_gameengine_public`, so both titles get it.
  - Headers that D1 and C2 build against, in `Core/GameEngine/Include/GameNetwork/`, free of ENet and engine types:
    - `ServerRegions.h`: `ServerRegion`, `ServerRegions` (`parse`, `load`, `getCount`, `getRegion`, `findByNumber`, `findByTag`, `getError`) and `ServerHostResolver`.
    - `ServerEntryCheck.h`: `ServerClockMs()`, `ServerPass` (`isValid`, `remainingMs`, `needsRenewal`), `ServerRefuseCode`, `ServerEntryCheck` (`start`, `startRegion`, `update`, `cancel`, `getState`, `getPass`, `getRegion`, `getRefuseCode`, `getRttMs`, `getLimitMs`).
    - `ServerLink.h`: `ServerDropReason`, `ServerLinkEvent`, `ServerLink` (`create`, `join`, `update`, `nextEvent`, `sendData`, `leave`, `close`, `getState`).
  - Sources in `Core/GameEngine/Source/GameNetwork/`: `ServerRegions.cpp`, `ServerEntryCheck.cpp`, `ServerLink.cpp`. ENet headers are included in the last two only.
  - Server mode flag: `ConnectionManager::setServerMode/isServerMode` (off in the constructor) and `Network::setServerMode/isServerMode` in `Network.cpp`, all behind `RTS_SERVERS`.
- Left to do:
  - Lint: `clang-tidy` is not installed here, so `scripts/run-clang-tidy.py` was not run. As a substitute the three new `.cpp` files were compiled with `/W4` and gave no warnings. Someone with clang-tidy must run the script on the new files and on `ConnectionManager.*` and `Network.cpp`.
  - The reviewer box stays unticked.
  - For C2 or 2.1: `NetworkInterface.h` is not in C1's list, so callers cannot reach the flag yet. Add `virtual void setServerMode(Bool) = 0;` (and `isServerMode`) there and add `override` to the two `Network` methods. `Network::setServerMode` must be called after `Network::init()` and before `parseUserList`. `init()` creates a new `ConnectionManager`, so the flag is off again for every new game.
  - Nothing reads the flag yet. Its use is C2's job.
- Surprises and open questions:
  - The pass is 100 bytes (PROTOCOL.md 4.3 corrected). The client sizes the PASS, CREATE and JOIN bodies against 100: `CREATE` is 105 bytes with the type byte, `JOIN` is 120, `PASS` is 101.
  - V1 settled: ENet 1.3.18 builds with the game's toolchain (Visual Studio 18 build tools, x86, Release). It is isolated in `core_serverlink`, so its `winsock2.h` never meets the engine's `windows.h`. As a separate check, the three new headers were included in `Network.cpp` for both titles with the engine's precompiled header, and compiled (the probe was then removed and is not committed). ENet itself gives compiler warnings in its own sources (C5287 enum operands, C4996 `gethostbyname`). They are third-party and were not touched. ENet's `cmake_minimum_required(2.8.12...3.20)` is accepted by CMake 4.4.
  - PROTOCOL.md gives the client no deadline for `CREATE` and `JOIN`. `ServerLink::create` and `join` take `timeoutMs` (time allowed until `WELCOME`), and the link ends with a `LOST` event when it passes. No default was invented. 6000 ms would match the 6 seconds of 5.1 if the owner wants a value.
  - Host names are looked up on a worker thread (`ServerHostResolver`, in `ServerRegions.h`) because ENet's own lookup blocks. This class is an addition to what the package listed. A dotted IPv4 address is answered at once.
  - The relay times the `PONG` from its own clock, so the measured ping includes the gap between the client's `update()` calls. The caller of `ServerEntryCheck::update` (D1) should call it every frame, also while a menu is shown.
  - The region parser is strict: an unknown key, a repeated key, a missing key, a repeated region number or a bad value rejects the whole file with a line number in `getError()`. Comments are whole lines that start with `#` after trimming. A UTF-8 byte order mark is skipped. Limits not in PROTOCOL.md: file at most 64 KB, name at most 63 characters, host at most 127 characters. Region tag and port rules follow 5.2.
  - `regions.txt` has no defined place next to the installed game yet (see the 0.2 notes). `ServerRegions::load(path)` takes the path, so the caller decides.
  - Refusal when no region answers: `ServerEntryCheck::getRefuseCode()` is 0 (`SERVER_REFUSE_NO_ANSWER`, a local value, not a relay code). D1 maps 0 and any code without its own text to "The server cannot be reached." The message texts are not in these classes.
  - `ServerLink` drops a message that has the wrong size or is not valid in the current state, as PROTOCOL.md 3 says. `WELCOME` is accepted only for the slot and session that were asked for (the host: after `CREATED`, for the session `CREATED` named). `DATA` in both directions must be whole records of 7 to 4096 bytes in a message of at most 16384 bytes; otherwise `sendData` returns false and a received message is dropped. The per-command checks of 4.6 are left to C2.
  - Events are queued in the order received and `nextEvent` hands them out one at a time. A `DATA` event's pointer is valid until the next `nextEvent`, `update` or `close` call. `REFUSE` and `LOST` end the link; `close()` gives no event.
  - Verification run: a throwaway test program outside the repository (`C:\cnc-build\c1test`, not committed) links the built `core_serverlink.lib` and plays a fake relay over ENet on 127.0.0.1. 113 checks passed and none failed. It covers: parsing `Servers/Client/regions.txt` and 17 invalid files; the entry check with pass, both refusals, two regions (lowest ping, tie to the first, first refused, both refused), a silent region (finished after 6015 ms), an unknown host, and a pass for the wrong region; `PONG` echoes each nonce; the longest `update()` took under 1 ms; `CREATE`/`JOIN` byte layouts; `CREATED`, `WELCOME`, `ROSTER`, `START`, `DROP`, `REFUSE` and `DATA` events; malformed and out-of-state messages ignored; `sendData` limits; `leave`; loss; setup timeout. Converting this into unit tests needs a test folder, which is outside this package's list.
  - Full builds with `RTS_BUILD_OPTION_SERVERS` on and off, Release, both exited 0 with no error lines and produced `generalszh.exe` and `generalsv.exe` in each tree. The off tree's compile database has no entry for the new files and none for ENet; the on tree has 27 entries for the new files.
- Review findings and fixes:
