# D1 Lobby: Servers button, entry check on entry, refusal messages

State: todo
Branch: servers-D1

## Goal

Add the Servers button to the Multiplayer menu. Clicking it runs the entry check, and on success opens the existing online login and lobby with a "Servers mode" state set.

## Depends on

0.2, 0.3, C1 (for the `ServerEntryCheck` and `ServerRegions` headers)

## Read first

- PROTOCOL.md sections 4.4, 4.9, 5.1 and 7.1.
- DECISIONS D9, D16, D17, D21 to D23.
- CODEMAP "Online lobby": `Menus/MainMenu.cpp`, and how `ButtonOnline` leads into the login screen.

## May edit

- `Menus/MainMenu.cpp` in `GeneralsMD/`, then the same change in `Generals/`
- One new file pair in `Core/GameEngine/.../GameNetwork/GameSpy/` holding the Servers mode state: whether it is on, the region, the current pass
- The Multiplayer menu layout and the text file entries for the new strings, added as override files next to the code

## Must not edit

`WOL*.cpp` screens, `PeerThread.cpp`, `LobbyUtils.cpp` (D2 and D3 own those changes), `NAT.*`, `LANAPI*`.

## Done when

- [ ] The Multiplayer menu shows a Servers button next to Online and Network, in builds with the Servers switch on.
- [ ] Clicking it shows a short "Checking connection" state while `ServerEntryCheck` runs. The menu stays responsive.
- [ ] On a pass: Servers mode state is set with the region and pass, and the existing online login screen opens.
- [ ] On `VPN_DETECTED`: the message from PROTOCOL.md 4.4 is shown, and OK returns to the Multiplayer menu.
- [ ] On `PING_TOO_HIGH`: the message with the measured value and the limit is shown, and OK returns to the Multiplayer menu.
- [ ] On any other refusal, or no answer from any region: "The server cannot be reached." and back to the Multiplayer menu.
- [ ] Leaving the online lobby back to the menus clears the Servers mode state.
- [ ] Clicking Online never runs the entry check and never sets the state.
- [ ] All new text is English and comes from the game's text system, not from literals in code.
- [ ] The handoff notes record how the layout override was produced, since the original layout files are not in the repository.
- [ ] Builds with the switch on and off. Lint reports no new warnings.
- [ ] Reviewed by someone other than the author.

## Out of scope

The tag and the list filter (D2). The entry check on join, Accept and exit (D3).

## Handoff notes

- Finished:
- Left to do:
- Surprises and open questions:
- Review findings and fixes:
