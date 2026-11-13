# Coding standards

These add to the project's [CONTRIBUTING.md](../../CONTRIBUTING.md). Where they differ, this file wins for Servers code.

## Scope of a change

- Touch only the lines that serve the package's goal. No drive-by cleanups or refactors.
- Stay inside the "May edit" list of the package.
- Zero Hour (`GeneralsMD/`) is changed first. The same change is then replicated for Generals (`Generals/`), as close to identical as possible.
- Prefer shared code in `Core/` over duplicating it per title.

## Game client code

- Match the style of the surrounding code. The engine is C++98 in style and must stay simple to read.
- Use newer language features only when they are clearly more robust. The project macros (for example `CPP_11`) show what is allowed.
- Every Servers code path is behind the build switch from package 0.2. With the switch off, behaviour is identical to upstream.
- Do not change the behaviour of the Online or Network modes.
- Do not change anything that affects the simulation result. Networking changes must never alter game logic.
- Mark changes with the project comment format:

```
// TheSuperHackers @feature Kayoti DD/MM/YYYY Present-tense description of the change.
```

Keywords: `@bugfix`, `@fix`, `@build`, `@feature`, `@performance`, `@refactor`, `@tweak`, `@info`, `@todo`.

- Indentation follows `.editorconfig`: indent size 2 for `.h`, `.cpp`, `.inl`; final newline; no trailing whitespace.

## Relay server code

- One formatter, run before every commit. Its configuration is committed with the program.
- The relay contains no game engine code. It reads only the record header described in PROTOCOL.md.
- Keep functions short and single-purpose. Split a function that does several jobs.
- No duplicated logic.

## Security

The relay faces the internet. Treat everything a client sends as hostile.

- Check every length, count and index from the network before using it.
- Never trust a slot number or player id from a packet. Derive it from the session.
- Bind a connection to its slot when the pass is checked. Every later message is attributed to that slot, whatever the message says.
- Reject unknown or out-of-state messages. Do not try to repair them.
- Limit rates per address and per session.
- Never write a path or file name that came from the network without validating it.
- Never log passes, signing keys or full IP addresses at normal log levels.
- No secrets in the repository. Configuration that holds secrets is read from the environment.

## Performance

- Relay forwarding path: no memory allocation per packet, and no copy beyond the one needed to send.
- No blocking calls on the relay's network thread. VPN data lookups are in memory.

## Size

- A package that grows beyond its file list or its goal stops and reports in the handoff notes, so it can be split.
- A change that cannot be reviewed in one sitting is too large.

## Before a package is done

1. The game builds with the Servers switch on and with it off.
2. Lint reports no new warnings (`scripts/run-clang-tidy.py` for game code).
3. A reviewer other than the author has approved. Findings and their fixes are listed in the handoff notes.
4. The package's own "Done when" list is met.

## Commits

- Present tense, in the style used by the project: `feat(network): Add server mode to connection setup`.
- One logical change per commit.
- Message body explains why, when the reason is not obvious from the change.
- No trailers.
