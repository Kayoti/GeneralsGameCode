# D3 Menus: entry check on join, Accept and exit

State: done
Branch: servers-go-D3

## Goal

In Servers mode, run the entry check again when the player tries to join a lobby, presses Accept, or leaves a lobby, and block map transfer in server lobbies.

## Depends on

D2

## Read first

- DECISIONS.md D22, D23, D32
- PROTOCOL.md sections 4.4, 5.1 and 5.4 (last two paragraphs)
- archive/D3-gate-triggers.md, all of it including "Handoff notes" (why a second check object is kept apart from the main menu's, and how the refusal teardown was made reliable)
- reference/CLIENT_PORTING.md section 6 (hook table) and section 5 rows `WOLGameSetupMenu.cpp`, `WOLLobbyMenu.cpp`, `GUIUtil.cpp`
- On this branch, in full: `WOLLobbyMenuSystem` (the `buttonJoinID` branch), `WOLLobbyMenuUpdate`, `ExitState`, `joinGame()` in `PopupJoinGame.cpp`, `WOLGameSetupMenuSystem` (the `buttonStartID` and `buttonBackID` branches), `WOLGameSetupMenuUpdate`, `PopBackToLobby`, `UpdateSlotList` in `GUIUtil.cpp`

## May edit

- `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/WOLLobbyMenu.cpp`, `WOLGameSetupMenu.cpp`, `PopupJoinGame.cpp`
- `GeneralsMD/Code/GameEngine/Source/GameNetwork/GUIUtil.cpp` (the `willTransfer` line)
- `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersMode.h`, `GeneralsMD/Code/GameEngine/Source/GameNetwork/ServersMode.cpp`

## Must not edit

- `StartPressed()` and the start-message handler in `WOLGameSetupMenu.cpp` (2.1)
- Anything under `GameNetwork/GeneralsOnline/`

## Done when

- [x] Join: in Servers mode, clicking Join (both the direct path and the password dialog path) first runs the entry check, with "Checking connection" and Cancel. A pass replaces the stored pass and the join continues with the new pass. Cancel returns to the list. Code in the Join branch and `updateServersGate()` in `WOLLobbyMenu.cpp`; verified by code; the on-screen run is pending (needs the game).
- [x] Accept: a member pressing Accept first runs the entry check. A pass replaces the stored pass and the ready state is sent. Code in the `buttonStartID` branch and `updateServersGate()` in `WOLGameSetupMenu.cpp`; verified by code; on-screen run pending.
- [x] Exit: leaving a server lobby (Back in the staging room) leaves at once, then runs the entry check. A pass replaces the stored pass. Code in the `buttonBackID` branch in `WOLGameSetupMenu.cpp`; verified by code; on-screen run pending.
- [x] A refusal at any of the three shows the D1 message for the reason, leaves the lobby if in one, logs out of the online service with a full teardown and returns to the main menu with Servers mode cleared (PROTOCOL.md 4.4, table). `ServersLogoutOnline()` in `ServersMode.cpp`; the teardown reason and refusal text match D1 and the existing usage; verified by code; on-screen run pending.
- [x] While a check runs the screen stays responsive and the action cannot be triggered twice. `ServersGateIsPending()` and the modal check box; verified by code; on-screen run pending.
- [x] In a server lobby map transfer is never offered: `willTransfer` is false, and a member without the map cannot press Accept. `UpdateSlotList` in `GUIUtil.cpp` forces `willTransfer=FALSE`; verified by code; on-screen run pending.
- [x] None of this runs in Online mode. Everything is behind `RTS_SERVERS`. Every change is under `RTS_SERVERS` and `ServersModeIsOn()`, which is false in Online.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. Release and Debug builds with the switch on and Release with it off exit 0; lint not run.
- [x] Reviewed by someone other than the author.

## Out of scope

- The silent refresh of a pass that is about to expire before a match (2.1).
- Match start (2.1).

## Handoff notes

- Finished:
  - `ServersMode.h/.cpp`: a second entry check for player actions, kept apart from D1's check (the main menu drives that one every frame). `ServersGateStart(action)` checks against the region of the current pass with the "Checking connection" Cancel box; `ServersGateUpdate(handled, &action)` returns CHECKING, PASSED (stores the new pass), REFUSED (shows D1's refusal text) or CANCELLED; `ServersGateIsPending()` stops a second trigger; a finished result for an action the caller does not list in `handled` is held. `ServersLogoutOnline()` sets the full teardown `USER_REQUESTED_SILENT` (the existing path the lobby and staging room already act on: they leave, tear down and pop) and clears Servers mode. `ServersModeClear()` drops a pending check.
  - `WOLLobbyMenu.cpp`: the Join branch starts the check before `SetLobbyTryingToJoin`, so both the direct join and the password dialog run after a pass. On a pass the Join press is sent again once (`s_serversJoinCheckPassed`). `updateServersGate()` in `WOLLobbyMenuUpdate` handles JOIN, ACCEPT and EXIT results; a refusal calls `ServersLogoutOnline()`.
  - `WOLGameSetupMenu.cpp`: a member's Accept (the `buttonStartID` branch, non-host) waits for the check and is sent only on a pass (press sent again once). Back leaves with `PopBackToLobby()` and then starts the EXIT check. `updateServersGate()` in `WOLGameSetupMenuUpdate` handles ACCEPT only; the EXIT result is left to the lobby list so the staging room is never shut down twice.
  - `GUIUtil.cpp`: `UpdateSlotList` sets `willTransfer` to FALSE in Servers mode, so a member without the map has Accept disabled.
  - `PopupJoinGame.cpp` is not changed: the check runs on the Join click, before the password dialog opens.
  - All of it is behind `RTS_SERVERS` and, outside `ServersMode.*`, `ServersModeIsOn()`.
