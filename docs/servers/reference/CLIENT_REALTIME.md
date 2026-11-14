# Generals Online client: real-time side (WebSocket, P2P mesh, match transport)

Source: Generals Online client at commit `6ff404436`. Paths below are abbreviated:

- `GO/` = `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/`
- `GOh/` = `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/`
- `Core/` = `Core/GameEngine/Source/GameNetwork/`, `Coreh/` = `Core/GameEngine/Include/GameNetwork/`
- `Menus/` = `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/`

Everything here comes from reading the client code. Server behaviour is inferred from what the client sends and expects; such points are marked "not verified".

Note on file locations: most of the `WebSocket` class (constructor, `Connect`, `Tick`, `Send`, `Disconnect`, reconnect logic, the whole receive dispatcher and the JSON message structs) is implemented in `GO/OnlineServices_RoomsInterface.cpp`, not in `OnlineServices_Init.cpp`. `OnlineServices_Init.cpp` holds the `SendData_*` helpers, `OnLogin`, `GetAPIEndpoint`, `InitSentry`. `GOh/NetworkPacket.h` is an empty file (0 lines). `NetworkBitstream.*` (`CBitStream`, `MemoryBuffer`) is not used by any live code path (only referenced by the unused `QueuedGamePacket` struct in `NetworkMesh.h`).

---

## 1. WebSocket

### 1.1 URL

- The URL comes from the login response: `AuthResponse::ws_uri` (JSON field `ws_uri`), parsed in `NGMP_OnlineServices_AuthInterface::Tick` (endpoint `CheckLogin`) and in the refresh-token login path (endpoint `LoginWithToken`), both in `GO/OnlineServices_Auth.cpp`. It is passed to `OnLoginComplete(ELoginResult::Success, authResp.ws_uri.c_str())`.
- `NGMP_OnlineServicesManager::OnLogin` (`GO/OnlineServices_Init.cpp`) overrides it with the hard-coded `"wss://api-ru.playgenerals.online/ws"` when `Settings.Network_UseAlternativeEndpoint()` is true; otherwise it uses `szWSAddr` unchanged.
- HTTP API base is separate: `NGMP_OnlineServicesManager::GetAPIEndpoint` builds `https://api.playgenerals.online/env/prod/contract/1/{endpoint}` (PROD), `https://api-ru.playgenerals.online/env/prod/contract/1/{endpoint}` (PROD + alternative endpoint), `https://api.playgenerals.online:2087/env/test/contract/1/{endpoint}` (`USE_TEST_ENV`), `https://localhost:9000/env/dev/contract/1/{endpoint}` (`_DEBUG`). Environment selection is compile-time (`g_Environment` in `GOh/OnlineServices_Init.h`).

### 1.2 When it connects

1. Login succeeds (`OnLoginComplete`) -> `AnticheatPlugInterface::Authenticate()` -> `NGMP_OnlineServicesManager::OnLogin`.
2. `OnLogin` calls `NetworkMeshLibrary::EnsureInitialized(userID)` (GameNetworkingSockets init, see 2.x), destroys any previous socket, creates `std::make_shared<WebSocket>()` and calls `WebSocket::Connect(url, false /*bIsReconnect*/, cb)`.
3. The connected callback fetches friends/block lists (`GetFriendsList`, `GetBlockList`) and then `GoToDetermineNetworkCaps` (HTTP `MOTD`). That function fails the whole login (`ELoginResult::Failed`) if `pWS->IsConnected()` is false. So the WebSocket is mandatory for login.

### 1.3 Transport / auth

`WebSocket::Connect` (`GO/OnlineServices_RoomsInterface.cpp`):

- libcurl `CURLOPT_CONNECT_ONLY = 2` (curl WebSocket mode), driven through a `curl_multi` handle; HTTP version from `Settings.Network_GetHTTPVersionForCurl()` (comment: HTTP/1 "bypasses DPI" better).
- TLS: release uses `CURLSSLOPT_NATIVE_CA`, verify peer + host. `_DEBUG` disables verification.
- Headers: `Authorization: Bearer <session token>` (`AuthInterface::GetAuthToken()`), and `is-reconnect: true|false`.
- No sub-protocol, no query string auth, no first "hello" message: authentication is only the bearer header on the upgrade request.

### 1.4 Framing

- Outbound: every message is a JSON object serialized with nlohmann `dump()`, queued by `WebSocket::Send` (thread-safe, max 256 queued, drops oldest) and flushed in `WebSocket::Tick` with `curl_ws_send(..., CURLWS_BINARY)`. So the client sends JSON text inside **binary** frames.
- Inbound: only frames with `CURLWS_TEXT` are parsed. `CURLWS_BINARY` frames are ignored ("noop"). Fragmented text frames are reassembled in `m_vecWSPartialBuffer` (cap 2 MB).
- Every JSON message has `msg_id` (integer, `EWebSocketMessageID`). Base parse via `WebSocketMessageBase`; unknown ids are logged ("Unhandled WebSocketMessage").
- `WebSocket::Tick` calls `curl_ws_recv` **once per tick** with a 32784-byte stack buffer, so the client consumes at most one frame (or frame fragment) per game-loop tick. A server that bursts many small messages will see them drained slowly.
- Structs declared with `NLOHMANN_DEFINE_TYPE_INTRUSIVE` (not `_WITH_DEFAULT`) throw on any missing key, which makes the whole message be dropped; this matters for `NETWORK_SIGNAL`, `NETWORK_CONNECTION_START_SIGNALLING`, `ANTICHEAT_MESSAGE`, `START_GAME` and others (see table).
- Byte payloads (`std::vector<uint8_t>`) are JSON arrays of integers 0..255, not base64.

### 1.5 Keep-alive and timeout

- Every `m_timeBetweenUserPings = 1000` ms, `WebSocket::Ping` sends both a WebSocket PING control frame (payload `"wsping"`) and a JSON `{"msg_id":14}` (`PING`).
- Any inbound frame (not just `PONG`) refreshes `m_lastPong`. If nothing arrives for `m_timeForWSTimeout = 20000` ms the client marks itself disconnected and calls `BeginReconnect`.
- Inbound `CURLWS_PING` frames are not answered by client code (comment "TODO_NGMP"); curl may auto-respond (not verified).
- Server-originated `WS_KEEPALIVE` (42) is not a liveness probe from the client's point of view: the client answers `WS_KEEPALIVE_CLIENT` (43) with the list of every module loaded in the process (`GetLoadedModules`, path + image size). This is an anti-tamper report.

### 1.6 Reconnect

`WebSocket::BeginReconnect`, `UpdateReconnect`, `ComputeReconnectDelay`, `EndReconnect`, `RestoreSessionState` (`GO/OnlineServices_RoomsInterface.cpp`):

- Triggered by `CURLE_RECV_ERROR` from `curl_ws_recv` or by the 20 s silence timeout. In release builds both also send a Sentry event (`WEBSOCKET_DISCONNECT_ERROR` / `WEBSOCKET_DISCONNECT_TIMEOUT`) when `TheNGMPGame != nullptr`.
- A received `CURLWS_CLOSE` frame does **not** reconnect: it calls `SetPendingFullTeardown(LOST_CONNECTION)` immediately (full logout). A failed *initial* connect also tears down immediately.
- Backoff: first retry 0.5-3 s, later 60-100 % of a doubling step 2 s..10 s (`m_reconnectBackoffBase 2000`, `m_reconnectBackoffCap 10000`); an attempt in flight is abandoned after `m_reconnectAttemptTimeout 15000` ms.
- Reconnect attempts send `is-reconnect: true`. If the upgrade fails with HTTP **205**, the client concludes "session gone" (`m_bFreshSession = true`) and reconnects as a new session (`is-reconnect: false`), then runs `RestoreSessionState` (friends, block list, realtime subscription if the overlay is open, re-fetch rooms and rejoin the previous room by id).
- Out of a match, the client gives up after `m_reconnectMenuWindow = 30000` ms and tears down. During a match (`TheNGMPGame->isGameInProgress()`) it keeps retrying indefinitely, and a fresh-session reconnect is postponed until `TheNGMPGame` is gone. Header comment: the 30 s window "should not exceed the service's abandoned-session hold, else reconnects get 205".

### 1.7 Message table

Direction: C>S client to server, S>C server to client. "req" = required key (missing key drops the message). Unless stated, handler code is the `switch (msgID)` in `WebSocket::Tick`.

