# A2 Relay: entry check with ping measurement and ping limit

State: todo
Branch: servers-A2

## Goal

Implement the entry check: a client sends `CHECK_HELLO`, the relay measures round-trip time itself and answers with a pass or a refusal.

## Depends on

A1

## Read first

- PROTOCOL.md sections 4.2, 4.3, 4.4, 4.9 and 6.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [ ] `CHECK_HELLO` with a wrong `proto` gets `REFUSE` `PROTO_MISMATCH`.
- [ ] The relay sends 8 `PING` messages 250 ms apart, each with a fresh random nonce, times each matching `PONG` with its own clock, and uses the median. ENet's own round-trip figure is not used.
- [ ] A `PONG` with an unknown or already answered nonce is ignored. Fewer than 5 answers in 4 seconds gives `REFUSE` `TIMEOUT`.
- [ ] Above the configured limit: `REFUSE` `PING_TOO_HIGH` carrying the measured value and the limit.
- [ ] Otherwise: `PASS` with every field filled as PROTOCOL.md 4.3 lists, including a random `pass_id`, then disconnect.
- [ ] A hook exists where A3 plugs in the VPN check before the measurement. Until A3, the hook allows everyone.
- [ ] Entry checks are limited per source address as PROTOCOL.md 6 states. Above the limit: `REFUSE` `RATE_LIMITED`.
- [ ] A small command-line test client in `Servers/Relay/tools/` runs the entry check and prints the result. It is used by the Phase 1 gate.
- [ ] Reviewed by someone other than the author.

## Out of scope

VPN data (A3). Sessions (A4).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
