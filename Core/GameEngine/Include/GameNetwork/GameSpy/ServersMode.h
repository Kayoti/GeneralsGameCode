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
// The state of Servers mode in the online lobby (whether it is on, the region
// and the pass), and the entry check that the Servers button runs.
// Protocol: docs/servers/PROTOCOL.md, sections 4.4 and 5.1.

// TheSuperHackers @feature Kayoti 13/11/2026 Adds the Servers mode state and the entry check run by the Servers button.

#pragma once

#ifdef RTS_SERVERS

#include "Common/AsciiString.h"
#include "Common/UnicodeString.h"
#include "GameNetwork/ServerEntryCheck.h"
#include "GameNetwork/ServerRegions.h"

// State ------------------------------------------------------------------------------------------

Bool ServersModeIsOn();																	///< True between a passed entry check and leaving the online lobby
const ServerRegion &ServersModeGetRegion();							///< Only valid while ServersModeIsOn()
const ServerPass &ServersModeGetPass();									///< Only valid while ServersModeIsOn()
void ServersModeSet( const ServerRegion &region, const ServerPass &pass );
void ServersModeClear();																///< Turns Servers mode off and forgets the region and the pass

/// The text shown to the player for a refused entry check. code is a ServerRefuseCode.
UnicodeString ServersModeRefusalText( UnsignedByte code, UnsignedShort rttMs, UnsignedShort limitMs );

// Lobby tag ----------------------------------------------------------------------------------------
// A lobby created in Servers mode carries the tag of the player's region in its ladder fields.
// Tags are compared exactly, text and port together. Protocol: docs/servers/PROTOCOL.md, section 5.2 and 5.3.

/// True if the ladder fields equal the tag of any region in the region file
Bool ServersModeIsAnyTag( const AsciiString &ladderIP, UnsignedShort ladderPort );

/// True if Servers mode is on and the ladder fields equal the tag of the player's region
Bool ServersModeIsOwnTag( const AsciiString &ladderIP, UnsignedShort ladderPort );

/// The lobby list filter. Servers mode shows only the player's region tag, otherwise tagged lobbies are hidden.
Bool ServersModeShowsLobby( const AsciiString &ladderIP, UnsignedShort ladderPort );

// Entry check ------------------------------------------------------------------------------------

enum ServersEntryResult
{
	SERVERS_ENTRY_IDLE,				///< No entry check is running
	SERVERS_ENTRY_CHECKING,		///< Running, ServersEntryUpdate() must be called every frame
	SERVERS_ENTRY_PASSED,			///< Returned once: Servers mode is now on
	SERVERS_ENTRY_REFUSED,		///< Returned once: the refusal message is already shown
};

/**
 * Starts the entry check against the regions of the region file and shows the "Checking connection" box.
 * Servers mode is turned off first. Returns FALSE when the check could not start. The refusal message
 * is then already shown.
 */
Bool ServersEntryStart();

/// Call once per frame, also while a menu is shown. Does not block. On PASSED it turns Servers mode on.
ServersEntryResult ServersEntryUpdate();

/// Drops a check in progress and closes the "Checking connection" box.
void ServersEntryCancel();

// Entry check on player actions ---------------------------------------------------------------------
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
	SERVERS_GATE_REFUSED,			///< Returned once: the refusal message is already shown. The caller leaves the online lobby.
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
 * Call once per frame, also while a screen is shutting down. Does not block. handled is the set of ServersGate values
 * the caller can act on: the result of any other action is held until a caller that handles it asks.
 * action is set to the action the result belongs to.
 */
ServersGateResult ServersGateUpdate( UnsignedInt handled, ServersGate *action );

/// Closes the overlays and asks the online services to log out. The caller then pops its screen back to the main menu.
void ServersLogoutOnline();

#endif // RTS_SERVERS
