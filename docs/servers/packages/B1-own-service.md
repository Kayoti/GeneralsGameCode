# B1 Client talks only to our own service

State: todo
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
- `GeneralsMD/Code/GameEngine/Source/Common/GameEngine.cpp` (the crash-reporting and Discord start calls only)
- `GeneralsMD/Code/GameEngine/Source/GameClient/GUI/GUICallbacks/Menus/ScoreScreen.cpp`, `WOLLobbyMenu.cpp`, `WOLGameSetupMenu.cpp`, `WOLWelcomeMenu.cpp` (only the lines that open a web link or show the service name)
- `Core/GameEngine/Source/GameClient/Intro.cpp`, `Core/GameEngine/Source/GameNetwork/GameSpy/MainMenuUtils.cpp` (only the text that names the web site)
- `docs/servers/TESTING.md`

## Must not edit

- Lobby, match or netcode logic in any of those files
- `vcpkg.json` and the build files (the libraries stay linked)

## Done when

- [ ] `ServiceHosts.h` holds every address the client uses: API base, STUN list, TURN list, and the service's display name. Nothing else in the code contains a host name.
- [ ] A Debug build uses `http://127.0.0.1:9000` as the API base. A Release build uses the value of one constant in `ServiceHosts.h`, set to `https://lobby.example.net` until the project owner supplies the real one. The path after the host stays `/env/<env>/contract/1/`.
- [ ] The "alternative endpoint" setting no longer changes any address.
- [ ] STUN and TURN lists come from `ServiceHosts.h`. Debug: empty STUN list is not allowed by the library, so use `stun:127.0.0.1:3478`; TURN list `turn:127.0.0.1:3478`. Release: placeholders on `example.net`.
- [ ] Crash reporting is never initialised and sends nothing, in any build.
- [ ] Discord presence is never started.
- [ ] Login never opens a web browser (`DoFullLoginFlow`). The "continue in your web browser" text becomes "Logging in".
- [ ] `machine_guid`, `mac_addr` and `vol_serial` are no longer sent at login. `exe_crc`, `ini_crc` and `client_id` stay.
- [ ] The client no longer answers message 42 with its module list, and ignores message 16 (no screenshot is taken or uploaded).
- [ ] A "needs update" answer from the version check is ignored: nothing is downloaded and nothing is run. The download and patcher-launch functions are not reachable.
- [ ] No anti-cheat plugin is loaded: the load call is skipped, including the fixed debug path.
- [ ] Buttons and links that open a Generals Online or Discord web page are hidden or do nothing. The "view match online" button on the score screen is hidden.
- [ ] Text shown to players uses the display name from `ServiceHosts.h` in place of "Generals Online". The display name is `Servers Community` until the project owner supplies one.
- [ ] `git grep -n -i -E "playgenerals|sentry\.io|gamereplays" -- "*.cpp" "*.h"` prints nothing outside comments that this package did not write.
- [ ] Two instances of a Debug build keep separate credential files, as they do today.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass. These changes are not behind the Servers switch.
- [ ] Reviewed by someone other than the author.

## Out of scope

- The Servers button and anything behind `RTS_SERVERS` (D1).
- Removing the linked libraries from the build.
- A live login against the lobby service. That is the Phase 1 check.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