| id | name | dir | JSON fields (besides `msg_id`) | client action / sender function |
|---|---|---|---|---|
| 1 | NETWORK_ROOM_CHAT_FROM_CLIENT | C>S | `message` string (UTF-8; dumped with ensure_ascii), `action` bool | `WebSocket::SendData_RoomChatMessage` via `RoomsInterface::SendChatMessageToCurrentRoom` |
| 2 | NETWORK_ROOM_CHAT_FROM_SERVER | S>C | req: `message` string, `action` bool, `admin` bool, `name_change` bool | formats `[hh:mm] msg`, `DetermineColorForChatMessage`, `RoomsInterface::m_OnChatCallback` |
| 3 | NETWORK_ROOM_CHANGE_ROOM | C>S | `room` int (-1 = leave), optional `request_id` uint64 | `SendData_JoinNetworkRoom` / `SendData_LeaveNetworkRoom`; called from `RoomsInterface::JoinRoom` (10 s client timeout in `RoomsInterface::Tick`) |
| 4 | NETWORK_ROOM_MEMBER_LIST_UPDATE | S>C | `members` array of `{UserID int64, Name string, IsAdmin bool}`; optional `selected_room_id` int, `effective_room_id` int, `rejected_room_id` int, `room_selection_error` string, `request_id` uint | `RoomsInterface::OnRosterUpdated` (resolves pending room change, fires `m_RoomChangedCallback`, `m_RosterNeedsRefreshCallback`) |
| 5 | NETWORK_ROOM_MARK_READY | C>S | `ready` bool | `SendData_MarkReady`, called by `LobbyInterface::ApplyLocalUserPropertiesToCurrentNetworkRoom` (host always `true`). Used for *lobby* ready state despite the name |
| 6 | LOBBY_CURRENT_LOBBY_UPDATE | S>C | none | `LobbyInterface::UpdateRoomDataCache(nullptr)` -> HTTP GET `Lobby/{id}` (the "dirty, refetch" notice for the current lobby) |
| 7 | NETWORK_ROOM_LOBBY_LIST_UPDATE | S>C | none | `LobbyInterface::SetLobbyListDirty()` (lobby browser refetch) |
| 8 | ANTICHEAT_MESSAGE | both | req: `target_user_id` int64, `payload` byte array | C>S: `SendData_ACMessage` from the plugin's `SetSendMessageViaTransportCallback` lambda (`PluginInterfaces.cpp`). S>C: `AnticheatPlugInterface::AC_NetworkMessageArrived(acMsg.target_user_id, ...)` - the received `target_user_id` is handed to the plugin as the *peer* id, so the server presumably rewrites it to the sender's id (not verified) |
| 9 | PLAYER_NAME_CHANGE | C>S | `name` string | `SendData_ChangeName` (`/name`, `/nick` in `Menus/WOLLobbyMenu.cpp`) |
| 10 | LOBBY_ROOM_CHAT_FROM_CLIENT | C>S | `message`, `action` bool, `announcement` bool, `show_announcement_to_host` bool | `SendData_LobbyChatMessage` (lobby chat and host countdown announcements via `SendAnnouncementMessageToCurrentLobby`) |
| 11 | LOBBY_CHAT_FROM_SERVER | S>C | req: `message`, `action`, `announcement`, `show_announcement_to_host`, `user_id` int64 | colour by sender slot, `LobbyInterface::m_OnChatCallback` |
| 12 | NETWORK_SIGNAL | both | req: `target_user_id` int64, `payload` byte array | C>S: `SendData_Signalling` from `CSignalingClient::Poll`. S>C: `PushPendingSignal(payload)`; `target_user_id` is only logged. See section 2 |
| 13 | START_GAME | both | C>S: none. S>C: req `screenshot_url` string | C>S: `SendData_StartGame` (host, end of countdown). S>C: `SetScreenshotS3URI_StartMatch(url)` then always `LobbyInterface::m_callbackStartGamePacket()` (starts the match even if parse failed) |
| 14 | PING | C>S | none | `WebSocket::Ping`, every 1 s |
| 15 | PONG | S>C | none | updates `m_lastPong` |
| 16 | PROBE | S>C | req: `url` string | `CaptureScreenshotForProbe(SCREENSHOT_TYPE_GAMEPLAY, url)` (JPEG upload by HTTP PUT to `url` in `NGMP_OnlineServicesManager::Tick`, only if `ServiceConfig.do_probes`), reply `PROBE_RESP` |
| 17 | NETWORK_CONNECTION_START_SIGNALLING | S>C | req: `lobby_id` int64, `user_id` int64, `preferred_port` uint16, `middleware_id` string | prefetch stats (`findPlayerStatsByID`), then `NetworkMesh::StartConnectionSignalling(middleware_id, user_id, preferred_port)` |
| 18 | NETWORK_CONNECTION_DISCONNECT_PLAYER | S>C | req: `lobby_id`, `user_id` | if `lobby_id` matches current lobby: `NetworkMesh::DisconnectUser(user_id)` |
| 19 | NETWORK_CONNECTION_CLIENT_REQUEST_SIGNALLING | C>S | `target_user_id` int64 | `SendData_RequestSignalling`, only from the retry path in `OnSteamNetConnectionStatusChanged` |
| 20 | MATCHMAKING_ACTION_JOIN_PREARRANGED_LOBBY | S>C | req: `lobby_id` | `InvokeMatchmakingMatchFoundCallback`, `JoinLobby` (placeholder map path `Maps\Alpine Assault\Alpine Assault.map`), `InvokeMatchmakingMessageCallback` |
| 21 | MATCHMAKING_ACTION_START_GAME | S>C | none | `InvokeMatchmakingStartGameCallback` -> `TheNGMPGame->startGame(0)` (`Menus/WOLQuickMatchMenu.cpp`) |
| 22 | MATCHMAKING_MESSAGE | S>C | req: `message` | `InvokeMatchmakingMessageCallback` |
| 23 | START_GAME_COUNTDOWN_STARTED | C>S | none | `SendData_CountdownStarted` from `NGMPGame::StartCountdown` |
| 24 | LOBBY_REMOVE_PASSWORD | C>S | none | `SendData_RemoveLobbyPassword` (`Menus/WOLGameSetupMenu.cpp`) |
| 25 | LOBBY_CHANGE_PASSWORD | C>S | `new_password` string | `SendData_ChangeLobbyPassword` |
| 26 | FULL_MESH_CONNECTIVITY_CHECK_HOST_REQUESTS_BEGIN | C>S | none | `SendData_StartFullMeshConnectivityCheck` (host presses Start) |
| 27 | FULL_MESH_CONNECTIVITY_CHECK_RESPONSE | both | S>C: optional `mesh_check_id` int64, `attempt` int. C>S reply: `mesh_check_id`, `attempt`, `connectivity_map` int64[] (peers in `CONNECTED_DIRECT`), `connecting_map` int64[] (peers in `CONNECTING_DIRECT` or `FINDING_ROUTE`) | built from `NetworkMesh::GetAllConnections()` |
| 28 | FULL_MESH_CONNECTIVITY_CHECK_RESPONSE_COMPLETE_TO_HOST | S>C | `mesh_complete` bool (req), optional `reason` string, `missing_connections` `[{source_user_id, target_user_id}]` when incomplete | ignored if lobby changed; calls `m_cbOnConnectivityCheckComplete(bool, list, reason)` (callback in `Menus/WOLGameSetupMenu.cpp`) |
| 29 | SOCIAL_NEW_FRIEND_REQUEST | S>C | req: `display_name` | `SocialInterface::InvokeCallback_NewFriendRequest` |
| 30 | SOCIAL_FRIEND_CHAT_MESSAGE_CLIENT_TO_SERVER | C>S | `target_user_id`, `message` | `SendData_FriendMessage` (`Menus/WOLBuddyOverlay.cpp`) |
| 31 | SOCIAL_FRIEND_CHAT_MESSAGE_SERVER_TO_CLIENT | S>C | req: `source_user_id`, `target_user_id`, `message` | `SocialInterface::OnChatMessage` |
| 32 | SOCIAL_FRIEND_ONLINE_STATUS_CHANGED | S>C | req: `display_name`, `online` bool | `SocialInterface::OnOnlineStatusChanged` |
| 33 | SOCIAL_SUBSCRIBE_REALTIME_UPDATES | C>S | none | `SendData_SubscribeRealtimeUpdates` (`OnlineServices_SocialInterface.cpp`) |
| 34 | SOCIAL_UNSUBSCRIBE_REALTIME_UPDATES | C>S | none | `SendData_UnsubscribeRealtimeUpdates` |
| 35 | SOCIAL_FRIENDS_OVERALL_STATUS_UPDATE | S>C | req: `num_online` int, `num_pending` int | notification box, `RegisterInitialPendingRequestsUponLogin` |
| 36 | SOCIAL_FRIEND_FRIEND_REQUEST_ACCEPTED_BY_TARGET | S>C | req: `display_name` | `SocialInterface::OnFriendRequestAccepted` |
| 37 | SOCIAL_FRIENDS_LIST_DIRTY | S>C | none | `updateBuddyInfo(true)` |
| 38 | SOCIAL_CANT_ADD_FRIEND_LIST_FULL | S>C | none | notification box |
| 39 | PROBE_RESP | C>S | `timestamp` string (always `"0"`) | reply to PROBE |
| 40 | AC_REGISTER_PLAYER | S>C | req: `user_id` int64, `mwid` string | `AnticheatPlugInterface::RegisterPlayer(mwid, user_id)` |
| 41 | AC_DEREGISTER_PLAYER | S>C | req: `user_id`, `mwid` | `AnticheatPlugInterface::DeregisterPlayer` |
| 42 | WS_KEEPALIVE | S>C | none | reply 43 |
| 43 | WS_KEEPALIVE_CLIENT | C>S | `resp` = array of `[path string, size string]` for every loaded module | built by `GetLoadedModules()` |
| 44 | MATCHMAKING_ACTION_REQUEUE | S>C | none | `ResetForMatchmakingRequeue`, `InvokeMatchmakingRequeueCallback` |
| 45 | MATCHMAKING_ACTION_SETUP_PROGRESS | S>C | `timeout_ms` int (clamped 1000..120000; ignored if <= 0), optional `countdown_ms` int (-1 if absent) | `InvokeMatchmakingSetupProgressCallback` |
| 46 | MODERATION_NOTICE | S>C | `action_type` (`"ban"`, `"kick"`, `"rate_limit"`), `reason`, `scope_type` (all defaulted) | `HandleModerationNotice` (`OnlineServices_Moderation.cpp`): ban/kick -> `SetPendingFullTeardown(MODERATION_BAN/KICK)` + dialog |
| 47 | MODERATION_COMMAND | - | - | enum value only; no sender or handler in client code |
| 48 | MODERATION_COMMAND_RESULT | S>C | `request_id` uint64, `success` bool, `error_code`, `message` (defaulted) | log only |

