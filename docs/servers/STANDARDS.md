# Coding standards

These add to the project's [CONTRIBUTING.md](../../CONTRIBUTING.md). Where they differ, this file wins for this project's code.

## Scope of a change

- Touch only the lines that serve the package's goal. No drive-by cleanups or refactors.
- Stay inside the "May edit" list of the package.
- Zero Hour only (`GeneralsMD/` and shared `Core/` code). Do not change `Generals/`.
- When a package tells you to take a file from the `servers` branch, take it unchanged first, commit that, and make your changes in a second commit. This keeps the difference reviewable.

## Game client code

- Match the style of the surrounding code. The engine is C++98 in style and must stay simple to read. Generals Online's own files use a newer style; match whichever file you are in.
- Every Servers code path is behind the build switch `RTS_SERVERS`. With the switch off there is no Servers button and no relay code, and Online behaves the same.
- Changes that apply to every build are not behind the switch. Only two packages make such changes: B1 (own service addresses, removed reporting) and D2 (hiding server lobbies from the Online list).
- Do not change anything that affects the simulation result. Networking changes must never alter game logic.
- New player-facing text uses `TheGameText->fetchOrSubstitute("GUI:Key", L"English fallback")`.
- Mark changes with the project comment format:

```
// TheSuperHackers @feature Kayoti DD/MM/YYYY Present-tense description of the change.
```

Keywords: `@bugfix`, `@fix`, `@build`, `@feature`, `@performance`, `@refactor`, `@tweak`, `@info`, `@todo`.

- Indentation follows `.editorconfig`.

## Relay code

- C++. One formatter, run before every commit. Its configuration is committed with the program.
- The relay contains no game engine code. It reads only the record header described in PROTOCOL.md.
- Keep functions short and single-purpose. No duplicated logic.

## Lobby service code

- Go. `gofmt` and `go vet` are clean before every commit.
- Standard library first. The only outside modules are the ones named in SERVICE.md section 1. Adding another needs the project owner.
- One package per concern: accounts, lobbies, WebSocket, pass checking, configuration. No package imports the HTTP layer.
- Every handler has a test. Tests use the standard `testing` package and `httptest`, need no network and no installed game, and run with `go test ./...`.
- All SQL uses parameters. No string building of SQL.
- State shared between connections is guarded. Run tests with `-race` where the platform supports it.
- Errors are returned, not ignored. A handler never panics on client input.

## Security

The relay and the lobby service face the internet. Treat everything a client sends as hostile.

- Check every length, count, index and type from the network before using it.
- Never trust a slot number, player id or lobby id from a message. Derive the player from the connection or the session token, and check membership.
- Reject unknown or out-of-state messages. Do not try to repair them.
- Limit rates per address and per session.
- Never write a path or file name that came from the network without validating it.
- Never log passes, tokens, passwords, signing keys or full IP addresses at normal log levels.
- No secrets in the repository. Configuration that holds secrets is read from the environment or from files that are not committed.
- The client never downloads or runs anything the service names.

## Performance

- Relay forwarding path: no memory allocation per packet, and no copy beyond the one needed to send.
- No blocking calls on the relay's network thread.
- Lobby service: no database access on the WebSocket message path. Lobby state is in memory.

## Size

- A package that grows beyond its file list or its goal stops and reports in the handoff notes, so it can be split.
- A change that cannot be reviewed in one sitting is too large.

## Before a package is done

| Program | Required |
|---|---|
| Game client | Builds with the Servers switch on and with it off, using the commands in [TESTING.md](TESTING.md). `scripts/run-clang-tidy.py` reports no new warnings on changed files, or lint is reported as "not run" with the reason. |
| Relay | Builds with its own build file. `relay_tests` pass. |
| Lobby service | `gofmt -l .` prints nothing, `go vet ./...` is clean, `go test ./...` passes. |

For every package: a reviewer other than the author has approved, findings and their fixes are listed in the handoff notes, and the package's own "Done when" list is met.

## Commits

- Present tense, in the style used by the project: `feat(network): Add server mode to connection setup`.
- One logical change per commit.
- Message body explains why, when the reason is not obvious from the change.
- No trailers.
