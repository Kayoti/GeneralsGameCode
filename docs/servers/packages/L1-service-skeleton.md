# L1 Lobby service: program skeleton, configuration and fixed answers

State: done
Branch: servers-go-L1

## Goal

Create the lobby service program with its configuration, routing, logging and tests, and answer every call that has a fixed empty response.

## Depends on

0.2

## Read first

- DECISIONS.md D38, D39, D46, D48, D49
- SERVICE.md sections 1, 2, 3, 5.2, 11, 12
- reference/CLIENT_HTTP.md sections 2 (common request rules) and 7.1 (minimal responses)
- STANDARDS.md, "Lobby service code" and "Security"

## May edit

- `Servers/Lobby/` (new)
- `docs/servers/TESTING.md` (section "Build and run the lobby service")

## Must not edit

- Anything outside those two places

## Done when

- [x] `Servers/Lobby/` is a Go module that builds a binary `lobby` with `go build ./...`.
- [x] `lobby --config <file>` reads the configuration file of SERVICE.md section 11. A missing or invalid required value stops it with a message naming the key. `Servers/Lobby/lobby.conf` is a working example with no secrets.
- [x] It listens on `listen` and answers `GET /healthz`.
- [x] Paths are routed as `/env/{env}/contract/1/{Endpoint}` for `prod`, `test` and `dev`.
- [x] Every call in SERVICE.md 5.2 returns exactly the body in CLIENT_HTTP.md 7.1, with every key the client reads. `POST VersionCheck` returns the four keys with `result` 0.
- [x] Any other path under the prefix returns `404` with `{}`.
- [x] Request bodies over 64 KB are refused with `413`.
- [x] A log line per request shows method, path, status and duration, and the caller's address with the last part hidden. No header values are logged.
- [x] SQLite opens the `database` file and creates its tables at start. The schema is in one place. No table is used yet beyond a version row.
- [x] Each handler has a test using `httptest`.
- [x] TESTING.md says how to build, configure and run the service.
- [x] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Login and tokens (L2). Until then the calls above do not check `Authorization`.
- WebSocket (L3).

## Handoff notes

- Finished:
  - `Servers/Lobby/go.mod` (module `servers-lobby`, Go 1.26.0, one outside module: `modernc.org/sqlite`), `go.sum`.
  - `Servers/Lobby/cmd/lobby/main.go`: the `lobby` program, `--config <file>`.
  - `Servers/Lobby/internal/config/` (`config.go`, `config_test.go`): reads every key of SERVICE.md 11.
  - `Servers/Lobby/internal/store/` (`store.go`, `store_test.go`): opens SQLite, creates the tables from one schema constant, keeps one version row, refuses another version.
  - `Servers/Lobby/internal/api/` (`handler.go`, `fixed.go`, `logging.go` and their tests): routing, the fixed answers of SERVICE.md 5.2 plus VersionCheck, `GET /healthz`, the 64 KB limit, the request log.
  - `Servers/Lobby/lobby.conf`: example configuration, no secrets.
  - `docs/servers/TESTING.md`: section "Build and run the lobby service".
  - Verified: `go build ./...`, `gofmt -l .` (no output), `go vet ./...`, `go test ./...` (and `go test -count=100 ./...`, all pass). The binary was built outside the repository and run against `lobby.conf` on 127.0.0.1:9000: `/healthz` gave `ok`; VersionCheck gave the four keys with result 0; `GET /env/dev/contract/1/Lobbies` gave 404 `{}`; a 70000-byte POST gave 413; the log lines showed method, path, status, duration and `127.0.0.x`, and no header value. Missing key and bad listen messages were checked from the binary.
- Left to do:
  - Reviewer (not the author) to check the code and tick the last item.
  - `go test -race` was not run: this PC has no C compiler, and the race detector needs cgo.
  - Calls from SERVICE.md 4 and 5.1 are not built here and answer 404 `{}` until their packages: `LoginCode`, `CheckLogin`, `LoginWithToken`, `RefreshToken`, `DELETE User/{id}` (L2); `ServiceConfig`, `MOTD`, `Rooms`, `Lobbies`, `Lobby/{id}` and `Lobby/Outcome`, `ConnectionOutcome` (L4 and later). `ConnectionOutcome` and `Lobby/Outcome` have the fixed bodies of CLIENT_HTTP.md 7.1 and could be added to the table in `internal/api/fixed.go` by whoever next touches it, if the owner wants them answered now. `/ws` is not served (L3).
  - Region public key files are named in the configuration but not read. L6 reads them.
- Surprises and open questions:
  - `PUT PlayerStats` (SERVICE.md 5.2): CLIENT_HTTP.md 4.20 and 7.1 say a 2xx makes the client fetch `PlayerStats/{id}` again, and that any other status only logs. This package answers 404 `{}` to avoid the re-fetch. The documents do not name a status; the owner may want to confirm it.
  - Routing answers 404 `{}` for a known path with the wrong method (for example `GET VersionCheck`). SERVICE.md 3 and 5 say "any other path" but do not name wrong methods. 405 would be the usual HTTP answer; I did not pick it, since the documents name no 405.
  - An unreadable request body (not the size limit) gets 400 `{}`. The documents name no status for this case.
  - `ReadHeaderTimeout` of 10 seconds is set in `cmd/lobby/main.go`. No document gives this value; it is my choice to limit idle connections. The owner should confirm it or change it.
  - Configuration choices not stated in SERVICE.md 11:
    - Required keys: `listen`, `database`, `public_ws_url`, `rooms`, `accounts_per_address_per_hour`, `trust_forwarded_for`. Optional, empty by default: `motd`, `turn_secret`. Regions are optional.
    - Unknown keys are refused, so a misspelt key is reported. A key given twice is refused.
    - Lines starting with `#` are comments.
    - `LOBBY_TURN_SECRET` replaces the file value when it is set and not empty.
    - Two regions with the same tag are refused. Region numbers do not have to be consecutive.
    - `public_ws_url` must start with `ws://` or `wss://`.
  - `go.mod` says `go 1.26.0`, the lowest version the dependencies need. `go mod init` had written 1.27.4.
  - The `modernc.org/sqlite` driver is the one named in SERVICE.md 1. Its registered name is `sqlite`, checked in its source.
  - `TESTING.md` text says the binary must go outside the repository, because `.gitignore` re-includes `*.exe`.
- Review findings and fixes:
  - None yet. Reviewer to add.
