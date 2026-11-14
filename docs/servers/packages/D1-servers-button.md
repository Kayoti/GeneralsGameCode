# D1 Menus: Servers button and entry check on entry

State: todo
Branch: servers-go-D1

## Goal

Add the Servers button to the Multiplayer menu. Clicking it runs the entry check and, on a pass, opens the normal online login with the client in Servers mode.

## Depends on

B1, C1

## Read first

- DECISIONS.md D9, D13, D16, D17, D21, D22, D23
- PROTOCOL.md sections 4.4 (entry check) and 5.1 (region)
- archive/D1-servers-button.md, all of it including "Handoff notes"
- reference/CLIENT_PORTING.md section 1 steps 1 to 4, section 3 (adding a button and text), and section 5 rows `MainMenu.cpp`, `WOLWelcomeMenu.cpp`, `ServersMode.h/.cpp`
- `git show servers:Core/GameEngine/Include/GameNetwork/GameSpy/ServersMode.h`, `git show servers:Core/GameEngine/Source/GameNetwork/GameSpy/ServersMode.cpp`
- `git diff upstream/main...servers -- GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/MainMenu.cpp GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/WOLWelcomeMenu.cpp`
- On this branch, in full: `MainMenuInit`, `MainMenuUpdate`, `MainMenuSystem`, `StartPatchCheck`, `WOLWelcomeMenuInit`, `WOLWelcomeMenuSystem`

## May edit

- New files `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersMode.h` and `GeneralsMD/Code/GameEngine/Source/GameNetwork/ServersMode.cpp`
- `GeneralsMD/Code/GameEngine/CMakeLists.txt` (add the two files)
- `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/MainMenu.cpp`, `WOLWelcomeMenu.cpp`

## Must not edit

- Anything under `GameNetwork/GameSpy/`
- `WOLLobbyMenu.cpp`, `WOLGameSetupMenu.cpp`, `PopupHostGame.cpp` (D2, D3)
- `ServerEntryCheck.*`, `ServerRegions.*`

## Done when

- [ ] `ServersMode` holds: whether Servers mode is on, the player's region, and the current pass with the time it was received. It offers the entry check start, per-frame update and result that the menus need. It has no GameSpy dependency: every use of `TheGameSpyPeerMessageQueue`, `TheGameSpyBuddyMessageQueue`, `PeerRequest`, `TearDownGameSpy` and ladder tags from the earlier file is removed, and teardown uses `NGMP_OnlineServicesManager::SetPendingFullTeardown`.
- [ ] `ServersMode` exposes the pass as the 200-digit lowercase hexadecimal text for the `X-Servers-Pass` header, and the region tag, for D2.
- [ ] The Multiplayer menu shows a Servers button, made at runtime as on the earlier base, without disturbing the other buttons.
- [ ] Clicking it shows "Checking connection" with Cancel, runs the entry check against every region in the region file, and keeps the pass with the lowest ping (PROTOCOL.md 5.1).
- [ ] On a pass: Servers mode is set and the normal online start runs (`StartPatchCheck`).
- [ ] On refusal: the message for the reason is shown in English and the player stays on the Multiplayer menu. The texts are the three in PROTOCOL.md 4.4, word for word, with the two ping values filled in.
- [ ] Clicking Online clears Servers mode. Returning to the main menu from the online screens clears it.
- [ ] In Servers mode the welcome screen hides Quick Match and the ladder button.
- [ ] All new text uses `fetchOrSubstitute` with the English fallback.
- [ ] Everything is behind `RTS_SERVERS`. With the switch off there is no button and no `ServersMode` code.
- [ ] The entry check never runs for Online, Network, Skirmish or single player.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Lobby list filter and create (D2). Entry check on join, Accept and exit (D3).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
