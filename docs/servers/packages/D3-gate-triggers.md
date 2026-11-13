# D3 Lobby: entry check on join, Accept and exit; no map transfer; Quick Match hidden

State: done
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

- [ ] Attempting to join a lobby runs the entry check first. The join proceeds only on a pass. Code written and built, not run: Join handler in `WOLLobbyMenu.cpp`, `updateServersGate()`.
- [ ] Pressing Accept in a lobby runs the entry check first. The accept is sent only on a pass. Code written and built, not run: `buttonStartID` branch in `WOLGameSetupMenu.cpp`.
- [ ] Exiting a lobby runs the entry check after the player has left. Code written and built, not run: `buttonBackID` branch in `WOLGameSetupMenu.cpp` (read as leaving the staging room, see notes).
- [ ] A refusal on any of the three shows the same messages as D1 (`ServersModeRefusalText`; code written and built, not run). The player leaves the lobby if in one, the online lobby is closed, and the player returns to the Multiplayer menu.
- [ ] While the entry check runs the screen stays responsive, and the action cannot be triggered twice. Code written and built, not run: `ServersGateIsPending()` and the modal check box.
- [x] Each successful entry check replaces the stored pass. `ServersGateUpdate()` calls `ServersModeSet()` on a pass.
- [ ] Map transfer is never offered (code written and built, not run: `GUIUtil.cpp` `UpdateSlotList` and `StartPressed`): a player without the map cannot press Accept, and the host cannot start while any player lacks the map, for official and custom maps alike. Both places that decide this today are changed.
- [ ] Quick Match cannot be reached. Code written and built, not run: `WOLWelcomeMenu.cpp` hides and ignores the Quick Match and Ladder buttons.
- [x] In Online, none of this code runs. Every change is under `RTS_SERVERS` and `ServersModeIsOn()`, which is false in Online.
- [x] Builds with the switch on and off. Lint reports no new warnings. Release builds exit 0. `scripts/run-clang-tidy.py --no-plugin` (switch-on PCH-free build): 0 issues on the 9 changed `.cpp` files after the review fix. Runs with several workers print exit code 1 with no issue listed; the five Zero Hour files run singly returned 0.
- [x] Reviewed by someone other than the author. Reviewed over two rounds; approved after the refusal-teardown fix.

## Out of scope

Match start (2.1).

## Handoff notes

- Finished:
  - `ServersMode.h/.cpp` (Core): a second entry check for player actions, kept apart from D1's check because the main menu drives that one every frame and would take its result. `ServersGateStart( action )` runs the check against the region of the current pass (`ServerEntryCheck::startRegion`) with the "Checking connection" Cancel box. `ServersGateUpdate( handled, &action )` is called each frame and returns CHECKING, PASSED (stores the new pass with `ServersModeSet`), REFUSED (shows D1's refusal text through `ServersModeRefusalText`) or CANCELLED. `ServersGateIsPending()` stops a second trigger. A finished result for an action the caller does not list in `handled` is held. `ServersModeClear()` drops a pending check. `ServersLogoutOnline()` closes the overlays and queues the peer and buddy logout, as the Welcome screen's Back does, and `ServersTearDownOnline()` finishes it with `TearDownGameSpy()` (see below).
  - `WOLLobbyMenu.cpp` (both titles): the Join button handler starts the check first in Servers mode. On a pass the button press is sent again once and the existing join code runs. A refusal leaves the group room, logs out and pops to the main menu. The lobby also drops the result of an Accept or Exit whose screen is gone, so no check box stays on screen.
  - `WOLGameSetupMenu.cpp` (both titles): a non-host Accept waits for the check and is sent only on a pass (the press is sent again once). Back (exit) leaves as before and then starts the check. A refusal leaves the staging room if still in it, logs out, and clears `nextScreen` so the shell returns to the main menu instead of the lobby list. The Start handler and the two chat messages treat every map as non-transferable in Servers mode, and the host also cannot start while an accepted human slot lacks the map.
  - `GUIUtil.cpp` (both titles): `UpdateSlotList` uses "no transfer" in Servers mode, so a player without the map has Accept disabled.
  - `WOLWelcomeMenu.cpp` (both titles): the Quick Match and Ladder buttons are hidden in Servers mode and their presses are ignored. Quick Match and the ladder screen are pushed from nowhere else.
  - Every change is behind `RTS_SERVERS` and, outside `ServersMode.*`, behind `ServersModeIsOn()`. Zero Hour and Generals changes are identical.
