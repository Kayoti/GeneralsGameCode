# D1 Lobby: Servers button, entry check on entry, refusal messages

State: partial
Branch: servers-D1

## Goal

Add the Servers button to the Multiplayer menu. Clicking it runs the entry check, and on success opens the existing online login and lobby with a "Servers mode" state set.

## Depends on

0.2, 0.3, C1 (for the `ServerEntryCheck` and `ServerRegions` headers)

## Read first

- PROTOCOL.md sections 4.4, 4.9, 5.1 and 7.1.
- DECISIONS D9, D16, D17, D21 to D23.
- CODEMAP "Online lobby": `Menus/MainMenu.cpp`, and how `ButtonOnline` leads into the login screen.

## May edit

- `Menus/MainMenu.cpp` in `GeneralsMD/`, then the same change in `Generals/`
- One new file pair in `Core/GameEngine/.../GameNetwork/GameSpy/` holding the Servers mode state: whether it is on, the region, the current pass
- The Multiplayer menu layout and the text file entries for the new strings, added as override files next to the code

## Must not edit

`WOL*.cpp` screens, `PeerThread.cpp`, `LobbyUtils.cpp` (D2 and D3 own those changes), `NAT.*`, `LANAPI*`.

## Done when

- [ ] The Multiplayer menu shows a Servers button next to Online and Network, in builds with the Servers switch on. Code written and built, not run in the game: `initServersButton()` in `MainMenu.cpp`. No game data is installed here, so the look and the place of the button are unchecked.
- [ ] Clicking it shows a short "Checking connection" state while `ServerEntryCheck` runs. The menu stays responsive. Code written and built, not run: `ServersEntryStart()` shows a cancel box, `MainMenuUpdate` calls `ServersEntryUpdate()` every frame.
- [ ] On a pass: Servers mode state is set with the region and pass, and the existing online login screen opens. Code written and built, not run: `ServersEntryUpdate()` calls `ServersModeSet()`, then `startServersOnline()` runs the same steps as the Online button.
- [ ] On `VPN_DETECTED`: the message from PROTOCOL.md 4.4 is shown, and OK returns to the Multiplayer menu. Code written and built, not run: `ServersModeRefusalText()` and an OK box. The menu is not left, so OK shows the Multiplayer menu again.
- [ ] On `PING_TOO_HIGH`: the message with the measured value and the limit is shown, and OK returns to the Multiplayer menu. Code written and built, not run: `fetchFormat( "GUI:ServersPingTooHigh", rtt, limit )`.
- [ ] On any other refusal, or no answer from any region: "The server cannot be reached." and back to the Multiplayer menu. Code written and built, not run: the `default` of `ServersModeRefusalText()`. A missing or invalid region file gives the same message.
- [ ] Leaving the online lobby back to the menus clears the Servers mode state. Code written and built, not run: `ServersModeClear()` in `MainMenuInit` (runs when the lobby screens are popped back to the main menu) and in `HandleCanceledDownload( TRUE )`.
- [x] Clicking Online never runs the entry check and never sets the state. `MainMenu.cpp` (Zero Hour) line 1521: the Online branch is unchanged. Only the new `serversID` branch (line 1540) calls `ServersEntryStart()`, and `ServersModeSet()` is called only from `ServersEntryUpdate()`.
- [x] All new text is English and comes from the game's text system, not from literals in code. Every string is fetched with `TheGameText->fetch` or `fetchFormat` (labels `GUI:Servers`, `GUI:ServersTitle`, `GUI:ServersChecking`, `GUI:ServersVpnDetected`, `GUI:ServersPingTooHigh`, `GUI:ServersCannotConnect`). The entries are in `Servers/Client/servers.str`. They must be added to the game string file, see the notes below.
- [x] The handoff notes record how the layout override was produced, since the original layout files are not in the repository. See "Surprises and open questions".
- [x] Builds with the switch on and off. Lint reports no new warnings. Both Release builds exited 0 (`C:\cnc-build\d1-on.log`, `d1-off.log`). `python scripts\run-clang-tidy.py --no-plugin` on the three changed `.cpp` files: 0 issues.
- [ ] Reviewed by someone other than the author.

## Out of scope

The tag and the list filter (D2). The entry check on join, Accept and exit (D3).

## Handoff notes

