# A1 Relay: ENet host, configuration, pass signing and checking

State: partial
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

- [x] The relay builds as a standalone program with its own build file, independent of the game build.
- [x] ENet and libsodium are fetched by the relay's build file at pinned versions.
- [x] It reads the relay configuration file and refuses to start on a missing or invalid value, saying which one.
- [x] It creates an ENet host with 2 channels on the configured port and logs connect and disconnect events.
- [x] A connection that sends no valid first message within 5 seconds is disconnected.
- [ ] Pass module: build, sign, parse, and check a pass exactly as PROTOCOL.md 4.3 lists, including expiry, the address comparison, and the start option that switches the comparison off for testing on one PC. Open: the pass_id-in-use rule is a caller check (see handoff notes).
- [x] A command-line option generates a new region key pair into files.
- [x] Message types and codes from PROTOCOL.md 4.2 and 4.9 exist as named constants in one header.
- [x] Formatter configuration is committed and applied.
- [ ] Reviewed by someone other than the author.

## Out of scope

Entry check (A2), VPN data (A3), sessions (A4), records (A5).

## Handoff notes

- Finished:
  - `Servers/Relay/CMakeLists.txt`: standalone build, replaces the placeholder. Option `RELAY_BUILD_TESTS` (off by default). ENet 1.3.18 fetched by commit `2662c0de09e36f2a2030ccc2c528a3e4c9e8138a`. libsodium 1.0.22 fetched as `libsodium-1.0.22-msvc.zip` from the release, checked with SHA-256 `3e03a726fac4bc09cb61d8f29d658ef7a5eca0811de59082130414f7ca2e4279`.
  - `src/protocol.h`: message types (4.2, including reserved 20 and 21), REFUSE codes and DROP reasons (4.9), body sizes (CHECK_HELLO 10, CREATE 104, JOIN 119, PASS 100), limits used so far.
  - `src/config.h/.cpp`: parser for section 7.2. Comment and blank lines, trimmed `=`, LF or CRLF. Refuses unknown, duplicate, missing, or out-of-range keys and names them.
  - `src/pass.h/.cpp`: `BuildPass` and `CheckPass`, with the 4.3 layout and checks (length, signature, version, reserved bytes, region, expiry, address). The address check can be turned off through the caller's context.
  - `src/region_keys.h/.cpp`: `GenerateRegionKeys` (never overwrites) and `LoadRegionKey` (derives the public key and checks it against the stored secret).
  - `src/host.h/.cpp`: ENet host on 2 channels, 256 peer slots. Logs connect and disconnect by connection number, with no IP addresses. Classifies the first message (exact length for CHECK_HELLO, CREATE, JOIN). Disconnects a connection with no valid first message after 5 seconds.
  - `src/main.cpp`: `relay [config] [--no-address-check]` and `relay --generate-key <file>`.
  - `src/text.h`: shared trim helper.
  - `tests/relay_tests.cpp`: unit tests for config, keys and pass checks. Built only with `RELAY_BUILD_TESTS=ON`.
  - `Servers/Relay/.clang-format` (force-added, see below) and `Servers/Relay/.gitignore` (`keys/`, `*.key`, `*.pub`).
  - Build commands (run inside the VS x86 environment, from a .bat file):
    - Configure: `cmake -S C:\cnc-wt\A1\Servers\Relay -B C:\cnc-build\A1 -G Ninja -DCMAKE_BUILD_TYPE=Release -DRELAY_BUILD_TESTS=ON`
    - Build: `cmake --build C:\cnc-build\A1`
    - Tests: `ctest --test-dir C:\cnc-build\A1`
- Left to do:
  - Pass_id-in-use rule (4.3, last rule). `CheckPass` returns the fields, and the caller rejects `PASS_IN_USE` when `fields.pass_id` belongs to a live match connection. Those connections come from A4, so no caller exists yet. The project owner needs to decide whether this item counts as met in A1 or stays open until A4.
  - A2 must call `CheckPass` for `CHECK_HELLO` and `JOIN`, and pass the start option (`check_address`) through. Today `main` only parses the flag and prints the warning.
  - Handlers for CHECK_HELLO (A2) and CREATE and JOIN (A4). In A1 a valid first message is accepted and every later message is dropped, so a client cannot yet get a pass or a session.
  - Reviewer (not ticked, by design).
