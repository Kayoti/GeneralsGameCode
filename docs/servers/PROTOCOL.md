# Protocol

Version 1. This is the contract between the game client and the relay server, and the rules for how a Servers match uses the lobby service. The lobby service's own contract is in [SERVICE.md](SERVICE.md).

Changes go through the `servers-go` branch and bump the version in section 2.

## 1. Parts

```
game client ── HTTP and WebSocket ───────────────> lobby service
game client ── relay link (ENet over UDP, binary) ──> relay server
```

| Part | Role in Servers mode |
|---|---|
| Lobby service | Login, lobby list, chat, staging room. Stores which lobbies are server lobbies and their region, checks passes, and hands each player its join token at match start. |
| Relay server | Checks each player (VPN, ping), issues a pass, and carries every match. |

The relay and the lobby service never talk to each other. The lobby service only needs each region's public key to check passes.

IPv4 only for the relay link.

## 2. Versions

| Field | Where | Rule |
|---|---|---|
| `proto` | First message of every relay connection | Integer, currently `1`. The relay refuses any other value with `PROTO_MISMATCH`. |
| `exe_crc`, `ini_crc` | Pass | The two build fingerprints the engine already computes. All players of a match must have the same pair. |

## 3. Common rules

- Integers are little-endian. `u8`, `u16`, `u32`, `u64` are unsigned.
- A receiver drops any message that is too short, too long, of unknown type, or not valid in the current state. It does not try to repair it.
- Slot numbers are 0 to 7. The relay never trusts a slot number inside match data. It uses the slot bound to the connection.
- Every message starts with a `u8` type.
- "Entry check" in this document means the VPN and ping check that produces a pass.

## 4. Relay link

### 4.1 Connection

ENet with 2 channels.

| Channel | Use | Delivery |
|---|---|---|
| 0 | Every message in this document | Reliable, ordered |
| 1 | Reserved | Unreliable |

All messages share one ordered channel on purpose. A client always receives `START`, `ROSTER`, `DROP` and match records in exactly the order the relay sent them.

ENet provides acknowledgement, resend, ordering, fragmentation and keep-alive. In server mode the client does not use the engine's own acknowledgement, resend, wrapper or frame-resend commands.

A connection has one of three purposes, set by its first message, which must arrive within 5 seconds:

| First message | Purpose |
|---|---|
| `CHECK_HELLO` | Run the entry check and receive a pass |
| `CREATE` | Host opens a match and takes its slot |
| `JOIN` | Another player enters that match |

### 4.2 Messages

| Type | Name | Direction | Body |
|---|---|---|---|
| 1 | `CHECK_HELLO` | client to relay | `u16 proto`, `u32 exe_crc`, `u32 ini_crc` |
| 2 | `PING` | relay to client | `u64 nonce` |
| 3 | `PONG` | client to relay | `u64 nonce`, copied from `PING` |
| 4 | `PASS` | relay to client | 100-byte pass (4.3). The relay disconnects after sending. |
| 5 | `REFUSE` | relay to client | `u8 code` (4.9), `u16 rtt_ms`, `u16 limit_ms`. Both values are 0 unless the code is `PING_TOO_HIGH`. The relay disconnects after sending. |
| 6 | `CREATE` | client to relay | `u16 proto`, 100-byte pass, `u8 host_slot`, `u8 slot_mask` |
| 7 | `CREATED` | relay to client | `u64 session_id`, 8 × `u64 join_token` (index = slot; 0 for slots not in `slot_mask` and for the host slot) |
| 8 | `JOIN` | client to relay | `u16 proto`, 100-byte pass, `u64 session_id`, `u8 slot`, `u64 join_token` |
| 9 | `WELCOME` | relay to client | `u64 session_id`, `u8 slot`, `u8 connected_mask`, `u8 leader_slot` |
| 10 | `ROSTER` | relay to client | `u8 connected_mask`, `u8 leader_slot`. Sent on every change. |
| 11 | `START` | relay to client | `u8 connected_mask`, `u8 leader_slot`. Sent once. |
| 12 | `DROP` | relay to client | `u8 slot`, `u8 reason` (4.9), `u32 last_frame` |
| 13 | `LEAVE` | client to relay | none |
| 16 | `DATA` | both | Match records (4.6) |
| 20 | `TIMING` | relay to client | Reserved for Phase 3 |
| 21 | `FRAME_CLOSE` | relay to client | Reserved for Phase 3 |