- Finished:
  - `Core/GameEngine/Include/GameNetwork/GameSpy/ServersMode.h` and `Core/GameEngine/Source/GameNetwork/GameSpy/ServersMode.cpp` (listed in `Core/GameEngine/CMakeLists.txt`, whole file behind `RTS_SERVERS`):
    - State: `ServersModeIsOn`, `ServersModeGetRegion`, `ServersModeGetPass`, `ServersModeSet`, `ServersModeClear`.
    - `ServersModeRefusalText( code, rtt, limit )`: VPN_DETECTED, PING_TOO_HIGH, and the catch-all "The server cannot be reached." for code 0 and every other code. D3 can reuse it.
    - Entry check driver: `ServersEntryStart` (loads the region file, starts `ServerEntryCheck` against every region, shows a modal "Checking connection" box with Cancel), `ServersEntryUpdate` (once per frame, returns `PASSED` or `REFUSED` once, shows the OK box on a refusal), `ServersEntryCancel`.
  - `MainMenu.cpp` in `GeneralsMD/` and `Generals/`, same change in both, all behind `RTS_SERVERS`: `initServersButton()` (called from `MainMenuInit`), `startServersOnline()`, the `serversID` click branch, `ServersEntryUpdate()` in `MainMenuUpdate`, `ServersEntryCancel()` in `MainMenuShutdown`, and `ServersModeClear()` in `MainMenuInit` and in `HandleCanceledDownload( TRUE )`.
  - `Servers/Client/servers.str`: the six string entries.
  - Region file path: `ServersMode.cpp` reads `regions.txt` from the working directory of the game (constant `SERVERS_REGION_FILE`). A dev copy is `Servers/Client/regions.txt` copied next to the exe. The real install location is still an open question (see 0.2 notes).
- Left to do:
  - Run the game and check each unticked item by hand. Nothing was run here: no game data is installed, so the menu cannot be shown. To test: copy `Servers/Client/regions.txt` next to the exe, merge `Servers/Client/servers.str` into the string file, start a relay (A1) with a matching region.
  - Strings: the game loads one string file (`Data\Generals.str` or `Data\<language>\Generals.csf`, both in the data archives, not in the repository), so there is no loose override that adds entries. `servers.str` must be merged into that file at packaging time. Until then the labels show as `MISSING: 'GUI:...'`.
  - Reviewer box stays unticked.
- Surprises and open questions:
  - Layout override: none was produced. `MainMenu.wnd` is not in the repository, so a `.wnd` override could not be written. Instead `initServersButton()` creates the button in code, as the debug button in the same function already does. It finds the window `MainMenu.wnd:ButtonNetwork`, creates a push button one button step below the lower of `ButtonOnline` and `ButtonNetwork` (step = distance between those two), copies the size, status (minus hidden), font, image and colour draw data, text colours and owner from the Network button, and gives it the id `MainMenu.wnd:ButtonServers` and the text `GUI:Servers`. Sibling windows that start below the Network button move down by one step and the parent frame grows by one step. The guard looks the window up by id first, because `MainMenuInit` runs again each time the lobby screens are popped. Not checked on screen: the order of the buttons, the frame size and the transition of the Multiplayer menu (`MainMenuMultiPlayerMenuTransition`) are assumptions from the code. If the look is wrong, the fix is a `MainMenu.wnd` edit by someone with the original file; remove `initServersButton()` then.
  - The click does not run the dropdown transition. The Multiplayer menu stays on screen under the modal box, so "OK returns to the Multiplayer menu" needs no extra code. The transition and `StartPatchCheck()` run only after a pass, as for Online.
  - Servers mode also goes through the Online patch check (`StartPatchCheck`) before the login, because the package says the existing login opens and that is the Online path. A failed or cancelled patch check goes back to the main menu through `HandleCanceledDownload( TRUE )`, which clears the state.
  - `MainMenuShutdown` calls `HandleCanceledDownload( FALSE )` through `CancelPatchCheckCallback`, so the state is cleared only for `TRUE`, not on the shutdown that happens when the login screen is pushed.
  - The "Leaving the online lobby" clear relies on `MainMenuInit` running when the screens above it are popped (`Shell::doPop` runs `runInit` on the new top). The WOL screens were not edited, as the package requires. A match start or a return from a match that does not pass through the main menu is not covered; 2.2 and D3 should check this.
  - No place is defined for `regions.txt` (see above). A missing or invalid file shows "The server cannot be reached." and the reason goes to the debug log.
  - `ServerEntryCheck::update()` is called from `MainMenuUpdate` every frame while the check runs, as the C1 notes ask.
  - clang-tidy: calling `clang-tidy -p` directly reported thousands of style warnings from the default configuration (not the project's). The project script `scripts/run-clang-tidy.py --no-plugin` with a PCH-free build (`C:\cnc-build\D1-tidy`, switch on) reported 0 issues for `ServersMode.cpp` and both `MainMenu.cpp` files.
  - Builds: switch on and off, Release, 2539 and 2525 steps, exit 0 both. With the switch off `ServersMode.cpp` is compiled as an empty file.
  - PROTOCOL.md section 10: no assumption concerns this package.
- Review findings and fixes:
