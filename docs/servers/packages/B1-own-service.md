# B1 Client talks only to our own service

State: done
Branch: servers-go-B1

## Goal

Put every outside address of the client in one header, point them at our service, and remove everything the client reports to the Generals Online team or lets a service make it do.

## Depends on

0.2

## Read first

- DECISIONS.md D36, D44, D46, D47, D48, D49
- SERVICE.md sections 2, 4 and 12
- reference/CLIENT_HTTP.md sections 1 (all hosts), 2.5 (TLS), 3 (login flow)
- reference/CLIENT_REALTIME.md sections 1.5 (keep-alive reply), 6 (anti-cheat plugin), 7 (third-party pieces and hosts)
- Each function named in those sections, in full

## May edit

- New file `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/ServiceHosts.h`
- `GeneralsMD/Code/GameEngine/Source/GameNetwork/GeneralsOnline/OnlineServices_Init.cpp`, `OnlineServices_Auth.cpp`, `OnlineServices_RoomsInterface.cpp`, `OnlineServices_Moderation.cpp`, `NetworkMesh.cpp`, `DiscordRichPresence.cpp`, `GeneralsOnline_Settings.cpp`, `PluginInterfaces.cpp`
- `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/OnlineServices_Init.h`, `NextGenMP_defines.h`
- `GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp` (the crash-reporting and Discord start calls, and the player-facing text that shows the service name)
- `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/ScoreScreen.cpp`, `WOLLobbyMenu.cpp`, `WOLGameSetupMenu.cpp`, `WOLWelcomeMenu.cpp` (only the lines that open a web link or show the service name)
- `Core/GameEngine/Source/GameClient/Intro.cpp`, `Core/GameEngine/Source/GameNetwork/GameSpy/MainMenuUtils.cpp` (only the text that names the web site or shows the service name)
- `docs/servers/TESTING.md`

## Must not edit

- Lobby, match or netcode logic in any of those files
- `vcpkg.json` and the build files (the libraries stay linked)

## Done when

- [x] `ServiceHosts.h` holds every address the client uses: API base, STUN list, TURN list, and the service's display name. Nothing else in the code contains a host name.
- [x] A Debug build uses `http://127.0.0.1:9000` as the API base. A Release build uses the value of one constant in `ServiceHosts.h`, set to `https://lobby.example.net` until the project owner supplies the real one. The path after the host stays `/env/<env>/contract/1/`.
- [x] The "alternative endpoint" setting no longer changes any address.
- [x] STUN and TURN lists come from `ServiceHosts.h`. Debug: empty STUN list is not allowed by the library, so use `stun:127.0.0.1:3478`; TURN list `turn:127.0.0.1:3478`. Release: placeholders on `example.net`.
- [x] Crash reporting is never initialised and sends nothing, in any build.
- [x] Discord presence is never started.
- [x] Login never opens a web browser (`DoFullLoginFlow`). The "continue in your web browser" text becomes "Logging in".
- [x] `machine_guid`, `mac_addr` and `vol_serial` are no longer sent at login. `exe_crc`, `ini_crc` and `client_id` stay.
- [x] The client no longer answers message 42 with its module list, and ignores message 16 (no screenshot is taken or uploaded).
- [x] A "needs update" answer from the version check is ignored: nothing is downloaded and nothing is run. The download and patcher-launch functions are not reachable.
- [x] No anti-cheat plugin is loaded: the load call is skipped, including the fixed debug path.
- [x] Buttons and links that open a Generals Online or Discord web page are hidden or do nothing. The "view match online" button on the score screen is hidden.
- [x] Text shown to players uses the display name from `ServiceHosts.h` in place of "Generals Online". The display name is `Servers Community` until the project owner supplies one.
- [x] `git grep -n -i -E "playgenerals|sentry\.io|gamereplays" -- "*.cpp" "*.h"` prints nothing outside comments that this package did not write.
- [x] Two instances of a Debug build keep separate credential files, as they do today.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. These changes are not behind the Servers switch.
- [ ] Reviewed by someone other than the author.

## Out of scope

- The Servers button and anything behind `RTS_SERVERS` (D1).
- Removing the linked libraries from the build.
- A live login against the lobby service. That is the Phase 1 check.

## Handoff notes

