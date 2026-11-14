# Code map

Where things are in the client code on `servers-go`. Function names are given, not line numbers, because lines move. Details and evidence are in the three files under [reference/](reference/); the section numbers below point there.

Abbreviations:

| Short | Path |
|---|---|
| `GO/` | `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/` |
| `GOh/` | `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/` |
| `Core/` | `Core/GameEngine/Source/GameNetwork/` |
| `Coreh/` | `Core/GameEngine/Include/GameNetwork/` |
| `Menus/` | `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/` |

## Things that surprise people

- The old GameSpy code is compiled but never reached. `TheGameSpyInfo` and `TheGameSpyPeerMessageQueue` stay null. Do not hook anything into `GameSpy/` code paths except `LobbyUtils.cpp` and `MainMenuUtils.cpp`, which Generals Online reuses.
- The `WOL*` menu files keep their names but call the `NGMP_*` interfaces.
- `.wnd` layout files and the string file are not in the repository. New controls are made at runtime; new text uses `TheGameText->fetchOrSubstitute("GUI:Key", L"fallback")`.
- Generals Online's switches are `#define`s in `GOh/NextGenMP_defines.h`, not CMake options.
- A Debug build talks to `https://localhost:9000` and allows several instances; a Release build talks to the production host. Package B1 replaces this with our own addresses.
- This branch forked from the community source 472 commits before the earlier work's base. Some files sit elsewhere: `CommandLine.cpp` is still under `GeneralsMD/Code/GameEngine/Source/Common/`.
- To see only Generals Online's change to a file: `git diff --ignore-cr-at-eol e760b3695 servers-go -- <file>`.
- Only the `win32-vcpkg` presets build the game. The `Generals/` title does not build and is configured off.

## Service addresses and outside hosts

| What | Where |
|---|---|
| API base URL | `NGMP_OnlineServicesManager::GetAPIEndpoint`, `GO/OnlineServices_Init.cpp`; environment in `GOh/OnlineServices_Init.h` (`g_Environment`) |
| WebSocket URL | From the login answer; overridden in `NGMP_OnlineServicesManager::OnLogin` |
| STUN list | `NetworkMeshLibrary::EnsureInitialized`, `GO/NetworkMesh.cpp` |
| TURN list | `NetworkMesh::NetworkMesh`, `GO/NetworkMesh.cpp` |
| Crash reporting | `NGMP_OnlineServicesManager::InitSentry`, called from `GameEngine::init` |
| Discord | `GO/DiscordRichPresence.cpp` |
| Web links | `DoFullLoginFlow` (`GO/OnlineServices_Auth.cpp`), `ContinueUpdate`, `LaunchPatcher`, `Menus/ScoreScreen.cpp`, `OpenDiscord` |
| Full list | CLIENT_HTTP.md 1.3, CLIENT_REALTIME.md 7 |

## Login

| What | Where |
|---|---|
| Online button | `MainMenuSystem` (`Menus/MainMenu.cpp`), branch `controlID == onlineID`, calls `StartPatchCheck()` |
| Service start and version check | `StartPatchCheck`, `startOnline` in `Core/GameSpy/MainMenuUtils.cpp`; `StartVersionCheck` in `GO/OnlineServices_Init.cpp` |
| Login | `WOLLoginMenuInit` (`Menus/WOLLoginMenu.cpp`); `BeginLogin`, `DoFullLoginFlow`, `Tick`, `OnLoginComplete` in `GO/OnlineServices_Auth.cpp` |
| Hardware identifiers sent at login | `BeginLogin` and `Tick` in `GO/OnlineServices_Auth.cpp` |
| Full teardown of the online session | `NGMP_OnlineServicesManager::SetPendingFullTeardown(EGOTearDownReason::...)` |
| Sequence | CLIENT_HTTP.md 3 |

## Lobby list and lobby

| What | Where |
|---|---|
| Welcome screen, Quick Match and ladder buttons | `WOLWelcomeMenuInit`, `WOLWelcomeMenuSystem` (`Menus/WOLWelcomeMenu.cpp`) |
| List request and parsing | `NGMP_OnlineServices_LobbyInterface::SearchForLobbies` (`GO/OnlineServices_LobbyInterface.cpp`) |
| List drawing and the place to filter | `RefreshGameListBox` in `Core/GameSpy/LobbyUtils.cpp`: the `onComplete` lambda, at the `theLobbyFilter` block; rows by `insertGame` |
| Create dialog | `createGame()` in `Menus/PopupHostGame.cpp` calls `NGMP_OnlineServices_LobbyInterface::CreateLobby` |
| Join | `WOLLobbyMenuSystem`, branch `controlID == buttonJoinID` (`Menus/WOLLobbyMenu.cpp`); password path `joinGame()` in `Menus/PopupJoinGame.cpp`; both call `JoinLobby`; result in `NGMP_WOLLobbyMenu_JoinLobbyCallback` |
| Leaving the list screen | `ExitState()` in `Menus/WOLLobbyMenu.cpp` |
| HTTP request building and headers | `GO/HTTP/HTTPManager.cpp`, `GO/HTTP/HTTPRequest.cpp` |
| Lobby data | `LobbyEntry`, `LobbyMemberEntry` in `GOh/OnlineServices_LobbyInterface.h`; refreshed by `UpdateRoomDataCache` |
| Details | CLIENT_PORTING.md 1, 2 and 6; CLIENT_HTTP.md 4.12 to 4.17 and 5 |

