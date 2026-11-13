# D2 Lobby: tag on created lobbies, list filter in both modes

State: done
Branch: servers-D2

<!-- The code for every "Done when" item is complete and builds. Items that can only be seen with the running game and a lobby service connection stay unticked; they are listed under Left to do. -->

## Goal

Mark every lobby created in Servers mode with the region's tag, and filter the lobby list so that Servers shows only tagged lobbies of the player's region and Online shows none of them.

## Depends on

D1

## Read first

- PROTOCOL.md sections 5.2, 5.3 and 7.1, and V4 in section 10.
- CODEMAP "Online lobby": `PeerThread.cpp` (ladder fields), `LobbyUtils.cpp` (list loop), `Menus/PopupHostGame.cpp`, `Menus/WOLLobbyMenu.cpp` (the two join refusals), `Menus/ScoreScreen.cpp`.
- `Core/GameEngine/Source/GameNetwork/GameSpy/LadderDefs.cpp`: how known ladders are looked up by address text and port.

## May edit

- `GameNetwork/GameSpy/LobbyUtils.cpp`
- `Menus/PopupHostGame.cpp`, `Menus/WOLLobbyMenu.cpp` and `Menus/ScoreScreen.cpp` in `GeneralsMD/`, then the same changes in `Generals/`
- The Servers mode state files from D1

## Must not edit

`PeerThread.cpp` and `LadderDefs.cpp`, unless the handoff notes of this package explain why it was unavoidable. `NAT.*`, `LANAPI*`, `StagingRoomGameInfo.cpp` (2.1 owns that change).

## Done when

- [ ] A lobby created in Servers mode carries the tag text and tag port of the creator's region in its ladder fields. A lobby created in Online is unchanged. Code written and built, not run: `createGame()` in `PopupHostGame.cpp`; Online path untouched.
- [x] Tags are compared exactly, text and port together. `ServersModeIsAnyTag` / `ServersModeIsOwnTag` in `ServersMode.cpp` use `findByTag` (`strcmp` and port).
- [ ] In Servers mode the list shows only lobbies whose ladder fields equal the player's region tag. Code written and built, not run: `RefreshGameListBox` in `LobbyUtils.cpp`.
- [ ] In Online the list hides every lobby whose ladder fields equal any tag in the region file. Code written and built, not run: same loop.
- [ ] In Servers mode the "unknown ladder" refusal on join does not fire for the player's region tag. It still fires for any other unknown ladder. Code written and built, not run: `WOLLobbyMenu.cpp` join handler.
- [x] Every way of joining a lobby is covered, including joining with a password and joining through the buddy list. Only the join button handler can start a join (see Handoff notes).
- [x] In Online, and in builds with the switch off, the existing refusals behave exactly as before. All changes are under `RTS_SERVERS`; the switch-off build compiles the original code. Online with the switch on only gains the tag filter.
- [ ] The ladder choice in the create-lobby dialog is hidden in Servers mode. Code written and built, not run: `PopupHostGameInit`.
- [ ] After a Servers match the score screen does not report the match to the tag address and does not store the tag as the last ladder used. Code written and built, not run: `ScoreScreen.cpp`, both spots.
- [ ] This settles PROTOCOL.md V4. If the lobby service does not carry the ladder fields, the fallback (a fixed prefix in the lobby name, stripped for display) is implemented instead and the handoff notes say so. Not settled: needs the live lobby service. Fallback not implemented.
- [x] Builds with the switch on and off. Lint reports no new warnings. Release builds exit 0 on and off; clang-tidy script: 0 issues on the 8 changed files.
- [x] Reviewed by someone other than the author, with a dedicated pass confirming Online behaviour is unchanged.

## Out of scope

The entry check on join (D3). Match start (2.1).

## Handoff notes

