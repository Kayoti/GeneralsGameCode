# A1 Relay: ENet host, configuration, pass signing and checking

State: todo
Branch: servers-A1

## Goal

Create the relay program: it starts, reads its configuration, listens with ENet, and can sign and check a pass. No entry check and no sessions yet.

## Depends on

0.1, 0.2, 0.3

## Read first

- PROTOCOL.md sections 3, 4.1, 4.3, 7.2 and 8.
- STANDARDS.md "Relay server code", "Security", "Performance".

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`. The relay includes no game engine code.

## Done when

- [ ] The relay builds as a standalone program with its own build file, independent of the game build.
- [ ] ENet and libsodium are fetched by the relay's build file at pinned versions.
- [ ] It reads the relay configuration file and refuses to start on a missing or invalid value, saying which one.
- [ ] It creates an ENet host with 2 channels on the configured port and logs connect and disconnect events.
- [ ] A connection that sends no valid first message within 5 seconds is disconnected.
- [ ] Pass module: build, sign, parse, and check a pass exactly as PROTOCOL.md 4.3 lists, including expiry, the address comparison, and the start option that switches the comparison off for testing on one PC.
- [ ] A command-line option generates a new region key pair into files.
- [ ] Message types and codes from PROTOCOL.md 4.2 and 4.9 exist as named constants in one header.
- [ ] Formatter configuration is committed and applied.
- [ ] Reviewed by someone other than the author.

## Out of scope

Entry check (A2), VPN data (A3), sessions (A4), records (A5).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
