# Lobby service

Version 1. This is the specification of the project's own lobby service: what it answers, what it stores and which choices it makes. The game client's side of the contract was read from the client code and is recorded in two reference files:

- [reference/CLIENT_HTTP.md](reference/CLIENT_HTTP.md): every HTTP call the client makes, with field names, types and failure behaviour.
- [reference/CLIENT_REALTIME.md](reference/CLIENT_REALTIME.md): the WebSocket messages and the player-to-player connection setup.

This document does not repeat those shapes. It says which parts are implemented, and fixes every behaviour the client code cannot tell us. Where a reference file and this document differ, this document wins and the difference is reported in the package's handoff notes.

Changes go through the `servers-go` branch.

## 1. Program

| Item | Choice |
|---|---|
| Language | Go, standard library `net/http` |
| Folder | `Servers/Lobby/` |
| Binary | `lobby` |
| Storage | SQLite through `modernc.org/sqlite` (pure Go, BSD licence). Accounts and tokens only. |
| WebSocket | `github.com/coder/websocket` (ISC licence) |
| Pass signatures | Go standard library `crypto/ed25519` |
| Configuration | One text file of `key = value` lines, path given with `--config`. Secrets may be given by environment variable instead. |

Lobbies, rooms and connections live in memory. A restart ends all lobbies and logs every player out of the WebSocket; accounts survive.

The service listens for plain HTTP on one port. In deployment a free reverse proxy in front of it provides TLS (Phase 4).

## 2. Addresses

| What | Path |
|---|---|
| HTTP calls | `/env/{env}/contract/1/{Endpoint}`. `{env}` is accepted as any of `prod`, `test`, `dev` and has no effect. |
| WebSocket | `/ws` |
| Health check | `GET /healthz` answers `200` with `ok` |

The `ws_uri` returned at login is the configuration value `public_ws_url`.

## 3. Common rules

- Every request body and every WebSocket message is hostile input. Check type, length and range of every field before use. Unknown keys are ignored.
- Request bodies are limited to 64 KB. A WebSocket message is limited to 256 KB.
- A call that needs a login carries `Authorization: Bearer <session token>`. A missing, unknown or expired token gets `401` with an empty JSON object.
- Every JSON response is an object and contains every key the client reads for that call, because a missing key can crash the client (CLIENT_HTTP.md, sections 4 and 5).
- User ids are 64-bit integers starting at 1000. Lobby ids and match ids are 64-bit integers starting at 1.
- Text from players (names, lobby names, chat) is valid UTF-8 after trimming, with control characters removed.
- Tokens, passes and passwords are never logged. Addresses are logged with the last part hidden.
- Clocks: all times are the service's own clock.

## 4. Accounts and login

Decisions D44 and D45: no password, no web page.

| Step | Behaviour |
|---|---|
| `POST VersionCheck` | Always `{"result":0,"patcher_name":"","patcher_path":"","patcher_size":0}`. The service never answers that an update is needed. |
| `GET LoginCode` | Creates a random login code valid for 5 minutes and returns `{"success":true,"login_code":"<code>"}`. At most 20 unused codes per address; beyond that `{"success":false,"login_code":""}`. |
| `POST CheckLogin` | For a known, unused code: creates a new account, marks the code used, and returns the login answer below. Unknown or used code: `{"result":2}`-style failure with every key present (CLIENT_HTTP.md 4.3). |
| `POST LoginWithToken` | `Authorization: Bearer <refresh token>`. Known token: the login answer below, with `refresh_token` set to the token from the header, because the client saves whatever comes back. Unknown token: `401`, which sends the client back to the code flow. |
| `POST RefreshToken` | `Authorization: Bearer <refresh token>`. Returns `{"result":1,"session_token":"<new>"}`. The old session token stays valid until it expires. |
| `DELETE User/{id}` | Only for the caller's own id. Deletes the refresh token, so the next start creates a new account. The account row is kept. |

Login answer: `result` 1, `session_token`, `refresh_token`, `user_id`, `display_name`, `ws_uri`, plus every other key the client reads from that structure with its neutral value.

| Item | Rule |
|---|---|
| Session token | 32 random bytes as hexadecimal. Valid 15 minutes. Kept in memory. |
| Refresh token | 32 random bytes as hexadecimal. No expiry. Only its SHA-256 is stored. |
| New account name | `Player` followed by the user id |
| Name change | WebSocket message 9. 3 to 20 characters from letters, digits, space, `_` and `-`; not starting or ending with a space; unique without regard to case. A name of the form `Player` followed only by digits is refused, unless it is the account's own default name. A refused name is answered with a room chat line from the service saying so. |
| Account creation limit | 5 new accounts per address per hour (configuration value; 0 means no limit). Loopback addresses are never limited, so several instances on one PC can be tested. |
| Hardware fields | `machine_guid`, `mac_addr`, `vol_serial` and `client_id` are ignored if present and never stored (D46). |

One live WebSocket per account. A second login closes the first.

## 5. HTTP calls

### 5.1 Implemented

| Call | Our rules beyond the reference |
|---|---|
| `GET ServiceConfig` | Returns the values in section 9. |
| `GET MOTD` | `{"MOTD":"<configuration value>"}` |
| `GET Rooms` | The rooms from configuration. At launch one room: id 1, name `Lobby`. |
| `GET Lobbies` | Every lobby that has not started and is public. `latencies` has one `0` per lobby. `playerlatencies` is an empty array. |
| `PUT Lobbies` | Creates a lobby (section 7). A player already in a lobby is removed from it first. |
| `GET Lobby/{id}` | Members only. Others get `404`. |
| `PUT Lobby/{id}` | Joins (section 7). |
| `POST Lobby/{id}` | Changes one field (section 7.3). Answers `200` with `{}`; a refused change simply has no effect. The one exception is field 19, which answers with a `result` (section 10). |
| `DELETE Lobby/{id}` | Leaves (section 7.4). Always `200`. |
| `POST Lobby/Outcome` | `{"screenshot_url":"","replay_url":""}`. Nothing is stored. |
| `POST ConnectionOutcome` | `200` with `{}`. Nothing is stored. |

### 5.2 Answered with a fixed empty response

Decision D49. The exact bodies are in CLIENT_HTTP.md section 7.1.

`GET PlayerStats/{id}` (`404`), `PUT PlayerStats`, `POST PlayerStats/Batch`, `GET GlobalStats`, `GET Social/Friends`, `GET Social/Blocked`, every other `Social/...` call, `GET Matchmaking/Playlists`, `PUT`/`DELETE Matchmaking`, `POST Matchmaking/Widen`, `POST ProvideMWToken`.

Any other path under the contract prefix gets `404` with `{}`.

## 6. WebSocket

### 6.1 Connection

- The upgrade request carries `Authorization: Bearer <session token>` and `is-reconnect: true|false`. An invalid token gets `401`.
- The client sends JSON inside **binary** frames. The service accepts JSON in binary or text frames.
- The service sends JSON only in **text** frames. The client ignores anything else.
- Every message is an object with an integer `msg_id`.
- The client reads at most one message per game tick. The service therefore never sends two identical notices in a row to one client: a pending "refetch the lobby" (6) or "refetch the list" (7) is sent once, not once per change.
- The service answers each `PING` (14) with `PONG` (15).
- The service never sends a close frame to a client it wants to keep. A close means "log out" to the client.
- A connection with no message from the client for 30 seconds is dropped.

### 6.2 Reconnect

When a connection drops, the service holds the player's place (room and lobby membership) for 45 seconds.

| Upgrade request | Held place exists | Answer |
|---|---|---|
| `is-reconnect: true` | yes | Accept. The player keeps room and lobby. |
| `is-reconnect: true` | no | HTTP `205`. The client then connects as a new session. |
| `is-reconnect: false` | either | Accept as a new session. Any held place is released first. |

While a place is held, the service keeps what the player would have missed: one pending 6, one pending 7, and a 13 with that player's own fields. They are sent, in that order, as soon as the player reconnects.

When the hold runs out, the player is removed from its lobby as if it had left (section 7.4).

### 6.3 Messages

Numbers and fields are in CLIENT_REALTIME.md, section 1.7.

| Messages | Behaviour |
|---|---|
| 14, 15 | Ping and pong. |
| 3, 4 | Room change and member list. `room` -1 leaves the current room. The answer (4) repeats the `request_id` and is also sent to everyone already in the room. When a player leaves a room or disconnects, the remaining players of that room get a new member list (4). |
| 1, 2 | Room chat, to everyone in the sender's room. Text beyond 300 characters is cut off. `admin` and `name_change` are `false`. |
| 9 | Name change (section 4). On success everyone in the room gets a new member list (4), and lobby members get 6. |
| 5 | Ready state of a lobby member. Followed by 6 to the lobby. |
| 10, 11 | Lobby chat. Message 11 always carries all of `message`, `action`, `announcement`, `show_announcement_to_host` and `user_id` (the sender). Text beyond 300 characters is cut off. `announcement` is kept only when the sender is the host; from anyone else it is sent as `false`. An announcement is sent to every member, but to the host only when `show_announcement_to_host` is true. Ordinary chat goes to every member. |
| 6, 7 | Sent by the service after any lobby change (6, to members) and after any change to the list of open lobbies (7, to everyone in a room and not in a lobby). |
| 24, 25 | Host removes or changes the lobby password. |
| 23 | Countdown started. Accepted only from the host. The lobby is locked: joins are refused, and members cannot change their own fields or ready state. The client never reports a cancelled countdown, so the lock ends by itself after 25 seconds, and also when the match starts or the host sends a new 26. |
| 26, 27, 28 | Connectivity check (section 8.3 for Online lobbies, section 10 for server lobbies). |
| 17, 12, 19, 18 | Player-to-player connection setup (section 8). Never sent for server lobbies. |
| 13 | Match start (section 7.5). |

Messages never sent by the service (D46, D47, D49): 16, 20, 21, 22, 29, 31, 32, 35 to 38, 40, 41, 42, 44 to 46, 48, and 8.

Messages from the client that the service ignores: 8, 30, 33, 34, 39, 43, and any unknown number.

A client that sends more than 60 messages in one second, apart from 12 and 14, is disconnected.

## 7. Lobbies

### 7.1 Data

A lobby holds the fields of the list and detail shapes (CLIENT_HTTP.md 5.1 and 5.2), a password, and always exactly 8 member entries, one per slot.

| Field | Rule |
|---|---|
| `Name` | 1 to 40 characters |
| `MaxPlayers` | 2 to 8. Slots at or above it are `CLOSED`. |
| `Owner` | The host's user id |
| `ExeCRC`, `IniCRC` | As sent by the creator |
| `RNGSeed` | Random 31-bit number, chosen at creation and again each time a match ends |
| `MatchID` | 0 until a match starts, then the next match id |
| `LobbyType` | 0 for an Online lobby, 2 for a server lobby |
| `Region` | Empty for an Online lobby. The region tag for a server lobby (section 10). |
| `SlotIndex` | Always the slot's own number, 0 to 7, in every member entry |
| Empty slot entry | `UserID` -1, `DisplayName` empty, `SlotState` 0 (open) or 1 (closed), `Side`, `Color`, `Team` and `StartingPosition` -1, `Port` 0, booleans false, strings empty |
| `IsReady` of the host | Always true. The host's client sends its ready state only once, after creating the lobby, and cannot start without it. |

### 7.2 Create and join

| Call | Checks, in order | Failure |
|---|---|---|
| `PUT Lobbies` | Fields valid. For a server lobby: pass valid (section 10). | `result` 0 with all four keys |
| `PUT Lobby/{id}` | Lobby exists and has not started: else `404`. Not locked by a countdown: else `406`. Password matches: else `401`. Server lobby: pass valid, else `403`. A free open slot exists: else `406`. | Status code as listed |

The creator takes slot 0. A joiner takes the lowest open slot. `anticheat_id` is ignored. The service never answers `417`.

After a successful join the service sends 6 to all members and 7 to the list watchers, and for an Online lobby starts connection setup (section 8.2).

### 7.3 Changes

`POST Lobby/{id}` with `field` (CLIENT_HTTP.md 5.4):

| Fields | Who | Rule |
|---|---|---|
| 1, 2, 3, 4, 8 | Any member, for itself | Values range-checked. A start position or colour already taken by another member is refused. Refused while the lobby is locked by a countdown. |
| 0, 5, 6, 11, 12, 17, 18 | Host only | Range-checked. Kicking (11) removes that member as in 7.4 and the kicked client sees itself missing from the lobby. |
| 0 | Host only | `max_players` below the number of slots that hold a human or an AI is refused. |
| 12 | Host only | Only for a slot that holds no human. To remove a human the host kicks (11). |
| 13, 14, 15, 16 | Host only | For AI slots only |
| 7 | Anyone | Ignored. |
| 19 | Host only | Server session (section 10). Server lobbies only. |

Ready state:

- The host is always ready (7.1).
- A change by the host to field 0, 5, 6, 12, 13, 14, 15 or 16 clears the ready state of every member except the host.
- A change by a member to its own field 1, 2, 3 or 4 clears only that member's ready state.
- Field 8 with `has_map` false clears that member's ready state.

Every accepted change is followed by 6 to all members.

### 7.4 Leave

A member leaves by `DELETE Lobby/{id}`, by being kicked, by logging out, or when its held place runs out.

- Its slot becomes open.
- If the lobby has no human members left, it is deleted.
- If the host left a lobby that has not started, the human member who joined earliest becomes `Owner` and is set ready.
- Remaining members get 6. For an Online lobby they also get 18 for the player who left.
- List watchers get 7.

### 7.5 Match start

The host sends 13 when its countdown ends.

1. Ignored unless the sender is the host and every human is ready. A server lobby also needs at least 2 humans; an Online lobby may be one human with computer players.
2. `MatchID` is set. The lobby is marked started and leaves the list.
3. Every human member, including the host, gets 13 with `screenshot_url` set to an empty string.

For a server lobby step 3 adds the fields in section 10.

When every member of a started lobby has left, the lobby is deleted. There is no "return to lobby" in the service: the client leaves and comes back to the list by itself.

## 8. Online lobbies: player-to-player connections

### 8.1 TURN credentials

The answers to create and join carry `turn_username` and `turn_token` in the format `coturn` accepts with its `use-auth-secret` option:

- `turn_username` = `<expiry as Unix seconds>:<user id>`, with the expiry 6 hours ahead
- `turn_token` = Base64 of HMAC-SHA1 over `turn_username`, keyed with the configuration value `turn_secret`

If `turn_secret` is empty, both are empty strings and only direct connections work.

### 8.2 Connection setup

When a player joins an Online lobby with other humans in it, after sending 6:

- For each other human member X: send 17 to the joiner with `user_id` X, and 17 to X with `user_id` the joiner. `lobby_id` is the lobby, `preferred_port` 0, `middleware_id` an empty string.
- 12 from a member is forwarded to `target_user_id` only if both are members of the same lobby. The forwarded message has `target_user_id` set to the sender's id and `payload` unchanged.
- 19 from a member makes the service send 17 again to both players of that pair.

### 8.3 Connectivity check

On 26 from the host:

1. Send 27 to every human member with a new `mesh_check_id` and `attempt` 1.
2. Wait up to 5 seconds for each member's reply (27 with `connectivity_map` and `connecting_map`).
3. The check passes when every human lists every other human in `connectivity_map`.
4. If it does not pass and any member still lists a peer in `connecting_map`, repeat from step 1 after 2 seconds with the next `attempt`, at most 3 attempts in total.
5. Send 28 to the host: `mesh_complete`, and when false `reason` (`timeout`, `missing_connections` or `member_left`) and the list of missing pairs.

A new 26 cancels a check in progress. A lobby of one human passes at once.

## 9. Service configuration sent to clients

`GET ServiceConfig` returns every field of the client's structure (CLIENT_HTTP.md section 6) with the client's own default, except:

| Field | Value | Reason |
|---|---|---|
| `relay_all_traffic` | `false` | Direct connections first, TURN as fallback |
| `retry_signalling` | `true` | Lets a failed pair try again |
| `use_new_networking` | `false` | No anti-cheat transport (D47) |
| `do_probes`, `do_replay_upload` | `false` | Nothing is uploaded (D46) |

## 10. Server lobbies

Protocol rules are in [PROTOCOL.md](PROTOCOL.md) section 5.

| Point | Behaviour |
|---|---|
| Create | The body has `server_mode: true` and the request has the header `X-Servers-Pass`. The pass is checked. `LobbyType` becomes 2 and `Region` the tag of the pass's region. Without `server_mode` the header is ignored. |
| Join | The header is required and checked against the lobby's region and build fingerprints. Refusal is `403`. |
| Pass check | PROTOCOL.md 5.2. The region's public key and tag come from configuration. |
| TURN | `turn_username` and `turn_token` are empty strings. |
| Connection setup | None. 17, 12, 19 and 18 are never sent; 12 and 19 from members are ignored. |
| Connectivity check | 26 from the host is answered at once with 28, `mesh_complete` true. No 27 is sent. |
| Map | Members without the map (`has_map` false) cannot become ready. |

### Server session

Before the start message the host posts:

```
POST Lobby/{id}
{"field": 19, "server_session": "<16 hex digits>", "tokens": [{"slot": 1, "token": "<16 hex digits>"}, ...]}
```

- Host only, server lobby only, not yet started.
- `server_session` and each `token` are exactly 16 lowercase hexadecimal digits. A `token` of sixteen zeros is refused.
- Each `slot` is 0 to 7, is not the host's slot, and appears once. A human slot is one with `SlotState` 5 and a user id.
- `tokens` has exactly one entry for every human slot except the host's.
- The answer is `{"result":1}` when stored and `{"result":0}` when refused. The host sends the start message only after `result` 1.
- The service keeps it until the match starts or any member joins or leaves, whichever comes first. It is never returned by any `GET`.

Match start (7.5) in a server lobby:

- 13 from the host is ignored unless a server session is stored.
- Each member's 13 carries `screenshot_url` (empty), `server_session`, `server_slot` (that member's slot, integer) and `server_token` (that member's token; an empty string for the host).

## 11. Configuration file

| Key | Meaning | Example |
|---|---|---|
| `listen` | Address and port | `127.0.0.1:9000` |
| `database` | SQLite file | `lobby.db` |
| `public_ws_url` | Returned as `ws_uri` | `ws://127.0.0.1:9000/ws` |
| `motd` | Text | `Welcome` |
| `rooms` | Comma-separated room names; ids start at 1 | `Lobby` |
| `turn_secret` | Shared secret with `coturn`. May come from the environment variable `LOBBY_TURN_SECRET`. | (empty) |
| `accounts_per_address_per_hour` | Section 4 | `5` |
| `region.<number>.tag` | Region tag, as in the client region file | `region.1.tag = eu` |
| `region.<number>.public_key` | File with the region's public key, as written by `relay --generate-key` (`<key file>.pub`) | `region.1.public_key = keys/region.key.pub` |
| `trust_forwarded_for` | `on` behind the reverse proxy. The player's address is then the last entry of `X-Forwarded-For`, the one our own proxy adds. Earlier entries come from the client and are ignored. | `off` |

A missing or invalid required value stops the service at start with a message naming it. An example file with no secrets is committed as `Servers/Lobby/lobby.conf`.

## 12. What the service never does

- Ask for hardware identifiers, loaded modules or screenshots.
- Tell a client to download or run anything.
- Contact any other host. It only answers its clients.
- Return another player's token, pass or address.

## 13. Checking it without the game

`Servers/Lobby/tools/lobbyprobe` is a small Go program that speaks the client's side of this contract. One run logs in two accounts, joins the room, creates a lobby, joins it, readies up and starts, for an Online lobby and for a server lobby, and reports each step. It is the service's part of the Phase 1 check.

## 14. To verify with the real client

Behaviours inferred from client code, to confirm in the Phase 1 check.

| # | Assumption |
|---|---|
| W1 | The client completes login against the answers in section 4 without opening a web page (the client change is package B1). |
| W2 | Both players of a pair need message 17, and the lobby detail must already list the peer when it arrives. |
| W3 | The host needs message 13 sent back to it to start the match. |
| W4 | The forwarded `target_user_id` in message 12 is not used by the receiver. |
| W5 | The client accepts `ws://` and `http://` addresses in a debug build. |
| W6 | `coturn` accepts the credentials of section 8.1 as passed through by the client. |
| W7 | The client keeps working when every stubbed call returns the fixed empty response. |
