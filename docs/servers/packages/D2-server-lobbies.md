# D2 Menus: create and join server lobbies, list filter

State: todo
Branch: servers-go-D2

## Goal

In Servers mode, lobbies are created as server lobbies and joined with the pass, and the lobby list shows only server lobbies of the player's region. In Online mode server lobbies are hidden.

## Depends on

D1

## Read first

- DECISIONS.md D11, D12, D42, D43
- PROTOCOL.md sections 5.2 and 5.3
- SERVICE.md sections 7.2 and 10 (create, join, the `403` refusal)
- reference/CLIENT_HTTP.md sections 2.1 (request headers), 4.12, 4.13, 4.15 and 5.1
- reference/CLIENT_PORTING.md sections 1 (steps 5 to 7), 2 (lobby list) and 6 (hook table)
- On this branch, in full: `NGMP_OnlineServices_LobbyInterface::CreateLobby`, `JoinLobby`, `SearchForLobbies`; `RefreshGameListBox` and `insertGame` in `LobbyUtils.cpp`; `createGame()` in `PopupHostGame.cpp`; `NGMP_WOLLobbyMenu_JoinLobbyCallback`; how `HTTPManager` adds request headers

## May edit

- `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/OnlineServices_LobbyInterface.cpp`, `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/OnlineServices_LobbyInterface.h`
- `Core/GameEngine/Source/GameNetwork/GameSpy/LobbyUtils.cpp`
- `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/PopupHostGame.cpp`, `WOLLobbyMenu.cpp` (only the join result handling)
- `GeneralsMD/Code/GameEngine/Include/GameNetwork/ServersMode.h`, `GeneralsMD/Code/GameEngine/Source/GameNetwork/ServersMode.cpp` (small additions only)

## Must not edit

- `HTTP/HTTPManager.*`, `HTTP/HTTPRequest.*`, unless a per-request header cannot be added any other way; then only that
- `WOLGameSetupMenu.cpp` (D3, 2.1)

## Done when

- [ ] In Servers mode `CreateLobby` adds `server_mode: true` to the request and sends the pass in the `X-Servers-Pass` header. In Online mode the request is unchanged.
- [ ] In Servers mode `JoinLobby` sends the pass header.
- [ ] A `403` answer to a join shows "Your server check has expired. Return to the main menu and enter Servers again." and returns to the lobby list. A failed create in Servers mode shows the same text.
- [ ] The lobby list filter (PROTOCOL.md 5.3) is applied in the `onComplete` lambda of `RefreshGameListBox`, before sorting, in both modes: Servers shows only `LobbyType` 2 with the player's region tag; Online shows only lobbies whose `LobbyType` is not 2. The existing game-mode filter still works on top.
- [ ] `LobbyType` 2 is a named constant next to the existing lobby type values.
- [ ] In Servers mode the create dialog hides the ladder choice.
- [ ] The pass is never written to a log.
- [ ] Everything is behind `RTS_SERVERS`. With the switch off, requests and the list are exactly as before, and server lobbies are still hidden from the list. That one rule, hiding `LobbyType` 2, is compiled in every build.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Running the entry check again on join (D3).
- Match start (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