### 4.3 Pass

A pass states that the relay checked this player. The relay signs it with Ed25519 using the region key.

| Field | Type | Meaning |
|---|---|---|
| `version` | u8 | 1 |
| `reserved` | u8 | 0 |
| `region` | u16 | Region number from the region configuration |
| `client_ip` | u32 | Address the relay saw |
| `exe_crc` | u32 | As given in `CHECK_HELLO` |
| `ini_crc` | u32 | As given in `CHECK_HELLO` |
| `rtt_ms` | u16 | Round-trip time the relay measured |
| `reserved2` | u16 | 0 |
| `issued_at` | u32 | Relay clock, Unix seconds |
| `expires_at` | u32 | Relay clock, Unix seconds |
| `pass_id` | u64 | Random |
| `signature` | 64 bytes | Ed25519 over all fields above |

Total 100 bytes. Lifetime 10 minutes.

A pass is accepted when: the signature is valid, `version` is known, `region` is this relay's region, it has not expired by the relay's clock, the UDP source address equals `client_ip`, and no other live match connection is using the same `pass_id`.

The address comparison is always on. It can be switched off only by a relay start option meant for testing on one PC.

The client judges how long a pass has left from its own clock: the moment it received the pass plus `expires_at - issued_at`. It never compares the relay's times with its own.

### 4.4 Entry check

How a player gets a pass.

1. Client connects and sends `CHECK_HELLO`.
2. The relay checks the source address against its VPN data. A flagged address gets `REFUSE` `VPN_DETECTED`.
3. The relay sends 8 `PING` messages, 250 ms apart, each with a fresh random nonce. The client answers each with `PONG` at once. The relay times each answer itself and takes the median. A `PONG` with an unknown nonce is ignored. Fewer than 5 answers within 4 seconds gives `REFUSE` `TIMEOUT`.
4. If the median is above the region's limit, the relay sends `REFUSE` `PING_TOO_HIGH` with both values.
5. Otherwise the relay sends `PASS`.

The relay does the timing with its own nonces, so a client cannot make its ping look lower than it is.

The client runs the entry check:

| When | If refused |
|---|---|
| The player clicks Servers | Message. The player stays on the Multiplayer menu. |
| The player attempts to join a lobby | Message. The client logs out of the lobby service and returns to the main menu, with Servers mode cleared. |
| The player presses Accept in a lobby | Same. The player leaves the lobby first. |
| The player exits a lobby | Same |

The first four are the player actions the project owner listed. In addition, the client runs it silently before `CREATE` or `JOIN` if its pass has less than 60 seconds left. If that silent check is refused or gets no answer, the match start fails for that player as in 5.4 (step 4 for the host, step 7 for a member), with the entry check's message.

Messages shown:

- `VPN_DETECTED`: "VPN detected. Disable your VPN to play on Servers."
- `PING_TOO_HIGH`: "Your connection to the server is too slow (`rtt_ms` ms, limit `limit_ms` ms)."
- Anything else, or no answer: "The server cannot be reached."

### 4.5 Match session

**Creating**

1. The host presses Start. Its client connects and sends `CREATE`. `slot_mask` has one bit per human player and must include `host_slot` and at least one other slot.
2. The relay replies `CREATED` with a random `session_id` and one random `join_token` per other expected slot, then `WELCOME`. The host now holds its slot.
3. The host gives the lobby service the `session_id` and every `join_token` (5.4). The service gives each player only its own token, inside the start message. A token is never sent to the whole lobby.
4. The start message begins the match on every client.

**Joining**

5. Each other player connects and sends `JOIN`. The relay accepts it when the pass is valid, the session exists and has not started, the slot is expected and free, `join_token` equals the token issued for that slot, and the pass `exe_crc` and `ini_crc` equal the host's. A token works once.
6. Everyone gets `ROSTER` as players arrive.

**Starting**

7. When every expected slot is connected, the relay sends `START`.
8. If 30 seconds pass after `CREATED` first, the relay sends `DROP` with reason `NEVER_JOINED` for each missing slot, then `START`. If fewer than 2 players are connected, everyone gets `REFUSE` `NOT_ENOUGH_PLAYERS` instead.
9. After `START`, clients send records. Records sent by a client before it received `START` are a protocol violation.

**Ending**

