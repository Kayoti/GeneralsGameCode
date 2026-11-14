# C1 Client: relay link, entry check and region file, carried over

State: todo
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

- [ ] The six source files and `cmake/enet.cmake` are taken from `servers` unchanged, in one commit.
- [ ] The build file changes are re-applied by hand so that, with the switch on, ENet is fetched at the pinned version and `core_serverlink` builds and links into the Zero Hour engine library. With the switch off none of it is built or fetched.
- [ ] The headers stay free of ENet and engine types, as on `servers`.
- [ ] The client builds with the switch on and with it off. V1 is re-confirmed on this base, or the exact clash is recorded and the package stops as `partial`.
- [ ] No source change was needed to the six files. If one was, it is a separate commit and explained in the handoff notes.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Using the link in a match (C2) or from a menu (D1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
