# C1 Client: relay link, entry check and region file, carried over

State: done
Branch: servers-go-C1

## Goal

Bring the three finished, engine-independent client pieces over from the `servers` branch and make them build inside this client.

## Depends on

0.2

## Read first

- PROTOCOL.md sections 4.1 to 4.5 and 7.1, and V1 in section 10
- reference/CLIENT_PORTING.md section 5, rows `CMakeLists.txt`, `cmake/enet.cmake`, `Core/GameEngine/CMakeLists.txt`, `ServerLink`, `ServerEntryCheck`, `ServerRegions`; and section 4, last bullet (where to add ENet)
- archive/C1-client-link.md, "Handoff notes" (how the pieces were built and why ENet sits in its own library)
- `git show servers:cmake/enet.cmake`, and the `core_serverlink` part of `git show servers:Core/GameEngine/CMakeLists.txt`

## May edit

- `cmake/enet.cmake` (new, from `servers`)
- `CMakeLists.txt` in the repository root (one `include` line)
- `Core/GameEngine/CMakeLists.txt` (the `core_serverlink` library and its link line only)
- `Core/GameEngine/Include/GameNetwork/ServerLink.h`, `ServerEntryCheck.h`, `ServerRegions.h` (new, from `servers`)
- `Core/GameEngine/Source/GameNetwork/ServerLink.cpp`, `ServerEntryCheck.cpp`, `ServerRegions.cpp` (new, from `servers`)

## Must not edit

- `ConnectionManager.*`, `Network.cpp` (C2)
- Anything under `GameNetwork/GeneralsOnline/`
- `vcpkg.json`

## Done when

- [x] The six source files and `cmake/enet.cmake` are taken from `servers` unchanged, in one commit.
- [x] The build file changes are re-applied by hand so that, with the switch on, ENet is fetched at the pinned version and `core_serverlink` builds and links into the Zero Hour engine library. With the switch off none of it is built or fetched.
- [x] The headers stay free of ENet and engine types, as on `servers`.
- [x] The client builds with the switch on and with it off. V1 is re-confirmed on this base, or the exact clash is recorded and the package stops as `partial`.
- [x] No source change was needed to the six files. If one was, it is a separate commit and explained in the handoff notes.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Using the link in a match (C2) or from a menu (D1).

## Handoff notes

- Finished:
  - Commit `96ec88407`: `cmake/enet.cmake` and the six sources (`ServerLink`, `ServerEntryCheck`, `ServerRegions`; `.h` in `Core/GameEngine/Include/GameNetwork/`, `.cpp` in `Core/GameEngine/Source/GameNetwork/`) taken from `servers` with `git checkout servers -- <path>`. `git diff servers -- <path>` is empty for all seven.
  - Commit `f22eafcb3`: `include(cmake/enet.cmake)` after `gamespy.cmake` in the root `CMakeLists.txt`, and the `core_serverlink` static library in `Core/GameEngine/CMakeLists.txt`, inside `if(RTS_BUILD_OPTION_SERVERS)`, linked into `corei_gameengine_public`. Same block as on `servers`: no precompiled header, ENet, `ws2_32` and `winmm` linked privately. `enet.cmake` returns at once with the switch off.
  - The headers include only each other. No ENet or engine includes.
- Evidence (x86 `cmd` session, target `z_generals`):
  - Switch on: configure, Release build and Debug build all exit 0. ENet is fetched (`build/win32-vcpkg-servers/_deps/enet-build/{Release,Debug}/enet.lib`). `core_serverlink.lib` is built in both configurations. The Release link line of `GeneralsOnlineZH.exe` holds `core_serverlink.lib`, `enet.lib`, `ws2_32.lib` and `winmm.lib`.
  - Switch off: configure and Release build exit 0. `build/win32-vcpkg/_deps` has no `enet` folder and `build-Release.ninja` has no `serverlink` or `enet` entry.
  - V1 re-confirmed on this base (which also links GameNetworkingSockets and curl): ENet 1.3.18 builds and links with Visual Studio 18 build tools, x86, Debug and Release, and the engine builds with it.
- Left to do:
  - Lint: `scripts/run-clang-tidy.py` not run. The sources are unchanged from `servers`, where they compiled with `/W4` without warnings. The build log has no warning naming the three files.
  - The reviewer box stays unticked.
  - Nothing calls the link yet, so the linker drops it: the switch-on and switch-off Release executables have the same size (14425088 bytes). That is expected until C2 and D1 use it.
- Surprises and open questions:
  - The archive notes also list the `ConnectionManager` and `Network` server-mode flag. On this branch that is C2's, so it was not carried here.
  - The rest of the archive notes still apply to C2 and D1: the pass is 100 bytes; PROTOCOL.md gives no deadline for CREATE and JOIN, so `timeoutMs` is a parameter; keep one ENet user initialised at a time; call `ServerEntryCheck::update` every frame.
  - `regions.txt` has no defined place next to the installed game yet.
- Review findings and fixes:
