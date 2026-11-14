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

// FILE: ServersDirect.h //////////////////////////////////////////////////////
// Starts a Servers match from launch flags, without the lobby. The flags are stored by the
// command line parser. ServersDirectUpdate() then runs the entry check, the CREATE or JOIN
// handshake and the launch, one step per frame. Usage: docs/servers/TESTING.md.

// TheSuperHackers @feature Kayoti 14/11/2026 Adds the direct Servers match start driven by launch flags.

#pragma once

#ifdef RTS_SERVERS

/// The launch flags that describe a direct match.
enum ServersDirectFlag
{
	SERVERS_DIRECT_FLAG_HOST,			///< -serversHost, no value
	SERVERS_DIRECT_FLAG_JOIN,			///< -serversJoin, no value
	SERVERS_DIRECT_FLAG_RELAY,		///< -serversRelay <host name or IPv4 address>
	SERVERS_DIRECT_FLAG_PORT,			///< -serversPort <UDP port>
	SERVERS_DIRECT_FLAG_SLOT,			///< -serversSlot <0 to 7>
	SERVERS_DIRECT_FLAG_PLAYERS,	///< -serversPlayers <2 to 8>
	SERVERS_DIRECT_FLAG_MAP,			///< -serversMap <map name>
	SERVERS_DIRECT_FLAG_SESSION,	///< -serversSession <16 hex digits>
	SERVERS_DIRECT_FLAG_TOKEN,		///< -serversToken <16 hex digits>
};

/**
 * Records one launch flag. value is null for the flags without a value. A value that is missing or
 * out of range is logged and makes the whole set of flags unusable, so no direct start is attempted.
 */
void ServersDirectSetFlag( ServersDirectFlag flag, const char *value );

/**
 * Call once per frame from the engine update. Does nothing unless -serversHost or -serversJoin was given
 * with a usable set of flags. It waits for the main menu, runs the entry check against the given relay,
 * opens or joins the relay session and launches the match. It stops after the launch or the first failure.
 */
void ServersDirectUpdate();

#endif // RTS_SERVERS
