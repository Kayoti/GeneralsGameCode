# A1 Relay: ENet host, configuration, pass signing and checking

State: blocked
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

- Finished: nothing yet. Work did not start because the pass layout is not settled (see below).
- Left to do: the whole package, once the pass size is resolved.
- Surprises and open questions:
  - Blocking: PROTOCOL.md section 4.3 is internally inconsistent about the pass size. The field table (version 1, reserved 1, region 2, client_ip 4, exe_crc 4, ini_crc 4, rtt_ms 2, reserved2 2, issued_at 4, expires_at 4, pass_id 8, signature 64) totals 100 bytes, but the same section says "Total 96 bytes" and section 4.2 describes PASS, CREATE and JOIN as carrying a "96-byte pass". The pass size fixes the byte layout the relay signs and checks and the CREATE/JOIN body sizes, so it cannot be chosen by a package. Resolve on the `servers` branch: either change every "96" to 100, or remove 4 bytes from the field list and name the field. Likely cause: `rtt_ms` (2) and `reserved2` (2) were added after the 96 figure was written (without them the signed part is 32 bytes, 32 + 64 = 96).
  - Secondary questions for when work resumes: libsodium ships no CMake build, so a Windows-only relay build (MSVC source list) is the straightforward route for Phase 1, with Linux left to package 4.1 — confirm acceptable. The root `.gitignore` ignores `.clang-format` and all dot files and does not ignore `keys/`; committing the formatter config needs a force-add or a `.gitignore` change, which is outside this package's edit list.
- Review findings and fixes:
