# D3 Menus: entry check on join, Accept and exit

State: todo
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

- [ ] Join: in Servers mode, clicking Join (both the direct path and the password dialog path) first runs the entry check, with "Checking connection" and Cancel. A pass replaces the stored pass and the join continues with the new pass. Cancel returns to the list.
- [ ] Accept: a member pressing Accept first runs the entry check. A pass replaces the stored pass and the ready state is sent.
- [ ] Exit: leaving a server lobby (Back in the staging room) leaves at once, then runs the entry check. A pass replaces the stored pass.
- [ ] A refusal at any of the three shows the D1 message for the reason, leaves the lobby if in one, logs out of the online service with a full teardown and returns to the main menu with Servers mode cleared (PROTOCOL.md 4.4, table).
- [ ] While a check runs the screen stays responsive and the action cannot be triggered twice.
- [ ] In a server lobby map transfer is never offered: `willTransfer` is false, and a member without the map cannot press Accept.
- [ ] None of this runs in Online mode. Everything is behind `RTS_SERVERS`.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- The silent refresh of a pass that is about to expire before a match (2.1).
- Match start (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