- Left to do:
  - A tester with the game, a lobby service and a relay must check each item on screen: join, Accept and exit each show the Checking box and continue only on a pass; a flagged address or high ping shows the D1 message and lands on the main menu with the online session closed; Cancel on the box leaves the action undone; a player without the map has Accept disabled and the host gets the "no map" chat line instead of a start; Quick Match and Ladder are not on the Welcome screen.
  - Reviewer box stays unticked.
- Surprises and open questions:
  - "Exiting a lobby" is read as leaving the staging room (Back in `WOLGameSetupMenu.cpp`), because CODEMAP lists "leaving the lobby" under that file and the lobby tag is on staging rooms. Leaving the lobby list screen is not gated. If that was meant too, it needs a decision.
  - The online session is torn down deterministically on a refusal. `ServersLogoutOnline()` queues the peer and buddy logout, as the Welcome screen's Back does, and marks the teardown as due. `ServersTearDownOnline()` then calls `TearDownGameSpy()` from `shutdownComplete` of the lobby and the staging room, after their shutdown functions have used the session and before the shell initialises the main menu. It is not called inside `ServersLogoutOnline()`, because the lobby and staging room shutdown still use `TheGameSpyInfo` and the peer queue and would crash on a torn-down session. `ServersModeClear()` drops a teardown that was never taken. Not seen running.
  - The two staging room chat lines that said a map "will transfer" now also say "no map" in Servers mode. This goes beyond the two decision places in the package but keeps the text true.
  - The check box is the D1 message box, so Cancel on it is not a refusal: the player stays where they are and the action is not done (an Exit has already happened).
  - Builds and lint: switch on and off exit 0 (Release); clang-tidy clean on the nine changed files (see the Done-when build line above). No PROTOCOL.md section 10 assumption concerns this package.
- Review findings and fixes:
  - Medium, fixed: the refusal teardown depended on a race with the peer thread in `MainMenuInit`. Now `ServersTearDownOnline()` runs `TearDownGameSpy()` in the lobby and staging room `shutdownComplete`.
  - Low, fixed: the per-frame `updateServersGate()` calls are wrapped in `ServersModeIsOn()`.
  - Verdict after the fix: approved, no remaining findings. The reviewer confirmed from the code that the teardown is deterministic: the lobby and staging-room shutdown functions still use `TheGameSpyInfo` and the peer queue, so `TearDownGameSpy()` is correctly deferred to `shutdownComplete`, which runs after that shutdown and before `MainMenuInit` (`Shell::shutdownComplete` -> `doPop` -> the new screen's init). Every refusal path (lobby join, non-host Accept, exit, and a refusal that arrives mid-shutdown) reaches a `shutdownComplete` that tears the session down, so none lands on the main menu with a live online session. The teardown-due flag is set only on a refusal, cleared before the teardown and idempotent, and `ServersModeClear()` can only drop it, so a normal exit or a successful pass never tears down. Zero Hour and Generals changes are identical; every new path is behind `RTS_SERVERS` and `ServersModeIsOn()`, so the switch-off build and Online are unchanged. Commits are by Kayoti with no trailers.
  - Needs the game installed: every on-screen item above (the Checking box and Cancel on join, Accept and exit; the refusal message and return to the main menu with the session closed; Accept disabled without the map and the host "no map" line; Quick Match and Ladder absent from the Welcome screen). These are the Phase 1 D gate and were not run here. The teardown fix was verified by code only, not in the running game.
