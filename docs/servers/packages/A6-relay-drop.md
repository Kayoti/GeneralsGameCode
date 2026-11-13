# A6 Relay: removing players

State: todo
Branch: servers-A6

## Goal

Remove a player from a running match in a way every remaining client applies on the same frame, and make sure one stuck player cannot hold a match forever.

## Depends on

A5

## Read first

- PROTOCOL.md section 4.7 in full, and sections 4.5, 4.9 and 6.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [ ] Every cause in the PROTOCOL.md 4.7 table drops the player with the listed reason.
- [ ] Loading rule: no `LOADCOMPLETE` within the configured time after `START`.
- [ ] Stall rule: implemented exactly as the table states, and never fires before the first `FRAMEINFO` of the session has been forwarded.
- [ ] On a drop the relay stops forwarding that player's records, closes the connection, then sends `DROP` with `last_frame = next_frame - 1`, then `ROSTER` with the recalculated `leader_slot`.
- [ ] Clean leave: after a forwarded `PLAYERLEAVE` and every `FRAMEINFO` through its leave frame, `LEAVE` or a disconnect produces `ROSTER` only.
- [ ] A disconnect before that point produces a normal `DROP`.
- [ ] The ENet timeout that decides "connection lost", the loading time and the stall time are configuration values.
- [ ] Reviewed by someone other than the author.

## Out of scope

Dropping a player for being slower than the server clock (S1, S3).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
