# D2 Lobby: tag on created lobbies, list filter in both modes

State: todo
Branch: servers-D2

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

- [ ] A lobby created in Servers mode carries the tag text and tag port of the creator's region in its ladder fields. A lobby created in Online is unchanged.
- [ ] Tags are compared exactly, text and port together.
- [ ] In Servers mode the list shows only lobbies whose ladder fields equal the player's region tag.
- [ ] In Online the list hides every lobby whose ladder fields equal any tag in the region file.
- [ ] In Servers mode the "unknown ladder" refusal on join does not fire for the player's region tag. It still fires for any other unknown ladder.
- [ ] Every way of joining a lobby is covered, including joining with a password and joining through the buddy list.
- [ ] In Online, and in builds with the switch off, the existing refusals behave exactly as before.
- [ ] The ladder choice in the create-lobby dialog is hidden in Servers mode.
- [ ] After a Servers match the score screen does not report the match to the tag address and does not store the tag as the last ladder used.
- [ ] This settles PROTOCOL.md V4. If the lobby service does not carry the ladder fields, the fallback (a fixed prefix in the lobby name, stripped for display) is implemented instead and the handoff notes say so.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author, with a dedicated pass confirming Online behaviour is unchanged.

## Out of scope

The entry check on join (D3). Match start (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
