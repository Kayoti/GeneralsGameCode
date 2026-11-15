# L2 Lobby service: accounts, login and tokens

State: done
Branch: servers-go-L2

## Goal

Let a client log in without a password or web page, stay logged in, and be recognised on later starts.

## Depends on

L1

## Read first

- DECISIONS.md D44, D45, D46
- SERVICE.md sections 3 and 4
- reference/CLIENT_HTTP.md sections 3 (login flow, all of it) and 4.1 to 4.7 (the login calls and `User`)

## May edit

- `Servers/Lobby/`
- `docs/servers/TESTING.md` (how to run `lobbyprobe`)

## Must not edit

- Anything else outside `Servers/Lobby/`

## Done when

- [x] `GET LoginCode`, `POST CheckLogin`, `POST LoginWithToken`, `POST RefreshToken` and `DELETE User/{id}` behave as SERVICE.md section 4 says, and every answer has every key the client reads for that structure (CLIENT_HTTP.md 4.2 to 4.6). Evidence: `internal/api/auth.go`; `auth_test.go` checks the keys of each answer; live run below.
- [x] A new account gets the next user id from 1000 and the name `Player<id>`. Accounts are stored in SQLite and survive a restart. Evidence: `internal/store/accounts.go` (`firstUserID`, `CreateAccount`); `accounts/accounts.go` `defaultDisplayName`; `store/accounts_test.go` `TestAccountSurvivesRestart`; live run: user ids 1000, 1001, then 1002 after a restart.
- [x] Session tokens last 15 minutes and are held in memory. Refresh tokens are stored only as their SHA-256. Evidence: `accounts/sessions.go` `sessionLifetime`; `accounts/accounts.go` line 88 stores `hashToken(refreshToken)` and line 180 looks up by the same hash, so the plain token is never passed to the store.
- [x] A helper returns the logged-in account for a request from its `Authorization: Bearer` header, or refuses with `401` and `{}`. Every call from L1 that the client sends with a login now uses it, except `VersionCheck`, `LoginCode` and `CheckLogin`. Evidence: `api/handler.go` `accountForRequest`, `isLoggedIn`, check at line 90; `api/fixed.go` `needsLogin` set on every row but `VersionCheck`; `auth_test.go` `TestFixedCallWithoutLoginAnswers401`.
- [x] Account creation is limited per address per hour by the configuration value, with 0 meaning no limit and loopback addresses never limited. The limit answers `CheckLogin` with a failure that has every key. Evidence: `accounts/limit.go`; `auth_test.go` `TestCheckLoginAtAccountLimitAnswersFailureWithEveryKey`, `TestLoopbackAddressIsNotLimited`, `TestLimitUsesLastForwardedEntryWhenTrusted`; `accounts_test.go` `TestCreationLimitPerAddressPerHour`, `TestCreationLimitDoesNotApplyToLoopbackOrWhenZero`.
- [x] `LoginWithToken` returns the presented refresh token as `refresh_token`. Evidence: `accounts/accounts.go` `LoginWithRefreshToken`; `auth_test.go` `TestLoginWithTokenReturnsSameAccountAndTheGivenRefreshToken`.
- [x] At most 20 unused login codes per address. Evidence: `accounts/codes.go` `maxUnusedLoginCodesPerAddress`, `IssueLoginCode`; `auth_test.go` `TestLoginCodeIsRefusedPastTwentyUnusedCodes`; `accounts_test.go` `TestAtMostTwentyUnusedLoginCodesPerAddress`.
- [x] Hardware fields in the request are ignored and never stored or logged. Evidence: `api/auth.go` `checkLoginRequest` reads only `code`; the request log (`logging.go`) records method, path, status, duration and address, never a body or header; the CheckLogin test body carries `machine_guid`, `mac_addr`, `vol_serial` and `exe_crc` and the login still works.
- [x] A login code works once and expires after 5 minutes. Evidence: `accounts/accounts.go` `CheckLoginCode` deletes the code on use; `accounts/codes.go` `loginCodeLifetime`; `accounts_test.go` `TestLoginCodeWorksOnce`, `TestLoginCodeExpiresAfterFiveMinutes`.
- [x] Tokens never appear in logs. Evidence: the request log has no token; internal error logs (`api/auth.go` `writeInternalError`, `checkLogin`) carry the call name and the error, and errors never include a token. A live scan of the service log found no 64-character hexadecimal string after two probe runs, a restart and a probe run.
- [x] Tests cover: first login, later login by refresh token, refresh, expired session, unknown tokens, deleted token, the creation limit, and a restart keeping the account. Evidence: `accounts/accounts_test.go` (14 tests) and `store/accounts_test.go`; `api/auth_test.go`. `go test -count=1 ./...` passes.
- [x] `Servers/Lobby/tools/lobbyprobe` exists and performs: version check, login code, check login, refresh, login with token. It prints one line per step and exits non-zero on the first failure. TESTING.md says how to run it. Evidence: `tools/lobbyprobe/main.go`; live run below; failure exit checked with the service stopped (exit 1).
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass. Evidence: `gofmt -l .` printed nothing; `go vet ./...` printed nothing; `go test -count=1 ./...` reported `ok` for `accounts`, `api`, `config`, `store` and `no test files` for `cmd/lobby` and `tools/lobbyprobe`. `go test -race` was not run: `go env CGO_ENABLED` is 0 on this PC (no C compiler), so the race detector cannot build.
- [x] Reviewed by someone other than the author.

## Out of scope

- Name changes (L3, over the WebSocket).
- Passwords, e-mail, account recovery.

## Handoff notes

- Finished:
  - `Servers/Lobby/internal/store/accounts.go` and `accounts_test.go`: account and refresh token queries. `store.go` adds the tables `accounts` and `refresh_tokens`; the schema version stays 1.
  - `Servers/Lobby/internal/accounts/`: `accounts.go` (service, login flows, errors), `codes.go` (login codes), `sessions.go` (session tokens), `limit.go` (creation limit), `tokens.go` (random values, hashing, token shape), `accounts_test.go`.
  - `Servers/Lobby/internal/api/auth.go` and `auth_test.go`: `GET LoginCode`, `POST CheckLogin`, `POST LoginWithToken`, `POST RefreshToken`, `DELETE User/{id}`.
  - `Servers/Lobby/internal/api/handler.go`: `server` type; `NewHandler(trustForwardedFor, wsURI, accounts, logger)`; bearer helper; the login check for fixed answers.
  - `Servers/Lobby/internal/api/fixed.go`: `needsLogin` on every fixed answer except `VersionCheck`.
  - `Servers/Lobby/internal/api/logging.go`: the forwarded-for address is read from every `X-Forwarded-For` line; the last entry of the last line is used.
  - `Servers/Lobby/cmd/lobby/main.go`: creates the account service from the configuration.
  - `Servers/Lobby/tools/lobbyprobe/main.go`, and the section "Check the login calls with lobbyprobe" in `docs/servers/TESTING.md`.
- Live check (not part of the repository): `lobby.exe` built to the scratchpad, run on `127.0.0.1:9100` with a copy of `lobby.conf` (port and database changed). `lobbyprobe.exe --url http://127.0.0.1:9100 --env dev` printed the five steps with exit 0 on the first run (user 1000), again on the second run (user 1001), and after a restart (user 1002). Unauthenticated `GET Social/Friends` answered `401 {}`; a forged refresh token on `LoginWithToken` answered `401 {}`; `CheckLogin` with an unknown code answered `200` with all six keys and `result` 2. With the service stopped the probe exited 1.
- Left to do:
  - Reviewer (not the author) to check the code and tick the last item.
  - Run `go test -race ./...` on a PC with a C compiler.
  - Owner to confirm the choices listed under surprises (items 2 to 6).
- Surprises and open questions:
  1. L1 note corrected. The L1 handoff said `logging.go` read the first `X-Forwarded-For` entry. The code took the last entry of the first header line, but `Header.Get` ignored a second header line added by a proxy. Now every line is read and the last entry of the last line is used. Test: `TestRequestLogUsesLastForwardedLineWhenProxyAddsOwnLine`.
  2. Unknown, used or expired login codes answer `result` 2 (SERVICE.md 4). CLIENT_HTTP.md 4.3 lists `-1` as `CODE_INVALID`, and the client keeps polling on `-1`. With `2` the client shows "Login failed." right away. I followed SERVICE.md. The owner may want to confirm this, because it changes what a player sees if a code is mistyped or expired.
  3. `DELETE User/{id}` for an id that is not the caller's answers `403 {}`. The documents do not name a status for this case.
  4. Status codes the documents do not name: a malformed CheckLogin body answers `400` with the failure keys; an internal error answers `500` (`{}` for most calls, the failure keys for CheckLogin); `DELETE User/{id}` answers `200 {}`.
  5. Sizes and storage not stated in the documents: login codes are 16 random bytes in hexadecimal; session and refresh tokens are 32 random bytes in hexadecimal (64 characters, checked before any lookup). Login codes, session tokens and the per-address creation count are held in memory. A restart ends pending codes and sessions and resets the creation limit; accounts and refresh tokens survive.
  6. A refused account creation leaves its login code unused, so it stays valid until it expires (5 minutes).
  7. Behaviour change for L1 calls: `Social/...`, `Matchmaking`, `PlayerStats`, `GlobalStats` and `ProvideMWToken` now answer `401 {}` without a session, where L1 answered their fixed bodies. CLIENT_HTTP.md 2.1 says the client sends those calls only while logged in, so the client is not affected. The existing tests in `handler_test.go` were changed to log in first (`sessionFor`), and the assertions are unchanged.
  8. `NewHandler` takes the WebSocket address and the accounts service, so its callers changed; the only caller outside tests is `cmd/lobby/main.go`. `testHandler` now takes `*testing.T`.
- Review findings and fixes:
  - Verified on the branch: `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` passes
    (accounts, api, config, store). The five login calls carry every key the client reads with the
    exact spelling and case of CLIENT_HTTP 4.2-4.5; user ids start at 1000 in one SQL transaction;
    refresh tokens are stored only as SHA-256; sessions expire after 15 minutes; a Bearer that is not
    64 lowercase hex is refused before any lookup; all SQL is parameterised; the per-address creation
    limit exempts loopback and treats 0 as no limit; codes work once, expire after 5 minutes and are
    capped at 20 unused per address; hardware fields and tokens never reach the log. Scope, author and
    commit hygiene correct.
  - `result` 2 for an unknown, used or expired code: this follows SERVICE.md 4, which the client reads
    as "Login failed." The only visible effect is that an expired code (or a restart between the code
    call and the check call) fails at once instead of polling. Accepted; recorded in the handoff.
  - One low hardening note, no change for this package: `internal/accounts/codes.go` prunes the whole
    login-code map under the global lock on every `IssueLoginCode`. The per-address cap of 20 bounds a
    single address, but many source addresses could grow the map and the scan cost within the five-minute
    window. SERVICE.md sets no global cap; revisit if a global limit is wanted later.
  - The main checkout's lobby sources were normalised to LF after the merge (they predate the
    `Servers/Lobby/.gitattributes` added with L1); committed content was already LF, so this only
    refreshed the working tree.