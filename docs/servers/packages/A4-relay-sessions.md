# A4 Relay: sessions

State: todo
Branch: servers-A4

## Goal

Implement match sessions: the host creates one and receives a join token per player, other players join with their token, everyone is told who is connected, and the relay announces the start.

## Depends on

A1

## Read first

- PROTOCOL.md sections 4.2, 4.3, 4.5, 4.9 and 6.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [ ] `CREATE` is accepted only with a valid pass, `host_slot` inside `slot_mask`, and at least one other bit set. Otherwise `REFUSE` `BAD_REQUEST` or the pass code that applies.
- [ ] An accepted `CREATE` makes a session with a random 64-bit id and one random 64-bit join token per other expected slot, binds the connection to `host_slot`, and replies `CREATED` then `WELCOME`.
- [ ] `JOIN` is accepted only when every condition in PROTOCOL.md 4.5 step 5 holds. Each failure maps to its own `REFUSE` code, including `ALREADY_STARTED` and `BAD_TOKEN`. A token is used up by a successful join.
- [ ] A `pass_id` already in use by a live match connection gets `REFUSE` `PASS_IN_USE`.
- [ ] `ROSTER` is sent to all connected players on every change.
- [ ] `START` is sent once, when all expected slots are connected, or 30 seconds after `CREATED`. In the second case a `DROP` with `NEVER_JOINED` and `last_frame` 29 is sent for each missing slot first. With fewer than 2 connected players everyone gets `REFUSE` `NOT_ENOUGH_PLAYERS`.
- [ ] `leader_slot` is always the lowest connected slot.
- [ ] A session with no connected player is removed and its memory released.
- [ ] The limits in PROTOCOL.md 6 for `CREATE`, `JOIN`, failed joins per session, concurrent sessions and connections without a first message are enforced.
- [ ] Random values come from the operating system's secure source.
- [ ] The test client from A2 gains modes to create and join a session.
- [ ] Reviewed by someone other than the author.

## Out of scope

Forwarding records (A5). Removing players during a match (A6).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
