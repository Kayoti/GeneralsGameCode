# L6 Lobby service: server lobbies

State: todo
Branch: servers-go-L6

## Goal

Add server lobbies to the service: marked by type and region, admitted by pass, with no player-to-player setup and with join tokens handed out in the start message.

## Depends on

L5

## Read first

- DECISIONS.md D34, D42, D43, D50
- PROTOCOL.md sections 4.3 (pass layout) and 5 (all of it)
- SERVICE.md sections 7 and 10, and the `region.*` keys in section 11
- `Servers/Relay/src/pass.h`, `pass.cpp`, `region_keys.cpp` (the pass layout as the relay writes it, and the format of the public key file)

## May edit

- `Servers/Lobby/`

## Must not edit

- `Servers/Relay/`
- Anything else outside `Servers/Lobby/`

## Done when

- [ ] The service reads each region's tag and public key from configuration. The public key file format is the one `relay --generate-key` writes. A region with an unreadable key stops the service at start with a message.
- [ ] A function checks an `X-Servers-Pass` header value by the rules of PROTOCOL.md 5.2: 200 lowercase hexadecimal digits, valid signature for the region in the pass, `version` 1, not expired, and build fingerprints equal to given values. Any other input is refused without a panic.
- [ ] `PUT Lobbies` with `server_mode: true` requires a valid pass whose fingerprints equal the request's `exe_crc` and `ini_crc`. The lobby gets `LobbyType` 2 and `Region` set to that region's tag. Without a valid pass the answer has `result` 0 and all four keys.
- [ ] `PUT Lobby/{id}` on a server lobby requires a valid pass for the lobby's region with the lobby's fingerprints. Refusal is `403`.
- [ ] Server lobbies get empty TURN credentials. Messages 17 and 18 are never sent for them, and 12 and 19 from their members are dropped.
- [ ] Message 26 from the host of a server lobby is answered at once with 28, `mesh_complete` true.
- [ ] A member with `has_map` false cannot become ready in a server lobby.
- [ ] `POST Lobby/{id}` with `field` 19 validates and stores the server session by the rules in SERVICE.md section 10, answers `{"result":1}` or `{"result":0}`, and is cleared when any member joins or leaves. It never appears in any other answer or in a log.
- [ ] Message 13 from the host of a server lobby is ignored without a stored session. With one, each member's message 13 carries `server_session`, `server_slot` and `server_token` for that member only; the host's token is an empty string.
- [ ] Online lobbies behave exactly as after L5. A test proves a lobby created without `server_mode` ignores the header.
- [ ] Tests sign passes with a key pair generated in the test. They cover: good pass, bad signature, expired, wrong region, wrong fingerprints, wrong length, non-hexadecimal, create and join with and without a pass, token delivery to three members, and a join after the session was stored.
- [ ] `lobbyprobe` gets a mode that signs its own passes with a key file given on the command line and runs the server lobby path end to end, including field 19 and reading its own token from message 13.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Any change to the relay.
- Checking the caller's address against the pass.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
