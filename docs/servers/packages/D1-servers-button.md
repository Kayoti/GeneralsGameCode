# D1 Menus: Servers button and entry check on entry

State: done
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

- [x] `ServersMode` holds: whether Servers mode is on, the player's region, and the current pass with the time it was received. It offers the entry check start, per-frame update and result that the menus need. It has no GameSpy dependency: every use of `TheGameSpyPeerMessageQueue`, `TheGameSpyBuddyMessageQueue`, `PeerRequest`, `TearDownGameSpy` and ladder tags from the earlier file is removed, and teardown uses `NGMP_OnlineServicesManager::SetPendingFullTeardown`. Built, not run. `ServersMode.h/.cpp` hold state (`ServersModeSet/Clear/IsOn/GetRegion/GetPass`), `ServersEntryStart/Update/Cancel`. No GameSpy, peer, buddy or ladder references; no teardown code (the log-out and teardown on leaving belong to D3).
- [x] `ServersMode` exposes the pass as the 200-digit lowercase hexadecimal text for the `X-Servers-Pass` header, and the region tag, for D2. `ServersModeGetPassHex()` (200 lowercase hex digits from the 100 pass bytes) and `ServersModeGetRegionTag()` in `ServersMode.cpp`.
- [x] The Multiplayer menu shows a Servers button, made at runtime as on the earlier base, without disturbing the other buttons. `initServersButton()` in `MainMenu.cpp`, called from `MainMenuInit`. Built, not run (no game data installed).
- [x] Clicking it shows "Checking connection" with Cancel, runs the entry check against every region in the region file, and keeps the pass with the lowest ping (PROTOCOL.md 5.1). `serversID` branch calls `ServersEntryStart()`: Cancel box, `ServerEntryCheck::start` over all regions (lowest ping, first on a tie, is the C1 library's rule). Built, not run.
- [x] On a pass: Servers mode is set and the normal online start runs (`StartPatchCheck`). `MainMenuUpdate` calls `ServersEntryUpdate()`; on `PASSED` `startServersOnline()` runs the Online steps including `StartPatchCheck()`.
- [x] On refusal: the message for the reason is shown in English and the player stays on the Multiplayer menu. The texts are the three in PROTOCOL.md 4.4, word for word, with the two ping values filled in. `ServersModeRefusalText` with the three texts of PROTOCOL.md 4.4; the menu is not left, so OK shows the Multiplayer menu.
- [x] Clicking Online clears Servers mode. Returning to the main menu from the online screens clears it. `ServersModeClear()` in the Online branch, in `MainMenuInit` and in `HandleCanceledDownload( TRUE )`.
- [x] In Servers mode the welcome screen hides Quick Match and the ladder button. `WOLWelcomeMenuInit` hides both buttons; `WOLWelcomeMenuSystem` ignores their clicks.
- [x] All new text uses `fetchOrSubstitute` with the English fallback. `FETCH_OR_SUBSTITUTE` / `FETCH_OR_SUBSTITUTE_FORMAT` with English fallbacks in `ServersMode.cpp` and `MainMenu.cpp`.
- [x] Everything is behind `RTS_SERVERS`. With the switch off there is no button and no `ServersMode` code. All new code is inside `#ifdef RTS_SERVERS`. Switch-off Release build exits 0.
- [x] The entry check never runs for Online, Network, Skirmish or single player. `ServersEntryStart()` is called only from the `serversID` click; nothing else calls it.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. Release switch on, Debug switch on and Release switch off all built with exit 0 (`z_generals`). clang-tidy not run (not on PATH). No new compiler warnings in the changed files.
- [x] Reviewed by someone other than the author.

## Out of scope

- Lobby list filter and create (D2). Entry check on join, Accept and exit (D3).

## Handoff notes

- Finished:
  - New `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersMode.h` and `Source/GameNetwork/ServersMode.cpp`, listed in `GeneralsMD/Code/GameEngine/CMakeLists.txt`, whole file behind `RTS_SERVERS`. Taken from the earlier work with the lobby tag, gate, logout and match-start parts left out (D2, D3 and 2.1 add what they need).
  - `MainMenu.cpp`: `initServersButton()`, `startServersOnline()`, the `serversID` click, `ServersEntryUpdate()` in `MainMenuUpdate`, `ServersEntryCancel()` in `MainMenuShutdown`, `ServersModeClear()` in `MainMenuInit`, the Online click and `HandleCanceledDownload( TRUE )`.
  - `WOLWelcomeMenu.cpp`: Quick Match and ladder buttons hidden and ignored in Servers mode.
  - Builds: Release and Debug with the switch on, Release with the switch off, all exit 0.
- Left to do:
  - On-screen checks with the running game (button position, the Multiplayer menu frame, the modal flow, OK/Cancel return, the welcome screen). Not run here.
  - The label entries `GUI:Servers`, `GUI:ServersTitle`, `GUI:ServersChecking`, `GUI:ServersVpnDetected`, `GUI:ServersPingTooHigh`, `GUI:ServersCannotConnect` are not in a string file; the English fallbacks show until they are added.
- Surprises and open questions:
  - The region file is read as `regions.txt` from the game's working directory (constant in `ServersMode.cpp`). Where the installer puts it is still open.
  - D3 must add the log-out and teardown (`SetPendingFullTeardown`) when it needs it.
  - clang-tidy was not run (not on PATH).
  - PROTOCOL.md section 10: no assumption concerns this package.
- Review findings and fixes:
  - Verified: the `ServersMode` module has no GameSpy dependency; the pass is 200 lowercase hex into a
    201-byte buffer (no overrun) and the region tag is exposed for D2; the Servers button is built at
    runtime; the refusal texts and both ping values match PROTOCOL 4.4; the entry check starts only from
    the Servers click; the Welcome screen hides Quick Match and the ladder in Servers mode without
    reverting B1's edits; everything is behind `RTS_SERVERS` so the switch-off path is unchanged. Scope,
    change-comment format, author and commit hygiene all correct. After merging, the client built clean
    with the switch off (Release) and on (Release and Debug), all exit 0.
  - Two low notes, neither blocking, no change made:
    1. `MainMenu.cpp` Servers click does not set `dontAllowTransitions`/`buttonPushed` like the
       Online/Network branches; it relies on the modal "Checking connection" box to swallow clicks, which
       is how this engine's modal boxes behave. Safe; may set the guard flags for symmetry later.
    2. `initServersButton` reads `onlineX` via `winGetPosition` but uses only `onlineY`. Cosmetic.
  - The on-screen flow (button position, the modal check/refuse/pass transitions, the welcome-screen
    hiding) needs the running game and is a Phase 1 menu check, not run here.