- Surprises and open questions:
  - Pass size is 100 bytes, as corrected in PROTOCOL.md 4.3. CREATE's body is 104 bytes and JOIN's is 119.
  - libsodium has no CMake build. The relay uses the official prebuilt MSVC static library (v143, Win32 or x64, Release or Debug) from the 1.0.22 release. I checked the archive against the SHA-256 digest that GitHub publishes for it. I did not check the minisign signature the project also publishes. Owner decision: keep the prebuilt library, or build from the source tarball in a later package.
  - ENet's CMake file asks for a minimum CMake version of 2.8.12. CMake 4.4 refuses that, so `CMAKE_POLICY_VERSION_MINIMUM` is set to 3.5 around the fetch. ENet also does not export its include path, so the relay build file adds it.
  - libmaxminddb is not fetched. Nothing in A1 uses it. A3 needs it.
  - Not in PROTOCOL.md, chosen here. Owner to confirm:
    - `client_ip` is the IPv4 address as a host-order integer (`ntohl` of `ENetAddress.host`), stored little-endian like every integer. A2 must encode the same way.
    - Key file format: `region.key` holds the 64-byte libsodium secret key as 128 hex characters. `region.key.pub` holds the 32-byte public key as 64 hex characters. The client never needs the public key under the current protocol, because it does not verify passes.
    - Numeric config keys must be whole numbers from 1 to 65535. The document gives no range for `ping_limit_ms` or `max_sessions`.
    - Paths in the config (including `key_file`) are relative to the working directory.
  - PROTOCOL.md section 6 says every limit "can also be set here by name", but the document gives no key names. A1 accepts only the section 7.2 keys and refuses anything else. Owner: name the keys, or confirm they come with the packages that use them.
  - Section 6 limits not yet enforced: 4 unidentified connections per address, the per-address rate limits, and the per-session limits. They belong to A2, A4 and A6. The 256 in section 6 is used as the ENet peer count.
  - Section 10 assumptions. None are tested by A1. V1 belongs to C1.
  - The root `.gitignore` ignores `.clang-format` and `*.key` is not covered at the root. The formatter file is force-added (`git add -f`). `.gitignore` at the root was not edited. The new `Servers/Relay/.gitignore` covers the key files.
  - Gotcha for later packages: the ENet sources emit their own MSVC warnings (C5287, C4996). Those are not from relay code, and the relay's own targets build with `/W4 /WX`.
- Verification evidence:
  - Build: configure and build from scratch into `C:\cnc-build\A1` (Ninja, Release, x86). Result: `BUILD_OK`, log at `C:\cnc-build\A1-build.log`. No warnings from `Servers/`.
  - Unit tests: `relay_tests.exe` prints `all relay tests passed`. `ctest` reports 1 of 1 passed.
  - Formatting: `clang-format --dry-run --Werror` exits 0 on all relay sources. Version 22.1.3, from the VS 2026 LLVM install.
  - Lint: `clang-tidy` 22.1.3 with the root `.clang-tidy` reports 0 warnings on the relay `.cpp` files.
  - Runtime, on one PC, with `relay.conf` from the repo (CRLF) and a generated key:
    - Idle client: `disconnected by relay at 5090 ms`. The relay logs `sent no valid first message within 5 seconds, disconnecting`.
    - Valid CHECK_HELLO (11 bytes): still connected after 8018 ms. The relay logs `first message type 1 accepted`.
    - Invalid first message (2 bytes): the relay logs `not a valid first message`. The client is disconnected at 5044 ms.
    - Port already in use: `relay: cannot open UDP port 7777`, exit 1.
    - `ping_limit_ms` removed: `missing key 'ping_limit_ms'`, exit 1. `listen_port = 0`: `line 8: key 'listen_port' must be a whole number from 1 to 65535`, exit 1. `vpn_check = yes`: `line 12: key 'vpn_check' must be 'on' or 'off'`, exit 1. No config file: `cannot open configuration file 'relay.conf'`, exit 1.
    - `--generate-key` writes 130 and 66 bytes. A second run refuses to overwrite, exit 1. `--no-address-check` prints the warning.
  - Not verified: the game build with the Servers switch on or off (not applicable, no game files changed). Linux (out of scope). The address check end to end (needs A2). The scratch ENet client used for the runtime checks is in `C:\cnc-build\client-src\client.cpp`, outside the repository.
- Review findings and fixes:
