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

#endif // RTS_SERVERS
