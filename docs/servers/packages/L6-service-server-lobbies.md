# L6 Lobby service: server lobbies

State: done
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

- [x] The service reads each region's tag and public key from configuration. The public key file format is the one `relay --generate-key` writes. A region with an unreadable key stops the service at start with a message.
- [x] A function checks an `X-Servers-Pass` header value by the rules of PROTOCOL.md 5.2: 200 lowercase hexadecimal digits, valid signature for the region in the pass, `version` 1, not expired, and build fingerprints equal to given values. Any other input is refused without a panic.
- [x] `PUT Lobbies` with `server_mode: true` requires a valid pass whose fingerprints equal the request's `exe_crc` and `ini_crc`. The lobby gets `LobbyType` 2 and `Region` set to that region's tag. Without a valid pass the answer has `result` 0 and all four keys.
- [x] `PUT Lobby/{id}` on a server lobby requires a valid pass for the lobby's region with the lobby's fingerprints. Refusal is `403`.
- [x] Server lobbies get empty TURN credentials. Messages 17 and 18 are never sent for them, and 12 and 19 from their members are dropped.
- [x] Message 26 from the host of a server lobby is answered at once with 28, `mesh_complete` true.
- [x] A member with `has_map` false cannot become ready in a server lobby.
- [x] `POST Lobby/{id}` with `field` 19 validates and stores the server session by the rules in SERVICE.md section 10, answers `{"result":1}` or `{"result":0}`, and is cleared when any member joins or leaves. It never appears in any other answer or in a log.
- [x] Message 13 from the host of a server lobby is ignored without a stored session. With one, each member's message 13 carries `server_session`, `server_slot` and `server_token` for that member only; the host's token is an empty string.
- [x] Online lobbies behave exactly as after L5. A test proves a lobby created without `server_mode` ignores the header.
- [x] Tests sign passes with a key pair generated in the test. They cover: good pass, bad signature, expired, wrong region, wrong fingerprints, wrong length, non-hexadecimal, create and join with and without a pass, token delivery to three members, and a join after the session was stored.
- [x] `lobbyprobe` gets a mode that signs its own passes with a key file given on the command line and runs the server lobby path end to end, including field 19 and reading its own token from message 13.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [x] Reviewed by someone other than the author.

## Out of scope

- Any change to the relay.
- Checking the caller's address against the pass.

## Handoff notes

- Finished: `Servers/Lobby/internal/pass/` (new: `pass.go` check and key loading, `sign.go` signing for the probe and tests), `internal/config` unchanged, `internal/lobbies/` (`lobby.go`, `requests.go`, `views.go`, new `serversession.go`), `internal/realtime/` (`lobbies.go`, `lobby_handlers.go`, `meshcheck.go`, `messages.go`, `p2p.go`), `internal/api/` (`lobbies.go`, `handler.go`, `turn.go`), `cmd/lobby/main.go`, `tools/lobbyprobe/` (`main.go`, new `serverlobby.go`), tests in each package, and a comment line in `lobby.conf`. `Hub.JoinLobby` now also returns whether the lobby is a server lobby, so existing test call sites changed from `err :=` to `_, err :=`.
- Left to do: review by someone other than the author.
- Surprises and open questions:
  - `lobby.conf` names `keys/region.key.pub`. Since the service now stops at start when a region key cannot be read, the example config only starts once that file exists (generate with `relay --generate-key keys/region.key`). TESTING.md, which describes starting the service with that file, was not edited (outside "May edit").
  - A field 19 body with a wrong-typed key (for example a number as `server_session`) is answered `{"result":0}`, not `{}`, because the field can still be read.
  - The pass check does not compare the reserved bytes or `issued_at` (PROTOCOL.md 5.2 lists neither); the relay also checks reserved bytes, which the signature already covers for passes the relay made.
  - Host `has_map` is not required for ready, since the host is always ready (SERVICE.md 7.1).
  - Verified live: built the service and `lobbyprobe`, generated a key pair, ran `lobbyprobe --server-key` against the running service: every step ok, exit 0. The service refused to start with the `.pub` file missing, with a message naming the region and file.
- Review findings and fixes:
  - Approved. The pass-verification core was checked byte-for-byte against the relay: the field offsets
    (version@0, region@2, exe_crc@8, ini_crc@12, issued@20, expires@24, signature@36 over the first 36
    bytes, little-endian), the Ed25519 scheme and the key-file format (32-byte public hex, 64-byte secret)
    all match the relay's `pass.cpp`/`region_keys.cpp`, so a pass the relay signs verifies here and a
    forged or wrong-region/expired/wrong-fingerprint pass is refused; the 100-byte pass is bounds-fixed
    before any field is read and a nil verifier refuses. `PUT Lobbies` with `server_mode` requires a valid
    pass matching exe_crc/ini_crc and sets `LobbyType` 2 and the region; `PUT Lobby/{id}` of a server lobby
    requires a valid pass for the region, else 403, checked after the lobby exists; server lobbies get empty
    TURN credentials, never send 17/18 and drop 12/19; message 26 is answered at once with 28 and
    `mesh_complete` true; a member without the map cannot ready; field 19 is validated per SERVICE 10,
    cleared on any join or leave and never shown; message 13 is ignored without a stored session, else each
    member gets its own session, slot and token with the host's token empty; Online lobbies are unchanged.
    `gofmt`/`go vet`/`go test` all clean. Scope, author and commit hygiene correct.
  - Two low notes, no change: an idempotent re-join by an existing member returns early without
    re-checking the pass (the member was already admitted with a valid pass and nothing changes — not a
    hole); the example `lobby.conf` now makes the service refuse to start until the region key file is
    generated, which is the required behaviour.
  - The live interop of this pass check against a relay-signed pass is exercised by the L+D menu-flow phase
    gate (create/join a server lobby), which needs a person at the menus; it is verified here by format and
    offset analysis (standard Ed25519), pending that live run.
