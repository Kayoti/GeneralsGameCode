# C3 Client: launch flags to create or join a relay match directly

State: blocked
Branch: servers-C3

## Goal

Let two or more game instances on one PC start a match through the relay without the lobby. This exists for the Phase 1 gate and for later debugging.

## Depends on

C1. Needs C2 to be useful.

## Read first

- CODEMAP "Testing aids".
- `Core/GameEngine/Source/Common/CommandLine.cpp`: how flags are declared and parsed, and `-quickstart`.
- `ClientInstance.cpp` and the `RTS_MULTI_INSTANCE` define.
- How a LAN match is started from a `GameInfo` in `LANAPICallbacks.cpp` (read only), as a model for building a slot list and starting the game.

## May edit

- `Core/GameEngine/Source/Common/CommandLine.cpp` and the matching global data header, to add the flags
- One new file pair under `Core/GameEngine/.../GameNetwork/` that builds the match from the flags

## Must not edit

`LANAPI*`, `GameSpy/`, menus.

## Done when

- [ ] Flags exist, in builds with the Servers switch on: relay host and port, host or join, slot number, number of players, map name, and for joiners the session id and join token.
- [ ] The host instance runs the entry check, sends `CREATE`, and writes the session id and each join token to the log.
- [ ] A joining instance runs the entry check, sends `JOIN` with the given session id and join token, and waits for `START`.
- [ ] On `START` every instance loads the same map with the same seed and fixed default factions, and the match begins.
- [ ] Usage is documented in `docs/servers/TESTING.md`, including the multi-instance build option and window size flags.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Any menu. Any use of the online lobby.

## Handoff notes

- Finished: nothing. The package cannot be built as scoped (see below).
- Left to do: the whole package, once the block below is resolved.
- Surprises and open questions:
  - Blocking, scope: this package may edit only `CommandLine.cpp`, the global-data header, and one new file pair under `Core/.../GameNetwork/`. But a flag-started direct match has to set the server-mode flag, hand a connected `ServerLink` to the networking code, and be driven each frame. None of that is reachable from the allowed files: `class Network` and its `m_conMgr` are private in `Network.cpp`; `NetworkInterface.h` has no `setServerMode`/`isServerMode`/attach-link method (C1 and C2 left this for 2.1); and nothing in the allowed files runs the new code each frame (that would need `GameClient.cpp`). The networking-attach wiring is exactly what package 2.1 adds (2.1's "May edit" includes `Network.cpp` and `ConnectionManager.cpp` "to hand the connected ServerLink to the networking code", and 2.1 does not depend on C3). So C3 should be built AFTER 2.1, reusing whatever public attach/drive mechanism 2.1 exposes; if 2.1 does not expose enough, C3's "May edit" must be widened to include `NetworkInterface.h`, `Network.cpp` and the per-title `GameClient.cpp`, or the per-frame drive must be provided as a shared hook. Owner decision, unless 2.1's wiring turns out to be sufficient.
  - Blocking, unspecified values: the exact flag name strings, the fixed map seed, and the "fixed default factions" are not given in this package file or in PROTOCOL.md. Sensible choices can be made and documented in TESTING.md once the wiring above exists, but the owner may want specific values.
- Review findings and fixes:
