# D3 Lobby: entry check on join, Accept and exit; no map transfer; Quick Match hidden

State: todo
Branch: servers-D3

## Goal

Run the entry check again on the player actions the project owner listed, and remove the parts of the online lobby that do not apply in Servers mode.

## Depends on

D1

## Read first

- PROTOCOL.md section 4.4 (the table of triggers) and 5.4.
- DECISIONS D22, D23 and D32.
- CODEMAP "Online lobby": `Menus/WOLLobbyMenu.cpp` (join), `Menus/WOLGameSetupMenu.cpp` (Accept, Start, leaving the lobby), `GUIUtil.cpp`, `Menus/WOLQuickMatchMenu.cpp`, and the welcome screen that links to Quick Match.

## May edit

- `Menus/WOLLobbyMenu.cpp`, `Menus/WOLGameSetupMenu.cpp`, `Menus/WOLWelcomeMenu.cpp` in `GeneralsMD/`, then the same changes in `Generals/`
- `GameNetwork/GUIUtil.cpp` in `GeneralsMD/`, then the same change in `Generals/`
- The Servers mode state files from D1

## Must not edit

`LobbyUtils.cpp`, `PopupHostGame.cpp`, `ScoreScreen.cpp` (D2 owns those), `PeerThread.cpp`, `StagingRoomGameInfo.cpp`, `NAT.*`.

## Done when

Every item applies only in Servers mode.

- [ ] Attempting to join a lobby runs the entry check first. The join proceeds only on a pass.
- [ ] Pressing Accept in a lobby runs the entry check first. The accept is sent only on a pass.
- [ ] Exiting a lobby runs the entry check after the player has left.
- [ ] A refusal on any of the three shows the same messages as D1. The player leaves the lobby if in one, the online lobby is closed, and the player returns to the Multiplayer menu.
- [ ] While the entry check runs the screen stays responsive, and the action cannot be triggered twice.
- [ ] Each successful entry check replaces the stored pass.
- [ ] Map transfer is never offered: a player without the map cannot press Accept, and the host cannot start while any player lacks the map, for official and custom maps alike. Both places that decide this today are changed.
- [ ] Quick Match cannot be reached.
- [ ] In Online, none of this code runs.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

Match start (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
