# Generals Online client: service contract as implemented by the client

Source: Generals Online client at commit `6ff404436`. Everything below was read from code; nothing was observed on the wire.

Path abbreviations used below:

- `INC/` = `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/`
- `SRC/` = `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/`
- `MENUS/` = `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/`

JSON library: nlohmann json 3.11.3 (`INC/json.hpp`). Enums are serialized as plain integers (no `NLOHMANN_JSON_SERIALIZE_ENUM` is used anywhere in the GO code).

---

## 1. Base URL and hard-coded hosts

### 1.1 How the API URL is built

`NGMP_OnlineServicesManager::GetAPIEndpoint(const char* szEndpoint)` in `SRC/OnlineServices_Init.cpp`:

| Environment (`g_Environment`) | Result |
|---|---|
| `DEV` | `https://localhost:9000/env/dev/contract/1/{szEndpoint}` |
| `TEST` | `https://api.playgenerals.online:2087/env/test/contract/1/{szEndpoint}` |
| `PROD`, `Settings.Network_UseAlternativeEndpoint()` true | `https://api-ru.playgenerals.online/env/prod/contract/1/{szEndpoint}` |
| `PROD` (default) | `https://api.playgenerals.online/env/prod/contract/1/{szEndpoint}` |

So every endpoint is `{base}/{Name}`, for example `https://api.playgenerals.online/env/prod/contract/1/Lobby/42`. Callers append path parameters with `std::format("{}/{}", GetAPIEndpoint("Lobby"), id)`. There is no query string anywhere.

Environment selection: `NGMP_OnlineServicesManager::g_Environment` in `INC/OnlineServices_Init.h`:

- `USE_TEST_ENV` defined -> `TEST`
- else `USE_DEBUG_ON_LIVE_SERVER` defined -> `PROD`
- else `_DEBUG` defined -> `DEV`
- else -> `PROD`

`USE_TEST_ENV` and `USE_DEBUG_ON_LIVE_SERVER` are both commented out in `INC/NextGenMP_defines.h` (lines 29 and 33). So a Release build talks to `api.playgenerals.online`, a Debug build talks to `https://localhost:9000`.

`Network_UseAlternativeEndpoint()` (`INC/GeneralsOnline_Settings.h`, member `m_Network_UseAlternativeEndpoint`, default `false`) is loaded from `<UserData>/GeneralsOnlineData/settings.json`, key `network.use_alternative_endpoint` (`SRC/GeneralsOnline_Settings.cpp`, `GenOnlineSettings::Load`, define `SETTINGS_KEY_NETWORK_USE_ALTERNATIVE_ENDPOINT`). It only matters in `PROD`.

There is no setting, command line switch or file that lets you put an arbitrary host into the URL. Pointing the client at another service needs a code change in `GetAPIEndpoint` (or DNS redirection plus a certificate the client trusts, see 2.5).

Version constants sent to the service (`INC/NextGenMP_defines.h`): `GENERALS_ONLINE_VERSION 1`, `GENERALS_ONLINE_NET_VERSION 1`, `GENERALS_ONLINE_SERVICE_VERSION 1`, `GENERALS_ONLINE_CLIENT_ID "gen_online_60hz"` (because `GENERALS_ONLINE_HIGH_FPS_SERVER` is defined; otherwise `"gen_online_30hz"`). `GENERALS_ONLINE_VERSION_STRING "100126_QFE2"` is only used for Sentry.

### 1.2 WebSocket URL

The WebSocket URL comes from the login response field `ws_uri` (see 3). Exception: `NGMP_OnlineServicesManager::OnLogin` (`SRC/OnlineServices_Init.cpp`) replaces it with the hard-coded `wss://api-ru.playgenerals.online/ws` when `Network_UseAlternativeEndpoint()` is true. Any scheme libcurl accepts for WebSocket (`ws://` or `wss://`) should work because the client just passes the string to `CURLOPT_URL` (`WebSocket::Connect`, `SRC/OnlineServices_RoomsInterface.cpp`); not verified at runtime.

### 1.3 Every other hard-coded host

| Host / URL | Purpose | Where |
|---|---|---|
| `https://www.playgenerals.online/login/?gamecode={code}` (`&env=test` with `USE_TEST_ENV`) | Browser page opened during code login | `NGMP_OnlineServices_AuthInterface::DoFullLoginFlow`, `SRC/OnlineServices_Auth.cpp` |
| `https://www.playgenerals.online/updatefailed` | Opened when the patch download or patcher launch fails | `NGMP_OnlineServicesManager::ContinueUpdate` and `::LaunchPatcher`, `SRC/OnlineServices_Init.cpp` |
| `https://www.playgenerals.online/viewmatch?match={id}&env=test` (`USE_TEST_ENV` only) | Match page link on score screen | `MENUS/ScoreScreen.cpp` lines ~673 and ~1221 |
| `https://strata.gamereplays.org/zh/match/{id}` (non-test builds) | Match page link on score screen | `MENUS/ScoreScreen.cpp` lines ~675 and ~1223 |
| `https://discord.playgenerals.online` | Discord button (ban/kick dialog, lobby menus) | `OpenDiscord` in `SRC/OnlineServices_Moderation.cpp`; `MENUS/WOLGameSetupMenu.cpp` ~3655; `MENUS/WOLLobbyMenu.cpp` ~239 |
| `www.playgenerals.online` (text only) | Intro screen text, patch dialog text | `Core/GameEngine/Source/GameClient/Intro.cpp` ~209; `Core/GameEngine/Source/GameNetwork/GameSpy/MainMenuUtils.cpp` ~951 |
| `stun:stun.playgenerals.online:53`, `stun:stun.playgenerals.online:3478`, `stun:stun.l.google.com:19302` | STUN list for SteamNetworkingSockets ICE | `NetworkMeshLibrary::EnsureInitialized`, `SRC/NetworkMesh.cpp` ~633 (`k_ESteamNetworkingConfig_P2P_STUN_ServerList`) |
| `turn:turn.playgenerals.online:53`, `turn:turn.playgenerals.online:3478` | TURN list; credentials come from lobby create/join responses | `NetworkMesh::NetworkMesh`, `SRC/NetworkMesh.cpp` ~711 (`m_strTurnServerList`) |
| `wss://api-ru.playgenerals.online/ws` | Alternative WebSocket | `NGMP_OnlineServicesManager::OnLogin` |
| `the DSN literal in `InitSentry`` | Sentry DSN (Release only, `GENERALS_ONLINE_USE_SENTRY`) | `NGMP_OnlineServicesManager::InitSentry` |
| Discord application id `1354979004507226294` | Discord rich presence (not a host) | `DISCORD_APPLICATION_ID`, `SRC/DiscordRichPresence.cpp` |
| Steam app id `2732960` | `SteamAppId` env var (not a host) | `NGMP_OnlineServicesManager::AttemptLoadSteam` |

Patch download host: not hard-coded. It is whatever full URL the service puts in `patcher_path` (see 4, VersionCheck).

S3 upload hosts: not hard-coded. Pre-signed URLs come from the service (`screenshot_url` in WebSocket `START_GAME`, `screenshot_url`/`replay_url` in `Lobby/Outcome`, `url` in WebSocket `PROBE`).

---

## 2. Common request rules

All HTTP goes through `HTTPManager` (`SRC/HTTP/HTTPManager.cpp`) and `HTTPRequest` (`SRC/HTTP/HTTPRequest.cpp`), libcurl multi interface, polled from the main thread in `HTTPManager::Tick`.

### 2.1 Headers

Set in `HTTPRequest::PlatformStartRequest`:

- `User-Agent: GeneralsOnline Client` (`CURLOPT_USERAGENT`).
- `Authorization: Bearer <session_token>` added automatically when `NGMP_OnlineServices_AuthInterface::IsLoggedIn()` is true (`m_userID != -1 && !m_strToken.empty()`) and the request did not call `DisableServiceAuth()`. It overwrites any `Authorization` the caller put in the header map.
- Requests with `bDisableServiceAuth = true` (`LoginWithToken`, `RefreshToken`) put `Authorization: Bearer <refresh_token>` in the header map themselves.
- S3 uploads (`HTTPManager::SendS3PUTRequest`) never get the auth header; they set `Content-Type: image/jpeg` or `application/octet-stream`.
- No `Content-Type` is set for API calls. Bodies are given to `CURLOPT_POSTFIELDS`, so libcurl's default `Content-Type: application/x-www-form-urlencoded` is sent for POST, and for PUT/DELETE too (they are POSTFIELDS plus `CURLOPT_CUSTOMREQUEST`). The service must parse the body as JSON regardless of `Content-Type`. (libcurl default behaviour, not verified on the wire.)
- Requests before login (`VersionCheck`, `LoginCode`, `CheckLogin`) carry no `Authorization`.
- `GET` requests have no body. `POST`/`PUT`/`DELETE` always send `POSTFIELDS`, possibly the empty string `""`. `CancelMatchmaking` sends the literal body `null` (dump of an empty `nlohmann::json`).

### 2.2 Timeouts, redirects, HTTP version, proxy

- Default timeout: `HTTPRequest::m_timeoutMS = 5000` (`INC/HTTP/HTTPRequest.h`), applied to both `CURLOPT_CONNECTTIMEOUT_MS` and `CURLOPT_TIMEOUT_MS`. Callers passing `-1` get 5000 ms. Exceptions: S3 uploads `HTTP_UPLOAD_TIMEOUT 600000` (`INC/NextGenMP_defines.h`), patch download `30 * 60 * 1000` (`ContinueUpdate`).
- `CURLOPT_FOLLOWLOCATION 1`: redirects are followed.
- `CURLOPT_HTTP_VERSION` from settings `network.http_version` (0 auto, 1 = 1.0, 2 = 1.1, 3 = 2.0, 4 = 3.0; `GenOnlineSettings::Network_GetHTTPVersionForCurl`). Default auto.
- Proxy from IE settings is used only in `_DEBUG` builds (`HTTPManager::DeterminePlatformProxySettings`, `PlatformStartRequest`).

### 2.3 Retries and status handling

- `HTTPManager` never retries. Retries exist only in specific callers: token refresh (2 attempts, 30 s apart), `CheckLogin` polling (every 1000 ms), WebSocket reconnect.
- The completion callback's `bSuccess` is always `true` (`HTTPRequest::InvokeCallbackIfComplete` passes `true` unconditionally). A transport failure (DNS, TLS, timeout) shows up as `statusCode == 0` and an empty body. Code that tests `bSuccess` alone therefore never sees a failure.
- There is no generic status handling. Each call site decides; many ignore the status and just try to parse the body (details per endpoint in 4).
- Status codes with special meaning somewhere: `200`, `201` (ignored in practice), `401`, `404`, `406`, `417`, `423`, any `4xx`. See 4.

### 2.4 IPv4 / IPv6

Every call uses `EIPProtocolVersion::DONT_CARE` -> `CURL_IPRESOLVE_WHATEVER`. After the first successful request, `HTTPRequest::Threaded_SetComplete` records whether the primary IP contained `:` and calls `HTTPManager::SetProtocolInUse(FORCE_IPV6 / FORCE_IPV4)`. That value is only read back in the same function (to log once); it does not force later requests. The service can be IPv4-only, IPv6-only or dual stack.

### 2.5 TLS

- Release: `CURLSSLOPT_NATIVE_CA`, `VERIFYPEER 1`, `VERIFYHOST 2` (`PlatformStartRequest`, `WebSocket::Connect`). The certificate must chain to the Windows certificate store and match the host name.
- `_DEBUG`: `VERIFYPEER 0`, `VERIFYHOST 0` for both HTTP and WebSocket. A Debug build against `https://localhost:9000` accepts a self-signed certificate.
- The base URL is always `https://`.

### 2.6 Response logging

Release builds redact any response body containing `token` (case-insensitive) and the value of `"RNGSeed":` in logs (`Threaded_SetComplete`). This only affects the client log.

---

## 3. Login flow

### 3.1 Sequence from "Online" button to logged in

1. `MainMenuUtils.cpp` (`Core/GameEngine/Source/GameNetwork/GameSpy/`, around line 865): `NGMP_OnlineServicesManager::CreateInstance()`, `->Init()` (creates interfaces, `HTTPManager::Initialize`, loads an anti-cheat plugin if configured), then shows "Checking for patches" and calls `StartVersionCheck`.
2. `POST VersionCheck` (no auth). See 4. On `fnCallback(false, ...)` the user gets the "cannot connect" message box (`GUI:CannotConnectToServserv`) and online is torn down. On `(true, true)` the mandatory update dialog appears. On `(true, false)` `startOnline()` continues to the login menu.
3. `WOLLoginMenuInit` (`MENUS/WOLLoginMenu.cpp` ~469): shows "Logging In / Please wait...", registers `NGMP_WOLLoginMenu_LoginCallback`, calls `NGMP_OnlineServices_AuthInterface::BeginLogin()`.
4. `BeginLogin` calls `GetCredentials()`.
   - If a stored refresh token exists: `POST LoginWithToken` with `Authorization: Bearer <refresh_token>` (step 5).
   - Otherwise: `DoFullLoginFlow()` (step 6).
5. `LoginWithToken` result:
   - `423` -> `ShowLoginBanDialog(ban_reason)`, stop.
   - other `4xx` -> `DoFullLoginFlow()`.
   - else parse `AuthResponse`: `result == 1` -> `SaveCredentials(refresh_token)`, store `session_token`, `user_id`, `display_name`, `OnLoginComplete(Success, ws_uri)` (step 8). `result == 2` -> `DoFullLoginFlow()`. Parse failure -> `DoFullLoginFlow()`. `result` `-1` or `0` -> nothing happens (client stays on "Logged in!/Please wait"; there is no timeout).
6. `DoFullLoginFlow`: `GET LoginCode` (no auth). On `success == true`: stores `login_code`, sets `m_bWaitingLogin = true`, shows "Please continue in your web browser" with a Cancel button, and in non-Debug builds (`!defined(_DEBUG) || USE_TEST_ENV || USE_DEBUG_ON_LIVE_SERVER`) runs `ShellExecuteA` on `https://www.playgenerals.online/login/?gamecode={code}`. Failure -> "Login Failed / Failed to retrieve login code".
7. `NGMP_OnlineServices_AuthInterface::Tick`, while `m_bWaitingLogin`: every 1000 ms `POST CheckLogin` (no auth) with the code. Results: `423` ban dialog; `result -1` (code unknown) and `0` (waiting) keep polling; `1` -> same as step 5 success; `2` -> `OnLoginComplete(Failed)` -> "Login failed." message box. Unparseable bodies are silently ignored and polling continues forever until the user cancels.
8. `OnLoginComplete(Success, ws_uri)`: `AnticheatPlugInterface::Authenticate()`, then `NGMP_OnlineServicesManager::OnLogin`: `NetworkMeshLibrary::EnsureInitialized(user_id)`, creates `WebSocket` and calls `Connect(ws_uri, false, cb)` with headers `Authorization: Bearer <session_token>` and `is-reconnect: false`.
   - If the WebSocket upgrade fails on the first attempt: `SetPendingFullTeardown(LOST_CONNECTION)` and the connected callback never runs (`WebSocket::Tick`).
9. When the WebSocket reports connected (`WebSocket::Tick`, `CURLMSG_DONE` with `CURLE_OK`): `GET Social/Friends` and `GET Social/Blocked` are fired (results not awaited), then `GoToDetermineNetworkCaps()`.
10. `GoToDetermineNetworkCaps`: `GET MOTD` (with session auth). Parse success or failure both continue; the login result is `Success` only if the WebSocket `IsConnected()`, else `Failed`. `m_cb_LoginPendingCallback(result)`.
11. `NGMP_WOLLoginMenu_LoginCallback(Success)` -> `checkLogin()` -> `nextScreen = "Menus/WOLWelcomeMenu.wnd"`. The player is logged in.

After that, while `IsLoggedIn()`, `AuthInterface::Tick` refreshes the session token (3.3).

If an anti-cheat plugin is loaded and logged in, the plugin path additionally sends `POST ProvideMWToken` (`PluginInterfaces.cpp` ~448 -> `NGMP_OnlineServices_AuthInterface::SendMiddlewareToken`). Without a plugin this never happens.

### 3.2 What the client stores locally

| Data | Where | Format |
|---|---|---|
| Refresh token | File `<UserData>/GeneralsOnlineData/credentials.json` (Release). `credentials_env_test.json` with `USE_TEST_ENV`. `credentials_dev_env_<instanceIndex>.json` in Debug DEV builds. (`GetCredentialsFilePath`, `CREDENTIALS_FILENAME`, `SRC/OnlineServices_Auth.cpp`) | JSON `{"refresh_token": "<token>"}` (dumped with indent 1). Encrypted with Windows DPAPI `CryptProtectData` (current user scope, description `"GO Credentials"`, no entropy) when `GENERALS_ONLINE_ENCRYPT_CREDENTIALS` is defined; plain JSON otherwise. Written via `.tmp` file plus `MoveFileExA`. |
| Session token, user id, display name | Memory only (`m_strToken`, `m_userID`, `m_strDisplayName`) | - |
| Settings | `<UserData>/GeneralsOnlineData/settings.json` (`GenOnlineSettings::Save`) | Plain JSON |
| Patcher | `<UserData>/GeneralsOnlineData/Update/<patcher_name>` (`GetPatcherDirectoryPath`) | Binary |

`<UserData>` is `TheGlobalData->getPath_UserData()`. Nothing is written to the registry by the GO login code. The registry is only read for `MachineGuid` (`GetMachineGuid`, `SRC/NGMP_Helpers.cpp`).

`SaveCredentials` stores whatever `refresh_token` the login response contains, including an empty string. An empty token makes the next `GetCredentials()` return false, so the next start goes through the code flow again. `RefreshToken` only saves when the response has a non-empty `refresh_token`.

`LogoutOfMyAccount` sends `DELETE User/{user_id}` and deletes the credentials file.

`GetCredentials()` returns false immediately in Debug DEV builds (`_DEBUG && !USE_TEST_ENV && !USE_DEBUG_ON_LIVE_SERVER`), so Debug builds always use the code flow, and they also never open the browser.

### 3.3 Token types and lifetimes as the client assumes them

- Session token (`session_token`): opaque string, sent as `Bearer`. The client never decodes it. `NGMP_OnlineServices_AuthInterface::Tick` refreshes it when `now - m_tokenCreationTime >= m_minutesUntilTokenRefresh (10) * 60 * 1000`. The code comment says "refresh every 10m, tokens last 15m". `m_tokenCreationTime` is set in `SaveCredentials`, at the start of every `RefreshToken`, and after a successful refresh.
- Refresh token (`refresh_token`): opaque string, persisted (3.2), used as `Bearer` for `LoginWithToken` and `RefreshToken`. No lifetime is assumed by the client; it is replaced whenever a response carries a new one.
- Refresh failure: `OnRefreshTokenFailed` retries after `m_secondsUntilRefreshRetry = 30` s, up to `m_maxRefreshAttempts = 2`, then `SetPendingFullTeardown(AUTH_FAILED)`. `423` during refresh -> `HandleModerationDisconnect(BAN, ban_reason)`.
- Consequence: a service that does not implement `RefreshToken` disconnects every player about 10.5 minutes after login.

### 3.4 `ALLOW_NON_PROFILED_LOGIN` and `GENERALS_ONLINE_ENCRYPT_CREDENTIALS`

- `ALLOW_NON_PROFILED_LOGIN` (`INC/NextGenMP_defines.h` line 50, always defined): used only in `MENUS/WOLLoginMenu.cpp` around the old GameSpy login screen (`GameSpyUseProfiles`, buttons for `GameSpyLoginProfile.wnd` / `GameSpyLoginQuick.wnd`, GameSpy `PeerRequest` login). `WOLLoginMenuInit` calls `BeginLogin()` and the window setup that would make those buttons work is commented out. It has no effect on the HTTP contract.
- `GENERALS_ONLINE_ENCRYPT_CREDENTIALS` (`INC/NextGenMP_defines.h` lines 58-60, defined when `!_DEBUG || USE_DEBUG_ON_LIVE_SERVER`): switches the credentials file between DPAPI-encrypted bytes and plain JSON (`SaveCredentials`, `GetCredentials`). A Release client cannot read a plain-text credentials file (decrypt fails, `strJSON` stays empty, parse throws, returns false -> code flow). A file can only be pre-seeded for a Release client by encrypting it with DPAPI as the same Windows user.

### 3.5 The browser step

The browser page is the real service's way of binding the code to an account. The client does not wait for the page; it only polls `CheckLogin`. A replacement service can answer `CheckLogin` with `result: 1` on the first poll and login completes without anyone touching the page. The `ShellExecuteA` call is unconditional in non-Debug builds, so a Release client will still open `https://www.playgenerals.online/login/?gamecode=...` in the browser once per code login (cosmetic, but it opens the third-party site). It is avoided only by:
- the stored-token path (`LoginWithToken`), which applies from the second start on if the service returned a non-empty `refresh_token`, or
- a Debug build, or
- a code change in `DoFullLoginFlow`.

### 3.6 Smallest server behaviour that gets a player logged in

1. `POST /env/prod/contract/1/VersionCheck` -> body `{"result":0,"patcher_name":"","patcher_path":"","patcher_size":0}` (all four keys are required; status code is ignored).
2. `GET .../LoginCode` -> `200` `{"success":true,"login_code":"anything"}`.
3. `POST .../CheckLogin` -> `{"result":1,"session_token":"S","refresh_token":"R","user_id":123,"display_name":"Name","ws_uri":"wss://host/ws"}`. `user_id` must not be `-1` and `session_token` must not be empty (`IsLoggedIn`).
4. WebSocket at `ws_uri`: accept the upgrade (`101`), then send at least one server frame every 20 s. The client sends a JSON text `{"msg_id":14}` (PING) every second, so answering each with a text frame `{"msg_id":15}` (PONG) is enough. Do not send a close frame (close = full teardown).
5. `GET .../MOTD` -> `{"MOTD":"text"}` (any failure is tolerated).
6. `GET .../Social/Friends`, `GET .../Social/Blocked` -> anything (failures are swallowed).
7. To stay logged in past 10 minutes: `POST .../RefreshToken` -> `{"result":1,"session_token":"S2"}`.

On the second start, `POST .../LoginWithToken` with `Bearer R` -> same body as 3.

---

## 4. Endpoints

Notation: `{base}` = `https://<host>/env/<env>/contract/1`. "Auth" = session `Bearer` added automatically (2.1). Types are JSON types; C++ type in brackets where it matters. "Required" means the client fails or misbehaves if the key is missing.

How missing keys behave, by parsing style:

- `NLOHMANN_DEFINE_TYPE_INTRUSIVE`: uses `j.at(key)`; a missing key throws `out_of_range`.
- `NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT`: missing keys keep the struct default; a wrong type still throws. A non-object body throws.
- Manual `obj["Key"].get_to(x)` on a non-const `json` (copy or root): a missing key becomes `null`; `get_to` into a number, bool or string from `null` throws `type_error`.
- Manual `iter["Key"].get_to(x)` inside `for (const auto& iter : ...)`: `iter` is const. nlohmann's const `operator[]` with a missing key is undefined behaviour (only a `JSON_ASSERT`, which is `assert`, so nothing in Release). Treat every such key as mandatory; a missing one can crash the client.
- Iterating `obj["list"]` when the key is missing or `null` yields zero elements (no error).
- Booleans must be JSON `true`/`false`; numbers are accepted into any C++ numeric type (int, uint16, float) by conversion.
- Unknown extra keys are ignored everywhere.

### 4.1 `VersionCheck`

- Caller: `NGMP_OnlineServicesManager::StartVersionCheck` (`SRC/OnlineServices_Init.cpp`), invoked from `MainMenuUtils.cpp` ~901.
- `POST {base}/VersionCheck`, no auth, timeout 5 s.
- Request: `execrc` number [uint32, `CRC_Memory` of the exe file bytes], `ver` number (1), `netver` number (1), `servicesver` number (1).
- Response, `VersionCheckResponse` with `NLOHMANN_DEFINE_TYPE_INTRUSIVE` (all required): `result` number [`EVersionCheckResponseResult`: 0 OK, 1 FAILED, 2 NEEDS_UPDATE], `patcher_name` string, `patcher_path` string, `patcher_size` number [int64].
- Status code is ignored; only the body matters. Parse failure or any missing key -> `fnCallback(false,false)` -> "cannot connect" dialog.
- `result 2`: `patcher_name` must be a bare file name (no `/ \ :` or `..`). The client downloads `patcher_path` (full URL, GET, no auth, 30 min timeout), saves it to `<UserData>/GeneralsOnlineData/Update/<patcher_name>`, checks the file size equals `patcher_size`, then runs it with `ShellExecuteExA` verb `runas` (administrator) and quits (`StartDownloadUpdate`, `ContinueUpdate`, `LaunchPatcher`). With `USE_TEST_ENV`/`USE_DEBUG_ON_LIVE_SERVER` the update flag is forced off (`MainMenuUtils.cpp`).

### 4.2 `LoginCode`

- Caller: `NGMP_OnlineServices_AuthInterface::DoFullLoginFlow`.
- `GET {base}/LoginCode`, no auth.
- Response, `GetLoginCodeResponse` `_WITH_DEFAULT`: `success` bool (default false), `login_code` string (default "").
- `statusCode` 4xx -> failure dialog. Otherwise parse; `success == false` or parse error -> "Login Failed / Failed to retrieve login code", then `TheShell->pop()`.

### 4.3 `CheckLogin`

- Caller: `NGMP_OnlineServices_AuthInterface::Tick` (polls every 1000 ms while `m_bWaitingLogin`).
- `POST {base}/CheckLogin`, no auth.
- Request: `code` string (the `login_code`), `client_id` string (`"gen_online_60hz"`), `machine_guid` string (registry `MachineGuid` + `"YY"`), `mac_addr` string (`"AA:BB:CC:DD:EE:FFZZ"` of the first adapter), `vol_serial` string (C: volume serial as `%08X` + `"ZZ"`), `exe_crc` string (hex SHA-256 of the exe `.text` section, `getGameExeCRC`), `ini_crc` number [uint32 `TheGlobalData->m_iniCRC`]. Helper functions are in `SRC/NGMP_Helpers.cpp`.
- Response, `AuthResponse` `_WITH_DEFAULT`: `result` number [`EAuthResponseResult`: -1 CODE_INVALID, 0 WAITING_USER_ACTION, 1 SUCCEEDED, 2 FAILED; default 2], `session_token` string, `refresh_token` string, `user_id` number [int64, default -1], `display_name` string, `ws_uri` string.
- `423` -> stop polling, ban dialog with `ban_reason` (string, optional, read by `GetBanReason`). Every other status is ignored and the body is parsed. `json::parse` exceptions are swallowed and polling continues.

