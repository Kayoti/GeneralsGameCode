# Code map

The files that matter for Servers mode. Entries name functions instead of line numbers, so they stay valid when the code moves.

Paths starting with `GameNetwork/` are under `Core/GameEngine/Source/GameNetwork/`, with headers in `Core/GameEngine/Include/GameNetwork/`.

## Match networking (track C)

| File | Role | Key items |
|---|---|---|
| `GameNetwork/Network.cpp` | Top-level loop that ties networking to the game logic | `Network::update`, `AllCommandsReady`, `timeForNewFrame`, `RelayCommandsToCommandList`, `getExecutionFrame` |
| `GameNetwork/ConnectionManager.cpp` | Owns all connections, routing, run-ahead and frame readiness | `parseUserList` (creates connections from the slot list), `sendLocalCommand`, `sendLocalCommandDirect`, `doRelay`, `allCommandsReady`, `updateRunAhead`, `determineRouterFallbackPlan`, `disconnectPlayer`, `m_packetRouterSlot` |
| `GameNetwork/Connection.cpp` | One remote player: send queue, acknowledgements, resends | `sendNetCommandMsg`, `doSend` |
| `GameNetwork/Transport.cpp` | The UDP socket, packet CRC and XOR, in and out buffers | `init`, `doSend`, `doRecv`, `queueSend`, `setLatency`, `setPacketLoss` |
| `GameNetwork/udp.cpp` | Thin Winsock wrapper | `UDP` class |
| `GameNetwork/NetPacket.cpp` | Packs and unpacks several commands into one packet | `NetPacket`, `getCommandList` |
| `GameNetwork/NetCommandMsg.cpp` | One class per command type | `NetGameCommandMsg`, `NetFrameCommandMsg`, `NetRunAheadCommandMsg`; `copyBytesForNetPacket` writes one command in full form |
| `GameNetwork/NetPacketStructs.cpp`, `.h` | Byte layout of every command and its header tags (`T`, `R`, `F`, `P`, `C`, `D`) | `NetPacketFieldTypes`, `NetPacket::ConstructNetCommandMsgFromRawData` reads one command from raw bytes |
| `GameNetwork/NetworkUtil.cpp` | Per-command rules | `CommandRequiresDirectSend`, `CommandRequiresAck`, `IsCommandSynchronized` |
| `GameNetwork/FrameDataManager.cpp`, `FrameData.cpp` | Per-player buffer of commands for upcoming frames | `allCommandsReady`, `addNetCommandMsg` |
| `GameNetwork/DisconnectManager.cpp` | Disconnect screen, timeouts, votes | `updateDisconnectStatus`, `processDisconnectVote`, `allowedToContinue` |
| `Include/GameNetwork/NetworkDefs.h` | Constants and packet structures | `MAX_SLOTS`, `MIN_RUNAHEAD`, `MAX_FRAMES_AHEAD`, `MAX_PACKET_SIZE`, `NetCommandType`, `TransportMessage` |
| `Include/GameNetwork/NetworkInterface.h` | Interface the rest of the game uses | `NetworkInterface` |

## Online lobby (track D)

Paths starting with `Menus/` are under `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/`, with the same files under `Generals/`. Zero Hour is changed first, then replicated for Generals.

| File | Role | Key items |
|---|---|---|
| `Menus/MainMenu.cpp` | Main and Multiplayer menu | `ButtonOnline` and `ButtonNetwork` are looked up by name here. The Servers button is added beside them. |
| `GameNetwork/GameSpy/Thread/PeerThread.cpp` | Talks to the lobby service on its own thread | `peerStartListingGames` requests the lobby list with a fixed set of properties per lobby. `LADIP_STR` and `LADPORT_STR` are the ladder address fields used as the server tag. `listingGamesCallback` reads them. The ladder address is free text, the port a number. The private player message request is how the host reaches one member. |
| `GameNetwork/GameSpy/LobbyUtils.cpp` | Draws the lobby list | The loop that calls `insertGame` for each lobby is where the list filter goes. |
| `Menus/WOLLobbyMenu.cpp` | Lobby screen, including joining | The join handler already refuses a lobby with a different build fingerprint, and a lobby with an unknown ladder address. |
| `Menus/PopupHostGame.cpp` | Create-lobby dialog | Where a created lobby gets its properties, including the ladder fields. |
| `Menus/WOLGameSetupMenu.cpp` | Staging room screen | Accept, Start and leaving the lobby are handled here. |
| `GameNetwork/GameSpy/StagingRoomGameInfo.cpp` | Staging room state and match start | The place that creates `NAT` and calls `establishConnectionPaths` is where a Servers match connects to the relay instead. |
| `Menus/WOLQuickMatchMenu.cpp` | Quick Match | Not available in Servers mode. |
| `Menus/ScoreScreen.cpp` | Score screen after a match | Reports results to the ladder address when the ladder port is not 0, and stores the last ladder used. |
| `GameNetwork/GUIUtil.cpp` (per title, under `GeneralsMD/Code/GameEngine/Source/`) | Staging room button states | One of the two places that decide whether a player without the map may accept. The other is the Start handler in `Menus/WOLGameSetupMenu.cpp`. |
| `.wnd` layout files | Screen layouts | Not in the repository. They ship in the game's data archives. The Servers button needs an edited Multiplayer menu layout added alongside the code. |

## Not used in Servers mode (reference only)

| File | Role |
|---|---|
| `GameNetwork/NAT.cpp`, `FirewallHelper.cpp` | Pairwise UDP path negotiation before an Online match. Never edited. |
| `GameNetwork/LANAPI*.cpp` | LAN lobby. Never edited. |
| `GameNetwork/GameInfo.cpp` | `GameInfo` and `GameSlot`: the slot list, map and options of a match. Read by track C, not edited. |

## Testing aids

| File | Role |
|---|---|
| `Core/GameEngine/Source/GameClient/ClientInstance.cpp` | Several game instances on one PC. Enabled with the `RTS_MULTI_INSTANCE` build define. |
| `GameNetwork/IPEnumeration.cpp` | Gives each instance its own `127.x.x.x` address. |
| `Core/GameEngine/Source/Common/CommandLine.cpp` | Launch flags: `-win`, `-xres`, `-yres`, `-nologo`, `-quickstart`, `-NetCRCInterval`. New Servers flags go here. |
| `GameNetwork/Transport.cpp` | Latency and packet loss injection, debug builds only. |

## Build

| File | Role |
|---|---|
| `CMakePresets.json` | Build presets. All are Win32. |
| `cmake/config-*.cmake` | Build options. The Servers switch is added here in package 0.2. |
| `cmake/gamespy.cmake` | Pulls the GameSpy SDK. |
| `.clang-tidy`, `scripts/run-clang-tidy.py` | Lint configuration and runner. |
