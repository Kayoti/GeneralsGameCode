# C2 Client: commands as records, player removal from the server

State: todo
Branch: servers-C2

## Goal

In server mode, send every outgoing network command to the relay as a record and feed every incoming record into the existing command handling, so a match runs with no player-to-player connections. Apply a `DROP` from the relay.

## Depends on

C1

## Read first

- PROTOCOL.md sections 4.6 and 4.7 in full.
- CODEMAP "Match networking", all rows.
- In `ConnectionManager.cpp`: `parseUserList`, `update`, `doRelay`, `processNetCommand`, `sendLocalCommand`, `sendLocalCommandDirect`, `sendRemoteCommand`, `ackCommand`, `updateRunAhead`, `allCommandsReady`, `disconnectPlayer`, `processPlayerLeave`, `doKeepAlive`.
- In `Network.cpp`: `init`, `updateLogicFrame`, `processRunAheadCommand`, `processDestroyPlayerCommand`.
- `NetworkUtil.cpp`: `IsCommandSynchronized`, `CommandRequiresAck`, `CommandRequiresDirectSend`.
- `DisconnectManager.cpp`: `update`, `allCommandsReady`, `allowedToContinue`.
- Where the game waits for every player's load-complete before starting, in `GameLogic.cpp`.

## May edit

- `ConnectionManager.h/.cpp`, `Network.cpp`, `DisconnectManager.h/.cpp`
- `ServerLink.*` from C1, if its interface needs a small addition

## Must not edit

`Transport.*`, `Connection.*`, `NetPacket*`, `NetCommandMsg*`, `FrameData*`, `NAT.*`, `FirewallHelper.*`, `LANAPI*`, `GameSpy/`. The command encoding must not change.

## Done when

Every item applies only when the server mode flag is on. With it off, behaviour is identical to before.

- [ ] `parseUserList` creates a `FrameDataManager` for each human slot as today, and creates no `Connection` objects and no `Transport`.
- [ ] Outgoing: a carried command is written with `copyBytesForNetPacket` and sent as one record. Synchronized commands are also added to the local frame data, as today. Nothing is sent before `START`. Commands that are not carried are never sent.
- [ ] A command longer than 4096 bytes is not sent and not counted.
- [ ] Incoming: each record is turned into a command with `NetPacket::ConstructNetCommandMsgFromRawData` and handled by the code that handles a relayed command today, without acknowledgement and without relaying. A record that fails to parse is discarded.
- [ ] Client checks from PROTOCOL.md 4.6 are applied to received records: `RUNAHEAD` only from the leader and inside range, `PLAYERLEAVE` only for the sender's own slot, `TIMEOUTSTART` only from the host slot. `DESTROYPLAYER` and every other not-carried type is discarded.
- [ ] The acknowledgement path (`ackCommand`, pending and relayed command lists, resends, keep-alives, frame resend requests) is not entered.
- [ ] The packet router slot follows `leader_slot` from `WELCOME` and `ROSTER`. Run-ahead is calculated by the leader as today, and others send their metrics to the leader through the relay.
- [ ] `DROP` is applied as PROTOCOL.md 4.7 lists: discard that slot's records above `last_frame`; keep the slot through `last_frame`; at `last_frame + 1` apply locally what the destroy-player command does; count the slot as loaded if the match is still loading; ignore a `DROP` for a slot already removed.
- [ ] The disconnect screen never starts a vote and never removes a player in server mode. If frames stall it may show that the game is waiting. Removal comes only from `DROP` or from the player's own leave command.
- [ ] Loss of the relay connection ends the match for the local player with a message.
- [ ] This settles PROTOCOL.md V2 and V3, and gives the traffic needed for V7 and V8. Anything that does not hold is recorded in the handoff notes with the command type and the difference.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author, with a dedicated pass on anything that could change the simulation result.

## Out of scope

The server clock (S2). Starting a match from the staging room (2.1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
