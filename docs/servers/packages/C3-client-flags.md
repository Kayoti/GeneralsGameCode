# C3 Client: launch flags to create or join a relay match directly

State: todo
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

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
