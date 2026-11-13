# Servers mode

Servers mode adds dedicated-server multiplayer to Generals and Zero Hour. It is a new menu entry next to the existing Online and Network (LAN) modes. It uses the existing online lobby and routes the match itself through a relay server.

All work happens on the `servers` branch.

## Read in this order

1. [OVERVIEW.md](OVERVIEW.md): how multiplayer works today and what is being built.
2. [DECISIONS.md](DECISIONS.md): what is already decided. Do not reopen these without the project owner.
3. [STATUS.md](STATUS.md): every work package and its state.
4. Your work package in [packages/](packages/).
5. Only the [CODEMAP.md](CODEMAP.md) entries your package lists.
6. [STANDARDS.md](STANDARDS.md) before writing code.

[PROTOCOL.md](PROTOCOL.md) is the contract between the game client and the relay server, and the rules for how Servers mode uses the existing lobby.

## Working on a package

1. Check that every package under "Depends on" is `done`.
2. Set `State: in progress` at the top of your package file.
3. Work on a branch named `servers-<package id>`, for example `servers-A1`, created from `servers`.
4. Edit only the files your package lists under "May edit". If you need another file, stop and record it under "Handoff notes".
5. Before you stop, for any reason, fill in "Handoff notes" and set the state to `done`, `partial` or `blocked`.

## Rules that prevent collisions

- The state of a package lives in its own file. `STATUS.md` is updated only when packages are merged into `servers`.
- Each track owns its folders. See the ownership table in `STATUS.md`.
- `PROTOCOL.md` changes go through the `servers` branch first, never inside a track branch.

## Package states

| State | Meaning |
|---|---|
| `todo` | Not started |
| `in progress` | Someone is working on it |
| `partial` | Stopped before finishing. Handoff notes say what is left. |
| `blocked` | Cannot continue. Handoff notes say why. |
| `done` | "Done when" list is fully met |