10. A player leaves by sending its normal leave command as a record, continuing to send frame records through the leave frame, then `LEAVE`.
11. The session ends when no player is connected. The relay keeps nothing afterwards.

### 4.6 Match records

Message type 16 `DATA`, in both directions:

```
u8  type = 16
repeated until the end of the message:
    u16 length
    length bytes: one engine network command in full form
```

"Full form" is what `NetCommandMsg::copyBytesForNetPacket` writes, with no repeat compression. The receiver turns it back into a command with `NetPacket::ConstructNetCommandMsgFromRawData`.

A record starts with tagged header fields and then the data marker:

| Tag | Field | Size after tag |
|---|---|---|
| `T` | Command type | 1 |
| `R` | Relay mask | 1 |
| `F` | Execution frame | 4 |
| `P` | Player (sender slot) | 1 |
| `C` | Command id | 2 |
| `D` | Start of data. Everything after it is the command's data. | 0 |

Which tags a command has, and in what order, depends on its type:

| Command | Header | Data the relay reads |
|---|---|---|
| `GAMECOMMAND` | T F R P C D | none |
| `FRAMEINFO` | T F R P C D | `u16 command_count` |
| `PLAYERLEAVE` | T R F P C D | `u8 leaving_slot` |
| `RUNAHEAD` | T R F P C D | `u16 run_ahead`, `u8 frame_rate` |
| `RUNAHEADMETRICS` | T R P C D | none |
| `CHAT` | T F R P C D | none |
| `LOADCOMPLETE`, `TIMEOUTSTART` | T R P C D | none |
| `PROGRESS` | T R P D | none |

**Relay checks for each record from a client**

1. `length` is 7 to 4096. A whole `DATA` message is at most 16384 bytes.
2. The header contains only the tags listed for its type, each exactly once, ending with `D`. The scan never reads past the record. Any other byte where a tag is expected, including the repeat tag `Z`, fails.
3. `T` is one of the types in the table above.
4. `P` equals the slot of the connection.
5. The type-specific checks below pass.

A record that fails is dropped and counted. 32 failed records from one player drop that player with reason `PROTOCOL`.

**Per type**

| Command | Extra checks | Forwarded to |
|---|---|---|
| `GAMECOMMAND` | Frame window | All other connected players |
| `FRAMEINFO` | `F` equals the sender's `next_frame`. Then `next_frame` increases by 1. | All other connected players |
| `PLAYERLEAVE` | Frame window. `leaving_slot` equals the sender's slot. | All other connected players |
| `RUNAHEAD` | Sender is `leader_slot`. Frame window. `run_ahead` is 4 to 64. `frame_rate` is 5 to 60. | Connected players in the record's relay mask, except the sender |
| `RUNAHEADMETRICS` | none | `leader_slot` only |
| `CHAT` | none | Connected players in the record's relay mask, except the sender |
| `PROGRESS`, `LOADCOMPLETE` | none | All other connected players |
| `TIMEOUTSTART` | Sender is the host slot | All other connected players |

For the first three rows the relay ignores the record's own relay mask, so a client cannot show different commands to different players.

The relay forwards a record's bytes unchanged.

**Frame tracking.** For each slot the relay keeps `next_frame`, the frame of the next `FRAMEINFO` it expects. It starts at 30, because the engine starts with a run-ahead of 30 and treats frames 1 to 29 as empty. "Frame window" means `next_frame <= F <= next_frame + 128`.

**Command count.** The relay counts each slot's forwarded `GAMECOMMAND`, `PLAYERLEAVE` and `RUNAHEAD` records per frame and compares the total with `command_count` when that slot's `FRAMEINFO` for the frame arrives. Until V7 in section 10 is settled, a mismatch is logged and not acted on. After that it is a protocol violation.

**Not carried.** `ACKBOTH`, `ACKSTAGE1`, `ACKSTAGE2`, `KEEPALIVE`, `WRAPPER`, `FRAMERESENDREQUEST`, `MANGLERQUERY`, `MANGLERRESPONSE`, `PACKETROUTERQUERY`, `PACKETROUTERACK`, `DESTROYPLAYER`, every `DISCONNECT*` command, and `FILE`, `FILEANNOUNCE`, `FILEPROGRESS`. A client in server mode never sends these and discards them if received.

