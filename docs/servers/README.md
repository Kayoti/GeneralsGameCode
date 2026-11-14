# Servers mode

This project is a fork of the Generals Online client for Zero Hour. It runs on its own lobby service and adds a Servers mode, in which a match is carried by a dedicated relay instead of player-to-player links.

All work happens on the `servers-go` branch.

## Read in this order

1. [OVERVIEW.md](OVERVIEW.md): how multiplayer works today and what is being built.
2. [DECISIONS.md](DECISIONS.md): what is already decided. Do not reopen these without the project owner.
3. [STATUS.md](STATUS.md): every work package and its state.
4. Your work package in [packages/](packages/).
5. Only the document sections your package lists under "Read first".
6. [STANDARDS.md](STANDARDS.md) before writing code.

## The documents

| Document | What it is |
|---|---|
| [PROTOCOL.md](PROTOCOL.md) | Contract between the game client and the relay, and the rules for server lobbies |
| [SERVICE.md](SERVICE.md) | Specification of the lobby service |
| [CODEMAP.md](CODEMAP.md) | Where things are in the client code |
| [reference/CLIENT_HTTP.md](reference/CLIENT_HTTP.md) | Every HTTP call the client makes, read from its code |
| [reference/CLIENT_REALTIME.md](reference/CLIENT_REALTIME.md) | The client's WebSocket messages, player-to-player connections and match netcode |
| [reference/CLIENT_PORTING.md](reference/CLIENT_PORTING.md) | The client's menus and build, and how each piece of the earlier work maps onto this code |
| [TESTING.md](TESTING.md) | How to run the programs and the phase checks |
| [archive/](archive/) | Packages and notes of the earlier work on the plain community source. Read only when a package points there. |

The reference files are long. Read only the sections your package names.

## The earlier work

Before 2026-11-14 the project was built on the plain community source, on the `servers` branch. That branch holds a finished relay and working client code for a relay match. It is not developed further. Packages on this branch say exactly which files to take from it, with `git show servers:<path>` or `git checkout servers -- <path>`.

## Working on a package

1. Check that every package under "Depends on" is `done`.
2. Set `State: in progress` at the top of your package file.
3. Work on a branch named `servers-go-<package id>`, for example `servers-go-L1`, created from `servers-go`.
4. Edit only the files your package lists under "May edit". If you need another file, stop and record it under "Handoff notes".
5. Before you stop, for any reason, fill in "Handoff notes" and set the state to `done`, `partial` or `blocked`.

## Rules that prevent collisions

- The state of a package lives in its own file. `STATUS.md` is updated only when packages are merged into `servers-go`.
- Each track owns its folders. See the ownership table in `STATUS.md`.
- `PROTOCOL.md` and `SERVICE.md` changes go through the `servers-go` branch first, never inside a package branch.

## Package states

| State | Meaning |
|---|---|
| `todo` | Not started |
| `in progress` | Someone is working on it |
| `partial` | Stopped before finishing. Handoff notes say what is left. |
| `blocked` | Cannot continue. Handoff notes say why. |
| `done` | "Done when" list is fully met |
