# A3 Relay: VPN check in the entry check

State: partial
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

- [x] Three checks, each switchable in configuration: the address is inside a listed network range; the address belongs to a listed network operator number; the address is on an allow list, which overrides the other two.
- [x] Range lists are plain text files, one network per line. Operator lookup uses a free address-to-operator database read with libmaxminddb.
- [ ] All data is loaded into memory at start. A lookup does no file or network access.
- [x] Data is reloaded without a restart on a signal or a configured interval, and a failed reload keeps the previous data.
- [x] A flagged address gets `REFUSE` `VPN_DETECTED` before any ping measurement.
- [x] Starting with missing data files is an error unless the check is switched off.
- [x] The relay logs each refusal with the reason and a shortened address.
- [x] The file formats and where to obtain the free data are documented in `Servers/Relay/README.md`.
- [ ] Reviewed by someone other than the author.

## Out of scope

Detecting residential VPNs. Any check inside the game client.

## Handoff notes

- Finished:
  - `Servers/Relay/src/vpn_check.h` and `vpn_check.cpp`: VPN settings, parsing of the range, allow and operator lists, the libmaxminddb operator lookup, `VpnCheck::Load` (keeps the data in use when a load fails) and `VpnCheck::Check` (allow list first, then ranges, then operators).
  - `Servers/Relay/src/host.h` and `host.cpp`: the host takes the `VpnCheck`. The check runs after the rate limit and before the ping measurement. Refusals are logged with the reason and a shortened address. Reload requests are applied in the service loop.
  - `Servers/Relay/src/entry_check.h` and `entry_check.cpp`: the A2 placeholder `VpnCheckPasses` is removed.
  - `Servers/Relay/src/main.cpp`: builds the VPN settings from the configuration and loads the data before the host starts. A failure stops the relay. With the check on, Ctrl+Break reloads the data.
  - `Servers/Relay/CMakeLists.txt`: libmaxminddb 1.14.1, fetched by commit `4a3d17a621c0ebcb81498fd34d313f67fd940ab5` (same pattern as ENet). Static, no tests, binaries or install. Linked into `relay_core`.
  - `Servers/Relay/tests/relay_tests.cpp`: the placeholder test is replaced. The VPN tests write a small MaxMind DB themselves.
  - `Servers/Relay/README.md`: file formats, where to get the data, reload, logs.
- Evidence:
  - Build: Visual Studio 2026 BuildTools, x86, Release, `/W4 /WX` on the relay code. `relay`, the tools and `relay_tests` build. `ctest` passes and the test binary prints "all relay tests passed". Not built with the game switch on or off (relay only).
  - clang-format 22.1.3 with the relay `.clang-format`: clean on every changed file.
  - clang-tidy 22.1.3 with the root `.clang-tidy` on the five changed `.cpp` files: 0 warnings. Headers are checked only through the `.cpp` files that include them.
  - End to end with the built `relay.exe` on loopback, in a scratch folder outside the repository, using `entry_check_client`:
    - Operator rule: `REFUSE VPN_DETECTED` (code 2), no ping step. The log line is `connection 1: VPN check refused 127.0.0.x, listed network operator 64496`.
    - Allow list: `PASS`.
    - Missing operator file at start: exit code 1, `relay: VPN check: cannot open 'data/no-such-file.txt'`.
    - Same missing file with `vpn_check = off`: the relay starts and gives `PASS`.
    - Ctrl+Break, sent by a helper process to the relay's console: the log shows `VPN data reloaded` and the next client gets `PASS`. After a failed reload (operator file missing) the log shows `relay: VPN data not reloaded, the data in use is kept: ...` and the next client still gets `PASS`.
- Left to do:
  - Owner decision on the in-memory item (see the first open question). Left unticked, so the state is `partial`.
  - Review by someone other than the author.
  - Run against a real GeoLite2-ASN file. None is available offline.
- Surprises and open questions:
  1. In memory: libmaxminddb opens the database as a read-only memory mapping (`MMDB_MODE_MMAP`), the only mode it offers. The relay touches each page once at start, so lookups make no file call. Windows can still page the mapping out under memory pressure, and the next lookup then reads from disk. This does not fully meet "all data is loaded into memory". Options: accept the mapping, or copy the file into the heap and point the library's struct at the copy. The second option depends on the struct's internal pointers, so it is fragile. Owner decision.
  2. Per-check switches: PROTOCOL 7.2 has only `vpn_check` and the file keys. I made an empty list file switch that list's check off, which needs no new key. If separate keys are wanted, that is a PROTOCOL change first.
  3. Reload trigger: Windows has no SIGHUP. Ctrl+Break in the relay's console (`SetConsoleCtrlHandler`, `CTRL_BREAK_EVENT`) starts the reload. It is set up only when the check is on. A relay run as a service would need another trigger.
  4. Operator key: the lookup reads `autonomous_system_number` (uint32), the field in GeoLite2-ASN. This was tested only against a database the test and a scratch script write. It is not confirmed against a real MaxMind file.
  5. Formats I chose, since PROTOCOL does not fix them: CIDR notation; a bare address means one host; host bits are ignored; the operator number is decimal; `#` comments; 512-byte line limit; no size limit on the lists. A UTF-8 byte-order mark on the first line makes the load fail with a line-1 error.
  6. `Servers/Relay/.gitignore` does not exclude `data/`. MaxMind data is licensed and must not be committed. Not changed here.
  7. The libmaxminddb CMake prints "Perl not found. Unable to generate man pages". This does not affect the build.
  8. The example `relay.conf` names `data/` files that are not in the repository. With `vpn_check = on` the relay does not start until they exist, as the package requires.
  9. The helper process used for the Ctrl+Break test received the same console event and exited with a Ctrl+C status. The relay was not affected.
- Review findings and fixes: none yet.
