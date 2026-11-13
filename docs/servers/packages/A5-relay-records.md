# A5 Relay: record validation and forwarding

State: todo
Branch: servers-A5

## Goal

Carry the match: accept `DATA` messages from each player, check every record against the rules for its command type, and forward it to the right players.

## Depends on

A4

## Read first

- PROTOCOL.md section 4.6 in full, and sections 6 and 10.
- STANDARDS.md "Security" and "Performance".

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [ ] `DATA` is accepted only after `START`. A record before `START` hands the player to the drop path.
- [ ] Each record passes checks 1 to 5 of PROTOCOL.md 4.6 or is dropped and counted. The header must match the tag list for its type exactly. The scan never reads past the record.
- [ ] The per-type table is implemented as written: the extra checks, and who each type is forwarded to. For `GAMECOMMAND`, `FRAMEINFO` and `PLAYERLEAVE` the record's own relay mask is ignored.
- [ ] `next_frame` per slot starts at 30 and advances only on a valid `FRAMEINFO`. The frame window is enforced.
- [ ] `RUNAHEAD` is accepted only from the current `leader_slot` and only with values inside the stated ranges.
- [ ] `TIMEOUTSTART` is accepted only from the host slot.
- [ ] A valid record's bytes are forwarded unchanged.
- [ ] Command count: the relay counts per slot and frame as PROTOCOL.md describes, and logs a mismatch without acting on it. A configuration switch turns a mismatch into a violation, for use once V7 is settled.
- [ ] The data rate limits per player are enforced.
- [ ] The forwarding path allocates no memory per record.
- [ ] 32 failed records from one player hands that player to the drop path with reason `PROTOCOL`.
- [ ] The relay keeps counters per session: records in, forwarded, and dropped by reason. A debug setting logs each violation with type, frame and `next_frame`, so V7 and V8 can be checked against real traffic.
- [ ] Reviewed by someone other than the author, with a dedicated pass on the header scan and on every bound.

## Out of scope

Rewriting frames or closing frames (S1).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