Not over the WebSocket (HTTP, for reference): lobby create/join/leave/settings (`Lobbies`, `Lobby/{id}` PUT/GET/DELETE/POST), TURN credentials (in lobby create/join responses), `ConnectionOutcome`, `ServiceConfig`, `ProvideMWToken`, stats, matchmaking, social, `VersionCheck`, `MOTD`. All HTTP requests carry `Authorization: Bearer <session token>` (`HTTPRequest.cpp`) unless `bDisableServiceAuth`.

---

## 2. Player-to-player connection setup

### 2.1 Library and globals

- GameNetworkingSockets (static, `GameNetworkingSockets::GameNetworkingSockets_s`, vcpkg port from the custom registry `https://github.com/tintinhamans/GNS`, features `ice`, `webrtc`; see `vcpkg.json`).
- `NetworkMeshLibrary::EnsureInitialized(userID)` (`GO/NetworkMesh.cpp`), called once at login from `OnLogin` and again (no-op) by `NetworkMesh::NetworkMesh`:
  - local identity = `SteamNetworkingIdentity::SetGenericString(to_string(userID))`; peers are identified by their decimal user id string.
  - `GameNetworkingSockets_Init`.
  - global STUN list: `"stun:stun.playgenerals.online:53,stun:stun.playgenerals.online:3478,stun:stun.l.google.com:19302"` (`k_ESteamNetworkingConfig_P2P_STUN_ServerList`).
  - global status callback `OnSteamNetConnectionStatusChanged`.
- `NetworkMeshLibrary::Tick` runs `SteamNetworkingSockets()->RunCallbacks()` every `NGMP_OnlineServicesManager::Tick` (skipped if the anti-cheat plugin owns transport).

### 2.2 Per-lobby mesh

`NetworkMesh::NetworkMesh` (one mesh per lobby, owned by `NGMP_OnlineServices_LobbyInterface::m_pLobbyMesh`, survives into the match, destroyed in `LeaveCurrentLobby` / `ResetForMatchmakingRequeue`):

- TURN server list hard-coded: `m_strTurnServerList = "turn:turn.playgenerals.online:53,turn:turn.playgenerals.online:3478"` (comment: no `?transport=udp`).
- TURN user/pass come from the lobby interface (`GetLobbyTurnUsername`, `GetLobbyTurnToken`) and are duplicated to match the two servers: `m_strTurnUsernameString = "u,u"`, `m_strTurnTokenString = "t,t"` (`k_ESteamNetworkingConfig_P2P_TURN_UserList` / `_PassList`).
- `m_iceEnable` = `k_nSteamNetworkingConfig_P2P_Transport_ICE_Enable_Relay` if `g_bForceRelay || ServiceConfig.relay_all_traffic`, else `..._ICE_Enable_All`.
  - `relay_all_traffic` (service config, default false): every connection in this lobby uses TURN relay only.
  - `g_bForceRelay` (`NetworkMesh.cpp` global): set by chat command `/forcerelay`, cleared by `/allowrelay` (`Menus/WOLLobbyMenu.cpp`). `/forcerelay` also overwrites `TheWritableGlobalData->m_exeCRC` with `123456` so the player only matches lobbies created with the same flag. Takes effect for meshes created afterwards.
- `m_iceImplementation` = `ServiceConfig.ice_implementation` (0 library default, 1 native, 2 WebRTC; default 2).
- `m_pSignaling = new CSignalingClient(SteamNetworkingSockets())`.
- Unless the AC plugin owns transport: `CreateListenSocketP2P(0 /*virtual port*/, opts)` with `SymmetricConnect=1`, ICE enable/implementation, TURN server/user/pass lists.

### 2.3 TURN credential delivery

- Join: `LobbyInterface::JoinLobby` creates the mesh (if none), calls `NetworkMesh::AwaitTurnCredentials()` (start-signalling requests are queued in `m_vecSignallingAwaitingTurn`), then HTTP PUT `Lobby/{id}` with body `{preferred_port:0, anticheat_id:int, has_map:bool, password?:string}`. Response JSON `JoinLobbyResponse {success, turn_username, turn_token}` (strings). `NetworkMesh::SetTurnCredentials` stores them, pushes them onto the listen socket with `SetConfigValue(..., k_ESteamNetworkingConfig_ListenSocket, ...)`, and replays held `StartConnectionSignalling` calls. HTTP status: 200 ok, 401 bad password, 406 full, 417 anti-cheat mismatch, 404 not found.
- Create: `LobbyInterface::CreateLobby` -> HTTP PUT `Lobbies`; response `CreateLobbyResponse {result:int (1=SUCCEEDED), lobby_id:int64, turn_username, turn_token}`. Mesh is created afterwards in `OnJoinedOrCreatedLobby` and reads the already stored credentials.
- Credential format (static long-term vs. time-limited HMAC) cannot be told from the client; it passes them through to GNS unchanged. Not verified.

### 2.4 Sequence from "player joins lobby" to "all connected"