## Staging room and match start

| What | Where |
|---|---|
| Staging screen | `WOLGameSetupMenuInit`, `WOLGameSetupMenuUpdate`, `WOLGameSetupMenuSystem` (`Menus/WOLGameSetupMenu.cpp`) |
| Accept (member) | `WOLGameSetupMenuSystem`, branch `controlID == buttonStartID` when not host |
| Start (host) | `StartPressed()` sends the connectivity check; the countdown runs in `WOLGameSetupMenuUpdate` and ends with `WebSocket::SendData_StartGame()` |
| Start message received | The WebSocket message switch in `WebSocket::Tick` (`GO/OnlineServices_RoomsInterface.cpp`), case `START_GAME`, calls `m_callbackStartGamePacket`, registered by `RegisterForGameStartPacket` in `WOLGameSetupMenuInit` |
| Match launch | `NGMPGame::startGame`, `NGMPGame::launchGame` (`GO/NGMPGame.cpp`) |
| Leaving the staging room | `PopBackToLobby()` in `Menus/WOLGameSetupMenu.cpp` |
| Map transfer flag in the slot list | `UpdateSlotList` in `GeneralsMD/Code/GameEngine/Source/GameNetwork/GUIUtil.cpp` (`willTransfer`) |
| Sequence | CLIENT_REALTIME.md 4 |

## Player-to-player connections

| What | Where |
|---|---|
| Mesh per lobby | `NetworkMesh` (`GO/NetworkMesh.cpp`), created in `JoinLobby` and `OnJoinedOrCreatedLobby` |
| Connection start | `NetworkMesh::StartConnectionSignalling`, triggered by WebSocket message 17 |
| Transport under the engine | `NextGenTransport` (`GO/NextGenTransport.cpp`), chosen in `ConnectionManager::initTransport` |
| Details | CLIENT_REALTIME.md 2 and 3 |

## Match netcode (shared engine code)

| What | Where |
|---|---|
| Player list and connections for a match | `ConnectionManager::parseUserList`, `Network::parseUserList` |
| Sending local commands | `ConnectionManager::sendLocalCommand`, `sendLocalCommandDirect`, `sendLocalGameMessage` |
| Receiving and relaying | `ConnectionManager::update`, `doRelay`, `processNetCommand` |
| Wait rule | `ConnectionManager::allCommandsReady` |
| Run-ahead | `ConnectionManager::updateRunAhead` (heavily changed by Generals Online), `Network::processRunAheadCommand` |
| Latency used for run-ahead | `ConnectionManager::getMaximumLatency`, `FrameMetrics::init` (`Core/FrameMetrics.cpp`) |
| Disconnect screen | `DisconnectManager::updateDisconnectStatus` (`Core/DisconnectManager.cpp`) |
| Command encoding | `NetCommandMsg::copyBytesForNetPacket`, `NetPacket::ConstructNetCommandMsgFromRawData` |
| Launch flags | `paramsForEngineInit` in `GeneralsMD/Code/GameEngine/Source/Common/CommandLine.cpp` |
| Per-frame engine update | `GameEngine::update` in `GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp` |
| What Generals Online changed here | CLIENT_REALTIME.md 5 |
| Hazards when a match has no mesh connections | CLIENT_REALTIME.md 8, "Seam A" constraints |

## The earlier work, to port from

On branch `servers`. The table of every file and its verdict is CLIENT_PORTING.md section 5.

| Piece | Files on `servers` | Verdict |
|---|---|---|
| Relay | `Servers/Relay/` | Copy as is |
| Region file reader | `Coreh/ServerRegions.h`, `Core/ServerRegions.cpp` | Copy as is |
| Entry check client | `Coreh/ServerEntryCheck.h`, `Core/ServerEntryCheck.cpp` | Copy as is |
| Relay match link | `Coreh/ServerLink.h`, `Core/ServerLink.cpp`, `cmake/enet.cmake`, library `core_serverlink` | Copy as is |
| Records in the connection manager | Changes in `ConnectionManager.h/.cpp`, `Network.cpp`, `DisconnectManager.cpp` | Re-apply by hand |
| Servers state and entry check dialog | `Coreh/GameSpy/ServersMode.h`, `Core/GameSpy/ServersMode.cpp` | Keep the state and entry check parts; rewrite everything that touches GameSpy |
| Launch-flag match | `Coreh/ServersDirect.h`, `Core/ServersDirect.cpp` | Header as is; source rewritten around `NGMPGame` |
| Menu changes | `Menus/*.cpp` | Rewritten at the hooks listed above |