### 4.4 `LoginWithToken`

- Caller: `NGMP_OnlineServices_AuthInterface::BeginLogin`.
- `POST {base}/LoginWithToken`, `Authorization: Bearer <refresh_token>` (service auth disabled).
- Request: `machine_guid`, `mac_addr`, `vol_serial`, `exe_crc` (strings), `ini_crc` (number), same as `CheckLogin` but without `code` and `client_id`.
- Response: `AuthResponse` as in 4.3.
- `423` -> login ban dialog (`ban_reason`). Other `4xx` -> code flow. Else parse: `1` success, `2` code flow, parse error code flow, `-1`/`0` no action (stuck).

### 4.5 `RefreshToken`

- Caller: `NGMP_OnlineServices_AuthInterface::RefreshToken` (from `Tick`).
- `POST {base}/RefreshToken`, `Authorization: Bearer <refresh_token>`, body `""`.
- Response: `AuthResponse`. Needs `result 1` and non-empty `session_token`. `refresh_token` optional (saved only if non-empty). `user_id` used only if not -1, `display_name` only if non-empty.
- `423` -> ban teardown. Other `4xx`, parse errors, `result != 1`, empty `session_token` -> `OnRefreshTokenFailed` (retry rules in 3.3). If no refresh token is held, `RefreshToken` calls `DoFullLoginFlow` instead.

### 4.6 `User`

- Caller: `NGMP_OnlineServices_AuthInterface::LogoutOfMyAccount` (from `MENUS/PopupPlayerInfo.cpp` ~1634).
- `DELETE {base}/User/{user_id}`, auth, body `""`. No callback; response ignored.

### 4.7 `MOTD`

- Caller: `NGMP_OnlineServices_AuthInterface::GoToDetermineNetworkCaps`.
- `GET {base}/MOTD`, auth.
- Response `MOTDResponse` `NLOHMANN_DEFINE_TYPE_INTRUSIVE`: `MOTD` string (required; note the upper-case key). Status ignored. Any failure -> MOTD text becomes "Error retrieving MOTD" and login continues. Shown in `MENUS/WOLWelcomeMenu.cpp` ~244.

### 4.8 `ServiceConfig`

- Caller: `NGMP_OnlineServicesManager::GetAndParseServiceConfig`, called only from `NGMP_OnlineServices_LobbyInterface::JoinLobby` and `::CreateLobby` (not at login).
- `GET {base}/ServiceConfig`, auth.
- Parsed only if `statusCode == 200`; `ServiceConfig` uses `_WITH_DEFAULT`. Any failure keeps the previous config (initially the struct defaults). The join/create continues in all cases. Fields in 6.

### 4.9 `ProvideMWToken`

- Caller: `NGMP_OnlineServices_AuthInterface::SendMiddlewareToken`, only from the anti-cheat plugin login callback (`SRC/PluginInterfaces.cpp` ~448).
- `POST {base}/ProvideMWToken`, auth. Request: `mw_token` string.
- Response body ignored. Status outside `200..299` -> `SetPendingFullTeardown(MIDDLEWARE_LOGIN_FAILED)`.

### 4.10 `ConnectionOutcome`

- Callers: `NetworkMesh::RegisterConnectivity` and `NetworkMesh::UpdateConnectivity` (`SRC/NetworkMesh.cpp` ~786-812).
- `POST {base}/ConnectionOutcome`, auth.
- Request: `target` number [int64 user id], `direct` bool, `outcome` number [`EConnectionState`: 0 NOT_CONNECTED, 1 CONNECTING_DIRECT, 2 FINDING_ROUTE, 3 CONNECTED_DIRECT, 4 CONNECTION_FAILED, 5 CONNECTION_DISCONNECTED; `INC/PluginInterfaces.h`], `ipv4` bool.
- Response ignored entirely.

### 4.11 `Rooms`

- Caller: `NGMP_OnlineServices_RoomsInterface::GetRoomList` (`SRC/OnlineServices_RoomsInterface.cpp`), from `MENUS/WOLLobbyMenu.cpp` ~1472 on entering the lobby menu and from `RestoreSessionState` after a fresh WebSocket session. It also fires `GET Social/Friends`.
- `GET {base}/Rooms`, auth.
- Status must be `200`, else `cb(false)`.
- Response: `rooms` array; each entry `id` number [int] (required, const access), `name` string (required, const access), `flags` number [`ERoomFlags`: 0 default, 1 SHOW_ALL_MATCHES] optional/nullable, `parent_id` number optional/nullable. Top level optional bools via `.value()`: `supports_room_selection_results` (default false), `supports_moderation_commands` (default false).
- After success the lobby menu sends WebSocket `NETWORK_ROOM_CHANGE_ROOM` for room index 0. No rooms or failure -> red chat line "ERROR: No rooms are available. Try logging in again." (no popup).

### 4.12 `Lobbies` (list)

- Caller: `NGMP_OnlineServices_LobbyInterface::SearchForLobbies`, from `refreshGameList` -> `Core/GameEngine/Source/GameNetwork/GameSpy/LobbyUtils.cpp` ~1206. Re-run whenever the WebSocket sends `NETWORK_ROOM_LOBBY_LIST_UPDATE` (7) and on manual refresh.
- `GET {base}/Lobbies`, auth.
- Parsed only if `statusCode == 200`; any exception keeps the previous list and marks it dirty (another refresh later). The UI callback runs in all cases.
- Response shape in 5.1. `latencies` is required (array), `playerlatencies` and `lobbies` may be absent.

### 4.13 `Lobbies` (create)

- Caller: `NGMP_OnlineServices_LobbyInterface::CreateLobby` (after `ServiceConfig`), from `MENUS/PopupHostGame.cpp` ~695.
- `PUT {base}/Lobbies`, auth.
- Request: `name` string (UTF-8), `map_name` string, `map_path` string (file name only, path stripped at the last `\`), `map_official` bool, `max_players` number, `preferred_port` number (always 0), `vanilla_teams` bool, `track_stats` bool, `starting_cash` number [uint32], `passworded` bool, `password` string (always present, may be empty), `allow_observers` bool, `exe_crc` number [uint32 `m_exeCRC`], `ini_crc` number [uint32], `max_cam_height` number [float from `Camera_GetMaxHeight_WhenLobbyHost`, e.g. `310.0`], `anticheat_id` number [int, 0 without plugin].
- Response `CreateLobbyResponse` `NLOHMANN_DEFINE_TYPE_INTRUSIVE` (all four required): `result` number [0 FAILED, 1 SUCCEEDED], `lobby_id` number [int64], `turn_username` string, `turn_token` string.
- Status code is ignored. If any key is missing or the body is not JSON, the exception is swallowed by `catch (...) {}` and no create callback fires (the UI is left waiting; exact UI behaviour not verified). `result 0` -> `InvokeCreateLobbyCallback(false)`.
- On success the client immediately runs `OnJoinedOrCreatedLobby` -> `GET Lobby/{lobby_id}`; the create callback reports that GET's success.

### 4.14 `Lobby/{id}` (detail)

- Caller: `NGMP_OnlineServices_LobbyInterface::UpdateRoomDataCache`. Triggered after create, after join, and on every WebSocket `LOBBY_CURRENT_LOBBY_UPDATE` (6).
- `GET {base}/Lobby/{lobbyID}`, auth.
- `404` -> lobby treated as gone: `m_bPendingHostHasLeft = true`, callback(false), `LeaveCurrentLobby()` (unless a match is in progress).
- Other statuses are not checked; the body is parsed. Any exception -> callback(false) (join/create reported as failed).
- Response: `{"lobby": {...}}`, shape in 5.2.

### 4.15 `Lobby/{id}` (join)

- Caller: `NGMP_OnlineServices_LobbyInterface::JoinLobby` (after `ServiceConfig`), from `MENUS/PopupJoinGame.cpp` ~276, `MENUS/WOLLobbyMenu.cpp` ~2501, and the matchmaking WebSocket message (20).
- `PUT {base}/Lobby/{lobbyID}`, auth.
- Request: `preferred_port` number (always 0), `anticheat_id` number, `has_map` bool, `password` string (only if non-empty).
- Status: `200` success; `401` bad password; `406` full; `417` anti-cheat mismatch; `404` and everything else generic failure.
- Response on 200, `JoinLobbyResponse` `NLOHMANN_DEFINE_TYPE_INTRUSIVE`: `success` bool, `turn_username` string, `turn_token` string. Parse failure is tolerated (TURN credentials stay as before, possibly empty).
- On success the client does `GET Lobby/{id}`; the join is reported as success only if that GET parses (`OnJoinedOrCreatedLobby` callback).
- Before sending, the lobby menu compares the list entry's `ExeCRC`/`IniCRC` with its own (`MENUS/WOLLobbyMenu.cpp` ~2473) and refuses to join on mismatch.

### 4.16 `Lobby/{id}` (update, host and member actions)

- Callers: `NGMP_OnlineServices_LobbyInterface::UpdateCurrentLobby_*`, `UpdateCurrentLobbyMaxCameraHeight`, `SetJoinability`.
- `POST {base}/Lobby/{lobbyID}`, auth. Body always has `field` number [`ELobbyUpdateField`]; other keys in 5.4.
- Responses are ignored (empty callbacks), except `HOST_ACTION_KICK_USER` and `HOST_ACTION_FORCE_START`, whose callbacks send a lobby announcement over the WebSocket regardless of status.

### 4.17 `Lobby/{id}` (leave)

- Callers: `NGMP_OnlineServices_LobbyInterface::LeaveCurrentLobby`; also stale join/create cleanup in `JoinLobby` and `CreateLobby`.
- `DELETE {base}/Lobby/{lobbyID}`, auth, body `""`. No callback.
- Not sent by `ResetForMatchmakingRequeue` (the service is assumed to have removed the player already).

### 4.18 `Lobby/Outcome`

(Not a separate endpoint name: it is `GetAPIEndpoint("Lobby")` + `"/Outcome"`.)

- Caller: `NGMP_OnlineServices_StatsInterface::CommitMyOutcome`, from `NGMP_OnlineServices_LobbyInterface::Tick` once per match when the local player is defeated or wins.
- `POST {base}/Lobby/Outcome`, auth.
- Request: `buildings_built`, `buildings_killed`, `buildings_lost`, `units_built`, `units_killed`, `units_lost`, `total_money` (numbers), `won` bool, `match_id` number [uint64, from lobby `MatchID`], `side` number [player template index, -1 unknown], `desynced` bool, `duration` number [uint32 seconds].
- Response `MatchOutcomeResponse` `NLOHMANN_DEFINE_TYPE_INTRUSIVE`: `screenshot_url` string, `replay_url` string (pre-signed PUT URLs). Status ignored; parse errors swallowed. Empty strings disable the uploads.

### 4.19 `PlayerStats/{id}` (get)

- Caller: `NGMP_OnlineServices_StatsInterface::findPlayerStatsByID` (many UI places; also for every peer on WebSocket `NETWORK_CONNECTION_START_SIGNALLING`).
- `GET {base}/PlayerStats/{userID}`, auth.
- Parsed only for `200..299` and non-empty body; otherwise `cb(false, stats)`.
- Response `{"stats": {...}}`. `stats` is a non-const copy, so missing scalar keys throw (caught -> `cb(false)`), and missing per-general arrays just stay empty.
  - Scalars (all required): `userID` number (must equal the requested id, else ignored), `EloRating`, `EloMatches`, `MonthlyEloRating`, `locale`, `gamesAsRandom` numbers; `options`, `systemSpec` strings; `lastFPS` number [float]; `lastGeneral`, `gamesInRowWithLastGeneral`, `challengeMedals`, `battleHonors`, `QMwinsInARow`, `maxQMwinsInARow`, `winsInARow`, `maxWinsInARow`, `lossesInARow`, `maxLossesInARow`, `disconsInARow`, `maxDisconsInARow`, `desyncsInARow`, `maxDesyncsInARow`, `builtParticleCannon`, `builtNuke`, `builtSCUD`, `lastLadderPort` numbers; `lastLadderHost` string.
  - Per-general arrays of unsigned numbers (element i = general/player-template index i): `wins`, `losses`, `games`, `duration`, `unitsKilled`, `unitsLost`, `unitsBuilt`, `buildingsKilled`, `buildingsLost`, `buildingsBuilt`, `earnings`, `techCaptured`, `discons`, `desyncs`, `surrenders`, `gamesOf2p` ... `gamesOf8p`, `customGames`, `QMGames`.

### 4.20 `PlayerStats` (update)

- Caller: `NGMP_OnlineServices_StatsInterface::UpdateMyStats`, from `MENUS/ScoreScreen.cpp` ~2282.
- `PUT {base}/PlayerStats`, auth.
- Request body is a JSON array, not an object: `JSONSerialize` writes `j[(int)EStatIndex::X] = value`, which turns `j` into an array padded with `null`s. Index layout from `EStatIndex` (`INC/OnlineServices_StatsInterface.h`): 24 per-general groups of 15 slots each (indices 0-359, order `WINS, LOSSES, GAMES, DURATION, UNITSKILLED, UNITSLOST, UNITSBUILT, BUILDINGSKILLED, BUILDINGSLOST, BUILDINGSBUILT, EARNINGS, TECHCAPTURED, DISCONS, DESYNCS, SURRENDERS, GAMESOF2P..GAMESOF8P, CUSTOMGAMES, QUICKMATCHES`), then `LOCALE`=360 ... `LASTLADDERHOST`=383. Within a group the offset is the position in the `std::map` iteration, not the general id (most groups only write entries > 0). The user id is not in the body (the service must use the token).
- Response body ignored. `200..299` -> marks cache stale and re-fetches `PlayerStats/{id}`; else logs.

### 4.21 `PlayerStats/Batch`

- Caller: `NGMP_OnlineServices_StatsInterface::findPlayerStatsByBatch`, from `MENUS/WOLLobbyMenu.cpp` ~995 (lobby player list). Skipped when the id list is empty.
- `POST {base}/PlayerStats/Batch`, auth. Request: `user_ids` array of numbers [int64], de-duplicated.
- Status outside `200..299` -> `cb(false)`.
- Response `{"stats": [ {...}, ... ]}`; each element has the same keys as 4.19's `stats` object, at the top level of the element. Elements are iterated as `const`, so every key, including every per-general array, must be present (missing key = undefined behaviour, can crash). Per-element exceptions are caught. `cb(true)` only if every requested id came back exactly once. The only caller ignores the bool.

### 4.22 `GlobalStats`

- Caller: `NGMP_OnlineServices_StatsInterface::GetGlobalStats`, from `MENUS/WOLWelcomeMenu.cpp` ~389.
- `GET {base}/GlobalStats`, auth. Status ignored; empty body skipped.
- Response `{"globalstats": {"wins": [n,...], "matches": [n,...]}}` (arrays per general). Everything is optional; `cb(stats)` always runs.

### 4.23 `Matchmaking` (start / cancel)

- `NGMP_OnlineServices_MatchmakingInterface::StartMatchmaking` (from `MENUS/WOLQuickMatchMenu.cpp` ~2408): `PUT {base}/Matchmaking`, auth. Request: `playlist` number [uint16], `maps` array of numbers (selected map indexes), `exe_crc` number [uint32], `ini_crc` number [uint32], `anticheat_id` number. The code checks for `201` but does nothing different; the callback gets `bSuccess`, which is always true.
- `NGMP_OnlineServices_MatchmakingInterface::CancelMatchmaking`: `DELETE {base}/Matchmaking`, auth, body `null`. Response ignored. Also calls `LeaveCurrentLobby()`.
- Matches are delivered over the WebSocket (`MATCHMAKING_*` messages, 7.3).

### 4.24 `Matchmaking/Widen`

- `NGMP_OnlineServices_MatchmakingInterface::WidenSearch` (from `MENUS/WOLQuickMatchMenu.cpp` ~2325): `POST {base}/Matchmaking/Widen`, auth, body `""`. Response ignored.

### 4.25 `Matchmaking/Playlists`

- `NGMP_OnlineServices_MatchmakingInterface::RetrievePlaylists` (from `MENUS/WOLQuickMatchMenu.cpp` ~1216): `GET {base}/Matchmaking/Playlists`, auth. Status ignored.
- Response `{"playlists": [ ... ]}`; each entry (const access, all required): `PlaylistID` number [uint16], `Name` string, `MinPlayers` number, `DesiredPlayers` number, `MinSelectedMaps` number, `AllowTeams` bool, `TeamSize` number, `AllowArmySelection` bool, `GracePeriodAtMinPlayersMSec` number [uint16, values above 65535 wrap], `Maps` array of `{ "Name": string, "Path": string, "Custom": bool }` (const access, required).
- On any exception the callback is never invoked (Quick Match menu waits; UI effect not verified).

### 4.26 `Social/Friends`

- `GET {base}/Social/Friends` (`NGMP_OnlineServices_SocialInterface::GetFriendsList`), auth. Called on WebSocket connect, from `GetRoomList`, and from the communicator UI.
  - Response: `friends` array of `{user_id: number, display_name: string, online: bool, presence: string}`, `pending_requests` array of `{user_id, display_name}`. Entries are const (keys required). Status ignored; parse errors swallowed; callback always runs.
- `DELETE {base}/Social/Friends/{target_user_id}` (`RemoveFriend`), auth, body `""`, response ignored.

### 4.27 `Social/Friends/Requests`

- `PUT {base}/Social/Friends/Requests/{target_user_id}` (`AddFriend`), body `""`, response ignored.
- `POST {base}/Social/Friends/Requests/{target_user_id}` (`AcceptPendingRequest`), body `""`, response ignored.
- `DELETE {base}/Social/Friends/Requests/{target_user_id}` (`RejectPendingRequest`), body `""`, response ignored.

### 4.28 `Social/Blocked`

- `GET {base}/Social/Blocked` (`GetBlockList`), auth. Response `blocked` array of `{user_id: number, display_name: string}` (const, required). Errors swallowed; callback always runs.
- `PUT {base}/Social/Blocked/{target_user_id}` (`IgnoreUser`), body `""`, response ignored.
- `DELETE {base}/Social/Blocked/{target_user_id}` (`UnignoreUser`), body `""`, response ignored.

### 4.29 Moderation

`SRC/OnlineServices_Moderation.cpp` has no HTTP endpoint. Moderation reaches the client through HTTP `423` + `ban_reason` (login, refresh) and the WebSocket messages `MODERATION_NOTICE` (46: `action_type` `"ban"`/`"kick"`/`"rate_limit"`, `reason`, `scope_type` `"lobby"` or other) and `MODERATION_COMMAND_RESULT` (48: `request_id`, `success`, `error_code`, `message`), both `_WITH_DEFAULT`.

### 4.30 Endpoint name summary

| Name | Methods | Function(s) |
|---|---|---|
| `VersionCheck` | POST | `NGMP_OnlineServicesManager::StartVersionCheck` |
| `ServiceConfig` | GET | `NGMP_OnlineServicesManager::GetAndParseServiceConfig` |
| `LoginCode` | GET | `AuthInterface::DoFullLoginFlow` |
| `CheckLogin` | POST | `AuthInterface::Tick` |
| `LoginWithToken` | POST | `AuthInterface::BeginLogin` |
| `RefreshToken` | POST | `AuthInterface::RefreshToken` |
| `User/{id}` | DELETE | `AuthInterface::LogoutOfMyAccount` |
| `MOTD` | GET | `AuthInterface::GoToDetermineNetworkCaps` |
| `ProvideMWToken` | POST | `AuthInterface::SendMiddlewareToken` |
| `ConnectionOutcome` | POST | `NetworkMesh::RegisterConnectivity`, `::UpdateConnectivity` |
| `Rooms` | GET | `RoomsInterface::GetRoomList` |
| `Lobbies` | GET, PUT | `LobbyInterface::SearchForLobbies`, `::CreateLobby` |
| `Lobby/{id}` | GET, PUT, POST, DELETE | `UpdateRoomDataCache`, `JoinLobby`, `UpdateCurrentLobby_*` etc., `LeaveCurrentLobby` |
| `Lobby/Outcome` | POST | `StatsInterface::CommitMyOutcome` |
| `PlayerStats/{id}` | GET | `StatsInterface::findPlayerStatsByID` |
| `PlayerStats` | PUT | `StatsInterface::UpdateMyStats` |
| `PlayerStats/Batch` | POST | `StatsInterface::findPlayerStatsByBatch` |
| `GlobalStats` | GET | `StatsInterface::GetGlobalStats` |
| `Matchmaking` | PUT, DELETE | `MatchmakingInterface::StartMatchmaking`, `::CancelMatchmaking` |
| `Matchmaking/Widen` | POST | `MatchmakingInterface::WidenSearch` |
| `Matchmaking/Playlists` | GET | `MatchmakingInterface::RetrievePlaylists` |
| `Social/Friends`, `Social/Friends/{id}` | GET, DELETE | `SocialInterface::GetFriendsList`, `::RemoveFriend` |
| `Social/Friends/Requests/{id}` | PUT, POST, DELETE | `SocialInterface::AddFriend`, `::AcceptPendingRequest`, `::RejectPendingRequest` |
| `Social/Blocked`, `Social/Blocked/{id}` | GET, PUT, DELETE | `SocialInterface::GetBlockList`, `::IgnoreUser`, `::UnignoreUser` |

---

## 5. Lobby JSON

C++ target types: `LobbyEntry` and `LobbyMemberEntry` (`INC/OnlineServices_LobbyInterface.h`; `LobbyMemberEntry` derives from `NetworkMemberBase` in `INC/NGMP_types.h`). Key names are PascalCase, unlike the request bodies.

### 5.1 List response (`GET Lobbies`, `SearchForLobbies`)

```json
{
  "lobbies": [
    {
      "LobbyID": 1, "Owner": 123, "Name": "My game",
      "MapName": "Alpine Assault", "MapPath": "alpine assault", "IsMapOfficial": true,
      "NumCurrentPlayers": 1, "MaxPlayers": 2,
      "IsVanillaTeamsOnly": false, "StartingCash": 10000, "IsLimitSuperweapons": false,
      "IsTrackingStats": true, "IsPassworded": false, "AllowObservers": true,
      "MaximumCameraHeight": 310, "ExeCRC": 0, "IniCRC": 0, "MatchID": 0,
      "LobbyType": 0, "Region": "EU",
      "Members": [
        { "UserID": 123, "DisplayName": "Name", "IsReady": true,
          "SlotIndex": 0, "SlotState": 5, "Region": "EU" }
      ]
    }
  ],
  "latencies": [42],
  "playerlatencies": [ { "user_id": 123, "latency": 42 } ]
}
```

| Key | JSON | C++ field | Use |
|---|---|---|---|
| `LobbyID` | number | `lobbyID` int64 | id for `Lobby/{id}` |
| `Owner` | number | `owner` int64 | host user id; friend filter |
| `Name` | string | `name` | display; game-mode filter by name (`detectGameMode` in `LobbyUtils.cpp`) |
| `MapName` | string | `map_name` | display |
| `MapPath` | string | `map_path` | converted with `utf8_to_local`, then prefixed `maps\` if official, else `<user map dir>\`; used for `TheMapCache->findMap` |
| `IsMapOfficial` | bool | `map_official` | path prefix |
| `NumCurrentPlayers` | number | `current_players` int | display, "full" check |
| `MaxPlayers` | number | `max_players` int | display, "full" check |
| `IsVanillaTeamsOnly` | bool | `vanilla_teams` | |
| `StartingCash` | number | `starting_cash` uint32 | |
| `IsLimitSuperweapons` | bool | `limit_superweapons` | |
| `IsTrackingStats` | bool | `track_stats` | |
| `IsPassworded` | bool | `passworded` | password prompt before join |
| `AllowObservers` | bool | `allow_observers` | |
| `MaximumCameraHeight` | number | `max_cam_height` uint16 | |
| `ExeCRC` | number | `exe_crc` uint32 | compared to local before join |
| `IniCRC` | number | `ini_crc` uint32 | compared to local before join |
| `MatchID` | number | `match_id` uint64 | |
| `LobbyType` | number | `lobby_type` `ELobbyType` (-1 UNKNOWN, 0 CustomGame, 1 QuickMatch) | |
| `Region` | string | `region` | tooltip "Region: %s" (`LobbyUtils.cpp` ~380) |
| `Members[]` | array | `members` | see table below |

List member keys: `UserID` (int64), `DisplayName` (string), `IsReady` (bool -> `m_bIsReady`), `SlotIndex` (number -> uint16 `m_SlotIndex`), `SlotState` (number -> uint16 `m_SlotState`), `Region` (string, shown in tooltip with latency).

`latencies[i]` is matched to `lobbies[i]` by index; missing entries become 9001. `playerlatencies` maps `user_id` to `latency` for members (default 0).

Every key inside a lobby entry and inside a member entry is accessed through a const reference: all are mandatory (5 at the top of section 4). `latencies` must be an array (missing -> throw -> list not updated). `lobbies` and `playerlatencies` may be omitted.

### 5.2 Detail response (`GET Lobby/{id}`, `UpdateRoomDataCache`)

`{"lobby": { ... }}` with every key of 5.1 plus `RNGSeed` (number -> `rng_seed` int, passed to `NGMPGame::setSeed`). The `lobby` object is a non-const copy, so a missing lobby-level key throws (caught -> callback false). Members are iterated as const (all keys mandatory):

| Member key | JSON | C++ | Use |
|---|---|---|---|
| `UserID` | number | `user_id` int64 | local slot detection (`NGMPGame::getLocalSlotNum` matches the auth `user_id`); -1 for AI/empty |
| `DisplayName` | string | `display_name` | slot name |
| `IsReady` | bool | `m_bIsReady` | accept flag |
| `Port` | number | `preferredPort` uint16 | |
| `Side` | number | `side` int | player template; -1 random, -2 observer (`PLAYERTEMPLATE_*`, `Core/GameEngine/Include/GameNetwork/GameInfo.h`) |
| `Color` | number | `color` int | -1 random |
| `Team` | number | `team` int | -1 none |
| `StartingPosition` | number | `startpos` int | -1 random |
| `HasMap` | bool | `has_map` | |
| `SlotState` | number | `m_SlotState` uint16 | `SlotState` enum: 0 OPEN, 1 CLOSED, 2 EASY_AI, 3 MED_AI, 4 BRUTAL_AI, 5 PLAYER |
| `SlotIndex` | number | `m_SlotIndex` uint16 | must be < 8 (`MAX_SLOTS`) or the entry is skipped |
| `Region` | string | `region` | player info tooltip (`MENUS/WOLGameSetupMenu.cpp` ~574) |
| `MiddlewareUserID` | string | `middlewareUserID` | anti-cheat only |

Notes from `UpdateRoomDataCache` and `NGMPGame::UpdateSlotsFromCurrentLobby` (`SRC/NGMPGame.cpp`):

- `NGMPGame::UpdateSlotsFromCurrentLobby` reads `members[i]` for `i = 0..7` and uses each member's `SlotIndex` to pick the game slot. The service should return one member entry per slot (humans, AI, open and closed), with human players at `SlotState 5`. Not verified whether the real service always returns 8 entries or `MaxPlayers` entries.
- A human is detected with `user_id != -1 && SlotState == 5` (`LobbyMemberEntry::IsHuman`). If the local user was a human member before and is missing now, the client treats it as a kick (`OnKickedFromLobby`).
- `Owner` changing (and `LobbyType != 1`) sets `m_bHostMigrated`.
- `MatchID` is stored as the current match id (used for `Lobby/Outcome` and uploads).
- `LobbyType == 1` marks the game as Quick Match.
- If `MapPath` differs from the current map, the client sends `LOCAL_PLAYER_HAS_MAP`.
- During gameplay (game in progress and load screen not active) the members list in the response is ignored.

### 5.3 Region / server-mode markers and extra fields

- Region-like fields: lobby `Region` (string), member `Region` (string), plus `latencies` / `playerlatencies`. All are display-only in the client (tooltips). `EQoSRegions` and `RegionResponse` (`INC/OnlineServices_Init.h`) are declared but not used anywhere.
- `LobbyType` is read into an enum without range checking; the client only compares it with `1` (QuickMatch). Any other value behaves like a custom game. Not verified whether other code paths switch on it.
- `Name` is the only field the lobby list filter looks at (`detectGameMode`).
- Extra unknown keys in lobby objects, member objects or the root are ignored (all parsing is by explicit key). Adding a new field (for example a server mode flag) is safe for the current client, but the client will not use it without code changes.

### 5.4 `ELobbyUpdateField` and request bodies

Enum in `SRC/OnlineServices_LobbyInterface.cpp`. All are `POST {base}/Lobby/{currentLobbyID}`.

| Value | Name | Body keys besides `field` | Sender function |
|---|---|---|---|
| 0 | `LOBBY_MAP` | `map` string, `map_path` string (file name only), `map_official` bool, `max_players` number | `UpdateCurrentLobby_Map` |
| 1 | `MY_SIDE` | `side` number, `start_pos` number | `UpdateCurrentLobby_MySide` |
| 2 | `MY_COLOR` | `color` number | `UpdateCurrentLobby_MyColor` |
| 3 | `MY_START_POS` | `startpos` number (note: no underscore) | `UpdateCurrentLobby_MyStartPos` |
| 4 | `MY_TEAM` | `team` number | `UpdateCurrentLobby_MyTeam` |
| 5 | `LOBBY_STARTING_CASH` | `startingcash` number [uint32] | `UpdateCurrentLobby_StartingCash` |
| 6 | `LOBBY_LIMIT_SUPERWEAPONS` | `limit_superweapons` bool | `UpdateCurrentLobby_LimitSuperweapons` |
| 7 | `HOST_ACTION_FORCE_START` | none | `UpdateCurrentLobby_ForceReady` (marks everyone ready; name is misleading) |
| 8 | `LOCAL_PLAYER_HAS_MAP` | `has_map` bool | `UpdateCurrentLobby_HasMap` |
| 9 | `UNUSED_1` | - | not sent |
| 10 | `UNUSED_2` | - | not sent |
| 11 | `HOST_ACTION_KICK_USER` | `userid` number [int64] | `UpdateCurrentLobby_KickUser` |
| 12 | `HOST_ACTION_SET_SLOT_STATE` | `slot_index` number [uint16], `slot_state` number [uint16, `SlotState`] | `UpdateCurrentLobby_SetSlotState` |
| 13 | `AI_SIDE` | `slot` number, `side` number, `start_pos` number | `UpdateCurrentLobby_AISide` |
| 14 | `AI_COLOR` | `slot` number, `color` number | `UpdateCurrentLobby_AIColor` |
| 15 | `AI_TEAM` | `slot` number, `team` number | `UpdateCurrentLobby_AITeam` |
| 16 | `AI_START_POS` | `slot` number, `start_pos` number | `UpdateCurrentLobby_AIStartPos` |
| 17 | `MAX_CAMERA_HEIGHT` | `max_camera_height` number [uint16] | `UpdateCurrentLobbyMaxCameraHeight` (host only) |
| 18 | `JOINABILITY` | `joinability` number (0 public, 1 friends only; `ELobbyJoinability`) | `SetJoinability` (host only) |

Password changes do not use HTTP; they go over the WebSocket (`LOBBY_CHANGE_PASSWORD` 25 with `new_password`, `LOBBY_REMOVE_PASSWORD` 24). Ready state is WebSocket `NETWORK_ROOM_MARK_READY` (5, `ready` bool). After any accepted change the service is expected to push `LOBBY_CURRENT_LOBBY_UPDATE` (6) so members re-fetch the lobby; the client does not update its own copy from the POST.

---

## 6. `ServiceConfig`

Struct in `INC/OnlineServices_Init.h`, `NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT`, so `{}` yields all defaults. Fetched only before lobby join/create (4.8); the last good copy is kept.

| Field | Type | Default | What the client does with it |
|---|---|---|---|
| `retry_signalling` | bool | false | Whether a failed peer connection is re-signalled (`NetworkMesh.cpp` ~117, connection status callback) |
| `use_mapped_port` | bool | true | If true the remote virtual port is the peer's `preferred_port`, else 0 (`NetworkMesh.cpp` ~955). The client always sends `preferred_port 0`, so the service should send 0 back |
| `min_run_ahead_frames` | int | 4 | `MIN_RUNAHEAD` at match start (`NGMPGame.cpp` ~432) unless `use_default_config` |
| `ra_update_frequency_frames` | int | 10 | `m_networkRunAheadMetricsTime = value * (1000/60)` ms (`NGMPGame.cpp` ~435) |
| `relay_all_traffic` | bool | false | `NetworkMesh::NetworkMesh`: true sets `k_ESteamNetworkingConfig_P2P_Transport_ICE_Enable` to `ICE_Enable_Relay` (TURN only, via the hard-coded `turn.playgenerals.online` and the lobby's `turn_username`/`turn_token`); false = `ICE_Enable_All` |
| `ra_slack_percent` | int | 20 | `m_networkRunAheadSlack` (`NGMPGame.cpp` ~434) |
| `frame_grouping_frames` | int | 2 | `FRAME_GROUPING_CAP = value * (1000/60)` (`NGMPGame.cpp` ~437) |
| `enable_host_migration` | bool | true | Host migration handling in the setup menu (`MENUS/WOLGameSetupMenu.cpp` ~2513) |
| `network_do_immediate_flush_per_frame` | bool | true | `NetworkMesh::Flush` flushes every connection each tick |
| `network_send_flags` | int | -1 | Read in `NetworkMesh.cpp` ~1339 into a local that is never used (send flags are fixed `Reliable | AutoRestartBrokenSession`) |
| `network_latency_logic_model` | int | 0 | `ConnectionManager::getMaximumLatency` (`Core/GameEngine/Source/GameNetwork/ConnectionManager.cpp` ~1645): 0 original, 1-4 variants using Valve latency |
| `use_default_config` | bool | false | true: retail values (FPS history 30, latency history 200, slack = `ra_slack_override_percent_in_default`, metrics time 5000) and ignores the run-ahead fields above |
| `ra_slack_override_percent_in_default` | int | 10 | Slack when `use_default_config` |
| `do_probes` | bool | true | Screenshot capture for `PROBE` and load/score screens (`CaptureScreenshotForProbe`) |
| `do_replay_upload` | bool | true | Replay caching for upload (`CommitReplay`) |
| `network_mesh_histogram_duration` | int | 20000 | `NetworkMesh.cpp` ~1427 |
| `ibra_ra_tweaks` | bool | false | Enables jitter-based dynamic slack in `ConnectionManager.cpp` ~1427 |
| `ibra_minslack_default` / `ibra_maxslack_default` | float | 0.25 / 1.0 | Dynamic slack range, max latency <= 200 ms |
| `ibra_minslack_greaterthan200ms` / `ibra_maxslack_greaterthan200ms` | float | 0.3 / 1.0 | 200-300 ms |
| `ibra_minslack_greaterthan300ms` / `ibra_maxslack_greaterthan300ms` | float | 0.35 / 1.0 | > 300 ms |
| `screenshot_width` / `screenshot_height` | int | 557 / 333 | Resize for uploaded screenshots |
| `ice_implementation` | int | 2 | 0 library default, 1 native, 2 WebRTC; out of range -> 2 (`NetworkMesh::NetworkMesh`) |
| `use_new_networking` | bool | false | Passed to the anti-cheat plugin's secure transport (`PluginInterfaces.cpp` ~510) |

Run-ahead values are applied when the match starts (`NGMPGame` ~410-440) from whatever config was last fetched.

---

## 7. Needed versus stub-able

### 7.1 Table

"Needed" = needed for a player to log in, see lobbies, create or join a lobby and start a match.

| Endpoint / channel | Needed? | Minimal response that keeps the client quiet |
|---|---|---|
| `POST VersionCheck` | Needed | `{"result":0,"patcher_name":"","patcher_path":"","patcher_size":0}` |
| `GET LoginCode` | Needed (first login) | `{"success":true,"login_code":"x"}` |
| `POST CheckLogin` | Needed (first login) | `{"result":1,"session_token":"...","refresh_token":"...","user_id":N,"display_name":"...","ws_uri":"wss://..."}` |
| `POST LoginWithToken` | Needed (later logins) | same as `CheckLogin`; or any `4xx` to force the code flow |
| `POST RefreshToken` | Needed for sessions > 10 min | `{"result":1,"session_token":"..."}` |
| WebSocket at `ws_uri` | Needed | see 7.3 |
| `GET MOTD` | Stub | `{"MOTD":""}` (any failure is silent) |
| `GET Rooms` | Stub (but shows a red chat line if it fails) | `{"rooms":[{"id":1,"name":"Lobby"}]}` plus a WebSocket member list reply (7.3) |
| `GET Lobbies` | Needed | `{"lobbies":[...],"latencies":[]}` |
| `PUT Lobbies` | Needed | `{"result":1,"lobby_id":N,"turn_username":"","turn_token":""}` (all four keys) |
| `GET Lobby/{id}` | Needed | `{"lobby":{...}}` per 5.2 |
| `PUT Lobby/{id}` | Needed | `200` with `{"success":true,"turn_username":"","turn_token":""}` |
| `POST Lobby/{id}` | Needed for settings to take effect | any status; push WS 6 to members afterwards |
| `DELETE Lobby/{id}` | Needed for clean lobbies | any status |
| `GET ServiceConfig` | Stub | `{}` or any error (defaults used) |
| `POST Lobby/Outcome` | Stub | `200` `{"screenshot_url":"","replay_url":""}` (or anything; errors swallowed) |
| `POST ConnectionOutcome` | Stub | anything |
| `GET PlayerStats/{id}` | Stub | `404` (client logs and continues) or a full object per 4.19 |
| `PUT PlayerStats` | Stub | `200` triggers a re-GET; any other status just logs |
| `POST PlayerStats/Batch` | Stub | `{"stats":[]}` (caller ignores the result) |
| `GET GlobalStats` | Stub | `{}` |
| `GET Social/Friends` | Stub | `{"friends":[],"pending_requests":[]}` (anything works) |
| `GET Social/Blocked` | Stub | `{"blocked":[]}` (anything works) |
| `PUT/POST/DELETE Social/...` | Stub | anything |
| `GET Matchmaking/Playlists` | Stub (Quick Match only) | `{"playlists":[]}` |
| `PUT/DELETE Matchmaking`, `POST Matchmaking/Widen` | Stub (Quick Match only) | anything |
| `DELETE User/{id}` | Stub | anything |
| `POST ProvideMWToken` | Only with an anti-cheat plugin | `200` (non-2xx tears the session down) |

Upload URLs: return empty strings (or set `do_probes`/`do_replay_upload` false) and the client uploads nothing.

### 7.2 Lobby to match, HTTP part

1. Host: `GET ServiceConfig`, `PUT Lobbies`, `GET Lobby/{id}`.
2. Joiner: `GET ServiceConfig`, `PUT Lobby/{id}`, `GET Lobby/{id}`.
3. Settings: `POST Lobby/{id}` with `field`, followed by WS 6 from the service.
4. Match start is WebSocket-driven (7.3). `MatchID` in the lobby detail should be set by the time the match starts (used for the outcome and uploads; 0 disables replay caching in `CommitReplay`).

### 7.3 WebSocket behaviour needed for login and match start

This is outside the HTTP contract but the client cannot log in or start a match without it. Implementation: `WebSocket` in `SRC/OnlineServices_RoomsInterface.cpp`; ids in `EWebSocketMessageID`, `INC/OnlineServices_Init.h`.

- Upgrade request headers: `Authorization: Bearer <session_token>`, `is-reconnect: true|false`. A reconnect answered with HTTP `205` makes the client start a fresh session (`m_bFreshSession`) and later call `RestoreSessionState` (friends, blocked, rooms, rejoin room).
- The client sends its JSON as WebSocket **binary** frames (`CURLWS_BINARY` in `WebSocket::Tick` and `Disconnect`). It only parses server **text** frames; server binary frames are ignored. Every JSON message has `msg_id` (number).
- Liveness: the client sends `{"msg_id":14}` (and a WS ping control frame) every 1000 ms. Any server frame updates `m_lastPong`; after 20 s without one the client reconnects (`m_timeForWSTimeout`). In menus, 30 s of failed reconnects -> teardown (`m_reconnectMenuWindow`). A server close frame -> immediate teardown.
- Minimum server-to-client messages for the match path:
  - 4 `NETWORK_ROOM_MEMBER_LIST_UPDATE`: `members` array of `{UserID, Name, IsAdmin}` (const access, all required); optional `selected_room_id`, `effective_room_id`, `rejected_room_id`, `room_selection_error`, `request_id`. Reply to the client's room change (3: `room`, optional `request_id`) within 10 s, else a "Couldn't join that room" chat line.
  - 6 `LOBBY_CURRENT_LOBBY_UPDATE` (no fields): client re-GETs `Lobby/{id}`.
  - 7 `NETWORK_ROOM_LOBBY_LIST_UPDATE` (no fields): client re-GETs `Lobbies`.
  - 17 `NETWORK_CONNECTION_START_SIGNALLING`: `lobby_id`, `user_id`, `preferred_port` (send 0), `middleware_id` (string) — all required (`NLOHMANN_DEFINE_TYPE_INTRUSIVE`). Starts the P2P connection to that peer.
  - 12 `NETWORK_SIGNAL`: relay of the client's 12 messages (`target_user_id`, `payload` byte array) to the target; the receiver reads `target_user_id` and `payload` (both required) and only uses `payload`.
  - 27 `FULL_MESH_CONNECTIVITY_CHECK_RESPONSE` (to members, optional `mesh_check_id`, `attempt`) and 28 `FULL_MESH_CONNECTIVITY_CHECK_RESPONSE_COMPLETE_TO_HOST` (to the host, `mesh_complete` bool required, optional `reason`, `missing_connections` `[{source_user_id,target_user_id}]`). The host sends 26 when everyone is ready and only starts the countdown after 28 with `mesh_complete: true`. Members answer 27 with `connectivity_map` and `connecting_map` arrays of user ids.
  - 13 `START_GAME`: `screenshot_url` (string; parse failure tolerated). Sent by the host when the countdown ends; the service must deliver 13 to the lobby members to start the match (whether the host also needs it echoed back is not verified; it is safe to send it to everyone).
  - 15 `PONG` (no fields).
- Other server messages the client handles: 2 room chat (`message`, `action`, `admin`, `name_change`), 11 lobby chat (`message`, `action`, `announcement`, `show_announcement_to_host`, `user_id`), 16 probe (`url`), 18 disconnect player (`lobby_id`, `user_id`), 20/21/22/44/45 matchmaking, 29/31/32/35/36/37/38 social, 40/41/8 anti-cheat, 42 keepalive (client answers 43 with its loaded module list), 46/48 moderation.

---

## 8. Not determinable from the client code

- Whether the service expects or validates `Content-Type`, and what the real service returns for HTTP statuses on endpoints whose status the client ignores.
- The real service's lobby member list length (8 entries vs `MaxPlayers`), slot assignment rules, and how it applies each `ELobbyUpdateField` (for example whether `MY_SIDE` also moves the start position, ready reset rules).
- Who generates `RNGSeed` and `MatchID` and when `MatchID` becomes non-zero (client only reads them).
- Exact session and refresh token lifetimes on the server (client assumes about 15 min session, refreshes at 10 min; nothing about the refresh token).
- What the service does with `machine_guid`, `mac_addr`, `vol_serial`, `exe_crc`, `ini_crc`, `execrc`, `anticheat_id`.
- The meaning the service gives to `target_user_id` in a relayed `NETWORK_SIGNAL` (client ignores it) and the payload format (produced by the SteamNetworkingSockets signalling client, opaque to the service).
- Whether the host needs its own `START_GAME` echoed (see 7.3); the exact UI behaviour when `PUT Lobbies` returns an unparseable body or `Matchmaking/Playlists` fails (callbacks are never invoked; not run).
- TURN credential format expected by `turn.playgenerals.online` (the client passes `turn_username`/`turn_token` through as TURN user/pass lists).
- The `PUT PlayerStats` array index mapping as interpreted by the service (the client's per-general offsets come from map iteration order, see 4.20).
- Nothing was verified on the wire or by running the client; all statements come from reading code.