**Client checks on received records.** The client applies the same per-type checks to what it receives: `RUNAHEAD` only from `leader_slot` and inside the stated ranges, `PLAYERLEAVE` only where `leaving_slot` equals the record's player, `TIMEOUTSTART` only from the host slot. A record that fails is discarded.

**Size.** A client never creates a command longer than 4096 bytes in server mode. If one would be, it is not sent and not counted towards `command_count`.

**Leader slot.** In Phases 1 and 2 one player still calculates the run-ahead, as the packet router does today. The relay names that player in `leader_slot`: the lowest connected slot. When the leader leaves, the relay names the next one in `ROSTER`. In Phase 3 the relay takes over timing and the leader is no longer used.

### 4.7 Removing a player

**When the relay drops a player**

| Cause | Reason |
|---|---|
| ENet reports the connection lost | `TIMEOUT` |
| `LEAVE`, or connection lost, before the player's leave frame was reached | `LEFT` |
| 32 failed records, a record before `START`, or a rate limit exceeded | `PROTOCOL` |
| Did not send `LOADCOMPLETE` within 120 seconds of `START` | `TIMEOUT` |
| Stall: at least one `FRAMEINFO` has been forwarded in the session, this slot's `next_frame` is lower than every other connected slot's, and it has not increased for 20 seconds | `TIMEOUT` |
| Expected slot never connected | `NEVER_JOINED` |

The stall and loading rules exist because the engine's own recovery (the vote screen and resend requests) is not used in server mode.

**What the relay does**

1. Stops forwarding that player's records and closes the connection.
2. Sets `last_frame = next_frame - 1` for that slot. This is 29 if the player never sent a `FRAMEINFO`.
3. Sends `DROP`, then `ROSTER`, to everyone else.

**What each client does on `DROP`**

1. Discards every record it holds from that slot with a frame above `last_frame`.
2. Treats the slot as present through `last_frame` and absent from `last_frame + 1`.
3. At frame `last_frame + 1`, applies locally what the engine's destroy-player command does for that slot. Every client does this on the same frame, so the simulation stays in step.
4. If the match is still loading, counts that slot as having finished loading.
5. Ignores a `DROP` for a slot it has already removed.

Because every message is on one ordered channel, each client has received all of that slot's forwarded records before it receives the `DROP`.

**A clean leave.** When a player's `PLAYERLEAVE` and all its `FRAMEINFO` through the leave frame have been forwarded and the player then sends `LEAVE` or disconnects, the relay sends `ROSTER` only, with no `DROP`. If the connection ends earlier than that, the relay sends `DROP` as above, and the `DROP` takes priority over the pending leave.

No vote and no negotiation between players are needed in any of these cases.

### 4.8 Reserved for Phase 3

`TIMING` and `FRAME_CLOSE` are reserved so the server clock can be added without changing existing messages. Their bodies are defined in package S1. The intent:

- `TIMING`: the relay sets run-ahead and logic frame rate for a client.
- `FRAME_CLOSE`: the relay states, for a frame, how many commands each slot has, in place of each player's own `FRAMEINFO`.
- The relay may then rewrite the `F` field of a late `GAMECOMMAND` to a later frame.

A version 1 client that receives a reserved type drops it.

Until Phase 3, a slow player still slows the others, as in the engine today. What version 1 removes is the dependence on a player packet router, the player-to-player connections, and the vote.

### 4.9 Codes

`REFUSE` codes: 1 `PROTO_MISMATCH`, 2 `VPN_DETECTED`, 3 `PING_TOO_HIGH`, 4 `TIMEOUT`, 5 `BAD_PASS`, 6 `EXPIRED`, 7 `WRONG_REGION`, 8 `ADDRESS_MISMATCH`, 9 `PASS_IN_USE`, 10 `BAD_REQUEST`, 11 `NO_SESSION`, 12 `ALREADY_STARTED`, 13 `SLOT_TAKEN`, 14 `NOT_EXPECTED`, 15 `BAD_TOKEN`, 16 `BUILD_MISMATCH`, 17 `NOT_ENOUGH_PLAYERS`, 18 `SERVER_BUSY`, 19 `RATE_LIMITED`.

`DROP` reasons: 1 `LEFT`, 2 `TIMEOUT`, 3 `PROTOCOL`, 4 `NEVER_JOINED`, 5 `TOO_SLOW` (Phase 3).

## 5. Use of the lobby service