- Finished:
  - `ServersMode.h/.cpp` (Core): `ServersModeIsAnyTag`, `ServersModeIsOwnTag`, `ServersModeShowsLobby`. Tag comparison is exact, text and port together (`ServerRegions::findByTag`, `AsciiString ==`). Port 0 is never a tag. The region file for the Online filter is read once on first use from the same `regions.txt` as D1; if it cannot be read, no tags are known and Online shows everything as before.
  - `LobbyUtils.cpp`, `RefreshGameListBox`: lobbies are skipped before sorting unless `ServersModeShowsLobby` is true. Servers: only the own region tag. Online: nothing that equals any tag in the file.
  - `PopupHostGame.cpp` (both titles): in Servers mode `createGame()` sets the ladder IP and port to the region tag, and the ladder combo box is hidden in `PopupHostGameInit` (the popup layout is created fresh on each open, so nothing needs unhiding).
  - `WOLLobbyMenu.cpp` (both titles): the "unknown ladder" refusal is not raised for the own region tag in Servers mode. Any other unknown ladder still refuses.
  - `ScoreScreen.cpp` (both titles): the ladder result report and the "last ladder used" store skip any tag from the region file.
  - Every change is behind `RTS_SERVERS`. Zero Hour and Generals changes are identical.
  - Join paths: the only two `PEERREQUEST_JOINSTAGINGROOM` senders are the join button handler in `WOLLobbyMenu.cpp` (the refusal is above both the no-password and password branches) and `PopupJoinGame.cpp` (the password popup, reached only after that handler). Buddy joining only moves the player to the lobby; the join itself then uses the same list and button.
- Left to do:
  - Run with a lobby service connection and a relay (a tester): from Servers create a lobby and check it appears for a second instance in Servers and not in Online; check that Online and Servers list each other's lobbies correctly; check that the join does not show the unknown ladder message; check the host dialog has no ladder choice; play a match and confirm no ladder report and no change of the stored last ladder.
  - The first two "Done when" items about the tag text and port, and the V4 item, depend on the lobby service passing the ladder fields through. Not checked here.
- Surprises and open questions:
  - V4 is not settled: it needs a live lobby service. The tag is implemented through the ladder fields as the package says. The fixed-prefix fallback was not implemented because there is no evidence the fields are dropped. `PeerThread.cpp` already sends and reads `ladIP`/`ladPort` for every lobby and was not edited.
  - In Servers mode the list column "Ladder" still reads "Unknown Ladder" for tagged lobbies, because `insertGame` does not know the tag. Not changed, it is cosmetic and outside the package.
  - The right-click lobby details menu and the sort-to-bottom of unknown ladders in `LobbyUtils.cpp` are unchanged. In Servers mode all listed lobbies are tagged, so the sort has no visible effect.
  - The Online filter depends on `regions.txt` being found in the working directory (see D1 notes). Without it, tagged lobbies are not hidden in Online. The tag is only ever created when the file was already read by the entry check.
  - Building: the OFF build first failed on a lost network connection while fetching a dependency; a plain retry worked. Builds: switch on 2539 steps exit 0, switch off 2525 steps exit 0 (Release). Lint: `scripts/run-clang-tidy.py --no-plugin` on the 8 changed `.cpp` files, 0 issues (switch-on PCH-free build in `C:\cnc-build\D2-tidy`).
  - PROTOCOL.md section 10: V4 stays open (see above).
- Review findings and fixes: reviewed by a second developer, with a dedicated pass on unchanged Online behaviour. Verdict: approved, no changes required. Both findings are low.
  - The tag-region list is allocated once on first use and never freed, matching the existing region-list pattern in the same file. Benign one-time static, no correctness effect.
  - In Servers mode the lobby list's "Ladder" column still reads "Unknown Ladder" for tagged lobbies. Cosmetic and out of scope.
  - V4 (section 10) judged sound: `PeerThread.cpp` already sends and reads the ladder address fields for every lobby and is untouched, so the tag carried in those fields (D33) survives on the client side; the untested-prefix fallback is correctly not built. The only part that needs the live lobby service is whether the third-party service relays the custom ladder keys to other players.
  - Confirmed correct by a full switch-on build (2539 steps, both titles link) and a switch-off configure: with the switch off, and in Online with the switch on, the lobby list, create dialog, join, ladder choice and score screen behave as upstream (every new path is behind the switch and a Servers-mode check, and the score-screen guards use an `#else` with the original condition); the tag is formed from the region tag text and port in the ladder fields and compared by text and port together; from Servers the list shows only the player's own-region tag and from Online every tagged lobby is hidden; the unknown-ladder refusal is skipped only for the own tag; all join paths are covered; scope stays within track D (PeerThread, LadderDefs, NAT and StagingRoomGameInfo untouched); Zero Hour and Generals are byte-identical.
  - On-screen and live-service behaviour (the filtered list, the tag round-trip through the lobby service, the hidden controls, the score-screen report) needs the running game and a lobby service, which are not available here. These are the Phase 1 D gate and are recorded as pending: needs the game installed.
