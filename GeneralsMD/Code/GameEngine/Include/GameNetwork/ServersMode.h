/*
**	Command & Conquer Generals Zero Hour(tm)
**	Copyright 2025 Electronic Arts Inc.
**
**	This program is free software: you can redistribute it and/or modify
**	it under the terms of the GNU General Public License as published by
**	the Free Software Foundation, either version 3 of the License, or
**	(at your option) any later version.
**
**	This program is distributed in the hope that it will be useful,
**	but WITHOUT ANY WARRANTY; without even the implied warranty of
**	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
**	GNU General Public License for more details.
**
**	You should have received a copy of the GNU General Public License
**	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

// FILE: ServersMode.h ////////////////////////////////////////////////////////
// The state of Servers mode (whether it is on, the region and the pass), and the
// entry check that the Servers button runs.
// Protocol: docs/servers/PROTOCOL.md, sections 4.4 and 5.1.

// TheSuperHackers @feature Kayoti 15/11/2026 Adds the Servers mode state and the entry check run by the Servers button.

#pragma once

#ifdef RTS_SERVERS

#include "Common/AsciiString.h"
#include "Common/UnicodeString.h"
#include "GameNetwork/ServerEntryCheck.h"
#include "GameNetwork/ServerRegions.h"

class ServerLink;

// State ------------------------------------------------------------------------------------------

Bool ServersModeIsOn();																	///< True between a passed entry check and leaving the online lobby
const ServerRegion &ServersModeGetRegion();							///< Only valid while ServersModeIsOn()
const ServerPass &ServersModeGetPass();									///< Only valid while ServersModeIsOn()
void ServersModeSet( const ServerRegion &region, const ServerPass &pass );
void ServersModeClear();																///< Turns Servers mode off and forgets the region and the pass

/// The region tag of the player's region, for the lobby calls. Empty while Servers mode is off.
const char *ServersModeGetRegionTag();

/// The pass as 200 lowercase hexadecimal digits, the value of the X-Servers-Pass header. Empty while Servers mode is off or the pass is invalid.
AsciiString ServersModeGetPassHex();

/// The text shown to the player for a refused entry check. code is a ServerRefuseCode.
UnicodeString ServersModeRefusalText( UnsignedByte code, UnsignedShort rttMs, UnsignedShort limitMs );

// Lobby calls ------------------------------------------------------------------------------------

/// The text shown when the service refuses a server lobby call because the pass is not valid (PROTOCOL.md 5.2).
UnicodeString ServersCheckExpiredText();

/// Shows ServersCheckExpiredText() in a message box and ends the pending host or join attempt, so the player is back on the lobby list.
void ServersShowCheckExpired();

// Entry check ------------------------------------------------------------------------------------

enum ServersEntryResult
{
	SERVERS_ENTRY_IDLE,				///< No entry check is running
	SERVERS_ENTRY_CHECKING,		///< Running, ServersEntryUpdate() must be called every frame
	SERVERS_ENTRY_PASSED,			///< Returned once: Servers mode is now on
	SERVERS_ENTRY_REFUSED,		///< Returned once: the refusal message is already shown
};

/**
 * Starts the entry check against every region of the region file and shows the "Checking connection" box.
 * Servers mode is turned off first. Returns FALSE when the check could not start. The refusal message
 * is then already shown.
 */
Bool ServersEntryStart();

/// Call once per frame, also while a menu is shown. Does not block. On PASSED it turns Servers mode on.
ServersEntryResult ServersEntryUpdate();

/// Drops a check in progress and closes the "Checking connection" box.
void ServersEntryCancel();

// Entry check on player actions ------------------------------------------------------------------
// While Servers mode is on, the check is repeated against the player's own region when the player
// attempts to join a lobby, presses Accept in a lobby, or exits a lobby (PROTOCOL.md 4.4, DECISIONS D22).
// It is separate from the check above, which the main menu drives. Each screen that can start an action
// calls ServersGateUpdate() every frame for the actions it handles.

enum ServersGate
{
	SERVERS_GATE_NONE = 0,
	SERVERS_GATE_JOIN = 1,		///< Attempting to join a lobby
	SERVERS_GATE_ACCEPT = 2,	///< Pressing Accept in a lobby
	SERVERS_GATE_EXIT = 4,		///< Exiting a lobby. Started after the player has left.
};

