# A3 Relay: VPN check in the entry check

State: todo
Branch: servers-A3

## Goal

Refuse an entry check from an address that belongs to a VPN, proxy or hosting provider, using free data files loaded into memory.

## Depends on

A2

## Read first

- PROTOCOL.md sections 4.4 and 7.2.
- DECISIONS D20 to D24.

## May edit

- `Servers/Relay/` only

## Must not edit

Anything outside `Servers/Relay/`.

## Done when

- [ ] Three checks, each switchable in configuration: the address is inside a listed network range; the address belongs to a listed network operator number; the address is on an allow list, which overrides the other two.
- [ ] Range lists are plain text files, one network per line. Operator lookup uses a free address-to-operator database read with libmaxminddb.
- [ ] All data is loaded into memory at start. A lookup does no file or network access.
- [ ] Data is reloaded without a restart on a signal or a configured interval, and a failed reload keeps the previous data.
- [ ] A flagged address gets `REFUSE` `VPN_DETECTED` before any ping measurement.
- [ ] Starting with missing data files is an error unless the check is switched off.
- [ ] The relay logs each refusal with the reason and a shortened address.
- [ ] The file formats and where to obtain the free data are documented in `Servers/Relay/README.md`.
- [ ] Reviewed by someone other than the author.

## Out of scope

Detecting residential VPNs. Any check inside the game client.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
