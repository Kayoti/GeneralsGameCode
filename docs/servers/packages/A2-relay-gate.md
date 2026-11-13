# A2 Relay: entry check with ping measurement and ping limit

State: done
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

- [x] `CHECK_HELLO` with a wrong `proto` gets `REFUSE` `PROTO_MISMATCH`. Evidence: live run with `--proto 2` printed `REFUSE PROTO_MISMATCH (code 1)`, exit 1.
- [x] The relay sends 8 `PING` messages 250 ms apart, each with a fresh random nonce, times each matching `PONG` with its own clock, and uses the median. ENet's own round-trip figure is not used. Evidence: `--trace` run printed 8 PING arrivals spaced 249 to 251 ms; nonces from `RandomId()`; `PingRound` in `entry_check.cpp` times each PONG with `enet_time_get()`; unit tests `TestPingRoundVerdicts`, `TestPingRoundSchedule`; no use of `roundTripTime` in the new code.
- [x] A `PONG` with an unknown or already answered nonce is ignored. Fewer than 5 answers in 4 seconds gives `REFUSE` `TIMEOUT`. Evidence: unit tests `TestPingRoundRejectsBadPongs` and `TestPingRoundVerdicts` (4 answers gives TIMEOUT); live `--no-pong` run printed `REFUSE TIMEOUT (code 4)`.
- [x] Above the configured limit: `REFUSE` `PING_TOO_HIGH` carrying the measured value and the limit. Evidence: live `--delay-ms 200` run printed `REFUSE PING_TOO_HIGH (code 3) rtt 204 ms limit 150 ms`.
- [x] Otherwise: `PASS` with every field filled as PROTOCOL.md 4.3 lists, including a random `pass_id`, then disconnect. Evidence: live runs printed `PASS region 1 rtt 0 ms lifetime 600 s`; unit test `TestIssuePass` checks the signature through `CheckPass` and every field; the reply is sent with `enet_peer_disconnect_later` (`CloseEntryCheck`), and the relay log shows the connections closing.
- [x] A hook exists where A3 plugs in the VPN check before the measurement. Until A3, the hook allows everyone. Evidence: `VpnCheckPasses` in `entry_check.cpp` is called in `RelayHost::OnCheckHello` before `PingRound::Start`; unit test `TestVpnHookAllowsEveryoneForNow`.
- [x] Entry checks are limited per source address as PROTOCOL.md 6 states. Above the limit: `REFUSE` `RATE_LIMITED`. Evidence: live runs, the 8th check from 127.0.0.1 printed `REFUSE RATE_LIMITED (code 19)` after six accepted; unit test `TestAddressRateLimit`.
- [x] A small command-line test client in `Servers/Relay/tools/` runs the entry check and prints the result. It is used by the Phase 1 gate. Evidence: `tools/entry_check_client.cpp` builds as `entry_check_client.exe` and was used for every live case above.
- [ ] Reviewed by someone other than the author.

## Out of scope

VPN data (A3). Sessions (A4).

## Handoff notes

- Finished:
  - `src/entry_check.h/.cpp` (new): `PingRound` (8 PINGs at 250 ms, nonce matching, own-clock RTT, 4 s window, median, 5-answer minimum), `AddressRateLimiter` (6 per 60 s per address), `VpnCheckPasses` (A3 hook, allows all), `IssuePass` (signed 100-byte pass, random `pass_id`, 600 s lifetime), `RandomId`.
  - `src/host.h/.cpp`: CHECK_HELLO handling (proto, rate limit, VPN hook, measurement), PING sending, PONG routing, timed service step, PASS and REFUSE replies closed with `enet_peer_disconnect_later`. The loop wait shortens to the next PING so the spacing holds.
  - `src/main.cpp`: passes region, ping limit and region key to the host.
  - `src/protocol.h`: `REFUSE_BODY_SIZE`.
  - `src/wire.h` (new): little-endian helpers, moved out of `pass.cpp` so there is one copy.
  - `tools/entry_check_client.cpp` (new): test client, flags `--proto`, `--delay-ms`, `--no-pong`, `--trace`. Exit codes 0 PASS, 1 REFUSE, 2 no answer.
  - `tests/relay_tests.cpp`: entry check unit tests.
  - `CMakeLists.txt`: new sources and the `entry_check_client` target.
- Left to do: the reviewer's sign-off (last Done-when item). No other item is open.
- Built: relay with `RELAY_BUILD_TESTS=ON`, MSVC x86 Release, Ninja, from `C:\cnc-build\A2`. Build clean under `/W4 /WX` for all relay targets. `ctest` passes (`all relay tests passed`). Game build not run: this package lists only `Servers/Relay/`, so the switch on and off builds are not applicable.
- Live test: relay started from `C:\cnc-build\A2\relay.exe` with a generated region key, port 7788, on localhost. Eight client runs covered PASS, PROTO_MISMATCH, PING_TOO_HIGH, TIMEOUT and RATE_LIMITED. Only localhost was tested, so every measured RTT was about 0 ms apart from the held-PONG case.
- Lint: `clang-tidy` 22.1.3 on the six changed `.cpp` files, quiet after fixes. `clang-format --dry-run --Werror` reports no differences for every changed or new C++ file and header.
- Surprises and open questions:
  - `RelayHost` takes `EntryCheckSettings` in its constructor now. `host.h`, `host.cpp` and `main.cpp` will conflict with A4 if it changes the same spots. Rebase and re-verify when merging.
  - `pass.cpp` lost its private byte helpers (moved to `wire.h`). Output is unchanged, and the pass tests still pass.
  - The loop's 100 ms service wait would make PING spacing uneven, so the wait is now the time to the next PING. This touches `Run()` and `WaitMs()`, which A4 may also touch.
  - `Connection` in `host.cpp` gained default member values, because clang-tidy flagged the uninitialised fields.
  - Interpretations to confirm (PROTOCOL.md does not settle them):
    - Median of an even count is the average of the two middle values, rounded down.
    - The verdict is taken 4 s after the first PING, even when all 8 are answered sooner. Every check takes 4 s. Finishing early after 8 answers would be a change for the owner to approve.
    - Order of checks: protocol, then rate limit, then VPN hook, then measurement. PROTOCOL.md 4.4 lists VPN first, and section 6 gives no order for the rate limit. The rate limit counts every CHECK_HELLO that passes the protocol check, including ones later flagged by the VPN hook.
    - If signing fails, the relay closes the connection with no reply, because no REFUSE code fits. The client then reports "cannot be reached" (PROTOCOL.md 4.4).
    - The address table is swept once a minute. It grows with the number of distinct addresses seen in one window.
  - PROTOCOL.md 6 has two limits this package does not implement, and the Done-when list does not cover them: 4 connections per address that have not sent a first message, and 256 such connections in total. `MAX_PEERS` = 256 caps all peers, not only unfinished ones. Decide who adds these, probably A4 or A6.
  - The test client sends `exe_crc` 1 and `ini_crc` 2. Real values come from C1.
  - PROTOCOL.md section 10: no assumption in it applies to A2. V1 to V8 belong to C1, C2, D2, 2.1 and A5.
  - Not verified: a real game client (C1 is not done), a flood from many addresses, and real network latency. The pass signature is checked only by the unit test, not by an external verifier.
- Review findings and fixes:
  - Pending: reviewer other than the author.