enum ServersGateResult
{
	SERVERS_GATE_IDLE,				///< No check, or the finished check belongs to an action the caller does not handle
	SERVERS_GATE_CHECKING,		///< Running
	SERVERS_GATE_PASSED,			///< Returned once: the stored pass is replaced
	SERVERS_GATE_REFUSED,			///< Returned once: the refusal message is already shown. The caller logs out with ServersLogoutOnline().
	SERVERS_GATE_CANCELLED,		///< Returned once: the player cancelled, or the check was dropped. The action is not done.
};

/**
 * Starts the check for an action, against the region of the current pass, and shows the "Checking connection" box.
 * Only valid while Servers mode is on. Drops any earlier check. Returns FALSE when the check could not start.
 * The refusal message is then already shown, and the caller treats it as a refusal.
 */
Bool ServersGateStart( ServersGate action );

/// True from ServersGateStart() until ServersGateUpdate() has returned the result. Used to stop an action being triggered twice.
Bool ServersGateIsPending();

/**
 * Call once per frame. Does not block. handled is the set of ServersGate values the caller can act on:
 * the result of any other action is held until a caller that handles it asks.
 * action is set to the action the result belongs to.
 */
ServersGateResult ServersGateUpdate( UnsignedInt handled, ServersGate *action );

/// Logs out of the online service with a full teardown, which leaves the lobby and returns to the main menu, and turns Servers mode off.
void ServersLogoutOnline();

// Match start from a server lobby ---------------------------------------------------------------
// The match-start flow of a server lobby (PROTOCOL.md 5.4). The host opens the relay session, hands
// the service the join tokens and sends the start message; every member connects to the relay with its
// own token. The one relay link of the match is owned here until the launch takes it, or a failure
// closes it. Only the relay handshake, the service calls and the timeouts live here: the caller (the
// staging menu) owns the buttons, the status lines and the launch itself.

enum ServersMatchOutcome
{
	SERVERS_MATCH_IDLE,			///< Nothing is happening, or a flow is still running
	SERVERS_MATCH_LAUNCH,		///< Returned once: the relay slot is held, the caller launches the match now
	SERVERS_MATCH_ABORTED,	///< Returned once (host, before the start message): the lobby stays open and Start is re-enabled
	SERVERS_MATCH_FAILED,		///< Returned once: the match could not be joined, so the caller leaves the lobby and returns to the list
};

/// True while a host or member match-start flow is running. ServersMatchUpdate() must be called each frame while true.
Bool ServersMatchIsActive();

/**
 * The host's countdown ended in a server lobby. Opens the relay session (silent entry check first if the
 * pass is about to expire), posts the join tokens to the service and sends the start message. Only valid
 * for the host of a server lobby. Does nothing if a flow is already running.
 */
void ServersMatchBeginHost();

/**
 * Stores the server fields of a received start message (message 13), ready for ServersMatchBeginMemberFromStartMessage().
 * present is FALSE when the message carried no server_session. Called from the start-message handler for every message,
 * so an Online lobby's message simply stores "not present".
 */
void ServersMatchSetStartFields( Bool present, const char *serverSessionHex, Int serverSlot, const char *serverTokenHex );

/**
 * A member begins its JOIN to the relay from the fields stored by ServersMatchSetStartFields(). If no valid
 * fields were stored the flow fails at once, so ServersMatchUpdate() returns SERVERS_MATCH_FAILED next.
 */
void ServersMatchBeginMemberFromStartMessage();

/// Call each frame while ServersMatchIsActive(). Drives the relay link, the timeouts and the service calls. Does not block.
ServersMatchOutcome ServersMatchUpdate();

/// The message to show for the SERVERS_MATCH_ABORTED or SERVERS_MATCH_FAILED outcome just returned.
UnicodeString ServersMatchGetMessage();

/**
 * Takes the connected relay link for the launch. Ownership passes to the caller (the connection manager drives
 * it from here on). Returns NULL when no server-lobby match is starting. Called once, by NGMPGame::launchGame.
 */
ServerLink *ServersMatchTakeLink();

/// Forgets any match-start state and closes the relay link if it is still owned here. Safe to call at any time.
void ServersMatchReset();

#endif // RTS_SERVERS
