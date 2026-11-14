# L1 Lobby service: program skeleton, configuration and fixed answers

State: todo
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

- [ ] `Servers/Lobby/` is a Go module that builds a binary `lobby` with `go build ./...`.
- [ ] `lobby --config <file>` reads the configuration file of SERVICE.md section 11. A missing or invalid required value stops it with a message naming the key. `Servers/Lobby/lobby.conf` is a working example with no secrets.
- [ ] It listens on `listen` and answers `GET /healthz`.
- [ ] Paths are routed as `/env/{env}/contract/1/{Endpoint}` for `prod`, `test` and `dev`.
- [ ] Every call in SERVICE.md 5.2 returns exactly the body in CLIENT_HTTP.md 7.1, with every key the client reads. `POST VersionCheck` returns the four keys with `result` 0.
- [ ] Any other path under the prefix returns `404` with `{}`.
- [ ] Request bodies over 64 KB are refused with `413`.
- [ ] A log line per request shows method, path, status and duration, and the caller's address with the last part hidden. No header values are logged.
- [ ] SQLite opens the `database` file and creates its tables at start. The schema is in one place. No table is used yet beyond a version row.
- [ ] Each handler has a test using `httptest`.
- [ ] TESTING.md says how to build, configure and run the service.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Login and tokens (L2). Until then the calls above do not check `Authorization`.
- WebSocket (L3).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