- Finished:
  - New `GeneralsMD/Code/GameEngine/Include/GameNetwork/GeneralsOnline/ServiceHosts.h`: `ServiceHosts::API_BASE_URL`, `STUN_SERVER_LIST`, `TURN_SERVER_LIST`, `DISPLAY_NAME`, `DISPLAY_NAME_WIDE`. Debug: `http://127.0.0.1:9000`, `stun:127.0.0.1:3478`, `turn:127.0.0.1:3478`. Release: `https://lobby.example.net`, `stun:stun.example.net:3478`, `turn:turn.example.net:3478`. Display name `Servers Community`.
  - `OnlineServices_Init.cpp`: `GetAPIEndpoint` builds `<base>/env/<dev|test|prod>/contract/1/<Endpoint>`; the alternative endpoint and the alternative WebSocket address are gone; an update offer in the version check is logged and answered as "up to date" (nothing cached); the two "updatefailed" browser launches are gone; the plugin load call in `Init` is gone; `InitSentry` and `ShutdownSentry` are removed and `InitSentry` is replaced by `InitCurl` (it only calls `curl_global_init`, kept at the same point in `GameEngine::init`).
  - `NextGenMP_defines.h`: `GENERALS_ONLINE_USE_SENTRY` removed, so the CRC-mismatch report in `GameLogic.cpp` is no longer compiled. `OnlineServices_Init.h`: `<sentry.h>` include removed, `InitCurl` declared.
  - `GameEngine.cpp`: sentry start and close calls replaced, Discord presence is not created, logged-out / lost-connection / auth-failed texts use the display name.
  - `OnlineServices_Auth.cpp`: hardware identifiers removed from `LoginWithToken` and `CheckLogin`; no `ShellExecuteA`; wait box says "Logging in".
  - `OnlineServices_RoomsInterface.cpp`: message 42 and message 16 are ignored (cases kept, no reply); module list code, probe message class and the two sentry events removed.
  - `NetworkMesh.cpp`: STUN and TURN lists come from `ServiceHosts.h`; TURN user and password lists now hold one value because the list has one entry.
  - `PluginInterfaces.cpp`: the fixed `F:\Gen\...` debug path is removed.
  - `OnlineServices_Moderation.cpp`: ban and kick dialogs have only a Close button; text uses the display name.
  - `WOLLobbyMenu.cpp`, `WOLGameSetupMenu.cpp`: `/support` help line and command removed. `WOLWelcomeMenu.cpp`: welcome title uses the display name. `ScoreScreen.cpp`: match page links removed, the button is hidden.
  - `Intro.cpp`: web site line removed. `MainMenuUtils.cpp`: update dialog text no longer names a site; ARM message uses the display name. `DiscordRichPresence.cpp`: image text uses the display name. `TESTING.md`: section "Where the client connects".
- Left to do: only the reviewer. Not run: no live login against a service and no game start (that is the Phase 1 check).
- Surprises and open questions:
  - Verified: both Release configurations contain `lobby.example.net` and the Debug configuration contains `127.0.0.1:9000` (string search in the built executables); none of the three contains `playgenerals`, `sentry.io` or `gamereplays`.
  - Built: `win32-vcpkg` Release and `win32-vcpkg-servers` Release and Debug, target `z_generals`, all linked. Warnings seen are existing ones (C4150 in `OnlineServices_Init.h`, C4018 in `OnlineServices_RoomsInterface.h`, C5055 in `W3DView.cpp`). Lint: not run.
  - The package says `Intro.cpp` and `MainMenuUtils.cpp` may change only text naming the web site, and `GameEngine.cpp` only the crash-reporting and Discord calls. I also changed three player-facing "Generals Online" texts in `GameEngine.cpp` and the ARM message in `MainMenuUtils.cpp`, because the Done-when item on the display name needs them. Revert those lines if the project owner disagrees.
  - Left as it is, for the project owner to decide: the intro logo screen still credits "GeneralsOnline" (the source team) in `Intro.cpp`; `DiscordRichPresence.cpp` still holds the Discord application id of the previous team (presence is never started, so it is never used) and the class is still compiled; game text files (`.str`) may hold more "Generals Online" strings, they are data and outside the file list.
  - The download and patcher-launch functions (`StartDownloadUpdate`, `ContinueUpdate`, `LaunchPatcher`) still exist because `MainMenuUtils.cpp` calls them and only its text may change. They are unreachable: the only way in is `bNeedsUpdate`, which the version check never sets to true. Removing them needs a change to the menu code in a later package.
  - Unused after this package, left because their files are not in "May edit" or the header is not: `GenOnlineSettings::GetAnticheatPlugin`, `Network_UseAlternativeEndpoint` (still read and written in `settings.json`), `GetMachineGuid`, `GetPrimaryMacAddress`, `GetVolumeSerial` (`NGMP_Helpers.cpp`), `GSMessageBoxOkCancelWithLabels`.
  - `sentry::sentry` stays linked and `crashpad_handler.exe` is still copied next to the executable by the build files (they are not editable here). Crash reporting itself never starts.
  - Screenshots and replay upload are not removed in the client: they only run when the service config has `do_probes` / `do_replay_upload` true and the service gives upload addresses. SERVICE.md section 9 sets both to false.
  - The client limits a new name to 16 characters (`WOLLobbyMenu.cpp`), SERVICE.md section 4 says 3 to 20. Not part of this package.
  - PROTOCOL.md section 10 assumptions: none of them concern this package.
- Review findings and fixes:
  - Scope of `GameEngine.cpp` and `MainMenuUtils.cpp`: the display-name text edits stay; the "May edit" entries now allow player-facing text that shows the service name.
  - `OnlineServices_RoomsInterface.cpp`: removed the unused `ToWide` and `ToUtf8` helpers and the unused `<psapi.h>`, `<wincrypt.h>`, `<softpub.h>` includes with their three `#pragma comment(lib, ...)` lines (searched: no other use in `GeneralsMD` or `Core`).
  - `DiscordRichPresence.cpp`: the previous team's Discord application id is replaced by an empty string; the only use is the `m_initialize` call, and presence is never started.
  - Rebuilt: `win32-vcpkg` Release, `win32-vcpkg-servers` Release and Debug, target `z_generals`, all linked, exit 0. Lint: not run.
