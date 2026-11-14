# L2 Lobby service: accounts, login and tokens

State: todo
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

- [ ] `GET LoginCode`, `POST CheckLogin`, `POST LoginWithToken`, `POST RefreshToken` and `DELETE User/{id}` behave as SERVICE.md section 4 says, and every answer has every key the client reads for that structure (CLIENT_HTTP.md 4.2 to 4.6).
- [ ] A new account gets the next user id from 1000 and the name `Player<id>`. Accounts are stored in SQLite and survive a restart.
- [ ] Session tokens last 15 minutes and are held in memory. Refresh tokens are stored only as their SHA-256.
- [ ] A helper returns the logged-in account for a request from its `Authorization: Bearer` header, or refuses with `401` and `{}`. Every call from L1 that the client sends with a login now uses it, except `VersionCheck`, `LoginCode` and `CheckLogin`.
- [ ] Account creation is limited per address per hour by the configuration value, with 0 meaning no limit and loopback addresses never limited. The limit answers `CheckLogin` with a failure that has every key.
- [ ] `LoginWithToken` returns the presented refresh token as `refresh_token`.
- [ ] At most 20 unused login codes per address.
- [ ] Hardware fields in the request are ignored and never stored or logged.
- [ ] A login code works once and expires after 5 minutes.
- [ ] Tokens never appear in logs.
- [ ] Tests cover: first login, later login by refresh token, refresh, expired session, unknown tokens, deleted token, the creation limit, and a restart keeping the account.
- [ ] `Servers/Lobby/tools/lobbyprobe` exists and performs: version check, login code, check login, refresh, login with token. It prints one line per step and exits non-zero on the first failure. TESTING.md says how to run it.
- [ ] The checks for this program in STANDARDS.md, "Before a package is done", pass.
- [ ] Reviewed by someone other than the author.

## Out of scope

- Name changes (L3, over the WebSocket).
- Passwords, e-mail, account recovery.

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