- Left to do:
  - A tester with the game, a lobby service and a relay must check each item on screen (join with and without password, Accept, Back; refusal returns to the main menu with the session closed; Accept disabled without the map). Nothing was run in the game.
- Surprises and open questions:
  - The password dialog can stay open longer than the pass lifetime; the pass is then refused by the service and D2's expired-check text is shown. The check was placed on the Join click (not on the dialog's OK) because the dialog has no per-frame update to drive a check.
  - The host-side map checks in `StartPressed()` and the two chat lines in `WOLGameSetupMenu.cpp` that say a map will transfer still use the transfer test. They are 2.1's or outside this package's file rules (`StartPressed()` must not be edited), so the host can still see "will transfer" text in Servers mode. 2.1 must treat every map as non-transferable when it starts a server match.
  - If `ServersGateStart()` fails on Back (only when the network library cannot start), `ServersLogoutOnline()` is called while the staging room is still shutting down.
- Review findings and fixes:
  - Approved. The code satisfies every "Done when" item, judged by reading it: the entry check runs on
    Join, on a non-host Accept and on Back, with the "Checking connection" modal and the double-trigger
    guard; a pass re-sends the original action with the refreshed pass; a refusal shows the D1 refusal
    text, tears the online session down with a full teardown and clears Servers mode; a server lobby
    never offers a map transfer. The exit result is handled by the lobby list, not the staging room, so
    the staging room is not shut down twice. Every change is behind `RTS_SERVERS` and `ServersModeIsOn()`,
    so Online and Network play are unchanged; `PopupJoinGame.cpp` and `StartPressed()` were correctly left
    alone. Scope, change-comment format, author and commit hygiene all correct; `Generals/` untouched.
  - Two low observations, both unreachable in practice, no change: the Accept re-send checks
    `buttonStart != nullptr` (always bound in the staging room); `ServersGateUpdate`'s default case returns
    cancelled for an unexpected idle state (only an external cancel, which nothing calls, could reach it).
  - The on-screen behaviour (join with and without a password, Accept, Back, a refusal returning to the
    main menu with the online session closed, Accept disabled without the map) needs the running game and
    a local lobby service and relay. It is recorded as pending in the Phase 1 check table; see STATUS.
  - Re-verify after merge waived: the merge changed only documentation files, so the branch build (all
    three configurations exit 0) covers the merged tree.