1. Joiner: `JoinLobby` -> `GetAndParseServiceConfig` (HTTP `ServiceConfig`) -> mesh + `AwaitTurnCredentials` -> PUT `Lobby/{id}` -> `SetTurnCredentials` -> `TheNGMPGame = new NGMPGame()` -> `OnJoinedOrCreatedLobby` (`AnticheatPlugInterface::BeginSession`, `UpdateRoomDataCache`) -> GET `Lobby/{id}` fills `m_CurrentLobby.members` (`UserID, DisplayName, IsReady, Port, Side, Color, Team, StartingPosition, HasMap, SlotState, SlotIndex, Region, MiddlewareUserID`).
2. Server (inferred): sends `NETWORK_CONNECTION_START_SIGNALLING` `{lobby_id, user_id=<peer>, preferred_port, middleware_id}` for each pair. The client code requires both sides to receive it (inference, not verified): an incoming connection is only accepted in `OnSteamNetConnectionStatusChanged` (`k_ESteamNetworkingConnectionState_Connecting`, listen-socket branch) if a `PlayerConnection` with that handle already exists (`connectionID != -1`), which only happens when the local side also called `ConnectP2PCustomSignaling` for that identity and GNS symmetric-connect merged the two. A one-sided start would leave the inbound attempt unaccepted.
3. Each client in `NetworkMesh::StartConnectionSignalling`: if credentials pending, queue; drop any existing connection to that user (also calls `ConnectionManager::disconnectPlayer(int64)` which is a no-op, see 5); skip self; build options (`LocalVirtualPort 0` if needed, `SymmetricConnect 1`, ICE enable/impl, TURN lists); remote virtual port = `preferred_port` if `ServiceConfig.use_mapped_port` (default true) else 0. Clients always send `preferred_port: 0` on create/join, so the server should send 0. `ConnectP2PCustomSignaling(...)` -> `PlayerConnection(remoteUserID, hConn)` (`m_SignallingAttempts` carried over +1). The `PlayerConnection` constructor calls `NetworkMesh::RegisterConnectivity` (HTTP `ConnectionOutcome`, outcome 0).
4. GNS produces opaque signal blobs: `CSignalingClient::ConnectionSignaling::SendSignal` -> queue (max 128, drop oldest) -> `CSignalingClient::Poll` (from `NetworkMesh::Tick`, i.e. `LobbyInterface::Tick`) -> `WebSocket::SendData_Signalling(target_user_id, payload)` -> JSON `{"msg_id":12,"target_user_id":<peer>,"payload":[...bytes...]}`.
5. Server forwards to the target. On receipt `PushPendingSignal(payload)` (queue max 256); `CSignalingClient::Poll` drains and calls `ISteamNetworkingSockets::ReceivedP2PCustomSignal(payload, ..., &context)`. The client ignores the inbound `target_user_id`; GNS takes the sender identity from inside the blob (code comment in `Poll`: "outbound msg doesnt need sender ID ... everything else is included in the payload"). `Context::SendRejectionSignal` silently drops rejections.
6. Inbound connection accept: the remote generic-string identity must equal a `member.user_id` in `m_CurrentLobby.members` (HTTP lobby data), otherwise `CloseConnection(..., "Player is not in lobby (Rejected)")`. So the lobby GET must already list the peer when signalling starts (the server's `LOBBY_CURRENT_LOBBY_UPDATE` should precede or accompany start-signalling; not verified).
7. State transitions update `PlayerConnection::UpdateState` (`CONNECTING_DIRECT`, `FINDING_ROUTE`, `CONNECTED_DIRECT`), each reported by `NetworkMesh::UpdateConnectivity`. `CONNECTED_DIRECT` is used for relayed (TURN) links too; "direct" vs "relayed" is only the `direct` flag from `k_nSteamNetworkConnectionInfoFlags_Relayed`.
8. Before Start, host asks the server to verify the full mesh (messages 26/27/28). Only `mesh_complete:true` lets the host start the countdown (`Menus/WOLGameSetupMenu.cpp`).

### 2.5 What the server must do for signalling (minimum)

- Forward `NETWORK_SIGNAL` from user A to user `target_user_id`, keeping it a text frame JSON with keys `msg_id`, `target_user_id` (any int64; the client does not use it but the key must be present), `payload` (byte array, untouched). The payload is opaque GNS data; the server does not need to understand it. Scope to lobby members (security choice; client does not check).
- Send `NETWORK_CONNECTION_START_SIGNALLING` with all four keys to both members of every pair when a member joins (and in response to `NETWORK_CONNECTION_CLIENT_REQUEST_SIGNALLING`).
- Issue per-lobby TURN credentials valid on the TURN servers listed in `NetworkMesh::NetworkMesh` (or change that hard-coded list).
- Implement the full-mesh check (26 -> 27 to all -> collect -> 28 to host) and `NETWORK_CONNECTION_DISCONNECT_PLAYER` on leave.
So yes, the signalling core is "forward opaque blobs between two user ids", plus pairing triggers, TURN credentials and the mesh check.

### 2.6 Ports, IPv4/IPv6

- No UDP ports are configured by the client; GNS binds ephemeral local ports. "Ports" in the protocol (`preferred_port`, `Port`) are GNS P2P *virtual* ports, always 0 in practice.
- STUN ports 53 and 3478 on `stun.playgenerals.online`, plus Google STUN 19302; TURN ports 53 and 3478 on `turn.playgenerals.online`.
- No explicit IPv4/IPv6 restriction in client code; `PlayerConnection::IsIPV4` reports `m_addrRemote.IsIPv4()` to the service. Whether GNS gathers IPv6 candidates with this build: not verified.

### 2.7 Retry rules (`OnSteamNetConnectionStatusChanged`)

On `ClosedByPeer` / `ProblemDetectedLocally` for a known connection:
- `numSignallingAttempts = 2`.
- `bShouldRetry = retry_signalling && ((inMatch && !peerLeft) || (!weJoinedLater && !peerLeft) || attempts < 2)` where `weJoinedLater` uses `LobbyInterface::JoinedAfter` / `IsJoinOrderKnown`, `peerLeft` = no longer in lobby members, `inMatch` = `TheGameLogic->isInInternetGame()`.
- `bWasError` = problem detected locally or end reason != `k_ESteamNetConnectionEnd_App_Generic`. `PlayerConnection::SetDisconnected(bWasError, mesh, bShouldRetry && bWasError)`.
- On error + retry: only the side with the **higher user id** sends `NETWORK_CONNECTION_CLIENT_REQUEST_SIGNALLING {target_user_id: peer}`; the lower one waits.
- On error without retry: host keeps the lobby and only drops the peer; a non-host calls `QueueCannotConnectToLobby()` (leaves the lobby on next tick via `m_OnCannotConnectToLobbyCallback`).
- `retry_signalling` comes from `ServiceConfig` (default false in the struct; the comment in `GetAndParseServiceConfig` implies production sets it true; not verified).

### 2.8 `ConnectionOutcome` reports

HTTP POST `ConnectionOutcome` (`NetworkMesh::RegisterConnectivity`, `NetworkMesh::UpdateConnectivity`), response ignored:
`{"target": <peer user id>, "direct": bool, "outcome": <EConnectionState int>, "ipv4": bool}`.
`EConnectionState` (`GOh/PluginInterfaces.h`): 0 NOT_CONNECTED, 1 CONNECTING_DIRECT, 2 FINDING_ROUTE, 3 CONNECTED_DIRECT, 4 CONNECTION_FAILED, 5 CONNECTION_DISCONNECTED. Sent on `PlayerConnection` creation (always 0/false/true) and on every `UpdateState`.

---

## 3. `NextGenTransport`

### 3.1 How it plugs in

- `Transport` (`Coreh/Transport.h`) is now an abstract base (`init`, `reset`, `update`, `doRecv`, `doSend`, `queueSend`, `allowBroadcasts`, plus shared `m_outBuffer[MAX_MESSAGES]`, `m_inBuffer[MAX_MESSAGES]`, statistics and `isGeneralsPacket`). The legacy UDP implementation is `UDPTransport`.
- `ConnectionManager::initTransport` (`Core/ConnectionManager.cpp`, `#if defined(GENERALS_ONLINE)`): `new NextGenTransport` if `TheLAN == nullptr`, else `new UDPTransport`. Called from `Network::initTransport` <- `NGMPGame::launchGame`.
- `NextGenTransport::init` is a no-op (returns TRUE); no sockets are opened.
- Engine call sites: `ConnectionManager::update` -> `m_transport->doRecv()` ... `doRelay()` ... `Connection::doSend()` (each `Connection` calls `m_transport->queueSend(addr, port, data, len)`) ... `m_transport->doSend()`; `ConnectionManager::flushConnections` -> `doSend()`. `update()` is not called by the engine for this transport (`Network::liteupdate` -> `ConnectionManager::update`).

### 3.2 Slot to user mapping

- `NGMPGame::UpdateSlotsFromCurrentLobby` calls `slot->setState(state, name, pLobbyMember.m_SlotIndex)`: the slot's "IP" is set to the **slot index** (comment: "we dont have an IP, so just use player index for ip"), and `NGMPGameSlot::m_userID = user_id` (AI: -1).
- `ConnectionManager::parseUserList` creates `User(name, slot->getIP(), slot->getPort())`, so a `Connection`'s destination address is the slot index.
- `NextGenTransport::doSend`: `TheNGMPGame->getSlot(m_outBuffer[i].addr)` -> `NGMPGameSlot::m_userID` -> `NetworkMesh::SendGamePacket(buf, len, userID)`. Port is ignored.
- Receive: sender identity is not recorded (`m_inBuffer[i].addr/port` stay 0). The engine derives the player from the command's own `getPlayerID()` in `ConnectionManager::doRelay` / `processNetCommand`; there is no check that the packet came from that player's connection.
- `NGMPGame::startGame` sets local IP to `1337` ("dont care anymore"); `launchGame` sets port `8888`. Both only satisfy `ConnectionManager::update`'s non-zero address check.

### 3.3 Wire format, channels, reliability

- Wire payload to GNS: `[1 byte ENetworkChannel][TransportMessageHeader (crc u32, magic u16, packed, 6 bytes)][engine packet bytes]`. `ENetworkChannel` (`GOh/NGMP_include.h`): `NETWORK_CHANNEL_GAME = 0`, `NETWORK_CHANNEL_AC = 1`. CRC and magic are computed in `NextGenTransport::queueSend` exactly like UDP (`Transport::isGeneralsPacket` validates on receive).
- `PlayerConnection::SendGamePacket`: `SendMessageToConnection(..., k_nSteamNetworkingSend_Reliable | k_nSteamNetworkingSend_AutoRestartBrokenSession)`. Only if that call fails does it resend with flags from `ServiceConfig.network_send_flags` (0..4 map to Unreliable / UnreliableNoNagle / UnreliableNoDelay / Reliable / ReliableNoNagle) - and that fallback sends the buffer **without** the channel byte, which the receiver would then misparse. The function returns `k_EResultFail` (2) on success; callers treat any value >= 0 as success, negative values (-2 no connection, -3 bad buffer) as failure.
- `NetworkMesh::Flush` (called every `LobbyInterface::Tick`) calls `FlushMessagesOnConnection` on all connections when `network_do_immediate_flush_per_frame` (default true).
- Receive: `NextGenTransport::doRecv` iterates every `PlayerConnection`, `ReceiveMessagesOnConnection` (max 255), strips the channel byte, routes AC channel to `AC_NetworkMessageArrived`, validates header/size/CRC, stores in the first free `m_inBuffer` slot (`m_inBufferOccupied[]` tracks zero-length).
- Lobby phase: while `TheNGMPGame` exists and the game is not in progress, `PlayerConnection::LiteUpdateForAC` (from `NetworkMesh::Tick`) drains connections and **discards** any non-AC message.
- Plugin path (`DoesACPluginProvideSecureGameTransport()`): `AnticheatPlugInterface::SendPacket(mwid, userID, buf, len, ENetworkChannels::Game, PACKET_RELIABILITY_RELIABLE_ORDERED)` and `GetNextRecvPacketSize` / `RecvPacket` / `FreePacket` (no channel byte, sender id unknown, `userID = -1`).

### 3.4 Size limits

`Coreh/NetworkDefs.h` under `GENERALS_ONLINE`: `MAX_UDP_PAYLOAD = 1194 - 96 = 1098` ("96 for TURN + Valve overhead"), `MAX_PACKET_SIZE = 1098`, `MAX_MESSAGE_LEN = 1104`, `MAX_MESSAGES = 256`. `queueSend` rejects `len > MAX_PACKET_SIZE`; `doRecv` drops anything larger than `sizeof(TransportMessageHeader) + MAX_MESSAGE_LEN`. Largest GNS message is therefore 1 + 6 + 1098 = 1105 bytes. GNS reliable messages are fragmented by GNS itself, so the 1098 limit is engine-side only.

### 3.5 Engine ACK/resend still active

Yes. GNS reliable delivery sits *under* the unchanged engine layer: `Connection::doSend` still resends unacked commands after `m_retryTime = 2000` ms, `ConnectionManager::ackCommand` / `processAck*` (stage1/stage2/both), `m_pendingCommands`, `m_relayedCommands`, `FRAMERESENDREQUEST`, keep-alives (`doKeepAlive`) all run as in the stock engine. `NextGenTransport::doSend` additionally keeps a failed packet up to `OutgoingPacketState::MAX_RETRIES = 3` frames.

---

## 4. Match start

### 4.1 Custom lobby

1. Host presses Start (`Menus/WOLGameSetupMenu.cpp`, start handler after player/team checks): `WebSocket::SendData_StartFullMeshConnectivityCheck(cb)` (id 26, remembers `m_connectivityCheckLobbyID`).
2. Server runs the check (27 to every member, replies collected) and sends 28 to the host.
3. Callback with `bMeshFullyConnected == true` (and `GENERALS_ONLINE_ENABLE_MATCH_START_COUNTDOWN` defined, which it is): announcement via `SendAnnouncementMessageToCurrentLobby`, `NGMPGame::StartCountdown()` -> `m_bCountdownStarted`, start time, `SendData_CountdownStarted()` (id 23). Failure: prints missing links and reason.
4. Countdown is purely client-side on the host: `GetTotalCountdownDuration()` returns 5 (seconds). The WOL game-setup update loop announces remaining seconds every 1000 ms via lobby chat; at 1 s it calls `WOLLockSettings()`; at <= 0 `StopCountdown()` and `WebSocket::SendData_StartGame()` (id 13). The countdown is cancelled by the host on settings changes (`StopCountdown` in most `LobbyInterface` update functions) and when player count drops (`UpdateRoomDataCache`).
5. Server sends `START_GAME {screenshot_url}` to clients. The host also starts only through this callback, so the server must send it to the host too (inferred).
6. `LobbyInterface::m_callbackStartGamePacket` (registered by `RegisterForGameStartPacket` in `Menus/WOLGameSetupMenu.cpp`): closes overlays, `*TheNGMPGame = *myGame`, `NGMPGame::startGame(0)` -> `launchGame()` (both the "<2 humans" and normal branch just call `launchGame`; NAT negotiation is gone).

### 4.2 Quick match

`MATCHMAKING_ACTION_JOIN_PREARRANGED_LOBBY` -> `JoinLobby`; `MATCHMAKING_ACTION_SETUP_PROGRESS` drives the menu; `MATCHMAKING_ACTION_START_GAME` -> callback in `Menus/WOLQuickMatchMenu.cpp` marks all slots as having the map, `*TheNGMPGame = *myGame; TheNGMPGame->startGame(0)`. No client countdown or mesh check in that path (the service decides).

### 4.3 `NGMPGame::launchGame` (`GO/NGMPGame.cpp`)

1. Apply `ServiceConfig`: if `use_default_config`: FPS history 30, latency history 200, `m_networkRunAheadSlack = ra_slack_override_percent_in_default`, metrics time 5000 ms. Else FPS/latency history 10, `MIN_RUNAHEAD = min_run_ahead_frames`, slack `ra_slack_percent`, metrics time `ra_update_frequency_frames * 1000/60`, `FRAME_GROUPING_CAP = frame_grouping_frames * 1000/60`. (See 5.3 for why the last two assignments probably do not reach the engine.)
2. `setGameInProgress(TRUE)`.
3. `TheNetwork = NetworkInterface::createNetwork(); TheNetwork->init();` (`Network::init` creates `ConnectionManager`, initial `m_runAhead = min(max(30, MIN_RUNAHEAD), 64)`, `m_frameRate = 30`).
4. `TheNetwork->SeedLatencyData(mesh->getMaximumHistoricalLatency())` -> `ConnectionManager::SeedLatencyData` sets all `m_fpsAverages = GENERALS_ONLINE_HIGH_FPS_LIMIT (60)`, all `m_latencyAverages` and `FrameMetrics` latency to that value.
5. `setLocalAddress(getLocalIP() /*1337*/, 8888)`, `initTransport()` (NextGenTransport), `parseUserList(this)`.
6. `ConnectionManager::parseUserList`: for each `slot->isHuman()`: local slot gets `FrameDataManager(TRUE)`, others a `Connection` attached to the transport with `User(name, slotIndexAsIP, port)` and `FrameDataManager(FALSE)`; `m_packetRouterFallback[n] = slot`; then `m_packetRouterSlot = m_packetRouterFallback[0]` (lowest occupied human slot). `Network::parseUserList` then `destroyGameMessages()` and `zeroFrames(1, m_runAhead - 1)`.
7. `DoAnyMapTransfers(this)` (engine file transfer over the same transport), map check, camera, `MSG_NEW_GAME(GAME_INTERNET)`, FPS limit, `InitRandom(getSeed())`.
8. Random seed source: the service. `getSeed()` was set by `NGMPGame::SyncWithLobby` from `LobbyEntry::rng_seed`, parsed from the lobby GET JSON key `RNGSeed` in `LobbyInterface::UpdateRoomDataCache`. All clients must have the same lobby data at start (the GET happens on every `LOBBY_CURRENT_LOBBY_UPDATE`).
9. Start-of-game screenshot (`SCREENSHOT_TYPE_LOADSCREEN`) is captured elsewhere and uploaded to `screenshot_url` once known (`NGMP_OnlineServicesManager::Tick`).

From here the stock engine flow runs: `Network::update` -> `GetCommandsFromCommandList`, `updateRunAhead`, `liteupdate`, `AllCommandsReady` -> `RelayCommandsToCommandList`; local status moves `PREGAME` -> `INGAME` at logic frame 1 (`Network::processCommand`).

---

## 5. `GENERALS_ONLINE` / NGMP blocks in Core GameNetwork (top-level `*.cpp` / `*.h`)

`GENERALS_ONLINE` is a global compile definition (`add_compile_definitions(GENERALS_ONLINE)` in `GeneralsMD/Code/GameEngine/CMakeLists.txt`, also forced in `GOh/NextGenMP_defines.h`). `GENERALS_ONLINE_HIGH_FPS_SERVER` is defined (`NextGenMP_defines.h`), so `GENERALS_ONLINE_HIGH_FPS_LIMIT = 60`. `GENERALS_ONLINE_RUN_FAST` is not defined.

### 5.1 Conditional blocks

| File | Enclosing function / scope | Change vs stock |
|---|---|---|
| `Core/ConnectionManager.cpp` | `processRunAheadMetrics` | reported FPS clamped at 1000 instead of 100 |
| `Core/ConnectionManager.cpp` | `processChat` | `TheInGameUI->messageColor(true, &rgb, ...)` signature only |
| `Core/ConnectionManager.cpp` | `updateRunAhead` (packet-router branch) | if `ServiceConfig.ibra_ra_tweaks`: replaces configured slack with a dynamic slack from mesh jitter (max-min of `PlayerConnection::m_vecLatencyHistory`) relative to `NetworkMesh::getMaximumLatency()`, using `ibra_minslack_*`/`ibra_maxslack_*` bands (>300 ms, >200 ms, default) |
| `Core/ConnectionManager.cpp` | `updateRunAhead` (packet-router branch) | if `TheNGMPGame` and the mesh has **zero** connections, `newRunAhead = 0` ("instant responsiveness" for solo/AI games) |
| `Core/ConnectionManager.cpp` | `initTransport` | `NextGenTransport` unless `TheLAN` exists |
| `Core/ConnectionManager.cpp` | `disconnectPlayer(int64_t userID)` (new overload) | returns `PLAYERLEAVECODE_UNKNOWN` and does nothing; body commented out. Every `TheNetwork->GetConnectionManager()->disconnectPlayer(userID)` call in `NetworkMesh.cpp` is therefore a no-op |
| `Coreh/ConnectionManager.h` | class `ConnectionManager` | adds `SeedLatencyData(int)` (FPS averages = 60, latency averages and `FrameMetrics` seeded) and the `disconnectPlayer(int64_t)` declaration |
| `Core/Connection.cpp` | file scope | `MaxQuitFlushTime` 5000 ms instead of 30000 ms |
| `Coreh/Connection.h` | file scope | `CONNECTION_LATENCY_HISTORY_LENGTH` 10 instead of 200 |
| `Core/FrameMetrics.cpp` | `FrameMetrics::init` | `GENERALS_ONLINE_HIGH_FPS_SERVER`: average FPS and FPS list start at 60; `GENERALS_ONLINE`: initial average latency = mean of mesh `GetLatency()` (divides by `connections.size()`, which is 0 for a solo mesh) instead of 0.2 s |
| `Core/FrameMetrics.cpp` | ctor, dtor, `init`, `doPerFrameMetrics`, `processLatencyResponse` (`USE_NEW_FRAMEMETRIC_LOGIC`) | map-based pending/actual latency tracking instead of fixed arrays |
| `Coreh/FrameMetrics.h` | file scope | defines `USE_NEW_FRAMEMETRIC_LOGIC 1` |
| `Core/Network.cpp` | class `Network` declaration | adds `SeedLatencyData`, `setSawCRCMismatch(UnicodeString&)`, `GetConnectionManager()`, `IsSlugging()` |
| `Core/Network.cpp` | `SeedLatencyData` | forwards to `ConnectionManager::SeedLatencyData` |
| `Core/Network.cpp` | `setSawCRCMismatch` | shows `MessageBoxOk("Mismatch Occurred", details)` instead of `Menus/CRCMismatch.wnd`; `startEndGameTimer(true)` |
| `Core/Network.cpp` | `processCommand` (`GENERALS_ONLINE_RUN_FAST`) | both branches identical (`getFrame() == 1`); no effect |
| `Core/Network.cpp` | `processRunAheadCommand` | frame grouping capped at `FRAME_GROUPING_CAP` (32 ms with high-FPS) instead of 500 ms |
| `Core/Network.cpp` | `update` | logic frame rate 60 (`GENERALS_ONLINE_HIGH_FPS_LIMIT`) instead of 30 when in game |
| `Coreh/NetworkInterface.h` | `NetworkInterface` | pure virtuals `SeedLatencyData`, `IsSlugging`, `setSawCRCMismatch(UnicodeString&)`, `GetConnectionManager()`. The `#endif` line also swallows `virtual Bool isPlayerConnected(Int) = 0;` (text after `#endif` on the same line), so that pure virtual no longer exists in the interface |
| `Coreh/NetworkDefs.h` | file scope | `MIN_RUNAHEAD` becomes non-const `static Int = 6` (stock const 4); `MAX_UDP_PAYLOAD 1098`; `MAX_PACKET_SIZE 1098`, `MAX_MESSAGE_LEN 1104` |
| `Core/GameSpyOverlay.cpp` | `GameSpyOpenOverlay` | buddy overlay opens without the GameSpy buddy-connected check |
| `Coreh/Transport.h` | file scope (comment, not a block) | "NGMP NOTE": `Transport` is now an abstract base; the comment line also swallows the `#include "GameNetwork/udp.h"` |

Unconditional GO code in the same files (no `#if`): `ConnectionManager::getMaximumLatency` reads `ServiceConfig.network_latency_logic_model` when `TheNGMPGame != nullptr` (0 engine latency; 1/2 max of engine and mesh current/historic RTT; 3/4 mesh only; mesh RTT is round-trip ms / 1000); `FrameMetrics::SeedLatencyData`; `ConnectionManager::parseUserList` sets `m_packetRouterSlot = m_packetRouterFallback[0]` (comment says it fixes a hard-coded 0; whether this is a GO or upstream change is not verified); `NetworkLog` calls in `Connection.cpp`, `setFrameGrouping`, `timeForNewFrame`; includes of `../NextGenTransport.h`, `../NetworkMesh.h`, `../ngmp_interfaces.h` in `ConnectionManager.cpp`.

No GO blocks in `Transport.cpp`, `NetworkUtil.cpp`, `DisconnectManager.cpp`, `GameInfo.cpp`, `NetPacket.cpp`.

### 5.2 What did not change

- Packet router: still exists unchanged. `sendLocalCommand` routes through `m_packetRouterSlot`; the router relays (`doRelay` -> `sendRemoteCommand`) and computes run-ahead (`updateRunAhead`); fallback order is slot order (`m_packetRouterFallback`), with `disconnectPlayer(Int)` picking the next. With a full P2P mesh, non-router clients still send most commands via the router. GO only adds tweaks inside the router's run-ahead calculation.
- `ConnectionManager::allCommandsReady`: unchanged wait rule (every non-quitting `FrameDataManager` must report ready, `FRAMEDATA_RESEND` triggers `requestFrameDataResend`, then `DisconnectManager::allCommandsReady` / `allowedToContinue`).
- `DisconnectManager` (vote screen, `m_networkDisconnectTime`, `m_networkPlayerTimeoutTime`, packet-router timeout): no GO changes. Timeout overrides in `NGMPGame::launchGame` are commented out. Since `disconnectPlayer(int64_t)` is a stub, a dropped GNS link mid-match is handled only by the stock disconnect screen / timeouts (plus GNS `AutoRestartBrokenSession` and the retry-signalling path).

### 5.3 `static` globals in headers

`MIN_RUNAHEAD` (`Coreh/NetworkDefs.h`) and `FRAME_GROUPING_CAP` (`GOh/NextGenMP_defines.h`) are namespace-scope `static int` in headers, so each translation unit has its own copy. The assignments in `NGMPGame::launchGame` change only `NGMPGame.cpp`'s copies; `ConnectionManager.cpp` keeps `MIN_RUNAHEAD = 6` and `Network.cpp` keeps `FRAME_GROUPING_CAP = 32`. So `min_run_ahead_frames` and `frame_grouping_frames` from the service config probably have no effect. Follows from C++ linkage rules; not verified at runtime.

---

## 6. Anti-cheat plugin (`AnticheatPlugInterface`)

Files: `GOh/PluginInterfaces.h`, `GO/PluginInterfaces.cpp`. `GENERALS_ONLINE_USE_PLUGINS_INTERFACE` is defined (`NextGenMP_defines.h`), which defines `AC_ENABLED`; the header also contains an all-stub `#else` class.

### 6.1 Loading

- `NGMP_OnlineServicesManager::Init`: `Settings.GetAnticheatPlugin()`; if non-empty, `LoadPlugin("plugins/<name>/<name>.dll")`. `GenOnlineSettings::GetAnticheatPlugin` (`GO/GeneralsOnline_Settings.cpp`): if the setting is empty it probes `plugins/goanticheat/goanticheat.dll` (twice; the first check is labelled EAC but tests the goanticheat path and returns `"easyanticheat"`, a bug) and returns `"goanticheat"`; otherwise returns the setting (possibly empty).
- In `_DEBUG`, `LoadPlugin` ignores the name and loads `F:\Gen\eac_module\build\Debug\easyanticheat.dll`.
- `LoadPlugin` resolves required exports (`SetLoggingFunction`, `SetLobbyChatOutputFunction`, `Initialize`, `IsExternalProcessRunning`, `GetAnticheatIdentifier`, `SetACIntegrityViolationOccurredCallback`, `SetACActionRequiredCallback`, `SetSendMessageViaTransportCallback`, `ACMessageArrivedViaTransport`, `DoesACPluginProvideSecureGameTransport`, `StartSignalling`, `SendPacket`, `GetNextRecvPacketSize`, `RecvPacket`, `GetConnectionLatencyForUser`, `IsConnectionRelayed`, `DisconnectPlayer`, `DisconnectAll`, `Login`, `RefreshToken`, `IsLoggedIn`, `GetMiddlewareAuthToken`, `BeginSession`, `EndSession`, `RegisterPlayer`, `DeregisterPlayer`, `Tick`, `Shutdown`; optional `SetSecureGameTransportEnabled`, `FreePacket`). Any missing required export or `Initialize() != 0` unloads the module and sets `m_bPluginLoadFailed`.
- Unloaded in `NGMP_OnlineServicesManager::Shutdown` -> `UnloadPlugin`.

### 6.2 No plugin present

- `LoadLibraryA` fails (or is never called): `g_hACPluginModule == nullptr`, every wrapper checks `IsPluginLoaded()` and becomes a no-op; `GetAnticheatIdentifier()` returns 0; `DoesACPluginProvideSecureGameTransport()` returns false (it also returns false whenever `ServiceConfig.use_new_networking` is false); `RegisterPlayer`/`DeregisterPlayer` return false (only logged).
- `Authenticate()` does nothing, so `SendMiddlewareToken` / HTTP `ProvideMWToken` is never called and `MIDDLEWARE_LOGIN_FAILED` cannot happen.
- No client code checks `IsPluginLoaded`, `DidPluginFailToLoad` or `IsExternalProcessRunning` to block anything. Lobbies and matches are not blocked client-side. Blocking can only come from the service: lobby create/join send `anticheat_id` (0 without plugin) and the join returns HTTP 417 -> `JoinLobbyResult_AnticheatMismatch` if the service rejects it.
- `g_bPendingExitLobby` (set only by plugin callbacks) forces `PopBackToLobby` in `Menus/WOLGameSetupMenu.cpp`; never set without a plugin.

### 6.3 `MiddlewarePluginGeneric` and `ProvideMWToken`

- `EConnectionType::MiddlewarePluginGeneric` (`GOh/NetworkMesh.h`) is the `PlayerConnection` kind created by `PlayerConnection(int64_t userID, const char* szMiddlewareID)` when `DoesACPluginProvideSecureGameTransport()` is true: signalling goes to `AnticheatPlugInterface::StartSignalling(middleware_id, userID)` (the `middleware_id` from `NETWORK_CONNECTION_START_SIGNALLING`; lobby members also carry `MiddlewareUserID`), game packets via the plugin (`SendPacket`/`RecvPacket`), latency via `GetConnectionLatencyForUser`, state via the plugin's `Initialize` callback -> `NetworkMesh::UpdateConnectionStateForUser`. No GameNetworkingSockets connection or listen socket is used, and `NetworkMeshLibrary::Tick` stops running GNS callbacks. Comments call this EOS P2P.
- `ProvideMWToken`: after plugin `Login(sessionToken)` succeeds, `GetMiddlewareAuthToken` is read and `NGMP_OnlineServices_AuthInterface::SendMiddlewareToken` POSTs `{"mw_token": "..."}`; a non-2xx response causes full teardown (`MIDDLEWARE_LOGIN_FAILED`). Plugin token refresh every 45 min (`AnticheatPlugInterface::Tick`).

### 6.4 For a build without any plugin

Nothing must be stubbed for correctness; the no-plugin path already works. Recommended to make it explicit:
- Leave `Plugins_Anticheat` empty and do not ship `plugins/goanticheat/` (or remove the `LoadPlugin` call in `NGMP_OnlineServicesManager::Init`), or build without `GENERALS_ONLINE_USE_PLUGINS_INTERFACE` to get the header stub class. Caution: the stub's `IsPluginLoaded()`/`IsExternalProcessRunning()` return true; nothing in the code currently reads them, but future checks would be misled.
- Remove the hard-coded debug path in `LoadPlugin` for `_DEBUG`.
- Service side: always serve `use_new_networking:false`, accept `anticheat_id:0`, never answer 417, and either do not send `AC_REGISTER_PLAYER`/`AC_DEREGISTER_PLAYER`/`ANTICHEAT_MESSAGE` or accept that they are logged and ignored.
- `WS_KEEPALIVE` module listing and `PROBE` screenshots are independent of the plugin; a replacement service can simply never send them.

---

## 7. Third-party pieces and outbound hosts

| Library | Linked / initialised | Talks to | Symbol / location |
|---|---|---|---|
| libcurl (vcpkg, `openssl` + `websockets`) | `curl_global_init` in `NGMP_OnlineServicesManager::InitSentry`; `HTTPManager::Initialize` from `NGMP_OnlineServicesManager::Init`; WebSocket in `WebSocket::WebSocket`/`Connect` | GO API and WS (below), S3 presigned URLs from the service | `GetAPIEndpoint` (`OnlineServices_Init.cpp`), `OnLogin` alt WS URL |
| GameNetworkingSockets (static, custom registry `tintinhamans/GNS`) | `NetworkMeshLibrary::EnsureInitialized` (from `OnLogin` and `NetworkMesh::NetworkMesh`) | STUN `stun.playgenerals.online:53/3478`, `stun.l.google.com:19302`; TURN `turn.playgenerals.online:53/3478`; peers | `NetworkMeshLibrary::EnsureInitialized` (STUN string), `NetworkMesh::NetworkMesh` (`m_strTurnServerList`) |
| sentry-native (`backend`, `transport`; crashpad handler copied next to exe by `GeneralsMD/Code/Main/CMakeLists.txt`) | `NGMP_OnlineServicesManager::InitSentry`, called from `GameEngine::init` (`GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp`) right after GlobalData; only `#if !_DEBUG`; not gated by `GENERALS_ONLINE_USE_SENTRY` | DSN `the DSN literal in `InitSentry`` (release `generalsonline-client@<GENERALS_ONLINE_VERSION_STRING>`, user id + display name tags; crash dumps under `<UserData>/GeneralsOnlineCrashData/`) | literal in `InitSentry`. Manual events (`GENERALS_ONLINE_USE_SENTRY`): `WEBSOCKET_DISCONNECT_ERROR`, `WEBSOCKET_DISCONNECT_TIMEOUT` (`WebSocket::Tick`), `CRC_MISMATCH` (`GeneralsMD/.../GameLogic/System/GameLogic.cpp`) |
| Discord RPC (`discord-rpc.dll`, loaded dynamically) | `GameEngine` ctor/init: `m_discordRichPresence = new GeneralsOnlineDiscordRPC(); ->Initialize()`; `Tick` in the engine loop | local Discord client (IPC); presence shows lobby name, map, lobby id as `partyId`, player counts | `DISCORD_APPLICATION_ID = "1354979004507226294"` in `GO/DiscordRichPresence.cpp` (GO's Discord application) |
| FFmpeg (vcpkg) | `FFmpegVideoPlayer` via `W3DGameClient::createVideoPlayer` only `#ifdef RTS_HAS_FFMPEG` (`RTS_BUILD_OPTION_FFMPEG`, default OFF in `cmake/config-build.cmake`) | local video decode only; no network use found | - |
| Steam API (optional, dynamic) | `NGMP_OnlineServicesManager::AttemptLoadSteam` (`SteamAppId 2732960`, `steam_api.dll` if present) | Steam client | `OnlineServices_Init.cpp` |

Hosts owned by the Generals Online team that a separate service must remove or replace:

- `api.playgenerals.online` (HTTP API + presumably WS via `ws_uri`), `api.playgenerals.online:2087` (test), `api-ru.playgenerals.online` (HTTP and `wss://api-ru.playgenerals.online/ws`) - `GetAPIEndpoint`, `OnLogin`.
- `stun.playgenerals.online`, `turn.playgenerals.online` - `NetworkMeshLibrary::EnsureInitialized`, `NetworkMesh::NetworkMesh`.
- Sentry DSN above (org `o4509316925554688`) - `InitSentry`.
- Browser links (ShellExecute): `https://www.playgenerals.online/login/?gamecode=...` (`DoFullLoginFlow`), `.../updatefailed` (`ContinueUpdate`, `LaunchPatcher`), `.../viewmatch?match=...` (`Menus/ScoreScreen.cpp`), `https://discord.playgenerals.online` (`Menus/WOLLobbyMenu.cpp`, `Menus/WOLGameSetupMenu.cpp`, `OnlineServices_Moderation.cpp`).
- Update check: HTTP POST `VersionCheck` `{execrc, ver, netver, servicesver}`; on `NEEDS_UPDATE` the client downloads `patcher_path` (any URL the service returns) and runs it elevated (`runas`) - `StartVersionCheck`, `StartDownloadUpdate`, `LaunchPatcher`.
- Telemetry to the API: `CheckLogin` sends `machine_guid`, `mac_addr`, `vol_serial`, `exe_crc`, `ini_crc`, `client_id` (`AuthInterface::Tick`); `ConnectionOutcome`; WS module list (43); probe screenshots (16); match start/end screenshots and replay upload to service-provided S3 URLs (`do_replay_upload`, `do_probes` in `ServiceConfig`); stats `CommitMyOutcome`.

---

## 8. Seams for a second match transport (relay server)

Mapped only; not a design.

### Seam A: a different `Transport` implementation

- Where: `ConnectionManager::initTransport` (choose class), or after it via `Network::attachTransport` / `ConnectionManager::attachTransport` (deletes the old one). Called from `NGMPGame::launchGame`.
- Contract to honour: `queueSend(addr = slot index, port, buf, len <= MAX_PACKET_SIZE)` fills `m_outBuffer`; `doSend` delivers; `doRecv` fills `m_inBuffer` contiguously from index 0 (`ConnectionManager::doRelay` stops at the first `length == 0`), with header CRC/magic valid for `Transport::isGeneralsPacket`; statistics arrays used by bandwidth getters. Sender address is not needed. Slot -> user via `NGMPGameSlot::m_userID`.
- Engine reliability (ACK/resend, frame resend requests) runs on top, so a relay link may be unordered; duplicate and loss tolerance is handled by the engine (latency of resends: 2 s `m_retryTime`).
- Constraints from code that assumes the mesh exists during a match:
  - `NextGenTransport::doRecv` returns early if `GetNetworkMesh()` is null (only relevant if NextGenTransport is reused).
  - `ConnectionManager::updateRunAhead`: if `TheNGMPGame` and mesh has 0 connections -> run-ahead forced to 0. A relay match with an empty mesh would get run-ahead 0.
  - `ConnectionManager::getMaximumLatency` with `network_latency_logic_model` 1..4 and the `ibra_ra_tweaks` slack read mesh RTT/jitter.
  - `FrameMetrics::init` divides by `connections.size()` of the mesh.
  - `NGMPGame::launchGame` seeds latency from `getMaximumHistoricalLatency()`.
  - In-game UI stats read `PlayerConnection` (`GeneralsMD/.../GameClient/InGameUI.cpp`, connection status/quality; not fully traced).
  - Packet-router topology stays; with a relay, the engine's router is still one client (lowest human slot).

### Seam B: inside `NetworkMesh` / `PlayerConnection` (per-connection type)

- Existing precedent: `EConnectionType::MiddlewarePluginGeneric` with branches on `AnticheatPlugInterface::DoesACPluginProvideSecureGameTransport()` in `NetworkMesh::NetworkMesh` (skip listen socket), `NetworkMesh::StartConnectionSignalling`, `NetworkMesh::DisconnectUser`, `NetworkMesh::Disconnect`, `NetworkMesh::Tick` (skip `CSignalingClient::Poll`), `NetworkMeshLibrary::Tick`, `PlayerConnection::SendGamePacket`, `PlayerConnection::SendACPacket`, `PlayerConnection::LiteUpdateForAC`, `PlayerConnection::Close`, `PlayerConnection::GetLatency`, `PlayerConnection::GetConnectionType`, `NextGenTransport::doRecv` (separate receive loop), and `NetworkMesh::UpdateConnectionStateForUser` for thread-safe state updates.
- A relay type could follow the same branch points and keep `PlayerConnection` entries per peer, so mesh-dependent engine code (run-ahead, latency, full-mesh check replies, `ConnectionOutcome`) keeps working.
- Constraint: `NETWORK_CONNECTION_START_SIGNALLING` is the per-peer trigger; `middleware_id` is already a free string field the service can use. The full-mesh check reports `CONNECTED_DIRECT` peers, so relay connections must reach that state for the host to be allowed to start.
- `NetworkMesh` is per lobby and created on join/create (`JoinLobby`, `OnJoinedOrCreatedLobby`); its mode is fixed at construction (`m_iceEnable`). A per-match decision would need to be known before signalling begins or applied by re-signalling.

### Seam C: `ConnectionManager` level

- `Connection::doSend` -> `m_transport->queueSend`; `ConnectionManager::sendLocalCommand` / `sendLocalCommandDirect` / `sendRemoteCommand` / `ackCommand` decide per-slot sends; `doRelay` consumes `m_inBuffer`. Bypassing here means re-implementing packetisation (`NetPacket`) and ACK bookkeeping; the transport seam is lower-risk. `ConnectionManager::disconnectPlayer(int64_t)` is a stub that would need a real body if the relay should drop players.

### Related non-transport seams

- Match start gate: `SendData_StartFullMeshConnectivityCheck` / message 28 (service decides `mesh_complete`).
- `ServiceConfig` (`relay_all_traffic`, `use_new_networking`, `ice_implementation`, run-ahead fields) is fetched before every lobby join/create (`GetAndParseServiceConfig`) and is the existing service-driven switch.

---

## 9. Not determinable from the client code

- Which peers receive `NETWORK_CONNECTION_START_SIGNALLING` (one or both of a pair), when relative to `LOBBY_CURRENT_LOBBY_UPDATE`, and how `NETWORK_CONNECTION_CLIENT_REQUEST_SIGNALLING` is answered.
- What the server puts in `target_user_id` when forwarding `NETWORK_SIGNAL` and `ANTICHEAT_MESSAGE` (the AC path treats it as the sender id).
- Whether `START_GAME` is broadcast to the host as well (the client requires it), whether the server validates the countdown (`START_GAME_COUNTDOWN_STARTED`), and how `screenshot_url` / `match_id` are created.
- Full-mesh check algorithm: attempts, timeouts, how `connecting_map` is used, the meaning of reasons such as `missing_connections`, `timeout`, `member_left`, `check_superseded` (listed only in a header comment).
- TURN credential scheme (static vs time-limited/HMAC), lifetime, and TURN server configuration; how `rng_seed` is generated.
- WebSocket server details: session hold time behind HTTP 205, server-side ping/keep-alive timing, `WS_KEEPALIVE` interval, rate limits, whether the server accepts text frames from clients (client only sends binary).
- What the service does with `ConnectionOutcome`, module lists, hardware ids and probe screenshots.
- Production `ServiceConfig` values (e.g. `retry_signalling`, `use_new_networking`, `relay_all_traffic`, `network_latency_logic_model`, `ibra_ra_tweaks`).
- Changes in the custom GameNetworkingSockets fork (`tintinhamans/GNS` registry) versus upstream, and IPv6 behaviour.
- The anti-cheat plugin's own network endpoints and protocol (closed DLL).