Servers mode opens the same login, lobby list and staging room screens as Online. These rules are the only differences. Field names and message numbers are defined in [SERVICE.md](SERVICE.md).

### 5.1 Region

The client ships a region file (section 7). On entering Servers mode the client runs the entry check against every region in it and keeps the pass with the lowest `rtt_ms`. A tie goes to the region listed first. That region is fixed until the player leaves Servers mode.

A region that refuses or does not answer within 6 seconds is skipped. If no region gives a pass, the player returns to the Multiplayer menu with the message for the refusal of the region listed first.

### 5.2 Server lobbies

A server lobby is a lobby with `LobbyType` 2. Its `Region` is the `tag` of a region in the region file, for example `eu`. Both are set when the lobby is created and never change.

| Action | Rule |
|---|---|
| Create | The client sends `server_mode: true` and its pass. The service checks the pass, sets `LobbyType` 2, and sets `Region` to the tag of the pass's region. |
| Join | The client sends its pass. The service refuses the join unless the pass is valid for the lobby's region. |
| Any other lobby | No pass is sent or checked. `LobbyType` stays 0. |

The pass travels as the HTTP header `X-Servers-Pass`: the 100 pass bytes (4.3) as 200 lowercase hexadecimal digits.

The service accepts a pass when the signature is valid for the region's public key, `version` is 1, it has not expired by the service's clock, and its `exe_crc` and `ini_crc` equal the lobby's. The service does not compare addresses. The relay does that when the match starts.

### 5.3 List filter

| Mode | Lobbies shown |
|---|---|
| Servers | Only lobbies with `LobbyType` 2 and the `Region` of the player's region |
| Online | Only lobbies whose `LobbyType` is not 2 |

The filter is applied in the client where the lobby list is drawn. Enforcement is in the service (5.2) and in the relay, which refuses anyone without a valid pass for its region.

### 5.4 Match start

In a server lobby there are no player-to-player connections. The service sends no connection instructions and no TURN credentials, and answers the host's connectivity check as complete at once.

1. Start is unavailable with fewer than 2 human players.
2. The host presses Start. The client skips its own "players are still connecting" test, which counts player-to-player connections and would never pass. The connectivity check and the countdown then run as in Online.
3. When the countdown ends, the host's client sends `CREATE` to the relay instead of the start message.
4. If `CREATE` is refused or times out, the host sees a message, the lobby stays open and Start is enabled again.
5. On `CREATED`, the host posts the session to the service: the `session_id` and one `join_token` per other slot (SERVICE.md, "Server session"), and waits for the answer. A refusal or no answer within 5 seconds is handled as step 4.
6. After the service confirms, the host sends the normal start message. The service forwards it to every member, adding that member's own `server_session`, `server_slot` and `server_token`.
7. A member that receives the start message sends `JOIN` to the relay with those values. A member whose start message lacks them shows "Could not join the match." and returns to the lobby list.
8. Every client begins the match when the relay sends `START`.
9. If a player joins or leaves the lobby between steps 3 and 6, the host closes the relay connection, enables Start again, and sends nothing.
10. After the start message, a client that gets `REFUSE` from the relay, or no `START` within 45 seconds, closes the relay connection, shows "Could not join the match.", leaves the lobby and returns to the lobby list. This applies to the host as well.

Players who lack the map cannot press Accept, and the host cannot start, whether or not the map is an official one. Map transfer is never offered in Servers mode.

Quick Match is not available.

### 5.5 What stays as it is

Everything else in the lobby and staging room: chat, slots, sides, teams, settings, passwords, kicking.

## 6. Limits

| Limit | Value |
|---|---|
| Match record | 7 to 4096 bytes |
| `DATA` message | 16384 bytes |
| Players per session | 8 |
| First message deadline | 5 seconds |
| Connections from one address that have not sent a first message | 4 |
| Connections in total that have not sent a first message | 256 |
| Wait for all players to connect | 30 seconds after `CREATED` |
| Loading | 120 seconds after `START` |
| Stall before a drop | 20 seconds |
| Pass lifetime | 10 minutes |
| Entry checks per address | 6 per minute |
| `CREATE` per address | 6 per minute |
| `JOIN` per address | 12 per minute |
| Failed `JOIN` per session | 32, then the session refuses further joins |
| Concurrent sessions | Configured, default 200. Above it `CREATE` gets `SERVER_BUSY`. |
| Data from one player during a match | 64 KB per second and 600 records per second. Above either the player is dropped with `PROTOCOL`. |
| Failed records before a drop | 32 |

Time limits and counts in this table are relay configuration values with these defaults.

## 7. Region configuration

### 7.1 Client region file

Plain text, one region per block, shipped with the game.

```
[region]
number    = 1
name      = Europe
relay     = eu.relay.example.net:7777
tag       = eu
tag_port  = 1
```

| Key | Meaning |
|---|---|
| `number` | Region number, 1 to 65535, the same value as in the pass |
| `name` | Shown to the player |
| `relay` | Host name or address, and port, of the region's relay |
| `tag` | Short code of the region, 1 to 32 characters from `a-z`, `0-9`, `.` and `-`. It is the `Region` value of the region's server lobbies (5.2). |
| `tag_port` | Reserved. Always 1. |

Blocks are read in order, and that order decides ties. To add a region, add a block. The launch file contains Europe only.

### 7.2 Relay configuration file

```
region            = 1
listen_port       = 7777
ping_limit_ms     = 150
key_file          = keys/region.key
max_sessions      = 200
vpn_check         = on
vpn_ranges        = data/blocked-ranges.txt
vpn_operators     = data/blocked-operators.txt
vpn_operator_db   = data/address-to-operator.mmdb
vpn_allow         = data/allowed-ranges.txt
```

| Key | Meaning |
|---|---|
| `region` | This relay's region number |
| `listen_port` | UDP port |
| `ping_limit_ms` | Entry check limit |
| `key_file` | Region signing key. Never committed. |
| `max_sessions` | See section 6 |
| `vpn_check` | `on` or `off` |
| `vpn_ranges` | Text file, one network range per line, refused |
| `vpn_operators` | Text file, one network operator number per line, refused |
| `vpn_operator_db` | Free database mapping an address to its network operator |
| `vpn_allow` | Text file, one network range per line, always allowed |

Every limit in section 6 can also be set here by name. A missing or invalid required value stops the relay at start with a message naming it.

## 8. Libraries

All free and open source.

| Part | Library | Licence |
|---|---|---|
| Relay link, client and relay | ENet | MIT |
| Pass signatures in the relay | libsodium | ISC |
| Address lookups for the VPN check in the relay | libmaxminddb | Apache 2.0 |

## 9. Known limits of version 1

- Match traffic is not encrypted. Passes and join tokens stop strangers joining, but records are readable on the network path.
- The list filter and the entry check triggers run in the client. A modified client can see other lobbies.
- A pass is not tied to an account. Someone who is given another player's pass can enter that region's server lobbies for up to 10 minutes. They still cannot play, because the relay checks the address the pass was issued to.
- A slow player still slows the others until Phase 3.
- One relay per region. One lobby service for all regions.
- No reconnect, no mid-match spectators, no map transfer, no IPv6 on the relay link.

## 10. To verify during implementation

Assumptions this document relies on.

| # | Assumption | State |
|---|---|---|
| V1 | ENet builds with the game's toolchain and coexists with the engine's use of `winsock.h` | Confirmed on the earlier base. Re-check on this base in C1, because the client now also links GameNetworkingSockets and curl. |
| V2 | Every carried command type round-trips through `copyBytesForNetPacket` and `ConstructNetCommandMsgFromRawData` without loss | Held in a live two-instance match on the earlier base. Re-check in C2. |
| V3 | The engine can remove a player at a given frame from a `DROP` message, with the same result on every client, without the disconnect screen | Built on the earlier base, not seen live. C2. |
| V7 | `command_count` in `FRAMEINFO` equals the number of that slot's `GAMECOMMAND`, `PLAYERLEAVE` and `RUNAHEAD` records for the frame | C2, with the relay's `--strict-command-count` option |
| V8 | In real traffic every synchronized record satisfies the frame window, including when the run-ahead changes | C2. This base runs the logic at 60 frames per second. |
| V9 | With no player-to-player connections in a server lobby, the client's run-ahead, latency and frame metric code still works | C2. Known hazards are listed in the C2 package. |
| V10 | The client accepts the lobby service's answers as specified in SERVICE.md | L packages, then the Phase 1 check with the real client |

V4, V5 and V6 no longer apply. They concerned the third-party lobby used on the earlier base.
